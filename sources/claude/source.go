package claude

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

const defaultReadBudget = 4 << 20

type Options struct {
	Home       string
	Projects   string
	ReadBudget int64
	TailAfter  int64
}

type Source struct {
	projects  string
	budget    int64
	tailAfter int64
	files     map[string]*fileState
	dedup     Deduper
}

type fileState struct {
	path      string
	live      jsonl.Cursor
	hist      *jsonl.Cursor
	parser    Parser
	liveStart int64
	inited    bool
}

func New(opts Options) *Source {
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	projects := opts.Projects
	if projects == "" {
		projects = filepath.Join(home, ".claude", "projects")
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
		projects:  projects,
		budget:    budget,
		tailAfter: tailAfter,
		files:     make(map[string]*fileState),
	}
}

func (s *Source) Name() telemetry.SourceName { return telemetry.SourceClaude }

func (s *Source) Poll(ctx context.Context, now time.Time) telemetry.Batch {
	batch := telemetry.Batch{
		Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK},
	}
	if !dirExists(s.projects) {
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
	var bad, opened, unread int
	for _, item := range discovered {
		if err := ctx.Err(); err != nil {
			break
		}
		st := s.state(item.path)
		if err := s.initFile(st); err != nil {
			bad++
			continue
		}
		if remaining <= 0 {
			if !jsonl.FullyConsumed(st.live) || st.hist != nil {
				unread++
			}
			continue
		}
		n, b, evs, reset, err := s.readInto(st, &st.live, remaining)
		if err != nil {
			bad++
			continue
		}
		opened++
		remaining -= n
		bad += b
		batch.Events = append(batch.Events, evs...)
		if reset {
			s.resetFile(st)
			batch.Health.State = telemetry.HealthDegraded
			if batch.Health.Detail == "" {
				batch.Health.Detail = "session file replaced"
			}
		}
		if !jsonl.FullyConsumed(st.live) || st.hist != nil {
			unread++
		}
	}
	for _, item := range discovered {
		if remaining <= 0 || ctx.Err() != nil {
			break
		}
		st := s.files[item.path]
		if st == nil || st.hist == nil {
			continue
		}
		n, b, evs, reset, err := s.readInto(st, st.hist, remaining)
		if err != nil {
			bad++
			continue
		}
		remaining -= n
		bad += b
		batch.Events = append(batch.Events, evs...)
		if reset {
			s.resetFile(st)
			continue
		}
		if st.hist.Offset >= st.liveStart {
			st.hist = nil
		} else {
			unread++
		}
	}

	if unread > 0 {
		batch.Health.Indexing = true
		if batch.Health.Detail == "" {
			batch.Health.Detail = "indexing"
		}
	}
	if bad > 0 {
		batch.Health.State = telemetry.HealthDegraded
		if batch.Health.Detail == "" || batch.Health.Detail == "indexing" {
			batch.Health.Detail = "malformed records"
		}
	}
	if opened == 0 && len(discovered) > 0 && bad > 0 {
		batch.Health.State = telemetry.HealthFailed
		batch.Health.Detail = "detected source cannot be read"
	}
	return batch
}

func (s *Source) state(path string) *fileState {
	st := s.files[path]
	if st == nil {
		st = &fileState{path: path}
		s.files[path] = st
	}
	return st
}

func (s *Source) initFile(st *fileState) error {
	if st.inited {
		return nil
	}
	info, err := os.Stat(st.path)
	if err != nil {
		return err
	}
	st.parser = Parser{SessionID: SessionIDFromPath(st.path)}
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
	*st = fileState{path: st.path}
}

func (s *Source) readInto(st *fileState, cur *jsonl.Cursor, budget int64) (read int64, bad int, events []telemetry.TokenEvent, reset bool, err error) {
	res, err := jsonl.Read(st.path, *cur, budget)
	if err != nil {
		return 0, 1, nil, false, err
	}
	*cur = res.Cursor
	if res.Reset {
		return res.Read, 0, nil, true, nil
	}
	for _, line := range res.Lines {
		got := st.parser.Consume(line)
		if got.Malformed || got.Unexpected {
			bad++
		}
		if got.Event == nil {
			continue
		}
		ev, ok := s.dedup.Filter(*got.Event, got.MessageID)
		if !ok {
			continue
		}
		events = append(events, ev)
	}
	return res.Read, bad, events, false, nil
}

type discoveredFile struct {
	path  string
	mtime time.Time
}

func (s *Source) discover() ([]discoveredFile, error) {
	var out []discoveredFile
	var firstErr error
	err := filepath.WalkDir(s.projects, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, discoveredFile{path: path, mtime: info.ModTime()})
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	return out, firstErr
}

func rank(mtime, now time.Time) int {
	if !mtime.Before(now.Add(-15 * time.Minute)) {
		return 0
	}
	if localDate(mtime) == localDate(now) {
		return 1
	}
	return 2
}

func localDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
