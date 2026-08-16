package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/markcmarshall/token-top/telemetry"
)

var sessionFromName = regexp.MustCompile(`(?i)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

type counters struct {
	Input      uint64
	Output     uint64
	CacheRead  uint64
	CacheWrite uint64
	Reasoning  uint64
}

type Parser struct {
	SessionID string
	CWD       string
	Model     string
	have      bool
	cum       counters
	seedOnly  bool
}

func (p *Parser) SeedNext() {
	p.seedOnly = true
}

type rawEvent struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Model     string `json:"model"`
}

type turnContext struct {
	CWD               string `json:"cwd"`
	Model             string `json:"model"`
	CollaborationMode *struct {
		Settings *struct {
			Model string `json:"model"`
		} `json:"settings"`
	} `json:"collaboration_mode"`
}

type eventMsg struct {
	Type           string     `json:"type"`
	Info           *tokenInfo `json:"info"`
	ThreadSettings *struct {
		Model string `json:"model"`
		CWD   string `json:"cwd"`
	} `json:"thread_settings"`
}

type tokenInfo struct {
	Model              string    `json:"model"`
	ModelContextWindow *uint64   `json:"model_context_window"`
	LastTokenUsage     *rawUsage `json:"last_token_usage"`
	TotalTokenUsage    *rawUsage `json:"total_token_usage"`
}

type rawUsage struct {
	InputTokens           *uint64 `json:"input_tokens"`
	OutputTokens          *uint64 `json:"output_tokens"`
	CachedInputTokens     *uint64 `json:"cached_input_tokens"`
	CacheWriteInputTokens *uint64 `json:"cache_write_input_tokens"`
	ReasoningOutputTokens *uint64 `json:"reasoning_output_tokens"`
	TotalTokens           *uint64 `json:"total_tokens"`
}

func SessionIDFromPath(path string) string {
	m := sessionFromName.FindStringSubmatch(path)
	if len(m) == 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

func (p *Parser) Reset() {
	id := p.SessionID
	*p = Parser{SessionID: id}
}

func (p *Parser) Consume(line []byte) (ev *telemetry.TokenEvent, sample *telemetry.ContextSample, malformed bool) {
	if len(bytesTrim(line)) == 0 {
		return nil, nil, false
	}
	var rec rawEvent
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, nil, true
	}
	switch rec.Type {
	case "session_meta":
		var meta sessionMeta
		if err := json.Unmarshal(rec.Payload, &meta); err != nil {
			return nil, nil, true
		}
		if meta.ID != "" {
			p.SessionID = meta.ID
		} else if meta.SessionID != "" {
			p.SessionID = meta.SessionID
		}
		if meta.CWD != "" {
			p.CWD = meta.CWD
		}
		if meta.Model != "" {
			p.Model = meta.Model
		}
	case "turn_context":
		var ctx turnContext
		if err := json.Unmarshal(rec.Payload, &ctx); err != nil {
			return nil, nil, true
		}
		if ctx.CWD != "" {
			p.CWD = ctx.CWD
		}
		if ctx.Model != "" {
			p.Model = ctx.Model
		} else if ctx.CollaborationMode != nil && ctx.CollaborationMode.Settings != nil && ctx.CollaborationMode.Settings.Model != "" {
			p.Model = ctx.CollaborationMode.Settings.Model
		}
	case "event_msg":
		var msg eventMsg
		if err := json.Unmarshal(rec.Payload, &msg); err != nil {
			return nil, nil, true
		}
		switch msg.Type {
		case "thread_settings_applied":
			if msg.ThreadSettings != nil {
				if msg.ThreadSettings.Model != "" {
					p.Model = msg.ThreadSettings.Model
				}
				if msg.ThreadSettings.CWD != "" {
					p.CWD = msg.ThreadSettings.CWD
				}
			}
		case "token_count":
			return p.tokenCount(rec.Timestamp, msg.Info)
		}
	}
	return nil, nil, false
}

func (p *Parser) tokenCount(ts string, info *tokenInfo) (*telemetry.TokenEvent, *telemetry.ContextSample, bool) {
	if info == nil {
		return nil, nil, false
	}
	if info.Model != "" {
		p.Model = info.Model
	}
	if isContextSample(info) {
		at, ok := parseTS(ts)
		if !ok || p.SessionID == "" {
			return nil, nil, false
		}
		occupied := uint64(0)
		if info.LastTokenUsage != nil && info.LastTokenUsage.TotalTokens != nil {
			occupied = *info.LastTokenUsage.TotalTokens
		}
		max := uint64(0)
		if info.ModelContextWindow != nil {
			max = *info.ModelContextWindow
		}
		sample := &telemetry.ContextSample{
			ID:        fmt.Sprintf("codex-ctx:%s:%s:%d:%d", p.SessionID, at.UTC().Format(time.RFC3339Nano), occupied, max),
			Source:    telemetry.SourceCodex,
			SessionID: p.SessionID,
			Timestamp: at,
			Model:     p.Model,
			Occupied:  occupied,
			Maximum:   max,
		}
		if info.TotalTokenUsage != nil {
			p.noteCumulative(fromRaw(info.TotalTokenUsage))
		}
		return nil, sample, false
	}
	if info.TotalTokenUsage == nil || !hasIO(info.TotalTokenUsage) {
		return nil, nil, false
	}
	cur := fromRaw(info.TotalTokenUsage)
	delta, ok := p.delta(cur)
	if !ok {
		return nil, nil, false
	}
	at, ok := parseTS(ts)
	if !ok || p.SessionID == "" {
		return nil, nil, false
	}
	opt := optionals(delta, info.LastTokenUsage)
	ev := telemetry.TokenEvent{
		ID:         eventID(p.SessionID, at, cur),
		Source:     telemetry.SourceCodex,
		SessionID:  p.SessionID,
		Timestamp:  at,
		Model:      p.Model,
		CWD:        p.CWD,
		Input:      delta.Input,
		Output:     delta.Output,
		CacheRead:  opt.cacheRead,
		CacheWrite: opt.cacheWrite,
		Reasoning:  opt.reasoning,
		Complete:   true,
	}
	return &ev, nil, false
}

type optionalSet struct {
	cacheRead  *uint64
	cacheWrite *uint64
	reasoning  *uint64
}

func optionals(delta counters, last *rawUsage) optionalSet {
	src := delta
	if last != nil && uintOr(last.InputTokens) == delta.Input && uintOr(last.OutputTokens) == delta.Output {
		src = fromRaw(last)
	}
	out := optionalSet{}
	if src.CacheRead <= delta.Input {
		out.cacheRead = telemetry.Uint64Ptr(src.CacheRead)
	}
	if src.CacheWrite <= delta.Input {
		out.cacheWrite = telemetry.Uint64Ptr(src.CacheWrite)
	}
	if src.Reasoning <= delta.Output {
		out.reasoning = telemetry.Uint64Ptr(src.Reasoning)
	}
	return out
}

func (p *Parser) delta(cur counters) (counters, bool) {
	if !p.have {
		p.have = true
		p.cum = cur
		if p.seedOnly {
			p.seedOnly = false
			return counters{}, false
		}
		if cur.Input == 0 && cur.Output == 0 {
			return counters{}, false
		}
		return cur, true
	}
	if cur == p.cum {
		return counters{}, false
	}
	if cur.Input < p.cum.Input || cur.Output < p.cum.Output {
		p.cum = cur
		if cur.Input == 0 && cur.Output == 0 {
			return counters{}, false
		}
		return cur, true
	}
	d := counters{
		Input:      cur.Input - p.cum.Input,
		Output:     cur.Output - p.cum.Output,
		CacheRead:  subOrZero(cur.CacheRead, p.cum.CacheRead),
		CacheWrite: subOrZero(cur.CacheWrite, p.cum.CacheWrite),
		Reasoning:  subOrZero(cur.Reasoning, p.cum.Reasoning),
	}
	p.cum = cur
	if d.Input == 0 && d.Output == 0 {
		return counters{}, false
	}
	return d, true
}

func (p *Parser) noteCumulative(cur counters) {
	if !p.have {
		p.have = true
		p.cum = cur
		return
	}
	if cur.Input < p.cum.Input || cur.Output < p.cum.Output {
		p.cum = cur
		return
	}
	p.cum = cur
}

func isContextSample(info *tokenInfo) bool {
	if info == nil || info.LastTokenUsage == nil || info.ModelContextWindow == nil {
		return false
	}
	last := info.LastTokenUsage
	in := uintOr(last.InputTokens)
	out := uintOr(last.OutputTokens)
	tot := uintOr(last.TotalTokens)
	return in == 0 && out == 0 && tot > 0
}

func hasIO(u *rawUsage) bool {
	return u != nil && (u.InputTokens != nil || u.OutputTokens != nil)
}

func fromRaw(u *rawUsage) counters {
	return counters{
		Input:      uintOr(u.InputTokens),
		Output:     uintOr(u.OutputTokens),
		CacheRead:  uintOr(u.CachedInputTokens),
		CacheWrite: uintOr(u.CacheWriteInputTokens),
		Reasoning:  uintOr(u.ReasoningOutputTokens),
	}
}

func uintOr(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

func subOrZero(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func parseTS(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func eventID(session string, at time.Time, cur counters) string {
	return fmt.Sprintf("codex:%s:%s:%d:%d:%d:%d:%d", session, at.UTC().Format(time.RFC3339Nano), cur.Input, cur.Output, cur.CacheRead, cur.CacheWrite, cur.Reasoning)
}

func bytesTrim(b []byte) []byte {
	return bytes.TrimSpace(b)
}
