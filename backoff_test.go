package flagsmith

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff(t *testing.T) {
	// Given
	b := newBackoff()

	// When
	first := b.next()
	second := b.next()
	third := b.next()

	// Then
	assert.LessOrEqual(t, third, maxBackoff, "Backoff should not exceed max")

	// Backoff increases across attempts
	assert.Greater(t, second, first, "Second backoff should be greater than the first")
	assert.Greater(t, third, second, "Third backoff should be greater than the second")
}

func TestBackoffReset(t *testing.T) {
	b := newBackoff()
	assert.Greater(t, b.next(), initialBackoff)
	b.reset()
	assert.Equal(t, initialBackoff, b.current, "Reset should return to initial backoff")
}

func TestBackoffWithJitterDoublesFromInitial(t *testing.T) {
	// Given
	b := newBackoffWithJitter(10*time.Millisecond, time.Second, func(d time.Duration) time.Duration { return d })

	// When, Then
	assert.Equal(t, 10*time.Millisecond, b.next())
	assert.Equal(t, 20*time.Millisecond, b.next())
	assert.Equal(t, 40*time.Millisecond, b.next())
	b.reset()
	assert.Equal(t, 10*time.Millisecond, b.next())
}

func TestEqualJitterBounds(t *testing.T) {
	for i := 0; i < 1000; i++ {
		d := equalJitter(100 * time.Millisecond)
		assert.GreaterOrEqual(t, d, 50*time.Millisecond)
		assert.LessOrEqual(t, d, 100*time.Millisecond)
	}
	assert.Equal(t, time.Duration(0), equalJitter(0))
}
