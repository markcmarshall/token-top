package telemetry

import (
	"math"
	"time"
)

type TokenEvent struct {
	ID        string
	Source    SourceName
	SessionID string
	Timestamp time.Time
	Model     string
	CWD       string

	Input      uint64
	Output     uint64
	CacheRead  *uint64
	CacheWrite *uint64
	Reasoning  *uint64

	Complete bool
}

func (e TokenEvent) Total() uint64 {
	sum, ok := AddUint64(e.Input, e.Output)
	if !ok {
		return math.MaxUint64
	}
	return sum
}

func (e TokenEvent) Validate() error {
	if e.ID == "" {
		return invalidError("missing id")
	}
	if e.Source == "" {
		return invalidError("missing source")
	}
	if e.SessionID == "" {
		return invalidError("missing session id")
	}
	if e.Timestamp.IsZero() {
		return invalidError("missing timestamp")
	}
	if _, ok := AddUint64(e.Input, e.Output); !ok {
		return invalidError("input+output overflow")
	}
	if e.CacheRead != nil && *e.CacheRead > e.Input {
		return invalidError("cache read exceeds input")
	}
	if e.CacheWrite != nil && *e.CacheWrite > e.Input {
		return invalidError("cache write exceeds input")
	}
	if e.Reasoning != nil && *e.Reasoning > e.Output {
		return invalidError("reasoning exceeds output")
	}
	return nil
}

func AddUint64(a, b uint64) (uint64, bool) {
	if b > math.MaxUint64-a {
		return 0, false
	}
	return a + b, true
}

func Uint64Ptr(v uint64) *uint64 {
	return &v
}

type invalidError string

func (e invalidError) Error() string { return string(e) }
