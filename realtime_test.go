package flagsmith

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v5/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeStartUnblocksOnContextCancel(t *testing.T) {
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

	client := &Client{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rt := newRealtime(client, ctx, server.URL+"/stream", time.Unix(0, 0))

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

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("realtime.start() remained blocked after context cancellation")
	}
}

func TestRealtimeConnectProcessesUpdatedAtEvent(t *testing.T) {
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

	client := NewClient(
		fixtures.EnvironmentAPIKey,
		WithBaseURL(server.URL+"/api/v1/"),
		WithSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	rt := newRealtime(client, ctx, server.URL+"/sse/environments/"+fixtures.EnvironmentAPIKey+"/stream", time.Unix(0, 0))

	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.start()
	}()

	assert.Eventually(t, func() bool {
		return envRequests.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("realtime.start() remained blocked after context cancellation")
	}
}
