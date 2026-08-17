package engine

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/markcmarshall/token-top/attribution"
	"github.com/markcmarshall/token-top/telemetry"
)

type sessionKey struct {
	source telemetry.SourceName
	id     string
}

type sessionState struct {
	source        telemetry.SourceName
	id            string
	firstEvent    time.Time
	lastEvent     time.Time
	model         string
	models        map[string]struct{}
	attr          attribution.Attribution
	lifetime      uint64
	today         uint64
	todayDate     string
	cacheRead     uint64
	cacheReadIn   uint64
	input         uint64
	output        uint64
	sawIncomplete bool
}

type ringItem struct {
	event telemetry.TokenEvent
}

type Engine struct {
	clock      Clock
	attributor attribution.Attributor
	seen       map[string]struct{}
	ring       []ringItem
	sessions   map[sessionKey]*sessionState
	health     map[telemetry.SourceName]telemetry.SourceHealth
	incomplete map[telemetry.SourceName]bool
}

func New(clock Clock, attr attribution.Attributor) *Engine {
	if clock == nil {
		clock = SystemClock{}
	}
	if attr == nil {
		attr = attribution.Func(attribution.GitRoot)
	}
	health := make(map[telemetry.SourceName]telemetry.SourceHealth, len(telemetry.AllSources))
	for _, name := range telemetry.AllSources {
		health[name] = telemetry.SourceHealth{Source: name, State: telemetry.HealthNotDetected}
	}
	return &Engine{
		clock:      clock,
		attributor: attr,
		seen:       make(map[string]struct{}),
		sessions:   make(map[sessionKey]*sessionState),
		health:     health,
		incomplete: make(map[telemetry.SourceName]bool),
	}
}

func (e *Engine) Poll(ctx context.Context, sources []telemetry.Source) {
	now := e.clock.Now()
	if len(sources) == 0 {
		return
	}
	type item struct {
		i int
		b telemetry.Batch
	}
	ch := make(chan item, len(sources))
	for i, src := range sources {
		go func(i int, src telemetry.Source) {
			ch <- item{i: i, b: pollOne(ctx, src, now)}
		}(i, src)
	}
	batches := make([]telemetry.Batch, len(sources))
	for range sources {
		it := <-ch
		batches[it.i] = it.b
	}
	for _, b := range batches {
		e.Apply(b)
	}
}

// PollUntilToday takes at most two bounded polls so the first frame stays
// inside the two-second render gate. Remaining today/history is indexed on
// later live ticks.
func (e *Engine) PollUntilToday(ctx context.Context, sources []telemetry.Source) Snapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	e.Poll(ctx, sources)
	snap := e.Snapshot()
	if !snap.Global.TodayApprox || ctx.Err() != nil {
		return snap
	}
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) < 200*time.Millisecond {
		return snap
	}
	e.Poll(ctx, sources)
	return e.Snapshot()
}

func pollOne(ctx context.Context, src telemetry.Source, now time.Time) (batch telemetry.Batch) {
	defer func() {
		if rec := recover(); rec != nil {
			batch = telemetry.Batch{
				Health: telemetry.SourceHealth{
					Source: src.Name(),
					State:  telemetry.HealthFailed,
					Detail: "source panic",
				},
			}
		}
	}()
	return src.Poll(ctx, now)
}

func (e *Engine) Apply(batch telemetry.Batch) {
	now := e.clock.Now()
	e.rollToday(now)
	e.prune(now)

	src := batch.Health.Source
	if src == "" && len(batch.Events) > 0 {
		src = batch.Events[0].Source
	}
	if src != "" {
		h := batch.Health
		h.Source = src
		if h.State == "" {
			if len(batch.Events) > 0 {
				h.State = telemetry.HealthOK
			} else {
				h.State = telemetry.HealthNotDetected
			}
		}
		if e.incomplete[src] && (h.State == telemetry.HealthOK || h.State == "") {
			h.State = telemetry.HealthDegraded
			if h.Detail == "" || h.Detail == "indexing" {
				h.Detail = "incomplete usage"
			}
		}
		e.health[src] = h
	}

	for _, ev := range batch.Events {
		e.ingest(ev, now)
	}
}

func (e *Engine) ingest(ev telemetry.TokenEvent, now time.Time) {
	if _, dup := e.seen[ev.ID]; dup {
		return
	}
	if err := ev.Validate(); err != nil {
		e.degrade(ev.Source, "invalid event")
		return
	}
	total, ok := telemetry.AddUint64(ev.Input, ev.Output)
	if !ok {
		e.degrade(ev.Source, "usage overflow")
		return
	}

	key := sessionKey{source: ev.Source, id: ev.SessionID}
	sess := e.sessions[key]
	if sess == nil {
		sess = &sessionState{
			source: ev.Source,
			id:     ev.SessionID,
			models: make(map[string]struct{}),
		}
		e.sessions[key] = sess
	}

	nextLife, ok := telemetry.AddUint64(sess.lifetime, total)
	if !ok {
		e.degrade(ev.Source, "usage overflow")
		return
	}

	todayDate := telemetry.LocalDate(now)
	if sess.todayDate != todayDate {
		sess.today = 0
		sess.todayDate = todayDate
	}
	var nextToday uint64
	if telemetry.LocalDate(ev.Timestamp) == todayDate {
		nextToday, ok = telemetry.AddUint64(sess.today, total)
		if !ok {
			e.degrade(ev.Source, "usage overflow")
			return
		}
	} else {
		nextToday = sess.today
	}

	if ev.CacheRead != nil {
		nextRead, ok1 := telemetry.AddUint64(sess.cacheRead, *ev.CacheRead)
		nextIn, ok2 := telemetry.AddUint64(sess.cacheReadIn, ev.Input)
		if !ok1 || !ok2 {
			e.degrade(ev.Source, "usage overflow")
			return
		}
		sess.cacheRead = nextRead
		sess.cacheReadIn = nextIn
	}

	nextIn, okIn := telemetry.AddUint64(sess.input, ev.Input)
	nextOut, okOut := telemetry.AddUint64(sess.output, ev.Output)
	if !okIn || !okOut {
		e.degrade(ev.Source, "usage overflow")
		return
	}

	e.seen[ev.ID] = struct{}{}
	sess.lifetime = nextLife
	sess.today = nextToday
	sess.input = nextIn
	sess.output = nextOut
	if sess.firstEvent.IsZero() || ev.Timestamp.Before(sess.firstEvent) {
		sess.firstEvent = ev.Timestamp
	}
	if sess.lastEvent.IsZero() || ev.Timestamp.After(sess.lastEvent) || ev.Timestamp.Equal(sess.lastEvent) {
		sess.lastEvent = ev.Timestamp
		sess.model = ev.Model
		sess.attr = e.attributor.Attribute(ev)
	}
	if ev.Model != "" {
		sess.models[ev.Model] = struct{}{}
	}
	if !ev.Complete {
		sess.sawIncomplete = true
		e.incomplete[ev.Source] = true
		e.degrade(ev.Source, "incomplete usage")
	}

	e.ring = append(e.ring, ringItem{event: ev})
}

func (e *Engine) Snapshot() Snapshot {
	now := e.clock.Now()
	e.rollToday(now)
	e.prune(now)

	type acc struct {
		tok1, tok5, tok15 uint64
		n1, n15           int
	}
	global := acc{}
	bySource := make(map[telemetry.SourceName]*acc, len(telemetry.AllSources))
	bySession := make(map[sessionKey]*acc, len(e.sessions))
	for _, name := range telemetry.AllSources {
		bySource[name] = &acc{}
	}

	start1 := now.Add(-Window1m)
	start5 := now.Add(-Window5m)
	start15 := now.Add(-Window15m)

	for _, item := range e.ring {
		ts := item.event.Timestamp
		if ts.After(now) {
			continue
		}
		total := item.event.Total()
		src := item.event.Source
		if bySource[src] == nil {
			bySource[src] = &acc{}
		}
		key := sessionKey{source: src, id: item.event.SessionID}
		if bySession[key] == nil {
			bySession[key] = &acc{}
		}
		if !ts.Before(start15) {
			addTok(&global.tok15, total)
			addTok(&bySource[src].tok15, total)
			addTok(&bySession[key].tok15, total)
			global.n15++
			bySession[key].n15++
		}
		if !ts.Before(start5) {
			addTok(&global.tok5, total)
			addTok(&bySource[src].tok5, total)
			addTok(&bySession[key].tok5, total)
		}
		if !ts.Before(start1) {
			addTok(&global.tok1, total)
			addTok(&bySource[src].tok1, total)
			addTok(&bySession[key].tok1, total)
			global.n1++
			bySession[key].n1++
		}
	}

	var today uint64
	var todayApprox bool
	var burning, recent, quietHidden int
	visible := make([]Session, 0, len(e.sessions))
	for key, sess := range e.sessions {
		if sess.todayDate != telemetry.LocalDate(now) {
			sess.today = 0
			sess.todayDate = telemetry.LocalDate(now)
		}
		today = satAdd(today, sess.today)

		win := bySession[key]
		if win == nil {
			win = &acc{}
		}
		activity := ActivityQuiet
		switch {
		case win.n1 > 0:
			activity = ActivityBurning
			burning++
		case win.n15 > 0:
			activity = ActivityRecent
			recent++
		default:
			if sess.today > 0 {
				quietHidden++
			}
			continue
		}

		srcHealth := e.health[sess.source]
		row := Session{
			Source:        sess.source,
			SessionID:     sess.id,
			Activity:      activity,
			Model:         sess.model,
			ModelCount:    len(sess.models),
			ProjectKey:    sess.attr.Key,
			ProjectLabel:  sess.attr.Label,
			ProjectMethod: sess.attr.Method,
			Rate1m:        ratePerMinute(win.tok1, Window1m),
			Rate5m:        ratePerMinute(win.tok5, Window5m),
			Rate15m:       ratePerMinute(win.tok15, Window15m),
			Total:         sess.lifetime,
			TotalApprox:   srcHealth.Indexing,
			Input:         sess.input,
			Output:        sess.output,
			FirstEvent:    sess.firstEvent,
			LastEvent:     sess.lastEvent,
		}
		if sess.cacheReadIn > 0 {
			ratio := float64(sess.cacheRead) / float64(sess.cacheReadIn)
			row.CacheRatio = &ratio
		}
		visible = append(visible, row)
	}

	sort.SliceStable(visible, func(i, j int) bool {
		a, b := visible[i], visible[j]
		if a.Activity != b.Activity {
			return a.Activity == ActivityBurning
		}
		if a.Rate1m != b.Rate1m {
			return a.Rate1m > b.Rate1m
		}
		if a.Rate5m != b.Rate5m {
			return a.Rate5m > b.Rate5m
		}
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		if a.SessionID != b.SessionID {
			return a.SessionID < b.SessionID
		}
		return a.Source < b.Source
	})

	sources := make([]SourceSnapshot, 0, len(telemetry.AllSources))
	globalRate1 := ratePerMinute(global.tok1, Window1m)
	for _, name := range telemetry.AllSources {
		h, ok := e.health[name]
		if !ok {
			h = telemetry.SourceHealth{Source: name, State: telemetry.HealthNotDetected}
		}
		h.Source = name
		if h.TodayIncomplete {
			todayApprox = true
		}
		win := bySource[name]
		if win == nil {
			win = &acc{}
		}
		rate1 := ratePerMinute(win.tok1, Window1m)
		share := 0.0
		if globalRate1 > 0 {
			share = rate1 / globalRate1
		}
		sources = append(sources, SourceSnapshot{
			Name:    name,
			Health:  h,
			Rate1m:  rate1,
			Share1m: share,
		})
	}

	return Snapshot{
		GeneratedAt: now,
		Global: Global{
			Rate1m:      globalRate1,
			Rate5m:      ratePerMinute(global.tok5, Window5m),
			Rate15m:     ratePerMinute(global.tok15, Window15m),
			Today:       today,
			TodayApprox: todayApprox,
			Burning:     burning,
			Recent:      recent,
		},
		Sources:     sources,
		Sessions:    visible,
		QuietHidden: quietHidden,
	}
}

func (e *Engine) rollToday(now time.Time) {
	today := telemetry.LocalDate(now)
	for _, sess := range e.sessions {
		if sess.todayDate != today {
			sess.today = 0
			sess.todayDate = today
		}
	}
}

func (e *Engine) prune(now time.Time) {
	cutoff := now.Add(-Window15m)
	kept := e.ring[:0]
	for _, item := range e.ring {
		if !item.event.Timestamp.Before(cutoff) {
			kept = append(kept, item)
		}
	}
	e.ring = kept
}

func (e *Engine) degrade(src telemetry.SourceName, detail string) {
	if src == "" {
		return
	}
	h := e.health[src]
	h.Source = src
	switch h.State {
	case telemetry.HealthFailed:
		if h.Detail == "" {
			h.Detail = detail
		}
	case telemetry.HealthDegraded:
		if h.Detail == "" {
			h.Detail = detail
		}
	default:
		h.State = telemetry.HealthDegraded
		h.Detail = detail
	}
	e.health[src] = h
}

func ratePerMinute(tokens uint64, window time.Duration) float64 {
	minutes := window.Minutes()
	if minutes <= 0 {
		return 0
	}
	return float64(tokens) / minutes
}

func addTok(dst *uint64, n uint64) {
	sum, ok := telemetry.AddUint64(*dst, n)
	if !ok {
		*dst = math.MaxUint64
		return
	}
	*dst = sum
}

func satAdd(a, b uint64) uint64 {
	sum, ok := telemetry.AddUint64(a, b)
	if !ok {
		return math.MaxUint64
	}
	return sum
}
