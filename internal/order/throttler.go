package order

import "time"

// Throttler manages message rate limiting.
type Throttler struct {
	interval time.Duration
}

// NewThrottler creates a throttler for a specified rate limit in messages per second.
func NewThrottler(rateLimitMPS int) *Throttler {
	if rateLimitMPS <= 0 {
		rateLimitMPS = 50
	}
	return &Throttler{
		interval: time.Second / time.Duration(rateLimitMPS),
	}
}

// Interval returns the calculated tick interval.
func (t *Throttler) Interval() time.Duration {
	return t.interval
}

// Enforce sleeps if processing elapsed faster than the tick interval.
func (t *Throttler) Enforce(elapsed time.Duration) {
	if elapsed < t.interval {
		time.Sleep(t.interval - elapsed)
	}
}
