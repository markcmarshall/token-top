package grok

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/markcmarshall/token-top/sources/internal/jsonl"
	"github.com/markcmarshall/token-top/telemetry"
)

const defaultReadBudget = 2 << 20

type Options struct {
	Home       string
	Sessions   string
	ReadBudget int64
	TailAfter  int64
}

type Source struct {
	sessions  string
	budget    int64
	tailAfter int64
	files     map[string]*fileState
}

type fileState struct {
	path      string
	dir       string
	live      jsonl.Cursor
	hist      *jsonl.Cursor
	parser    Parser
	liveStart int64
	inited    bool
	sumMTime  time.Time
	sawTurn   bool
}

func New(opts Options) *Source {
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	sessions := opts.Sessions
	if sessions == "" {
		sessions = filepath.Join(home, ".grok", "sessions")
	}
	budget := opts.ReadBudget
	if budget <= 0 {
		budget = defaultReadBudget
	}
	tailAfter := opts.TailAfter
	if tailAfter <= 0 {
		tailAfter = defaultReadBudget
	}
	return &Source{
		sessions:  sessions,
		budget:    budget,
		tailAfter: tailAfter,
		files:     make(map[string]*fileState),
	}
}

func (s *Source) Name() telemetry.SourceName { return telemetry.SourceGrok }

func (s *Source) Poll(ctx context.Context, now time.Time) telemetry.Batch {
	batch := telemetry.Batch{
		Health: telemetry.SourceHealth{Source: telemetry.SourceGrok, State: telemetry.HealthOK},
	}
	if !dirExists(s.sessions) {
		batch.Health.State = telemetry.HealthNotDetected
		return batch
	}

	discovered, discErr := s.discover()
	if discErr != nil && len(discovered) == 0 {
		batch.Health.State = telemetry.HealthFailed
		batch.Health.Detail = "unreadable source path"
		return batch
	}

	sort.SliceStable(discovered, func(i, j int) bool {
		ri, rj := rank(discovered[i].mtime, now), rank(discovered[j].mtime, now)
		if ri != rj {
			return ri < rj
		}
		return discovered[i].mtime.After(discovered[j].mtime)
	})

	remaining := s.budget
	var bad, opened, unread, missing, todayUnread int
	var incompleteToday bool
	var loc, missingLoc string
	for _, item := range discovered {
		if err := ctx.Err(); err != nil {
			break
		}
		if item.updates == "" {
			if todayFile(item.mtime, now) && missingUpdatesDegraded(item.summary) {
				missing++
				if missingLoc == "" {
					missingLoc = telemetry.LocateDetail("missing updates", item.dir, 0)
				}
			}
			continue
		}
		st := s.state(item)
		if err := s.refreshMeta(st); err != nil {
			bad++
			continue
		}
		if err := s.initFile(st); err != nil {
			bad++
			continue
		}
		if remaining <= 0 {
			if !jsonl.FullyConsumed(st.live) || st.hist != nil {
				unread++
				if todayFile(item.mtime, now) {
					todayUnread++
				}
			}
			continue
		}
		n, b, evs, reset, hit, err := s.readInto(st, &st.live, remaining)
		if err != nil {
			bad++
			if loc == "" {
				loc = telemetry.LocateDetail("unreadable", item.updates, 0)
			}
			continue
		}
		opened++
		remaining -= n
		if todayFile(item.mtime, now) {
			bad += b
			if loc == "" {
				loc = hit
			}
		}
		batch.Events = append(batch.Events, evs...)
		if hasIncompleteToday(evs, now) {
			incompleteToday = true
		}
		if reset {
			s.resetFile(st)
			batch.Health.State = telemetry.HealthDegraded
			if batch.Health.Detail == "" {
				batch.Health.Detail = telemetry.LocateDetail("session file replaced", item.updates, 0)
			}
		}
		if !jsonl.FullyConsumed(st.live) || st.hist != nil {
			unread++
			if todayFile(item.mtime, now) {
				todayUnread++
			}
		}
	}
	for _, item := range discovered {
		if remaining <= 0 || ctx.Err() != nil {
			break
		}
		if item.updates == "" {
			continue
		}
		st := s.files[item.updates]
		if st == nil || st.hist == nil {
			continue
		}
		n, b, evs, reset, hit, err := s.readInto(st, st.hist, remaining)
		if err != nil {
			bad++
			if loc == "" {
				loc = telemetry.LocateDetail("unreadable", item.updates, 0)
			}
			continue
		}
		remaining -= n
		if todayFile(item.mtime, now) {
			bad += b
			if loc == "" {
				loc = hit
			}
		}
		batch.Events = append(batch.Events, evs...)
		if hasIncompleteToday(evs, now) {
			incompleteToday = true
		}
		if reset {
			s.resetFile(st)
			continue
		}
		if st.hist.Offset >= st.liveStart {
			st.hist = nil
		} else {
			unread++
			if todayFile(item.mtime, now) {
				todayUnread++
			}
		}
	}

	if unread == 0 {
		for _, item := range discovered {
			if item.updates == "" {
				continue
			}
			st := s.files[item.updates]
			if st != nil && !st.sawTurn && todayFile(item.mtime, now) {
				missing++
				if missingLoc == "" {
					missingLoc = telemetry.LocateDetail("missing updates", item.dir, 0)
				}
			}
		}
	}

	if unread > 0 {
		batch.Health.Indexing = true
		if batch.Health.Detail == "" {
			batch.Health.Detail = "indexing"
		}
	}
	if todayUnread > 0 {
		batch.Health.TodayIncomplete = true
	}
	if incompleteToday {
		batch.Health.State = telemetry.HealthDegraded
		batch.Health.TodayIncomplete = true
		if batch.Health.Detail == "" || batch.Health.Detail == "indexing" {
			batch.Health.Detail = "incomplete usage"
		}
	}
	if missing > 0 {
		batch.Health.State = telemetry.HealthDegraded
		if missingLoc != "" {
			batch.Health.Detail = missingLoc
		} else if batch.Health.Detail == "" || batch.Health.Detail == "indexing" {
			batch.Health.Detail = "missing updates"
		}
	}
	if bad > 0 {
		batch.Health.State = telemetry.HealthDegraded
		if loc != "" {
			batch.Health.Detail = loc
		} else if batch.Health.Detail == "" || batch.Health.Detail == "indexing" {
			batch.Health.Detail = "malformed records"
		}
	}
	if opened == 0 && len(discovered) > 0 && bad > 0 && missing == 0 {
		batch.Health.State = telemetry.HealthFailed
		batch.Health.Detail = "detected source cannot be read"
	}
	return batch
}

func (s *Source) state(item discoveredSession) *fileState {
	st := s.files[item.updates]
	if st == nil {
		st = &fileState{path: item.updates, dir: item.dir}
		s.files[item.updates] = st
	}
	return st
}

func (s *Source) refreshMeta(st *fileState) error {
	sum := filepath.Join(st.dir, "summary.json")
	info, err := os.Stat(sum)
	if err != nil {
		if os.IsNotExist(err) {
			if st.parser.SessionID == "" {
				st.parser.SessionID = filepath.Base(st.dir)
			}
			return nil
		}
		return err
	}
	if !st.sumMTime.IsZero() && !info.ModTime().After(st.sumMTime) && st.parser.SessionID != "" {
		return nil
	}
	data, err := os.ReadFile(sum)
	if err != nil {
		return err
	}
	id, cwd, model, err := ParseSummary(data)
	if err != nil {
		return err
	}
	if id == "" {
		id = filepath.Base(st.dir)
	}
	st.parser.SessionID = id
	if cwd != "" {
		st.parser.CWD = cwd
	}
	if model != "" {
		st.parser.Model = model
	}
	st.sumMTime = info.ModTime()
	return nil
}

func (s *Source) initFile(st *fileState) error {
	if st.inited {
		return nil
	}
	info, err := os.Stat(st.path)
	if err != nil {
		return err
	}
	if info.Size() > s.tailAfter {
		start := info.Size() - s.budget
		if start < 0 {
			start = 0
		}
		st.liveStart = start
		st.live = jsonl.Cursor{Offset: start, MidStart: start > 0}
		zero := jsonl.Cursor{}
		st.hist = &zero
	}
	st.inited = true
	return nil
}

func (s *Source) resetFile(st *fileState) {
	dir, path := st.dir, st.path
	*st = fileState{path: path, dir: dir}
}

func (s *Source) readInto(st *fileState, cur *jsonl.Cursor, budget int64) (read int64, bad int, events []telemetry.TokenEvent, reset bool, loc string, err error) {
	res, err := jsonl.Read(st.path, *cur, budget)
	if err != nil {
		return 0, 1, nil, false, telemetry.LocateDetail("unreadable", st.path, cur.Offset), err
	}
	*cur = res.Cursor
	if res.Reset {
		return res.Read, 0, nil, true, "", nil
	}
	for _, line := range res.Lines {
		got := st.parser.Consume(line)
		if got.Malformed || got.Unexpected {
			bad++
			if loc == "" {
				loc = telemetry.LocateDetail("malformed records", st.path, cur.Offset)
			}
		}
		if len(got.Events) > 0 {
			st.sawTurn = true
			events = append(events, got.Events...)
		}
	}
	return res.Read, bad, events, false, loc, nil
}

type discoveredSession struct {
	dir     string
	summary string
	updates string
	mtime   time.Time
}

func (s *Source) discover() ([]discoveredSession, error) {
	seen := make(map[string]*discoveredSession)
	var firstErr error
	err := filepath.WalkDir(s.sessions, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "summary.json" && name != "updates.jsonl" {
			return nil
		}
		dir := filepath.Dir(path)
		item := seen[dir]
		if item == nil {
			item = &discoveredSession{dir: dir}
			seen[dir] = item
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if name == "updates.jsonl" {
			item.updates = path
		} else {
			item.summary = path
		}
		if info.ModTime().After(item.mtime) {
			item.mtime = info.ModTime()
		}
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	out := make([]discoveredSession, 0, len(seen))
	for _, item := range seen {
		out = append(out, *item)
	}
	return out, firstErr
}

func missingUpdatesDegraded(summaryPath string) bool {
	if summaryPath == "" {
		return true
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return true
	}
	numMessages, known, err := parseSummaryActivity(data)
	return err != nil || !known || numMessages > 0
}

func hasIncompleteToday(events []telemetry.TokenEvent, now time.Time) bool {
	today := telemetry.LocalDate(now)
	for _, ev := range events {
		if !ev.Complete && telemetry.LocalDate(ev.Timestamp) == today {
			return true
		}
	}
	return false
}

func rank(mtime, now time.Time) int {
	if !mtime.Before(now.Add(-15 * time.Minute)) {
		return 0
	}
	if telemetry.LocalDate(mtime) == telemetry.LocalDate(now) {
		return 1
	}
	return 2
}

func todayFile(mtime, now time.Time) bool {
	return rank(mtime, now) <= 1
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
