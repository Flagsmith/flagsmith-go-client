package flagsmith

import (
	"context"
	"time"
)

const (
	initialBackoff = 200 * time.Millisecond
	maxBackoff     = 30 * time.Second
)

// backoff handles exponential backoff with jitter
type backoff struct {
	current time.Duration
}

// newBackoff creates a new backoff instance
func newBackoff() *backoff {
	return &backoff{
		current: initialBackoff,
	}
}

// next returns the next backoff duration and updates the current backoff
func (b *backoff) next() time.Duration {
	// Add jitter between 0-1s
	backoff := b.current + time.Duration(time.Now().UnixNano()%1e9)

	// Double the backoff time, but cap it
	if b.current < maxBackoff {
		b.current *= 2
	}

	return backoff
}

// reset resets the backoff to initial value
func (b *backoff) reset() {
	b.current = initialBackoff
}

// wait waits for the current backoff time, or until ctx is done
func (b *backoff) wait(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(b.next()):
	}
}
