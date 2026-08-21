package engine

import (
	"time"

	"github.com/markcmarshall/token-top/telemetry"
)

type Activity string

const (
	ActivityBurning Activity = "burning"
	ActivityRecent  Activity = "recent"
	ActivityQuiet   Activity = "quiet"
)

const (
	Window1m  = time.Minute
	Window5m  = 5 * time.Minute
	Window15m = 15 * time.Minute
)

type Snapshot struct {
	GeneratedAt  time.Time
	Global       Global
	Sources      []SourceSnapshot
	Attributions []AttributionSnapshot
	Sessions     []Session
	QuietHidden  int
}

type Global struct {
	Rate1m               float64
	Rate5m               float64
	Rate15m              float64
	Tokens5m             uint64
	Tokens15m            uint64
	Today                uint64
	TodayInput           uint64
	TodayOutput          uint64
	TodayCacheRead       uint64
	TodayCacheKnownInput uint64
	TodayApprox          bool
	Burning              int
	Recent               int
}

type SourceSnapshot struct {
	Name       telemetry.SourceName
	Health     telemetry.SourceHealth
	Rate1m     float64
	Share1m    float64
	Tokens15m  uint64
	Today      uint64
	ShareToday float64
}

// AttributionSnapshot is an event-level work aggregate. Standalone Token Top
// supplies project attribution; private consumers may inject another work
// subject through the public Attributor interface.
type AttributionSnapshot struct {
	Source      telemetry.SourceName
	Key         string
	Label       string
	Method      string
	Tokens15m   uint64
	Today       uint64
	TodayApprox bool
	LastEvent   time.Time
}

type Session struct {
	Source        telemetry.SourceName
	SessionID     string
	Activity      Activity
	Model         string
	ModelCount    int
	ProjectKey    string
	ProjectLabel  string
	ProjectMethod string
	Rate1m        float64
	Rate5m        float64
	Rate15m       float64
	Total         uint64
	TotalApprox   bool
	Input         uint64
	Output        uint64
	CacheRatio    *float64
	FirstEvent    time.Time
	LastEvent     time.Time
}
