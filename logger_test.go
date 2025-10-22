package flagsmith

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlogToRestyAdapter_Errorf(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	adapter := newSlogToRestyAdapter(logger)

	adapter.Errorf("test error: %s", "bad")

	output := buf.String()
	assert.Contains(t, output, "test error: bad")
	assert.Contains(t, output, "level=ERROR")
}

func TestSlogToRestyAdapter_Warnf(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	adapter := newSlogToRestyAdapter(logger)

	adapter.Warnf("test warning: %s: %d", "warn", 42)

	output := buf.String()
	assert.Contains(t, output, "test warning: warn: 42")
	assert.Contains(t, output, "level=WARN")
}

func TestSlogToRestyAdapter_Debugf(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	adapter := newSlogToRestyAdapter(logger)

	adapter.Debugf("debug info: %s", "details")

	output := buf.String()
	assert.Contains(t, output, "debug info: details")
	assert.Contains(t, output, "level=DEBUG")
}
