package timeutil

import "time"

var _ Ticker = (*fakeTicker)(nil)

type fakeTicker struct {
	ch chan time.Time
}

func (t *fakeTicker) Chan() <-chan time.Time {
	return t.ch
}

func (m *fakeTicker) Stop() {}

func (m *fakeTicker) Tick() {
	m.ch <- time.Now()
}

func NewFakeTicker() *fakeTicker {
	return &fakeTicker{make(chan time.Time)}
}

func WrapFakeTicker(ticker *fakeTicker) NewTickerFunc {
	return func(d time.Duration) Ticker {
		return ticker
	}
}
