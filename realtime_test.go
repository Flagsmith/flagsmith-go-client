package flagsmith

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v5/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newTestRealtimeLogger(buf *syncLogBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestRealtimeStartUnblocksOnContextCancel(t *testing.T) {
	// Given
	streamConnected := make(chan struct{})
	releaseStream := make(chan struct{})
	defer close(releaseStream)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "text/event-stream")
		rw.WriteHeader(http.StatusOK)
		if flusher, ok := rw.(http.Flusher); ok {
			_, _ = io.WriteString(rw, ": connected\n\n")
			flusher.Flush()
		}
		close(streamConnected)
		select {
		case <-req.Context().Done():
		case <-releaseStream:
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logs syncLogBuffer
	client := &Client{log: newTestRealtimeLogger(&logs)}
	rt := newRealtime(client, ctx, server.URL+"/stream", time.Unix(0, 0))

	// When
	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.start()
	}()

	select {
	case <-streamConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE stream connection")
	}

	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "msg=connected")
	}, 2*time.Second, 5*time.Millisecond)

	cancel()

	// Then
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("realtime.start() remained blocked after context cancellation")
	}

	logOutput := logs.String()
	assert.Contains(t, logOutput, `msg="connecting to realtime"`)
	assert.Contains(t, logOutput, "msg=connected")
	assert.Contains(t, logOutput, "msg=stopped")
}

func TestRealtimeConnectProcessesUpdatedAtEvent(t *testing.T) {
	// Given
	var envRequests atomic.Int32
	releaseStream := make(chan struct{})
	defer close(releaseStream)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/environment-document/":
			envRequests.Add(1)
			fixtures.EnvironmentDocumentHandler(rw, req)
		case "/sse/environments/" + fixtures.EnvironmentAPIKey + "/stream":
			rw.Header().Set("Content-Type", "text/event-stream")
			rw.WriteHeader(http.StatusOK)
			flusher, ok := rw.(http.Flusher)
			require.True(t, ok)

			_, _ = fmt.Fprintf(rw, "data: {\"updated_at\": %f}\n\n", 1900000000.0)
			flusher.Flush()
			select {
			case <-req.Context().Done():
			case <-releaseStream:
			}
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logs syncLogBuffer
	client := NewClient(
		fixtures.EnvironmentAPIKey,
		WithBaseURL(server.URL+"/api/v1/"),
		WithSlogLogger(newTestRealtimeLogger(&logs)),
	)
	rt := newRealtime(client, ctx, server.URL+"/sse/environments/"+fixtures.EnvironmentAPIKey+"/stream", time.Unix(0, 0))

	// When
	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.start()
	}()

	// Then
	assert.Eventually(t, func() bool {
		return envRequests.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("realtime.start() remained blocked after context cancellation")
	}

	logOutput := logs.String()
	assert.Contains(t, logOutput, `msg="connecting to realtime"`)
	assert.Contains(t, logOutput, "msg=connected")
	assert.Contains(t, logOutput, "msg=stopped")
}
