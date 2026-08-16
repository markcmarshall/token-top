package codex

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
	Sessions   string
	Archive    string
	ReadBudget int64
	TailAfter  int64
}

type Source struct {
	sessions  string
	archive   string
	budget    int64
	tailAfter int64
	files     map[string]*fileState
}

type fileState struct {
	path       string
	live       jsonl.Cursor
	hist       *jsonl.Cursor
	liveParser Parser
	histParser Parser
	liveStart  int64
	inited     bool
}

func New(opts Options) *Source {
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	sessions := opts.Sessions
	if sessions == "" {
		sessions = filepath.Join(home, ".codex", "sessions")
	}
	archive := opts.Archive
	if archive == "" {
		archive = filepath.Join(home, ".codex", "archived_sessions")
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
		archive:   archive,
		budget:    budget,
		tailAfter: tailAfter,
		files:     make(map[string]*fileState),
	}
}

func (s *Source) Name() telemetry.SourceName { return telemetry.SourceCodex }

func (s *Source) Poll(ctx context.Context, now time.Time) telemetry.Batch {
	batch := telemetry.Batch{
		Health: telemetry.SourceHealth{Source: telemetry.SourceCodex, State: telemetry.HealthOK},
	}
	sessExists := dirExists(s.sessions)
	archExists := dirExists(s.archive)
	if !sessExists && !archExists {
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
		n, b, evs, samples, reset, err := s.readInto(st, &st.live, &st.liveParser, remaining)
		if err != nil {
			bad++
			continue
		}
		opened++
		remaining -= n
		bad += b
		batch.Events = append(batch.Events, evs...)
		batch.ContextSamples = append(batch.ContextSamples, samples...)
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
		n, b, evs, samples, reset, err := s.readInto(st, st.hist, &st.histParser, remaining)
		if err != nil {
			bad++
			continue
		}
		remaining -= n
		bad += b
		batch.Events = append(batch.Events, evs...)
		batch.ContextSamples = append(batch.ContextSamples, samples...)
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
	id := SessionIDFromPath(st.path)
	st.liveParser = Parser{SessionID: id}
	st.histParser = Parser{SessionID: id}
	if info.Size() > s.tailAfter {
		start := info.Size() - s.budget
		st.liveStart = start
		st.live = jsonl.Cursor{Offset: start, MidStart: true}
		st.liveParser.SeedNext()
		zero := jsonl.Cursor{}
		st.hist = &zero
	}
	st.inited = true
	return nil
}

func (s *Source) resetFile(st *fileState) {
	*st = fileState{path: st.path}
}

func (s *Source) readInto(st *fileState, cur *jsonl.Cursor, p *Parser, budget int64) (read int64, bad int, events []telemetry.TokenEvent, samples []telemetry.ContextSample, reset bool, err error) {
	res, err := jsonl.Read(st.path, *cur, budget)
	if err != nil {
		return 0, 1, nil, nil, false, err
	}
	*cur = res.Cursor
	if res.Reset {
		return res.Read, 0, nil, nil, true, nil
	}
	for _, line := range res.Lines {
		ev, sample, malformed := p.Consume(line)
		if malformed {
			bad++
			continue
		}
		if ev != nil {
			events = append(events, *ev)
		}
		if sample != nil {
			samples = append(samples, *sample)
		}
	}
	return res.Read, bad, events, samples, false, nil
}

type discoveredFile struct {
	path  string
	mtime time.Time
}

func (s *Source) discover() ([]discoveredFile, error) {
	seen := make(map[string]discoveredFile)
	var firstErr error
	collect := func(root string, prefer bool) {
		if !dirExists(root) {
			return
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			name := filepath.Base(path)
			if _, exists := seen[name]; exists && !prefer {
				return nil
			}
			seen[name] = discoveredFile{path: path, mtime: info.ModTime()}
			return nil
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	collect(s.sessions, true)
	collect(s.archive, false)
	out := make([]discoveredFile, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
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
