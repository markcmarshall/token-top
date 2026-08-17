package grok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/markcmarshall/token-top/telemetry"
)

type Parser struct {
	SessionID string
	CWD       string
	Model     string
}

type parsed struct {
	Events     []telemetry.TokenEvent
	Malformed  bool
	Unexpected bool
	Incomplete bool
}

type rawLine struct {
	Method    string          `json:"method"`
	Timestamp json.RawMessage `json:"timestamp"`
	TS        json.RawMessage `json:"ts"`
	Params    struct {
		Update rawUpdate `json:"update"`
	} `json:"params"`
}

type rawUpdate struct {
	SessionUpdate string    `json:"sessionUpdate"`
	Usage         *rawUsage `json:"usage"`
}

type rawUsage struct {
	InputTokens         *uint64             `json:"inputTokens"`
	OutputTokens        *uint64             `json:"outputTokens"`
	TotalTokens         *uint64             `json:"totalTokens"`
	CachedReadTokens    *uint64             `json:"cachedReadTokens"`
	CacheCreationTokens *uint64             `json:"cacheCreationTokens"`
	ReasoningTokens     *uint64             `json:"reasoningTokens"`
	UsageIsIncomplete   *bool               `json:"usageIsIncomplete"`
	ModelUsage          map[string]rawUsage `json:"modelUsage"`
}

type summaryFile struct {
	Info *struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"info"`
	CurrentModel string  `json:"current_model_id"`
	GitRoot      string  `json:"git_root_dir"`
	NumMessages  *uint64 `json:"num_messages"`
}

func ParseSummary(data []byte) (sessionID, cwd, model string, err error) {
	var s summaryFile
	if err = json.Unmarshal(data, &s); err != nil {
		return "", "", "", err
	}
	if s.Info != nil {
		sessionID = s.Info.ID
		cwd = s.Info.CWD
	}
	if cwd == "" {
		cwd = filepath.Clean(s.GitRoot)
	}
	return sessionID, cwd, s.CurrentModel, nil
}

func parseSummaryActivity(data []byte) (numMessages uint64, known bool, err error) {
	var s summaryFile
	if err = json.Unmarshal(data, &s); err != nil {
		return 0, false, err
	}
	if s.NumMessages == nil {
		return 0, false, nil
	}
	return *s.NumMessages, true, nil
}

func (p *Parser) Consume(line []byte) parsed {
	if len(bytes.TrimSpace(line)) == 0 {
		return parsed{}
	}
	var rec rawLine
	if err := json.Unmarshal(line, &rec); err != nil {
		return parsed{Malformed: true}
	}
	if rec.Method != "session/update" && rec.Method != "_x.ai/session/update" {
		return parsed{}
	}
	if rec.Params.Update.SessionUpdate != "turn_completed" {
		return parsed{}
	}
	if rec.Params.Update.Usage == nil {
		return parsed{}
	}
	return p.turn(rec)
}

func (p *Parser) turn(rec rawLine) parsed {
	at, ok := parseGrokTime(rec.Timestamp)
	if !ok {
		at, ok = parseGrokTime(rec.TS)
	}
	if !ok || p.SessionID == "" {
		return parsed{Unexpected: true}
	}
	u := rec.Params.Update.Usage
	incomplete := u.UsageIsIncomplete != nil && *u.UsageIsIncomplete

	type piece struct {
		model string
		u     rawUsage
	}
	var pieces []piece
	if len(u.ModelUsage) > 0 {
		names := make([]string, 0, len(u.ModelUsage))
		for model := range u.ModelUsage {
			names = append(names, model)
		}
		sort.Strings(names)
		for _, model := range names {
			pieces = append(pieces, piece{model: model, u: u.ModelUsage[model]})
		}
	} else {
		pieces = append(pieces, piece{model: p.Model, u: *u})
	}

	out := parsed{Incomplete: incomplete}
	for _, pc := range pieces {
		ev, bad := p.component(at, pc.model, pc.u, incomplete)
		if bad {
			out.Unexpected = true
		}
		if ev != nil {
			out.Events = append(out.Events, *ev)
		}
	}
	if len(out.Events) == 0 && !out.Unexpected {
		out.Unexpected = true
	}
	return out
}

func (p *Parser) component(at time.Time, model string, u rawUsage, incomplete bool) (*telemetry.TokenEvent, bool) {
	if u.InputTokens == nil || u.OutputTokens == nil {
		return nil, true
	}
	in, out := *u.InputTokens, *u.OutputTokens
	if model == "" {
		model = p.Model
	}
	cacheRead := u.CachedReadTokens
	cacheWrite := u.CacheCreationTokens
	reasoning := u.ReasoningTokens
	unexpected := false
	if cacheRead != nil && *cacheRead > in {
		cacheRead = nil
		unexpected = true
	}
	if cacheWrite != nil && *cacheWrite > in {
		cacheWrite = nil
		unexpected = true
	}
	if reasoning != nil && *reasoning > out {
		reasoning = nil
		unexpected = true
	}
	ev := telemetry.TokenEvent{
		ID:         eventID(p.SessionID, at, model, in, out),
		Source:     telemetry.SourceGrok,
		SessionID:  p.SessionID,
		Timestamp:  at,
		Model:      model,
		CWD:        p.CWD,
		Input:      in,
		Output:     out,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
		Reasoning:  reasoning,
		Complete:   !unexpected && !incomplete,
	}
	return &ev, unexpected
}

func parseGrokTime(raw json.RawMessage) (time.Time, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return time.Time{}, false
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return time.Time{}, false
		}
		return parseTS(s)
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return time.Time{}, false
	}
	if f > 1e12 {
		return time.UnixMilli(int64(f)).UTC(), true
	}
	return time.Unix(int64(f), 0).UTC(), true
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

func eventID(session string, at time.Time, model string, in, out uint64) string {
	return fmt.Sprintf("grok:%s:%s:%s:%d:%d", session, at.UTC().Format(time.RFC3339Nano), model, in, out)
}
