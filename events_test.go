package flagsmith

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEventsTimeout      = time.Second
	testEventsRetryBackoff = 10 * time.Millisecond
)

// eventsServer is a fake events API whose behaviour is chosen per request by respond.
type eventsServer struct {
	*httptest.Server

	mu       sync.Mutex
	bodies   [][]byte
	headers  []http.Header
	paths    []string
	respond  func(body []byte) int
	finished int
}

func newEventsServer(t *testing.T, respond func(body []byte) int) *eventsServer {
	t.Helper()
	s := &eventsServer{respond: respond}
	s.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.headers = append(s.headers, req.Header.Clone())
		s.paths = append(s.paths, req.URL.Path)
		s.mu.Unlock()

		status := http.StatusAccepted
		if s.respond != nil {
			status = s.respond(body)
		}

		s.mu.Lock()
		s.finished++
		s.mu.Unlock()
		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, `{"accepted": 0, "rejected": []}`)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *eventsServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *eventsServer) request(i int) (string, http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paths[i], s.headers[i]
}

func (s *eventsServer) finishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished
}

func (s *eventsServer) events(t *testing.T) []map[string]interface{} {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []map[string]interface{}
	for _, b := range s.bodies {
		var payload struct {
			Events []map[string]interface{} `json:"events"`
		}
		require.NoError(t, json.Unmarshal(b, &payload))
		events = append(events, payload.Events...)
	}
	return events
}

func newTestEventProcessor(ctx context.Context, url string, maxBufferSize int, flushInterval time.Duration) *EventProcessor {
	client := resty.New().SetHeader(EnvironmentKeyHeader, EnvironmentAPIKey)
	return newEventProcessor(ctx, client, url, maxBufferSize, flushInterval, testEventsTimeout, testEventsRetryBackoff, createLogger())
}

// bodyHas reports whether a request body contains an event with the given name.
func bodyHas(body []byte, name string) bool {
	return strings.Contains(string(body), `"event":"`+name+`"`)
}

func bufferedEvents(p *EventProcessor) []event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]event(nil), p.buffer...)
}

func TestEventProcessorBuffersCustomEvent(t *testing.T) {
	// Given
	p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
	before := time.Now().UnixMilli()

	// When
	p.TrackEvent("purchase", &EventOptions{
		Identifier: "user-123",
		Value:      49.0,
		Metadata:   map[string]interface{}{"currency": "EUR", "sdk_version": "caller"},
	})

	// Then
	events := bufferedEvents(p)
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, "purchase", e.Event)
	assert.Nil(t, e.FeatureName)
	require.NotNil(t, e.Identifier)
	assert.Equal(t, "user-123", *e.Identifier)
	require.NotNil(t, e.Value)
	assert.Equal(t, "49", *e.Value)
	assert.Nil(t, e.Traits)
	assert.Equal(t, map[string]interface{}{"currency": "EUR", "sdk_version": getSDKVersion()}, e.Metadata)
	assert.GreaterOrEqual(t, e.Timestamp, before)
	assert.LessOrEqual(t, e.Timestamp, time.Now().UnixMilli())

	raw, err := json.Marshal(e)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"feature_name":null`)
	assert.Contains(t, string(raw), `"traits":null`)
}

func TestEventProcessorTrackEventWithNilOptions(t *testing.T) {
	// Given
	p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)

	// When
	p.TrackEvent("signup", nil)

	// Then
	events := bufferedEvents(p)
	require.Len(t, events, 1)
	assert.Nil(t, events[0].Identifier)
	assert.Nil(t, events[0].Value)
	assert.Equal(t, map[string]interface{}{"sdk_version": getSDKVersion()}, events[0].Metadata)
}

func TestEventProcessorCopiesCallerMaps(t *testing.T) {
	// Given
	p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
	traits := map[string]interface{}{"plan": "premium"}
	metadata := map[string]interface{}{"k": "v"}

	// When
	p.TrackEvent("purchase", &EventOptions{Traits: traits, Metadata: metadata})
	traits["plan"] = "free"
	metadata["k"] = "changed"

	// Then
	events := bufferedEvents(p)
	assert.Equal(t, "premium", events[0].Traits["plan"])
	assert.Equal(t, "v", events[0].Metadata["k"])
	assert.NotContains(t, metadata, "sdk_version")
}

func TestStringifyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected *string
	}{
		{"nil", nil, nil},
		{"string", "buy-now", strPtr("buy-now")},
		{"float64 whole", 49.0, strPtr("49")},
		{"float64 fraction", 49.5, strPtr("49.5")},
		{"bool", true, strPtr("true")},
		{"int", 7, strPtr("7")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, stringifyValue(tt.input))
		})
	}
}

func strPtr(s string) *string { return &s }

func TestEventProcessorExposureDedupe(t *testing.T) {
	exposure := func(p *EventProcessor, feature, identifier string, value interface{}, experimentID int) {
		p.TrackExposureEvent(feature, identifier, value, nil, map[string]interface{}{"experiment_id": experimentID})
	}

	t.Run("equal exposures within a window are deduped", func(t *testing.T) {
		p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
		exposure(p, "f", "u", "treatment", 1)
		exposure(p, "f", "u", "treatment", 1)
		assert.Len(t, bufferedEvents(p), 1)
	})

	t.Run("differing exposures are not deduped", func(t *testing.T) {
		p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
		exposure(p, "f", "u", "treatment", 1)
		exposure(p, "f", "other-user", "treatment", 1)
		exposure(p, "f", "u", "control", 1)
		exposure(p, "g", "u", "treatment", 1)
		exposure(p, "f", "u", "treatment", 2)
		assert.Len(t, bufferedEvents(p), 5)
	})

	t.Run("equal exposure after a flush is buffered again", func(t *testing.T) {
		server := newEventsServer(t, nil)
		p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)
		exposure(p, "f", "u", "treatment", 1)
		require.NoError(t, p.Flush(t.Context()))
		exposure(p, "f", "u", "treatment", 1)
		assert.Len(t, bufferedEvents(p), 1)
	})

	t.Run("custom events are never deduped", func(t *testing.T) {
		p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
		p.TrackEvent("purchase", &EventOptions{Identifier: "u", Value: "1"})
		p.TrackEvent("purchase", &EventOptions{Identifier: "u", Value: "1"})
		assert.Len(t, bufferedEvents(p), 2)
	})

	t.Run("exposure without an identifier is not buffered", func(t *testing.T) {
		p := newTestEventProcessor(t.Context(), "http://localhost:1/", 100, 0)
		exposure(p, "f", "", "treatment", 1)
		assert.Empty(t, bufferedEvents(p))
	})
}

func TestEventProcessorFlushPostsBatch(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
	p.TrackExposureEvent("checkout_cta", "user-123", "treatment",
		map[string]interface{}{"plan": "premium"}, map[string]interface{}{"experiment_id": 167})
	p.TrackEvent("purchase", &EventOptions{Identifier: "user-123", Value: "49"})

	// When
	err := p.Flush(t.Context())

	// Then
	require.NoError(t, err)
	require.Equal(t, 1, server.requestCount())
	path, h := server.request(0)
	assert.Equal(t, "/v1/events", path)
	assert.Equal(t, EnvironmentAPIKey, h.Get(EnvironmentKeyHeader))
	assert.Regexp(t, `^flagsmith-go-sdk/`, h.Get("Flagsmith-SDK-User-Agent"))
	assert.Regexp(t, `^application/json`, h.Get("Content-Type"))

	events := server.events(t)
	require.Len(t, events, 2)
	assert.Equal(t, map[string]interface{}{
		"event":        FlagExposureEvent,
		"feature_name": "checkout_cta",
		"identifier":   "user-123",
		"value":        "treatment",
		"traits":       map[string]interface{}{"plan": "premium"},
		"metadata":     map[string]interface{}{"experiment_id": 167.0, "sdk_version": getSDKVersion()},
		"timestamp":    events[0]["timestamp"],
	}, events[0])
	assert.Equal(t, "purchase", events[1]["event"])
	assert.Nil(t, events[1]["feature_name"])
	assert.Empty(t, bufferedEvents(p))
}

func TestEventProcessorFlushEmptyBufferSendsNothing(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)

	// When
	err := p.Flush(t.Context())

	// Then
	assert.NoError(t, err)
	assert.Equal(t, 0, server.requestCount())
}

func TestEventProcessorAutoFlushAtMaxBufferSize(t *testing.T) {
	// Given: a slow server
	server := newEventsServer(t, func([]byte) int {
		time.Sleep(200 * time.Millisecond)
		return http.StatusAccepted
	})
	p := newTestEventProcessor(t.Context(), server.URL+"/", 2, 0)

	// When
	start := time.Now()
	p.TrackEvent("a", nil)
	p.TrackEvent("b", nil)
	elapsed := time.Since(start)

	// Then: the caller is not blocked by the POST
	assert.Less(t, elapsed, 100*time.Millisecond)
	assert.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, 5*time.Millisecond)
	assert.Len(t, server.events(t), 2)
}

func TestEventProcessorFlushWaitsForInFlightBatch(t *testing.T) {
	// Given: a batch started by the buffer-full trigger on a slow server
	server := newEventsServer(t, func([]byte) int {
		time.Sleep(100 * time.Millisecond)
		return http.StatusAccepted
	})
	p := newTestEventProcessor(t.Context(), server.URL+"/", 1, 0)
	start := time.Now()
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When: the buffer is already empty
	err := p.Flush(t.Context())

	// Then: Flush still waited for the batch in flight
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
	assert.Equal(t, 1, server.finishedCount())
}

func TestEventProcessorFlushReturnsWhileLaterBatchInFlight(t *testing.T) {
	// Given: a server that holds each batch until it is released
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	server := newEventsServer(t, func(body []byte) int {
		if bodyHas(body, "first") {
			<-releaseFirst
		} else {
			<-releaseSecond
		}
		return http.StatusAccepted
	})
	// Registered after the server, so it runs first and a failing test cannot hang in Close.
	var releaseOnce, releaseSecondOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	releaseLater := func() { releaseSecondOnce.Do(func() { close(releaseSecond) }) }
	t.Cleanup(func() { release(); releaseLater() })
	p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)

	p.TrackEvent("first", nil)
	firstDone := make(chan error, 1)
	go func() { firstDone <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When: a second batch starts after the first Flush was called
	p.TrackEvent("second", nil)
	secondDone := make(chan error, 1)
	go func() { secondDone <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 2 }, time.Second, time.Millisecond)
	release()

	// Then: the first Flush returns without waiting for the second batch
	select {
	case err := <-firstDone:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Flush waited for a batch started after it was called")
	}
	select {
	case <-secondDone:
		t.Fatal("second Flush returned before its batch completed")
	default:
	}

	releaseLater()
	select {
	case err := <-secondDone:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("second Flush did not return")
	}
}

func TestEventProcessorFlushIsBoundedUnderSustainedTraffic(t *testing.T) {
	// Given: every event fills the buffer and every POST takes 20ms
	server := newEventsServer(t, func([]byte) int {
		time.Sleep(20 * time.Millisecond)
		return http.StatusAccepted
	})
	p := newTestEventProcessor(t.Context(), server.URL+"/", 1, 0)
	stop := make(chan struct{})
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				p.TrackEvent("tick", nil)
			}
		}
	}()
	defer func() {
		close(stop)
		<-streamDone
	}()
	require.Eventually(t, func() bool { return server.requestCount() >= 3 }, time.Second, time.Millisecond)

	// When
	flushed := make(chan error, 1)
	go func() { flushed <- p.Flush(t.Context()) }()

	// Then: Flush returns although batches keep starting
	select {
	case err := <-flushed:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Error("Flush did not return under sustained traffic")
	}
}

func TestEventProcessorFlushWaitsForEveryBatchWhenOneFails(t *testing.T) {
	// Given: one batch fails fast and another succeeds slowly
	var slowDone sync.WaitGroup
	slowDone.Add(1)
	var mu sync.Mutex
	slowFinished := false
	server := newEventsServer(t, func(body []byte) int {
		if bodyHas(body, "fail") {
			time.Sleep(20 * time.Millisecond)
			return http.StatusBadRequest
		}
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		slowFinished = true
		mu.Unlock()
		return http.StatusAccepted
	})
	p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)

	p.TrackEvent("fail", nil)
	go func() { _ = p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)
	p.TrackEvent("slow", nil)
	go func() { defer slowDone.Done(); _ = p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 2 }, time.Second, time.Millisecond)

	// When
	err := p.Flush(t.Context())

	// Then: Flush waited for the slow batch even though the other one failed
	assert.NoError(t, err)
	mu.Lock()
	assert.True(t, slowFinished)
	mu.Unlock()
	slowDone.Wait()
}

func TestEventProcessorFlushHonoursContextWhileWaiting(t *testing.T) {
	// Given: a batch in flight on a server that takes longer than the caller will wait
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	defer close(release)
	client := resty.New()
	p := NewEventProcessor(t.Context(), client, server.URL+"/", 1, 0, time.Second, createLogger())
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := p.Flush(ctx)

	// Then
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestEventProcessorRetries(t *testing.T) {
	t.Run("500 then 202 retries once and delivers once", func(t *testing.T) {
		statuses := []int{http.StatusInternalServerError, http.StatusAccepted}
		var mu sync.Mutex
		server := newEventsServer(t, func([]byte) int {
			mu.Lock()
			defer mu.Unlock()
			s := statuses[0]
			statuses = statuses[1:]
			return s
		})
		p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		assert.NoError(t, err)
		assert.Equal(t, 2, server.requestCount())
		assert.Empty(t, bufferedEvents(p))
	})

	t.Run("500 twice drops the batch", func(t *testing.T) {
		server := newEventsServer(t, func([]byte) int { return http.StatusServiceUnavailable })
		p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		assert.Error(t, err)
		assert.Equal(t, 2, server.requestCount())
		assert.Empty(t, bufferedEvents(p))
		require.NoError(t, p.Flush(t.Context()))
		assert.Equal(t, 2, server.requestCount())
	})

	t.Run("400 is dropped without a retry", func(t *testing.T) {
		server := newEventsServer(t, func([]byte) int { return http.StatusBadRequest })
		p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		var apiErr *FlagsmithAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.ResponseStatusCode)
		assert.Equal(t, 1, server.requestCount())
		assert.Empty(t, bufferedEvents(p))
	})

	t.Run("transport error is retried then dropped", func(t *testing.T) {
		server := newEventsServer(t, nil)
		url := server.URL + "/"
		server.Close()
		p := newTestEventProcessor(t.Context(), url, 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		assert.Error(t, err)
		assert.Empty(t, bufferedEvents(p))
	})
}

func TestEventProcessorTickerFlush(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	p := newTestEventProcessor(t.Context(), server.URL+"/", 100, 20*time.Millisecond)

	// When
	p.TrackEvent("purchase", nil)

	// Then
	assert.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, 5*time.Millisecond)
}

func TestEventProcessorCancelFlushesAndStops(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	p := newTestEventProcessor(ctx, server.URL+"/", 100, time.Hour)
	p.TrackEvent("purchase", nil)
	assert.Equal(t, 0, server.requestCount())

	// When
	cancel()

	// Then
	select {
	case <-p.stopped:
	case <-time.After(time.Second):
		t.Fatal("event processor goroutine did not exit")
	}
	assert.Equal(t, 1, server.requestCount())
	assert.Len(t, server.events(t), 1)
}

func TestEventProcessorCancelWaitsForInFlightBatch(t *testing.T) {
	// Given: a batch started by the buffer-full trigger on a slow server
	server := newEventsServer(t, func([]byte) int {
		time.Sleep(30 * time.Millisecond)
		return http.StatusAccepted
	})
	ctx, cancel := context.WithCancel(t.Context())
	client := resty.New()
	p := NewEventProcessor(ctx, client, server.URL+"/", 1, 0, time.Second, createLogger())
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When
	cancel()

	// Then: the in-flight batch is not aborted by the cancellation
	<-p.stopped
	assert.Equal(t, 1, server.finishedCount())
}

func TestEventProcessorZeroFlushIntervalDisablesTicker(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	p := newTestEventProcessor(t.Context(), server.URL+"/", 2, 0)

	// When
	p.TrackEvent("a", nil)
	time.Sleep(50 * time.Millisecond)

	// Then
	assert.Equal(t, 0, server.requestCount())

	// When
	p.TrackEvent("b", nil)

	// Then
	assert.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, 5*time.Millisecond)
}

// panickingHandler is a slog.Handler that panics on every record.
type panickingHandler struct{}

func (panickingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (panickingHandler) Handle(context.Context, slog.Record) error {
	panic(errors.New("logger failure"))
}
func (h panickingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h panickingHandler) WithGroup(string) slog.Handler      { return h }

func TestEventProcessorNeverPanicsWhenLoggerPanics(t *testing.T) {
	// Given
	server := newEventsServer(t, func([]byte) int { return http.StatusBadRequest })
	client := resty.New()
	p := newEventProcessor(t.Context(), client, server.URL+"/", 100, 0, testEventsTimeout, testEventsRetryBackoff, slog.New(panickingHandler{}))

	// When, Then
	assert.NotPanics(t, func() {
		p.TrackExposureEvent("f", "u", "v", nil, nil)
		p.TrackExposureEvent("f", "u", "v", nil, nil)
		p.TrackExposureEvent("f", "", "v", nil, nil)
		assert.Error(t, p.Flush(t.Context()))
	})
	assert.Equal(t, 1, server.requestCount())
}

func TestEventProcessorWorkerSurvivesPanickingLogger(t *testing.T) {
	// Given: batches sent by the worker are rejected, which is logged
	server := newEventsServer(t, func([]byte) int { return http.StatusBadRequest })
	client := resty.New()
	p := newEventProcessor(t.Context(), client, server.URL+"/", 1, 0, testEventsTimeout, testEventsRetryBackoff, slog.New(panickingHandler{}))

	// When
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.finishedCount() == 1 }, time.Second, time.Millisecond)
	p.TrackEvent("b", nil)

	// Then: the process is still alive and the worker still sends
	assert.Eventually(t, func() bool { return server.finishedCount() == 2 }, time.Second, time.Millisecond)
	assert.NoError(t, p.Flush(t.Context()))
}

func TestNewEventProcessorRetryBackoffDefault(t *testing.T) {
	assert.Equal(t, 50*time.Millisecond, defaultEventsRetryBackoff(50*time.Millisecond))
	assert.Equal(t, time.Second, defaultEventsRetryBackoff(10*time.Second))
	assert.Equal(t, time.Second, defaultEventsRetryBackoff(0))
}

func TestEventProcessorAttemptTimeout(t *testing.T) {
	// Given: a server slower than the per-attempt timeout
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	defer close(release)
	p := newEventProcessor(t.Context(), resty.New(), server.URL+"/", 100, 0, 30*time.Millisecond, testEventsRetryBackoff, createLogger())
	p.TrackEvent("a", nil)

	// When
	start := time.Now()
	err := p.Flush(t.Context())

	// Then: both attempts time out and the batch is dropped
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Equal(t, 2, server.requestCount())
}

func TestNewEventProcessorAddsTrailingSlash(t *testing.T) {
	// Given
	server := newEventsServer(t, nil)
	p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
	p.TrackEvent("a", nil)

	// When
	require.NoError(t, p.Flush(t.Context()))

	// Then
	path, _ := server.request(0)
	assert.Equal(t, "/v1/events", path)
}
