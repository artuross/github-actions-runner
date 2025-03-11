package timeutil

import "time"

var _ Ticker = (*timeTicker)(nil)

type timeTicker struct {
	*time.Ticker
}

func (t *timeTicker) Chan() <-chan time.Time {
	return t.C
}

// NewTicker creates a new `Ticker` wrapping `time.NewTicker`.
func NewTicker(d time.Duration) *timeTicker {
	return &timeTicker{time.NewTicker(d)}
}
