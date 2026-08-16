package engine

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type FixedClock struct {
	T time.Time
}

func (c *FixedClock) Now() time.Time { return c.T }

func (c *FixedClock) Set(t time.Time) { c.T = t }

func (c *FixedClock) Advance(d time.Duration) { c.T = c.T.Add(d) }
