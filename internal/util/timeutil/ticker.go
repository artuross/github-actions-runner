package timeutil

import "time"

// Ticker wraps `time.Ticker` in an interface for testing.
type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

// NewTickerFunc is a factory function that creates a new `Ticker`.
type NewTickerFunc newTickerFunc[Ticker]

type newTickerFunc[T Ticker] func(d time.Duration) T

func Generic[T Ticker](f newTickerFunc[T]) NewTickerFunc {
	return func(d time.Duration) Ticker {
		return f(d)
	}
}
