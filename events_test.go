package flagsmith

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
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
	finished int
}

func newEventsServer(t *testing.T, respond func(body []byte) int) *eventsServer {
	t.Helper()
	return newEventsServerWithBody(t, func(body []byte) (int, string) {
		status := http.StatusAccepted
		if respond != nil {
			status = respond(body)
		}
		return status, `{"accepted": 0, "rejected": []}`
	})
}

// newEventsServerWithBody is newEventsServer with control over the response body. A
// status of 0 closes the connection without a response, causing a transport error.
func newEventsServerWithBody(t *testing.T, respond func(body []byte) (int, string)) *eventsServer {
	t.Helper()
	s := &eventsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.headers = append(s.headers, req.Header.Clone())
		s.paths = append(s.paths, req.URL.Path)
		s.mu.Unlock()

		status, respBody := respond(body)

		s.mu.Lock()
		s.finished++
		s.mu.Unlock()
		if status == 0 {
			conn, _, err := rw.(http.Hijacker).Hijack()
			if assert.NoError(t, err) {
				_ = conn.Close()
			}
			return
		}
		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, respBody)
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

func testEventsConfig(url string, maxBufferSize int, flushInterval time.Duration) eventProcessorConfig {
	return eventProcessorConfig{
		baseURL:       url,
		maxBufferSize: maxBufferSize,
		flushInterval: flushInterval,
		timeout:       testEventsTimeout,
		retryBackoff:  testEventsRetryBackoff,
		log:           createLogger(),
	}
}

func newTestEventProcessor(ctx context.Context, url string, maxBufferSize int, flushInterval time.Duration) *EventProcessor {
	return newTestEventProcessorWith(ctx, testEventsConfig(url, maxBufferSize, flushInterval))
}

func newTestEventProcessorWith(ctx context.Context, cfg eventProcessorConfig) *EventProcessor {
	client := resty.New().SetHeader(EnvironmentKeyHeader, EnvironmentAPIKey)
	return newEventProcessor(ctx, client, cfg)
}

// statusSequence answers with each status in turn, then repeats the last one.
func statusSequence(statuses ...int) func([]byte) int {
	var mu sync.Mutex
	return func([]byte) int {
		mu.Lock()
		defer mu.Unlock()
		s := statuses[0]
		if len(statuses) > 1 {
			statuses = statuses[1:]
		}
		return s
	}
}

// recordingHandler is a slog.Handler that keeps every record.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// matching returns the attributes of every record at level whose message is msg.
func (h *recordingHandler) matching(level slog.Level, msg string) []map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	var found []map[string]interface{}
	for _, r := range h.records {
		if r.Level != level || r.Message != msg {
			continue
		}
		attrs := map[string]interface{}{}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.Any()
			return true
		})
		found = append(found, attrs)
	}
	return found
}

// text renders every record, message and attributes, as one string.
func (h *recordingHandler) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var b strings.Builder
	for _, r := range h.records {
		b.WriteString(r.Level.String() + " " + r.Message)
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(" " + a.String())
			return true
		})
		b.WriteString("\n")
	}
	return b.String()
}

func inFlightCount(p *EventProcessor) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inFlight)
}

func eventNames(events []event) []string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = e.Event
	}
	return names
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
	t.Run("503 then 202 retries and delivers once", func(t *testing.T) {
		server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable, http.StatusAccepted))
		p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		assert.NoError(t, err)
		assert.Equal(t, 2, server.requestCount())
		assert.Empty(t, bufferedEvents(p))
		assert.Zero(t, p.DroppedEvents())
	})

	t.Run("503 is attempted three times in total", func(t *testing.T) {
		server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
		p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		var apiErr *FlagsmithAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.ResponseStatusCode)
		assert.Equal(t, 3, server.requestCount())
	})

	t.Run("transport error is attempted three times in total", func(t *testing.T) {
		server := newEventsServerWithBody(t, func([]byte) (int, string) { return 0, "" })
		p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
		p.TrackEvent("purchase", nil)

		err := p.Flush(t.Context())

		assert.Error(t, err)
		assert.Equal(t, 3, server.requestCount())
		assert.Len(t, bufferedEvents(p), 1)
	})

	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(fmt.Sprintf("%d is retried then kept", status), func(t *testing.T) {
			server := newEventsServer(t, statusSequence(status))
			p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
			p.TrackEvent("purchase", nil)

			err := p.Flush(t.Context())

			var apiErr *FlagsmithAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.ResponseStatusCode)
			assert.Equal(t, 3, server.requestCount())
			assert.Len(t, bufferedEvents(p), 1)
			assert.Zero(t, p.DroppedEvents())
		})
	}

	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity,
		http.StatusInternalServerError,
		http.StatusNotImplemented,
	} {
		t.Run(fmt.Sprintf("%d is dropped after one attempt", status), func(t *testing.T) {
			server := newEventsServer(t, statusSequence(status))
			p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
			p.TrackEvent("purchase", nil)

			err := p.Flush(t.Context())

			var apiErr *FlagsmithAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.ResponseStatusCode)
			assert.Equal(t, 1, server.requestCount())
			assert.Empty(t, bufferedEvents(p))
			assert.Equal(t, int64(1), p.DroppedEvents())
			require.NoError(t, p.Flush(t.Context()))
			assert.Equal(t, 1, server.requestCount())
		})
	}
}

func TestEventProcessorRetryBackoffIsExponentialWithJitter(t *testing.T) {
	// Given: sleeps are recorded instead of waited
	server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
	var mu sync.Mutex
	var bases, waits []time.Duration
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.retryBackoff = 100 * time.Millisecond
	cfg.jitter = func(d time.Duration) time.Duration {
		mu.Lock()
		defer mu.Unlock()
		bases = append(bases, d)
		return d - time.Millisecond
	}
	cfg.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		return nil
	}
	p := newTestEventProcessorWith(t.Context(), cfg)
	p.TrackEvent("purchase", nil)

	// When
	require.Error(t, p.Flush(t.Context()))

	// Then: one wait before each retry, doubling, with the jitter applied
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, bases)
	assert.Equal(t, []time.Duration{99 * time.Millisecond, 199 * time.Millisecond}, waits)
}

func TestEventProcessorDefaultJitterStaysWithinBackoff(t *testing.T) {
	// Given
	server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
	var mu sync.Mutex
	var waits []time.Duration
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.retryBackoff = 100 * time.Millisecond
	cfg.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		return nil
	}
	p := newTestEventProcessorWith(t.Context(), cfg)
	p.TrackEvent("purchase", nil)

	// When
	require.Error(t, p.Flush(t.Context()))

	// Then: full jitter, between zero and the backoff
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, waits, 2)
	assert.GreaterOrEqual(t, waits[0], time.Duration(0))
	assert.LessOrEqual(t, waits[0], 100*time.Millisecond)
	assert.GreaterOrEqual(t, waits[1], time.Duration(0))
	assert.LessOrEqual(t, waits[1], 200*time.Millisecond)
}

func TestEventProcessorFailedRetryableBatchIsKeptForNextFlush(t *testing.T) {
	// Given: the events API is down for one flush, then back
	server := newEventsServer(t, statusSequence(
		http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable,
		http.StatusAccepted,
	))
	p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
	p.TrackEvent("first", nil)

	// When
	err := p.Flush(t.Context())

	// Then: Flush returns the error without looping, and the batch is back in the buffer
	assert.Error(t, err)
	assert.Equal(t, 3, server.requestCount())
	assert.Equal(t, []string{"first"}, eventNames(bufferedEvents(p)))

	// When: an event tracked since is flushed with it
	p.TrackEvent("second", nil)
	require.NoError(t, p.Flush(t.Context()))

	// Then: the kept event is sent first, and nothing is lost
	events := server.events(t)
	require.Len(t, events, 5)
	assert.Equal(t, "first", events[3]["event"])
	assert.Equal(t, "second", events[4]["event"])
	assert.Empty(t, bufferedEvents(p))
	assert.Zero(t, p.DroppedEvents())
}

func TestEventProcessorBufferOverflowDropsOldest(t *testing.T) {
	// Given: a batch held by an events API that then fails it
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusServiceUnavailable
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p := newTestEventProcessor(t.Context(), server.URL, 3, 0)
	p.TrackEvent("old-1", nil)
	p.TrackEvent("old-2", nil)
	flushed := make(chan error, 1)
	go func() { flushed <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When: newer events are tracked meanwhile, then the batch comes back
	p.TrackEvent("new-1", nil)
	p.TrackEvent("new-2", nil)
	once.Do(func() { close(release) })
	require.Error(t, <-flushed)

	// Then: the buffer keeps the newest maxBufferSize events and counts the dropped one
	assert.Equal(t, []string{"old-2", "new-1", "new-2"}, eventNames(bufferedEvents(p)))
	assert.Equal(t, int64(1), p.DroppedEvents())
}

func TestEventProcessorUnauthorisedStops(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			// Given: two batches in flight when the events API rejects the key
			release := make(chan struct{})
			server := newEventsServer(t, func([]byte) int {
				<-release
				return status
			})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			logs := &recordingHandler{}
			cfg := testEventsConfig(server.URL, 100, 20*time.Millisecond)
			cfg.log = slog.New(logs)
			p := newTestEventProcessorWith(t.Context(), cfg)

			p.TrackEvent("a", nil)
			first := make(chan error, 1)
			go func() { first <- p.Flush(t.Context()) }()
			require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)
			p.TrackEvent("b", nil)
			second := make(chan error, 1)
			go func() { second <- p.Flush(t.Context()) }()
			require.Eventually(t, func() bool { return server.requestCount() == 2 }, time.Second, time.Millisecond)

			// When
			once.Do(func() { close(release) })

			// Then: both fail without a retry, and the processor stops
			var apiErr *FlagsmithAPIError
			require.ErrorAs(t, <-first, &apiErr)
			assert.Equal(t, status, apiErr.ResponseStatusCode)
			require.Error(t, <-second)
			assert.Equal(t, 2, server.requestCount())
			select {
			case <-p.stopped:
			case <-time.After(time.Second):
				t.Fatal("worker goroutine and ticker were not stopped")
			}

			// Then: tracking is a no-op and nothing more is sent
			p.TrackEvent("c", nil)
			p.TrackExposureEvent("f", "u", "v", nil, nil)
			assert.Empty(t, bufferedEvents(p))
			assert.NoError(t, p.Flush(t.Context()))
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, 2, server.requestCount())
			assert.Equal(t, int64(2), p.DroppedEvents())

			// Then: logged exactly once
			assert.Len(t, logs.matching(slog.LevelWarn, "events API rejected the environment key; event tracking is disabled"), 1)
			assert.Empty(t, logs.matching(slog.LevelError, "events API rejected the environment key; event tracking is disabled"))
		})
	}
}

func TestEventProcessorUnauthorisedDiscardsBuffer(t *testing.T) {
	// Given: a batch in flight and more events buffered behind it
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusUnauthorized
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
	p.TrackEvent("a", nil)
	flushed := make(chan error, 1)
	go func() { flushed <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)
	p.TrackEvent("b", nil)
	p.TrackEvent("c", nil)

	// When
	once.Do(func() { close(release) })
	require.Error(t, <-flushed)

	// Then
	assert.Empty(t, bufferedEvents(p))
	assert.Equal(t, int64(3), p.DroppedEvents())
}

func TestEventProcessorLogsRejectedEventsAndDoesNotResend(t *testing.T) {
	// Given: the events API accepts the batch but rejects its second event
	server := newEventsServerWithBody(t, func([]byte) (int, string) {
		return http.StatusAccepted, `{"accepted": 1, "rejected": [{"index": 1, "error": "timestamp out of range"}]}`
	})
	logs := &recordingHandler{}
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.log = slog.New(logs)
	p := newTestEventProcessorWith(t.Context(), cfg)
	p.TrackEvent("good", nil)
	p.TrackExposureEvent("checkout_cta", "user@example.com", "treatment", map[string]interface{}{"email": "trait@example.com"}, nil)

	// When
	err := p.Flush(t.Context())

	// Then: the rejection is logged at warn with the event it refers to
	require.NoError(t, err)
	rejected := logs.matching(slog.LevelWarn, "events API rejected event")
	require.Len(t, rejected, 1)
	assert.Equal(t, int64(1), rejected[0]["index"])
	assert.Equal(t, "timestamp out of range", rejected[0]["error"])
	assert.Equal(t, FlagExposureEvent, rejected[0]["event"])
	assert.Equal(t, "checkout_cta", rejected[0]["feature"])
	assert.NotContains(t, rejected[0], "identifier")
	assert.NotContains(t, logs.text(), "user@example.com")
	assert.NotContains(t, logs.text(), "trait@example.com")
	assert.Equal(t, int64(1), p.DroppedEvents())

	// Then: it is not sent again
	assert.Empty(t, bufferedEvents(p))
	require.NoError(t, p.Flush(t.Context()))
	assert.Equal(t, 1, server.requestCount())
}

func TestEventProcessorLogsNoIdentifiersOrTraits(t *testing.T) {
	// Given: an events API that rejects every batch
	server := newEventsServer(t, statusSequence(http.StatusBadRequest))
	logs := &recordingHandler{}
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.log = slog.New(logs)
	p := newTestEventProcessorWith(t.Context(), cfg)
	traits := map[string]interface{}{"email": "trait@example.com"}

	// When: an exposure is deduped, a custom event is tracked, and the batch is rejected
	p.TrackExposureEvent("checkout_cta", "user@example.com", "treatment", traits, nil)
	p.TrackExposureEvent("checkout_cta", "user@example.com", "treatment", traits, nil)
	p.TrackEvent("purchase", &EventOptions{Identifier: "user@example.com", Traits: traits})
	require.Error(t, p.Flush(t.Context()))

	// Then
	require.NotEmpty(t, logs.matching(slog.LevelDebug, "skipping duplicate exposure"))
	require.NotEmpty(t, logs.matching(slog.LevelWarn, "events API rejected batch; dropping it"))
	assert.NotContains(t, logs.text(), "user@example.com")
	assert.NotContains(t, logs.text(), "trait@example.com")
}

func TestEventProcessorStalledServerBoundsInFlightBatches(t *testing.T) {
	// Given: an events API that holds every request until released
	const maxBufferSize = 10
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p := newTestEventProcessor(t.Context(), server.URL, maxBufferSize, 0)
	goroutinesBefore := runtime.NumGoroutine()

	// When: five buffers' worth of events are tracked while the first send is stalled
	for i := 1; i <= 5*maxBufferSize; i++ {
		p.TrackEvent(fmt.Sprintf("e%d", i), nil)

		// Then: one send in flight at most, and the buffer stays bounded
		require.LessOrEqual(t, inFlightCount(p), 1)
		require.LessOrEqual(t, len(bufferedEvents(p)), maxBufferSize)
	}
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	// Then: no further batches or goroutines piled up, and the overflow was counted
	assert.Equal(t, 1, server.requestCount())
	assert.LessOrEqual(t, runtime.NumGoroutine(), goroutinesBefore+10)
	assert.Equal(t, int64(3*maxBufferSize), p.DroppedEvents())
	buffered := eventNames(bufferedEvents(p))
	require.Len(t, buffered, maxBufferSize)
	assert.Equal(t, "e41", buffered[0])
	assert.Equal(t, "e50", buffered[maxBufferSize-1])

	// When: the events API recovers
	once.Do(func() { close(release) })

	// Then: the stalled batch and the full buffer behind it are both delivered
	require.Eventually(t, func() bool { return len(server.events(t)) == 2*maxBufferSize }, time.Second, time.Millisecond)
	events := server.events(t)
	assert.Equal(t, "e1", events[0]["event"])
	assert.Equal(t, "e10", events[maxBufferSize-1]["event"])
	assert.Equal(t, "e41", events[maxBufferSize]["event"])
	assert.Equal(t, "e50", events[2*maxBufferSize-1]["event"])
	assert.Equal(t, 2, server.requestCount())
	assert.Empty(t, bufferedEvents(p))
	assert.Equal(t, int64(3*maxBufferSize), p.DroppedEvents())
}

func TestEventProcessorTimerDoesNotStackOnPendingSend(t *testing.T) {
	// Given: a stalled send started by the timer
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p := newTestEventProcessor(t.Context(), server.URL, 100, 5*time.Millisecond)
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When: more events are tracked across many ticks
	p.TrackEvent("b", nil)
	time.Sleep(50 * time.Millisecond)

	// Then: the timer did not start another send
	assert.Equal(t, 1, server.requestCount())
	assert.Equal(t, 1, inFlightCount(p))

	// When
	once.Do(func() { close(release) })

	// Then: the next tick sends the rest
	assert.Eventually(t, func() bool { return len(server.events(t)) == 2 }, time.Second, time.Millisecond)
}

func TestEventProcessorFlushDuringStalledSend(t *testing.T) {
	// Given: a stalled buffer-full send and more events buffered behind it
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p := newTestEventProcessor(t.Context(), server.URL, 2, 0)
	p.TrackEvent("a", nil)
	p.TrackEvent("b", nil)
	p.TrackEvent("c", nil)
	require.Eventually(t, func() bool { return server.requestCount() == 1 }, time.Second, time.Millisecond)

	// When: an explicit Flush sends its own batch and waits for the stalled one
	flushed := make(chan error, 1)
	go func() { flushed <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.requestCount() == 2 }, time.Second, time.Millisecond)
	select {
	case <-flushed:
		t.Fatal("Flush returned before the batch in flight at the call completed")
	case <-time.After(20 * time.Millisecond):
	}
	once.Do(func() { close(release) })

	// Then
	assert.NoError(t, <-flushed)
	assert.Len(t, server.events(t), 3)
	assert.Zero(t, p.DroppedEvents())
}

func TestEventProcessorIgnoresUnparseableAcceptedBody(t *testing.T) {
	// Given
	server := newEventsServerWithBody(t, func([]byte) (int, string) { return http.StatusAccepted, "not json" })
	p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
	p.TrackEvent("a", nil)

	// When, Then
	assert.NoError(t, p.Flush(t.Context()))
	assert.Zero(t, p.DroppedEvents())
}

func TestEventProcessorDedupeSurvivesFailedFlush(t *testing.T) {
	t.Run("retryable failure", func(t *testing.T) {
		// Given
		server := newEventsServer(t, statusSequence(
			http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable,
			http.StatusAccepted,
		))
		p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)
		require.Error(t, p.Flush(t.Context()))

		// When
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)

		// Then: still deduped against the kept exposure
		assert.Len(t, bufferedEvents(p), 1)

		// When: the exposure is delivered
		require.NoError(t, p.Flush(t.Context()))
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)

		// Then: the dedupe set was cleared
		assert.Len(t, bufferedEvents(p), 1)
	})

	t.Run("non-retryable failure", func(t *testing.T) {
		// Given
		server := newEventsServer(t, statusSequence(http.StatusBadRequest, http.StatusAccepted))
		p := newTestEventProcessor(t.Context(), server.URL, 100, 0)
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)
		require.Error(t, p.Flush(t.Context()))

		// When
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)

		// Then
		assert.Empty(t, bufferedEvents(p))
	})
}

func TestEventProcessorFinalFlushDoesNotSleepPastTimeout(t *testing.T) {
	// Given: a retry backoff longer than the request timeout and an unavailable events API
	server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
	ctx, cancel := context.WithCancel(t.Context())
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.timeout = 100 * time.Millisecond
	cfg.retryBackoff = time.Second
	cfg.jitter = func(d time.Duration) time.Duration { return d }
	p := newTestEventProcessorWith(ctx, cfg)
	p.TrackEvent("a", nil)

	// When
	start := time.Now()
	cancel()

	// Then: the final flush gives up instead of sleeping, and counts the lost event
	select {
	case <-p.stopped:
	case <-time.After(time.Second):
		t.Fatal("final flush did not finish")
	}
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Equal(t, 1, server.requestCount())
	assert.Empty(t, bufferedEvents(p))
	assert.Equal(t, int64(1), p.DroppedEvents())
}

func TestEventProcessorFinalFlushRetriesWithinTimeout(t *testing.T) {
	t.Run("retries that fit are made, then the batch is dropped", func(t *testing.T) {
		// Given
		server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
		ctx, cancel := context.WithCancel(t.Context())
		cfg := testEventsConfig(server.URL, 100, 0)
		cfg.timeout = 500 * time.Millisecond
		cfg.retryBackoff = time.Millisecond
		p := newTestEventProcessorWith(ctx, cfg)
		p.TrackEvent("a", nil)

		// When
		cancel()

		// Then: the full ladder of three attempts, then dropped and counted
		<-p.stopped
		assert.Equal(t, 3, server.requestCount())
		assert.Empty(t, bufferedEvents(p))
		assert.Equal(t, int64(1), p.DroppedEvents())
	})

	t.Run("a retry that succeeds delivers the batch", func(t *testing.T) {
		// Given
		server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable, http.StatusAccepted))
		ctx, cancel := context.WithCancel(t.Context())
		cfg := testEventsConfig(server.URL, 100, 0)
		cfg.timeout = 500 * time.Millisecond
		cfg.retryBackoff = time.Millisecond
		p := newTestEventProcessorWith(ctx, cfg)
		p.TrackEvent("a", nil)

		// When
		cancel()

		// Then
		<-p.stopped
		assert.Equal(t, 2, server.requestCount())
		assert.Zero(t, p.DroppedEvents())
	})
}

func TestSleepContextFailsWhenDeadlineTooClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	assert.ErrorIs(t, sleepContext(ctx, time.Second), context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Millisecond)
	assert.NoError(t, sleepContext(t.Context(), time.Millisecond))
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
	p := newEventProcessor(t.Context(), client, eventProcessorConfig{
		baseURL: server.URL, maxBufferSize: 100, timeout: testEventsTimeout, retryBackoff: testEventsRetryBackoff,
		log: slog.New(panickingHandler{}),
	})

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
	p := newEventProcessor(t.Context(), client, eventProcessorConfig{
		baseURL: server.URL, maxBufferSize: 1, timeout: testEventsTimeout, retryBackoff: testEventsRetryBackoff,
		log: slog.New(panickingHandler{}),
	})

	// When: the next event is tracked once the first send has finished
	p.TrackEvent("a", nil)
	require.Eventually(t, func() bool { return server.finishedCount() == 1 && inFlightCount(p) == 0 }, time.Second, time.Millisecond)
	p.TrackEvent("b", nil)

	// Then: the process is still alive and the worker still sends
	assert.Eventually(t, func() bool { return server.finishedCount() == 2 }, time.Second, time.Millisecond)
	assert.NoError(t, p.Flush(t.Context()))
}

func TestNewEventProcessorRetryBackoffDefault(t *testing.T) {
	p := NewEventProcessor(t.Context(), resty.New(), "http://localhost:1/", 100, 0, 50*time.Millisecond, createLogger())
	assert.Equal(t, time.Second, p.cfg.retryBackoff)
	assert.Equal(t, time.Second, DefaultEventsRetryBackoff)
}

func TestEventProcessorRetryBackoffIsCapped(t *testing.T) {
	// Given: a backoff that doubles past the 10 second cap
	server := newEventsServer(t, statusSequence(http.StatusServiceUnavailable))
	var mu sync.Mutex
	var bases []time.Duration
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.retryBackoff = 8 * time.Second
	cfg.jitter = func(d time.Duration) time.Duration {
		mu.Lock()
		defer mu.Unlock()
		bases = append(bases, d)
		return d
	}
	cfg.sleep = func(context.Context, time.Duration) error { return nil }
	p := newTestEventProcessorWith(t.Context(), cfg)
	p.TrackEvent("purchase", nil)

	// When
	require.Error(t, p.Flush(t.Context()))

	// Then
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []time.Duration{8 * time.Second, 10 * time.Second}, bases)
}

func TestEventProcessorAttemptTimeout(t *testing.T) {
	// Given: a server slower than the per-attempt timeout
	release := make(chan struct{})
	server := newEventsServer(t, func([]byte) int {
		<-release
		return http.StatusAccepted
	})
	defer close(release)
	cfg := testEventsConfig(server.URL, 100, 0)
	cfg.timeout = 30 * time.Millisecond
	p := newTestEventProcessorWith(t.Context(), cfg)
	p.TrackEvent("a", nil)

	// When
	start := time.Now()
	err := p.Flush(t.Context())

	// Then: every attempt times out and the batch is kept for the next flush
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Equal(t, 3, server.requestCount())
	assert.Len(t, bufferedEvents(p), 1)
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
