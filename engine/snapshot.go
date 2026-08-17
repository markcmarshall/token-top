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
	GeneratedAt time.Time
	Global      Global
	Sources     []SourceSnapshot
	Sessions    []Session
	QuietHidden int
}

type Global struct {
	Rate1m      float64
	Rate5m      float64
	Rate15m     float64
	Today       uint64
	TodayApprox bool
	Burning     int
	Recent      int
}

type SourceSnapshot struct {
	Name    telemetry.SourceName
	Health  telemetry.SourceHealth
	Rate1m  float64
	Share1m float64
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
