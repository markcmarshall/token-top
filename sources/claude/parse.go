package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/markcmarshall/token-top/telemetry"
)

var sessionFromName = regexp.MustCompile(`(?i)^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

type Parser struct {
	SessionID string
}

type parsed struct {
	Event      *telemetry.TokenEvent
	MessageID  string
	Malformed  bool
	Unexpected bool
}

type rawRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Session   string          `json:"session_id"`
	CWD       string          `json:"cwd"`
	UUID      string          `json:"uuid"`
	Message   json.RawMessage `json:"message"`
}

type rawMessage struct {
	ID    string    `json:"id"`
	Model string    `json:"model"`
	Usage *rawUsage `json:"usage"`
}

type rawUsage struct {
	InputTokens         *uint64         `json:"input_tokens"`
	OutputTokens        *uint64         `json:"output_tokens"`
	CacheCreationTokens *uint64         `json:"cache_creation_input_tokens"`
	CacheReadTokens     *uint64         `json:"cache_read_input_tokens"`
	ThinkingTokens      *uint64         `json:"thinking_tokens"`
	CacheCreation       json.RawMessage `json:"cache_creation"`
	OutputTokensDetails *struct {
		ThinkingTokens *uint64 `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

func SessionIDFromPath(path string) string {
	m := sessionFromName.FindStringSubmatch(filepath.Base(path))
	if len(m) == 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

func (p *Parser) Consume(line []byte) parsed {
	if len(bytes.TrimSpace(line)) == 0 {
		return parsed{}
	}
	var rec rawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return parsed{Malformed: true}
	}
	if rec.Type != "assistant" {
		return parsed{}
	}
	if rec.SessionID != "" {
		p.SessionID = rec.SessionID
	} else if rec.Session != "" {
		p.SessionID = rec.Session
	}
	if len(bytes.TrimSpace(rec.Message)) == 0 {
		return parsed{}
	}
	var msg rawMessage
	if err := json.Unmarshal(rec.Message, &msg); err != nil {
		return parsed{Malformed: true, Unexpected: true}
	}
	if msg.Usage == nil {
		return parsed{}
	}
	return p.usage(rec, msg)
}

func (p *Parser) usage(rec rawRecord, msg rawMessage) parsed {
	u := msg.Usage
	if u.InputTokens == nil && u.OutputTokens == nil {
		return parsed{Unexpected: true}
	}
	if u.InputTokens == nil || u.OutputTokens == nil {
		return parsed{Unexpected: true}
	}

	input, ok := normalizeInput(u)
	if !ok {
		return parsed{Unexpected: true}
	}
	output := *u.OutputTokens
	cacheRead := u.CacheReadTokens
	cacheWrite := u.CacheCreationTokens
	reasoning, haveReasoning := thinking(u)

	unexpected := false
	if haveReasoning && *reasoning > output {
		reasoning = nil
		haveReasoning = false
		unexpected = true
	}
	if bucketsUnexpected(u) {
		unexpected = true
	}
	if cacheRead != nil && *cacheRead > input {
		cacheRead = nil
		unexpected = true
	}
	if cacheWrite != nil && *cacheWrite > input {
		cacheWrite = nil
		unexpected = true
	}

	at, ok := parseTS(rec.Timestamp)
	if !ok || p.SessionID == "" {
		return parsed{Unexpected: true}
	}

	id := eventID(p.SessionID, msg.ID, rec.UUID, at, input, output)
	ev := telemetry.TokenEvent{
		ID:         id,
		Source:     telemetry.SourceClaude,
		SessionID:  p.SessionID,
		Timestamp:  at,
		Model:      msg.Model,
		CWD:        rec.CWD,
		Input:      input,
		Output:     output,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
		Reasoning:  reasoning,
		Complete:   !unexpected,
	}
	return parsed{
		Event:      &ev,
		MessageID:  msg.ID,
		Unexpected: unexpected,
	}
}

func normalizeInput(u *rawUsage) (uint64, bool) {
	in := uint64(0)
	ok := true
	if u.InputTokens != nil {
		in = *u.InputTokens
	}
	if u.CacheCreationTokens != nil {
		in, ok = telemetry.AddUint64(in, *u.CacheCreationTokens)
		if !ok {
			return 0, false
		}
	}
	if u.CacheReadTokens != nil {
		in, ok = telemetry.AddUint64(in, *u.CacheReadTokens)
		if !ok {
			return 0, false
		}
	}
	return in, true
}

func thinking(u *rawUsage) (*uint64, bool) {
	if u.OutputTokensDetails != nil && u.OutputTokensDetails.ThinkingTokens != nil {
		return u.OutputTokensDetails.ThinkingTokens, true
	}
	if u.ThinkingTokens != nil {
		return u.ThinkingTokens, true
	}
	return nil, false
}

func bucketsUnexpected(u *rawUsage) bool {
	if u.CacheCreationTokens == nil || len(bytes.TrimSpace(u.CacheCreation)) == 0 {
		return false
	}
	var buckets map[string]uint64
	if err := json.Unmarshal(u.CacheCreation, &buckets); err != nil {
		return true
	}
	var sum uint64
	for _, v := range buckets {
		next, ok := telemetry.AddUint64(sum, v)
		if !ok {
			return true
		}
		sum = next
	}
	return sum != *u.CacheCreationTokens
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

func eventID(session, messageID, uuid string, at time.Time, in, out uint64) string {
	if messageID != "" {
		return fmt.Sprintf("claude:%s:%s", session, messageID)
	}
	if uuid != "" {
		return fmt.Sprintf("claude:%s:%s", session, uuid)
	}
	return fmt.Sprintf("claude:%s:%s:%d:%d", session, at.UTC().Format(time.RFC3339Nano), in, out)
}

type accounted struct {
	in, out               uint64
	cacheRead, cacheWrite uint64
	reasoning             uint64
	haveCR, haveCW, haveR bool
	rev                   int
}

// Deduper collapses streamed assistant records that share one message ID.
type Deduper struct {
	seen map[string]accounted
}

func (d *Deduper) Filter(ev telemetry.TokenEvent, messageID string) (telemetry.TokenEvent, bool) {
	if messageID == "" {
		return ev, true
	}
	if d.seen == nil {
		d.seen = make(map[string]accounted)
	}
	key := ev.SessionID + "\x00" + messageID
	prev, ok := d.seen[key]
	cur := snapshot(ev)
	if !ok {
		d.seen[key] = cur
		return ev, true
	}
	if sameUsage(prev, cur) {
		return telemetry.TokenEvent{}, false
	}
	if cur.in < prev.in || cur.out < prev.out {
		return telemetry.TokenEvent{}, false
	}
	delta := ev
	delta.Input = cur.in - prev.in
	delta.Output = cur.out - prev.out
	delta.CacheRead = deltaOpt(prev.haveCR, cur.haveCR, prev.cacheRead, cur.cacheRead)
	delta.CacheWrite = deltaOpt(prev.haveCW, cur.haveCW, prev.cacheWrite, cur.cacheWrite)
	delta.Reasoning = deltaOpt(prev.haveR, cur.haveR, prev.reasoning, cur.reasoning)
	if delta.Reasoning != nil && *delta.Reasoning > delta.Output {
		delta.Reasoning = nil
	}
	if delta.Input == 0 && delta.Output == 0 {
		d.seen[key] = cur
		return telemetry.TokenEvent{}, false
	}
	cur.rev = prev.rev + 1
	delta.ID = fmt.Sprintf("%s:r%d", ev.ID, cur.rev)
	d.seen[key] = cur
	return delta, true
}

func snapshot(ev telemetry.TokenEvent) accounted {
	a := accounted{in: ev.Input, out: ev.Output}
	if ev.CacheRead != nil {
		a.haveCR, a.cacheRead = true, *ev.CacheRead
	}
	if ev.CacheWrite != nil {
		a.haveCW, a.cacheWrite = true, *ev.CacheWrite
	}
	if ev.Reasoning != nil {
		a.haveR, a.reasoning = true, *ev.Reasoning
	}
	return a
}

func sameUsage(a, b accounted) bool {
	return a.in == b.in && a.out == b.out &&
		a.haveCR == b.haveCR && a.cacheRead == b.cacheRead &&
		a.haveCW == b.haveCW && a.cacheWrite == b.cacheWrite &&
		a.haveR == b.haveR && a.reasoning == b.reasoning
}

func deltaOpt(havePrev, haveCur bool, prev, cur uint64) *uint64 {
	if !havePrev || !haveCur {
		return nil
	}
	if cur < prev {
		return telemetry.Uint64Ptr(0)
	}
	return telemetry.Uint64Ptr(cur - prev)
}
