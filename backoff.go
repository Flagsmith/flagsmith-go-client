package flagsmith

import (
	"context"
	"math/rand/v2"
	"time"
)

const (
	initialBackoff = 200 * time.Millisecond
	maxBackoff     = 30 * time.Second
)

// backoff handles exponential backoff with jitter.
type backoff struct {
	initial time.Duration
	max     time.Duration
	current time.Duration
	// jitter turns the current base duration into the duration to wait.
	jitter func(time.Duration) time.Duration
}

// newBackoff creates a new backoff instance starting at 200ms, with up to one second of
// jitter added to every wait.
func newBackoff() *backoff {
	return newBackoffWithJitter(initialBackoff, maxBackoff, subSecondJitter)
}

// newBackoffWithJitter creates a backoff starting at initial, doubling on every call to
// next until it reaches max.
func newBackoffWithJitter(initial, max time.Duration, jitter func(time.Duration) time.Duration) *backoff {
	return &backoff{initial: initial, max: max, current: initial, jitter: jitter}
}

// next returns the next backoff duration and updates the current backoff.
func (b *backoff) next() time.Duration {
	d := b.jitter(b.current)

	// Double the backoff time, but cap it
	if b.current < b.max {
		b.current *= 2
	}

	return d
}

// reset resets the backoff to initial value.
func (b *backoff) reset() {
	b.current = b.initial
}

// wait waits for the current backoff time, or until ctx is done.
func (b *backoff) wait(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(b.next()):
	}
}

// subSecondJitter adds between 0 and 1s to d.
func subSecondJitter(d time.Duration) time.Duration {
	return d + time.Duration(time.Now().UnixNano()%1e9)
}

// fullJitter returns a random duration between 0 and d.
func fullJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return rand.N(d + 1)
}
