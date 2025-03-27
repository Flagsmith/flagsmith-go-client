package flagsmith

import (
	"testing"

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
