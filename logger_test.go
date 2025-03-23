package flagsmith

import (
	"log/slog"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))

	os.Exit(m.Run())
}
