package flagsmith

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// responder answers one events request. Status 0 closes the connection; -1 writes nothing.
type responder func(req *http.Request, body []byte) (int, string)

type eventsServer struct {
	*httptest.Server
	mu                     sync.Mutex
	bodies                 [][]byte
	headers                []http.Header
	paths                  []string
	finished, activeNumber int
}

func newEventsServer(t *testing.T, respond responder) *eventsServer {
	t.Helper()
	if respond == nil {
		respond = statuses(http.StatusAccepted)
	}
	s := &eventsServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		s.mu.Lock()
		s.bodies, s.headers, s.paths = append(s.bodies, body), append(s.headers, req.Header.Clone()), append(s.paths, req.URL.Path)
		s.activeNumber++
		s.mu.Unlock()
		status, resp := respond(req, body)
		s.mu.Lock()
		s.finished, s.activeNumber = s.finished+1, s.activeNumber-1
		s.mu.Unlock()
		switch status {
		case 0:
			if conn, _, err := rw.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
		case -1:
		default:
			rw.WriteHeader(status)
			_, _ = io.WriteString(rw, resp)
		}
	}))
	t.Cleanup(s.Close)
	return s
}
func (s *eventsServer) count() int  { s.mu.Lock(); defer s.mu.Unlock(); return len(s.bodies) }
func (s *eventsServer) done() int   { s.mu.Lock(); defer s.mu.Unlock(); return s.finished }
func (s *eventsServer) active() int { s.mu.Lock(); defer s.mu.Unlock(); return s.activeNumber }
func (s *eventsServer) events(t *testing.T) []map[string]interface{} {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []map[string]interface{}
	for _, b := range s.bodies {
		var payload struct{ Events []map[string]interface{} }
		require.NoError(t, json.Unmarshal(b, &payload))
		events = append(events, payload.Events...)
	}
	return events
}
func statuses(codes ...int) responder {
	var mu sync.Mutex
	return func(*http.Request, []byte) (int, string) {
		mu.Lock()
		defer mu.Unlock()
		code := codes[0]
		if len(codes) > 1 {
			codes = codes[1:]
		}
		return code, `{"accepted": 0, "rejected": []}`
	}
}
func withBody(code int, body string) responder {
	return func(*http.Request, []byte) (int, string) { return code, body }
}
func after(release <-chan struct{}, then responder) responder {
	return func(req *http.Request, body []byte) (int, string) { <-release; return then(req, body) }
}
func byEvent(routes map[string]responder) responder {
	return func(req *http.Request, body []byte) (int, string) {
		for name, respond := range routes {
			if strings.Contains(string(body), `"event":"`+name+`"`) {
				return respond(req, body)
			}
		}
		return http.StatusAccepted, "{}"
	}
}
func slow(d time.Duration, code int) responder {
	return func(*http.Request, []byte) (int, string) { time.Sleep(d); return code, "{}" }
}
func stall(req *http.Request, _ []byte) (int, string) { <-req.Context().Done(); return -1, "" }
func newRelease() (chan struct{}, func()) {
	ch := make(chan struct{})
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

type option func(*eventProcessorConfig)

func withTimeout(d time.Duration) option { return func(c *eventProcessorConfig) { c.timeout = d } }
func withBackoff(d time.Duration) option { return func(c *eventProcessorConfig) { c.retryBackoff = d } }
func withLogs(h slog.Handler) option     { return func(c *eventProcessorConfig) { c.log = slog.New(h) } }
func noJitter(c *eventProcessorConfig)   { c.jitter = func(d time.Duration) time.Duration { return d } }
func noSleep(c *eventProcessorConfig) {
	c.sleep = func(context.Context, time.Duration) error { return nil }
}
func newProc(ctx context.Context, url string, maxBufferSize int, flushInterval time.Duration, opts ...option) *EventProcessor {
	cfg := eventProcessorConfig{baseURL: url, maxBufferSize: maxBufferSize, flushInterval: flushInterval,
		timeout: time.Second, retryBackoff: 10 * time.Millisecond, log: createLogger()}
	for _, o := range opts {
		o(&cfg)
	}
	return newEventProcessor(ctx, resty.New().SetHeader(EnvironmentKeyHeader, EnvironmentAPIKey), cfg)
}

type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler            { return h }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *recordingHandler) matching(level slog.Level, msg string) (found []map[string]interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == level && r.Message == msg {
			attrs := map[string]interface{}{}
			r.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
			found = append(found, attrs)
		}
	}
	return found
}
func (h *recordingHandler) assertAbsent(t *testing.T, texts ...string) {
	t.Helper()
	h.mu.Lock()
	var b strings.Builder
	for _, r := range h.records {
		b.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool { b.WriteString(" " + a.String()); return true })
	}
	h.mu.Unlock()
	for _, text := range texts {
		assert.NotContains(t, b.String(), text)
	}
}

type panickingHandler struct{}

func (panickingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (panickingHandler) Handle(context.Context, slog.Record) error {
	panic(errors.New("logger failure"))
}
func (h panickingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h panickingHandler) WithGroup(string) slog.Handler      { return h }
func inFlightCount(p *EventProcessor) int                     { p.mu.Lock(); defer p.mu.Unlock(); return len(p.inFlight) }
func bufferedEvents(p *EventProcessor) []event {
	p.mu.Lock()
	defer p.mu.Unlock()
	events := make([]event, len(p.buffer))
	for i, be := range p.buffer {
		if err := json.Unmarshal(be.raw, &events[i]); err != nil {
			panic(err)
		}
	}
	return events
}
func bufferedNames(p *EventProcessor) []string {
	var names []string
	for _, e := range bufferedEvents(p) {
		names = append(names, e.Event)
	}
	return names
}
func sentNames(t *testing.T, s *eventsServer) []string {
	var names []string
	for _, e := range s.events(t) {
		names = append(names, e["event"].(string))
	}
	return names
}
func takeBuffer(p *EventProcessor) []bufferedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	events := p.buffer
	p.buffer = nil
	return events
}
func strPtr(s string) *string { return &s }
func track(p *EventProcessor, names ...string) {
	for _, name := range names {
		p.TrackEvent(name, nil)
	}
}
func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 2*time.Second, time.Millisecond)
}
func goFlush(ctx context.Context, p *EventProcessor) chan error {
	ch := make(chan error, 1)
	go func() { ch <- p.Flush(ctx) }()
	return ch
}
func receive[T any](t *testing.T, ch <-chan T, what string, within ...time.Duration) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(append(within, 2*time.Second)[0]):
		t.Fatal(what)
		panic(what)
	}
}
func waitStopped(t *testing.T, p *EventProcessor) {
	t.Helper()
	receive(t, p.stopped, "event processor goroutine did not exit")
}
func processorGoroutines() []string {
	buf := make([]byte, 1<<22)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n")[1:] {
		if strings.Contains(stack, "(*EventProcessor)") {
			found = append(found, stack)
		}
	}
	return found
}

const noServer = "http://localhost:1/"

// Tracked events are shaped for the wire, and exposures are deduplicated.
func TestEventProcessorBuffersEvents(t *testing.T) {
	p := newProc(t.Context(), noServer, 100, 0)
	before := time.Now().UnixMilli()
	meta := map[string]interface{}{"currency": "EUR", "sdk_version": "caller"}
	p.TrackEvent("purchase", &EventOptions{Identifier: "user-123", Value: 49.0, Metadata: meta})
	meta["currency"] = "USD"
	p.TrackEvent("signup", nil)
	events := bufferedEvents(p)
	require.Len(t, events, 2)
	assert.Equal(t, event{Event: "purchase", Identifier: strPtr("user-123"), Value: strPtr("49"), Timestamp: events[0].Timestamp,
		Metadata: map[string]interface{}{"currency": "EUR", "sdk_version": getSDKVersion()}}, events[0])
	assert.Equal(t, "caller", meta["sdk_version"])
	assert.GreaterOrEqual(t, events[0].Timestamp, before)
	assert.LessOrEqual(t, events[0].Timestamp, time.Now().UnixMilli())
	raw, err := json.Marshal(events[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"feature_name":null`)
	assert.Contains(t, string(raw), `"traits":null`)
	assert.Equal(t, event{Event: "signup", Timestamp: events[1].Timestamp,
		Metadata: map[string]interface{}{"sdk_version": getSDKVersion()}}, events[1])
	p = newProc(t.Context(), noServer, 100, 0)
	exposure := func(feature, identifier, value string, experimentID int) {
		p.TrackExposureEvent(feature, identifier, value, nil, map[string]interface{}{"experiment_id": experimentID})
	}
	exposure("f", "u", "treatment", 1)
	exposure("f", "u", "treatment", 1)
	exposure("f", "", "treatment", 1)
	assert.Len(t, bufferedEvents(p), 1, "equal exposures are deduped; one without an identifier is ignored")
	exposure("f", "other-user", "treatment", 1)
	exposure("f", "u", "control", 1)
	exposure("g", "u", "treatment", 1)
	exposure("f", "u", "treatment", 2)
	assert.Len(t, bufferedEvents(p), 5, "differing exposures are not deduped")
	p.TrackEvent("purchase", &EventOptions{Identifier: "u", Value: "1"})
	p.TrackEvent("purchase", &EventOptions{Identifier: "u", Value: "1"})
	assert.Len(t, bufferedEvents(p), 7, "custom events are never deduped")
}

func TestStringifyValue(t *testing.T) {
	for input, want := range map[interface{}]*string{
		"buy-now": strPtr("buy-now"), 49.0: strPtr("49"), 49.5: strPtr("49.5"), true: strPtr("true"), 7: strPtr("7"),
		1500000.0: strPtr("1500000"), 0.00001: strPtr("0.00001"), float32(0.1): strPtr("0.1"),
	} {
		assert.Equal(t, want, stringifyValue(input), "%#v", input)
	}
	assert.Nil(t, stringifyValue(nil))
}

// Flush posts the buffer to /v1/events with the SDK headers.
func TestEventProcessorFlushPostsBatch(t *testing.T) {
	server := newEventsServer(t, nil)
	p := newProc(t.Context(), server.URL, 100, 0)
	require.NoError(t, p.Flush(t.Context()))
	assert.Zero(t, server.count(), "an empty buffer sends nothing")
	p.TrackExposureEvent("checkout_cta", "user-123", "treatment",
		map[string]interface{}{"plan": "premium"}, map[string]interface{}{"experiment_id": 167})
	p.TrackEvent("purchase", &EventOptions{Identifier: "user-123", Value: "49"})
	require.NoError(t, p.Flush(t.Context()))
	require.Equal(t, 1, server.count())
	assert.Equal(t, "/v1/events", server.paths[0], "a trailing slash is added to the base URL")
	h := server.headers[0]
	assert.Equal(t, EnvironmentAPIKey, h.Get(EnvironmentKeyHeader))
	assert.Regexp(t, `^flagsmith-go-sdk/`, h.Get("Flagsmith-SDK-User-Agent"))
	assert.Regexp(t, `^application/json`, h.Get("Content-Type"))
	events := server.events(t)
	require.Len(t, events, 2)
	assert.Equal(t, map[string]interface{}{
		"event": FlagExposureEvent, "feature_name": "checkout_cta", "identifier": "user-123", "value": "treatment",
		"traits":    map[string]interface{}{"plan": "premium"},
		"metadata":  map[string]interface{}{"experiment_id": 167.0, "sdk_version": getSDKVersion()},
		"timestamp": events[0]["timestamp"],
	}, events[0])
	assert.Equal(t, "purchase", events[1]["event"])
	assert.Nil(t, events[1]["feature_name"])
	assert.Empty(t, bufferedEvents(p))
}

// Flush waits for exactly the batches in flight when it was called.
func TestEventProcessorFlushWaits(t *testing.T) {
	t.Run("for a batch already in flight, until ctx ends", func(t *testing.T) {
		release, done := newRelease()
		defer done()
		server := newEventsServer(t, after(release, statuses(http.StatusAccepted)))
		p := NewEventProcessor(t.Context(), resty.New(), server.URL+"/", 1, 0, time.Second, createLogger())
		track(p, "a")
		eventually(t, func() bool { return server.count() == 1 })
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		assert.ErrorIs(t, p.Flush(ctx), context.DeadlineExceeded)
		flushed := goFlush(t.Context(), p)
		time.Sleep(20 * time.Millisecond)
		assert.Empty(t, flushed, "Flush returned while a batch was in flight")
		done()
		assert.NoError(t, receive(t, flushed, "Flush did not return"))
		assert.Equal(t, 1, server.done())
	})
	t.Run("not for a batch started after the call", func(t *testing.T) {
		first, releaseFirst := newRelease()
		second, releaseSecond := newRelease()
		defer releaseFirst()
		defer releaseSecond()
		ok := statuses(http.StatusAccepted)
		server := newEventsServer(t, byEvent(map[string]responder{"first": after(first, ok), "second": after(second, ok)}))
		p := newProc(t.Context(), server.URL, 100, 0)
		track(p, "first")
		firstDone := goFlush(t.Context(), p)
		eventually(t, func() bool { return server.count() == 1 })
		track(p, "second")
		secondDone := goFlush(t.Context(), p)
		eventually(t, func() bool { return server.count() == 2 })
		releaseFirst()
		assert.NoError(t, receive(t, firstDone, "Flush waited for a batch started after it was called"))
		assert.Empty(t, secondDone, "second Flush returned before its batch completed")
		releaseSecond()
		assert.NoError(t, receive(t, secondDone, "second Flush did not return"))
	})
	t.Run("bounded under sustained traffic", func(t *testing.T) {
		server := newEventsServer(t, slow(20*time.Millisecond, http.StatusAccepted))
		p := newProc(t.Context(), server.URL, 1, 0)
		stop, streamDone := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(streamDone)
			for {
				select {
				case <-stop:
					return
				case <-time.After(5 * time.Millisecond):
					track(p, "tick")
				}
			}
		}()
		defer func() { close(stop); <-streamDone }()
		eventually(t, func() bool { return server.count() >= 3 })
		assert.NoError(t, receive(t, goFlush(t.Context(), p), "Flush did not return under sustained traffic", 500*time.Millisecond))
	})
	t.Run("for every batch when one fails", func(t *testing.T) {
		server := newEventsServer(t, byEvent(map[string]responder{
			"fail": slow(20*time.Millisecond, http.StatusBadRequest), "slow": slow(200*time.Millisecond, http.StatusAccepted)}))
		p := newProc(t.Context(), server.URL, 100, 0)
		var pending []chan error
		for i, name := range []string{"fail", "slow"} {
			track(p, name)
			pending = append(pending, goFlush(t.Context(), p))
			eventually(t, func() bool { return server.count() == i+1 })
		}
		assert.NoError(t, p.Flush(t.Context()))
		assert.Equal(t, 2, server.done(), "the slow batch finished although the other failed")
		for _, ch := range pending {
			<-ch
		}
	})
	t.Run("sends its own batch during a stalled send", func(t *testing.T) {
		release, done := newRelease()
		defer done()
		server := newEventsServer(t, after(release, statuses(http.StatusAccepted)))
		p := newProc(t.Context(), server.URL, 2, 0)
		track(p, "a", "b", "c")
		eventually(t, func() bool { return server.count() == 1 })
		flushed := goFlush(t.Context(), p)
		eventually(t, func() bool { return server.count() == 2 })
		time.Sleep(20 * time.Millisecond)
		assert.Empty(t, flushed, "Flush returned before the batch in flight at the call completed")
		done()
		assert.NoError(t, receive(t, flushed, "Flush did not return"))
		assert.Len(t, server.events(t), 3)
		assert.Zero(t, p.DroppedEvents())
	})
}

// Retryable failures get three attempts and are kept; others are dropped after one.
func TestEventProcessorRetries(t *testing.T) {
	flushOnce := func(t *testing.T, respond responder, opts ...option) (*EventProcessor, *eventsServer, error) {
		server := newEventsServer(t, respond)
		p := newProc(t.Context(), server.URL, 100, 0, opts...)
		track(p, "purchase")
		return p, server, p.Flush(t.Context())
	}
	p, server, err := flushOnce(t, statuses(http.StatusServiceUnavailable, http.StatusAccepted))
	assert.NoError(t, err, "503 then 202 delivers once")
	assert.Equal(t, 2, server.count())
	assert.Empty(t, bufferedEvents(p))
	assert.Zero(t, p.DroppedEvents())
	for status, name := range map[int]string{408: "408", 429: "429", 502: "502", 503: "503", 504: "504", 0: "transport error", -1: "attempt timeout"} {
		t.Run(name+" is retried then kept", func(t *testing.T) {
			respond, opts := statuses(status), []option(nil)
			if status == -1 {
				respond, opts = stall, []option{withTimeout(30 * time.Millisecond)}
			}
			start := time.Now()
			p, server, err := flushOnce(t, respond, opts...)
			var apiErr *FlagsmithAPIError
			require.ErrorAs(t, err, &apiErr)
			if status > 0 {
				assert.Equal(t, status, apiErr.ResponseStatusCode)
			}
			assert.Less(t, time.Since(start), 500*time.Millisecond)
			assert.Equal(t, 3, server.count())
			assert.Len(t, bufferedEvents(p), 1)
			assert.Zero(t, p.DroppedEvents())
		})
	}
	for _, status := range []int{400, 404, 409, 415, 422, 500, 501} {
		t.Run(fmt.Sprintf("%d is dropped after one attempt", status), func(t *testing.T) {
			p, server, err := flushOnce(t, statuses(status))
			var apiErr *FlagsmithAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.ResponseStatusCode)
			assert.Equal(t, 1, server.count())
			assert.Empty(t, bufferedEvents(p))
			assert.Equal(t, int64(1), p.DroppedEvents())
			require.NoError(t, p.Flush(t.Context()))
			assert.Equal(t, 1, server.count())
		})
	}
	t.Run("kept batch is sent first by the next flush", func(t *testing.T) {
		server := newEventsServer(t, statuses(503, 503, 503, http.StatusAccepted))
		p := newProc(t.Context(), server.URL, 100, 0)
		track(p, "first")
		assert.Error(t, p.Flush(t.Context()))
		assert.Equal(t, 3, server.count())
		assert.Equal(t, []string{"first"}, bufferedNames(p))
		track(p, "second")
		require.NoError(t, p.Flush(t.Context()))
		assert.Equal(t, []string{"first", "first", "first", "first", "second"}, sentNames(t, server))
		assert.Empty(t, bufferedEvents(p))
		assert.Zero(t, p.DroppedEvents())
	})
}

// The retry backoff starts at 1s by default, doubles, is capped at 10s and fully jittered.
func TestEventProcessorRetryBackoff(t *testing.T) {
	assert.Equal(t, time.Second, DefaultEventsRetryBackoff)
	p := NewEventProcessor(t.Context(), resty.New(), noServer, 100, 0, 50*time.Millisecond, createLogger())
	assert.Equal(t, time.Second, p.cfg.retryBackoff)
	assert.Equal(t, reflect.ValueOf(fullJitter).Pointer(), reflect.ValueOf(p.cfg.jitter).Pointer(), "full jitter by default")
	record := func(backoff time.Duration, jitter func(time.Duration) time.Duration) (bases, waits []time.Duration) {
		server := newEventsServer(t, statuses(http.StatusServiceUnavailable))
		var mu sync.Mutex
		p := newProc(t.Context(), server.URL, 100, 0, withBackoff(backoff), func(c *eventProcessorConfig) {
			c.jitter = func(d time.Duration) time.Duration {
				mu.Lock()
				defer mu.Unlock()
				bases = append(bases, d)
				return jitter(d)
			}
			c.sleep = func(_ context.Context, d time.Duration) error {
				mu.Lock()
				defer mu.Unlock()
				waits = append(waits, d)
				return nil
			}
		})
		track(p, "purchase")
		require.Error(t, p.Flush(t.Context()))
		mu.Lock()
		defer mu.Unlock()
		return bases, waits
	}
	bases, waits := record(100*time.Millisecond, func(d time.Duration) time.Duration { return d - time.Millisecond })
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, bases)
	assert.Equal(t, []time.Duration{99 * time.Millisecond, 199 * time.Millisecond}, waits)
	bases, _ = record(8*time.Second, func(d time.Duration) time.Duration { return d })
	assert.Equal(t, []time.Duration{8 * time.Second, 10 * time.Second}, bases, "capped at 10s")
}

// The timer and a full buffer send without blocking the caller, one send at a time.
func TestEventProcessorAutomaticSends(t *testing.T) {
	server := newEventsServer(t, nil)
	p := newProc(t.Context(), server.URL, 100, 20*time.Millisecond)
	track(p, "purchase")
	eventually(t, func() bool { return server.count() == 1 })
	server = newEventsServer(t, slow(200*time.Millisecond, http.StatusAccepted))
	p = newProc(t.Context(), server.URL, 2, 0)
	track(p, "a")
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, server.count(), "a zero interval disables the ticker")
	start := time.Now()
	track(p, "b")
	assert.Less(t, time.Since(start), 100*time.Millisecond, "a full buffer does not block the caller")
	eventually(t, func() bool { return server.count() == 1 })
	assert.Len(t, server.events(t), 2)
	release, done := newRelease()
	defer done()
	server = newEventsServer(t, after(release, statuses(http.StatusAccepted)))
	p = newProc(t.Context(), server.URL, 100, 5*time.Millisecond)
	track(p, "a")
	eventually(t, func() bool { return server.count() == 1 })
	track(p, "b")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, server.count(), "the timer does not stack on a pending send")
	assert.Equal(t, 1, inFlightCount(p))
	done()
	eventually(t, func() bool { return len(server.events(t)) == 2 })
}

// The buffer stays within maxBufferSize, dropping and counting the oldest events.
func TestEventProcessorBufferBounds(t *testing.T) {
	t.Run("re-queued batch drops the oldest to fit", func(t *testing.T) {
		release, done := newRelease()
		defer done()
		server := newEventsServer(t, after(release, statuses(http.StatusServiceUnavailable)))
		p := newProc(t.Context(), server.URL, 3, 0)
		track(p, "old-1", "old-2")
		flushed := goFlush(t.Context(), p)
		eventually(t, func() bool { return server.count() == 1 })
		track(p, "new-1", "new-2")
		done()
		require.Error(t, <-flushed)
		assert.Equal(t, []string{"old-2", "new-1", "new-2"}, bufferedNames(p))
		assert.Equal(t, int64(1), p.DroppedEvents())
	})
	t.Run("stalled server keeps one batch in flight", func(t *testing.T) {
		const size = 10
		release, done := newRelease()
		defer done()
		server := newEventsServer(t, after(release, statuses(http.StatusAccepted)))
		p := newProc(t.Context(), server.URL, size, 0)
		goroutinesBefore := runtime.NumGoroutine()
		for i := 1; i <= 5*size; i++ {
			track(p, fmt.Sprintf("e%d", i))
			require.LessOrEqual(t, inFlightCount(p), 1)
			require.LessOrEqual(t, len(bufferedEvents(p)), size)
		}
		eventually(t, func() bool { return server.count() == 1 })
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, server.count())
		assert.LessOrEqual(t, runtime.NumGoroutine(), goroutinesBefore+10)
		assert.Equal(t, int64(3*size), p.DroppedEvents())
		buffered := bufferedNames(p)
		require.Len(t, buffered, size)
		assert.Equal(t, []string{"e41", "e50"}, []string{buffered[0], buffered[size-1]})
		done()
		eventually(t, func() bool { return len(server.events(t)) == 2*size })
		sent := sentNames(t, server)
		assert.Equal(t, []string{"e1", "e10", "e41", "e50"}, []string{sent[0], sent[size-1], sent[size], sent[2*size-1]})
		assert.Equal(t, 2, server.count())
		assert.Empty(t, bufferedEvents(p))
		assert.Equal(t, int64(3*size), p.DroppedEvents())
	})
}

// A failed batch put back in the buffer waits for the next tick or an explicit Flush.
func TestEventProcessorHeldBatch(t *testing.T) {
	setup := func(t *testing.T) (*EventProcessor, *eventsServer) {
		server := newEventsServer(t, statuses(503, 503, 503, http.StatusAccepted))
		p := newProc(t.Context(), server.URL, 2, 0, withBackoff(0))
		track(p, "a", "b")
		eventually(t, func() bool { return server.count() == 3 && inFlightCount(p) == 0 })
		require.Equal(t, []string{"a", "b"}, bufferedNames(p))
		return p, server
	}
	t.Run("a full buffer drops instead of resending; the tick sends", func(t *testing.T) {
		p, server := setup(t)
		track(p, "c")
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 3, server.count())
		assert.Equal(t, []string{"b", "c"}, bufferedNames(p))
		assert.Equal(t, int64(1), p.DroppedEvents())
		p.onTick()
		eventually(t, func() bool { return server.count() == 4 && inFlightCount(p) == 0 })
		assert.Equal(t, []string{"b", "c"}, sentNames(t, server)[6:])
		assert.Empty(t, bufferedEvents(p))
	})
	t.Run("explicit Flush sends it and releases the hold", func(t *testing.T) {
		p, server := setup(t)
		require.NoError(t, p.Flush(t.Context()))
		assert.Equal(t, 4, server.count())
		track(p, "c", "d")
		eventually(t, func() bool { return server.count() == 5 })
		assert.Zero(t, p.DroppedEvents())
	})
}

// A 401 or 403 stops the processor for good, logging one warning.
func TestEventProcessorUnauthorised(t *testing.T) {
	const disabledMsg = "events API rejected the environment key; event tracking is disabled"
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			release, done := newRelease()
			defer done()
			server := newEventsServer(t, after(release, statuses(status)))
			logs := &recordingHandler{}
			p := newProc(t.Context(), server.URL, 100, time.Hour, withLogs(logs))
			var pending []chan error
			for i, name := range []string{"a", "b"} {
				track(p, name)
				pending = append(pending, goFlush(t.Context(), p))
				eventually(t, func() bool { return server.count() == i+1 })
			}
			track(p, "buffered")
			done()
			sawStatus := false
			for _, ch := range pending {
				err := <-ch
				require.Error(t, err)
				var apiErr *FlagsmithAPIError
				sawStatus = sawStatus || (errors.As(err, &apiErr) && apiErr.ResponseStatusCode == status)
			}
			assert.True(t, sawStatus, "the first rejection cuts the other batch, so only one is sure to carry the status")
			waitStopped(t, p)
			require.Equal(t, int64(3), p.DroppedEvents(), "both batches and the buffer are discarded")
			assert.Empty(t, bufferedEvents(p))
			track(p, "c")
			p.TrackExposureEvent("f", "u", "v", nil, nil)
			assert.Empty(t, bufferedEvents(p))
			assert.NoError(t, p.Flush(t.Context()))
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, 2, server.count())
			assert.Equal(t, int64(5), p.DroppedEvents(), "tracking afterwards is counted")
			assert.Len(t, logs.matching(slog.LevelWarn, disabledMsg), 1)
			assert.Empty(t, logs.matching(slog.LevelError, disabledMsg))
		})
	}
	t.Run("send skips a batch once disabled", func(t *testing.T) {
		server := newEventsServer(t, nil)
		p := newProc(t.Context(), server.URL, 100, 0)
		track(p, "a")
		p.TrackExposureEvent("f", "u", "v", nil, nil)
		events := takeBuffer(p)
		p.disabled.Store(true)
		assert.ErrorIs(t, p.send(context.Background(), events, false), errEventsDisabled)
		assert.Zero(t, server.count())
		assert.Equal(t, int64(2), p.DroppedEvents())
		assert.Empty(t, bufferedEvents(p))
	})
	t.Run("send stops before a retry once disabled", func(t *testing.T) {
		var p *EventProcessor
		server := newEventsServer(t, func(*http.Request, []byte) (int, string) {
			p.disabled.Store(true)
			return http.StatusServiceUnavailable, ""
		})
		p = newProc(t.Context(), server.URL, 100, 0, noSleep)
		track(p, "a")
		assert.ErrorIs(t, p.send(context.Background(), takeBuffer(p), false), errEventsDisabled)
		assert.Equal(t, 1, server.count())
		assert.Equal(t, int64(1), p.DroppedEvents())
	})
}

// Rejected entries of a 202 are logged by index, counted once, and never resent.
func TestEventProcessorRejectedEntries(t *testing.T) {
	const rejectedMsg = "events API rejected event"
	const summaryMsg = "events API rejected events in an accepted batch; dropping them"
	server := newEventsServer(t, withBody(http.StatusAccepted, `{"accepted": 1, "rejected": [{"index": 1, "error": "invalid identifier user@example.com"}]}`))
	logs := &recordingHandler{}
	p := newProc(t.Context(), server.URL, 100, 0, withLogs(logs))
	track(p, "good")
	p.TrackExposureEvent("checkout_cta", "user@example.com", "treatment", map[string]interface{}{"email": "trait@example.com"}, nil)
	require.NoError(t, p.Flush(t.Context()))
	assert.Equal(t, []map[string]interface{}{{"index": int64(1)}}, logs.matching(slog.LevelWarn, rejectedMsg))
	assert.Equal(t, []map[string]interface{}{{"count": int64(1)}}, logs.matching(slog.LevelWarn, summaryMsg))
	logs.assertAbsent(t, "invalid identifier", "user@example.com", "trait@example.com")
	assert.Equal(t, int64(1), p.DroppedEvents())
	assert.Empty(t, bufferedEvents(p))
	require.NoError(t, p.Flush(t.Context()))
	assert.Equal(t, 1, server.count(), "rejected events are not resent")
	server = newEventsServer(t, withBody(http.StatusAccepted, `{"accepted": 0, "rejected": [
		{"index": 0, "error": {"identifier": "user@example.com"}},
		{"index": 0, "error": "duplicate"},
		{"index": 1, "error": "invalid identifier user@example.com"},
		{"index": 7, "error": "out of range"},
		{"index": -1, "error": "negative"}]}`))
	logs = &recordingHandler{}
	p = newProc(t.Context(), server.URL, 100, 0, withLogs(logs))
	track(p, "a", "b")
	require.NoError(t, p.Flush(t.Context()))
	assert.Equal(t, int64(2), p.DroppedEvents(), "each in-range index is counted once")
	assert.Equal(t, []map[string]interface{}{{"index": int64(0)}, {"index": int64(1)}}, logs.matching(slog.LevelWarn, rejectedMsg))
	assert.Len(t, logs.matching(slog.LevelWarn, "events API rejected an event outside the batch"), 2)
	assert.Equal(t, []map[string]interface{}{{"count": int64(2)}}, logs.matching(slog.LevelWarn, summaryMsg))
	logs.assertAbsent(t, "user@example.com", "invalid identifier", "duplicate", "out of range", "negative")
	server = newEventsServer(t, withBody(http.StatusAccepted, "not json"))
	p = newProc(t.Context(), server.URL, 100, 0)
	track(p, "a")
	assert.NoError(t, p.Flush(t.Context()))
	assert.Zero(t, p.DroppedEvents(), "an unparseable accepted body drops nothing")
}

// The exposure dedupe set survives failures, is cleared on 2xx, and frees dropped keys.
func TestEventProcessorDedupeRelease(t *testing.T) {
	exposeTwice := func(respond responder) *EventProcessor {
		p := newProc(t.Context(), newEventsServer(t, respond).URL, 100, 0)
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)
		_ = p.Flush(t.Context())
		p.TrackExposureEvent("f", "u", "treatment", nil, nil)
		return p
	}
	p := exposeTwice(statuses(503, 503, 503, http.StatusAccepted))
	assert.Len(t, bufferedEvents(p), 1, "survives a retryable failure")
	require.NoError(t, p.Flush(t.Context()))
	p.TrackExposureEvent("f", "u", "treatment", nil, nil)
	assert.Len(t, bufferedEvents(p), 1, "cleared on 2xx")
	assert.Len(t, bufferedEvents(exposeTwice(statuses(http.StatusBadRequest))), 1, "released by a non-retryable drop")
	rejected := withBody(http.StatusAccepted, `{"accepted": 0, "rejected": [{"index": 0, "error": "invalid"}]}`)
	assert.Len(t, bufferedEvents(exposeTwice(rejected)), 1, "released by a rejected entry")
	p = newProc(t.Context(), noServer, 100, 0)
	p.TrackExposureEvent("f", "u", "v", map[string]interface{}{"ch": make(chan int)}, nil)
	p.TrackExposureEvent("f", "u", "v", nil, nil)
	assert.Len(t, bufferedEvents(p), 1, "not taken by an unencodable exposure")
	assert.Equal(t, int64(1), p.DroppedEvents())
	release, unblock := newRelease()
	defer unblock()
	server := newEventsServer(t, byEvent(map[string]responder{FlagExposureEvent: after(release, statuses(http.StatusAccepted))}))
	p = newProc(t.Context(), server.URL, 100, 0)
	p.TrackExposureEvent("f", "u", "treatment", nil, nil)
	pending := make(chan error, 2)
	go func() { pending <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.active() == 1 }, time.Second, time.Millisecond)
	track(p, "purchase")
	go func() { pending <- p.Flush(t.Context()) }()
	require.Eventually(t, func() bool { return server.done() == 1 }, time.Second, time.Millisecond)
	p.TrackExposureEvent("f", "u", "treatment", nil, nil)
	assert.Empty(t, bufferedEvents(p), "kept while its own batch is in flight, despite another batch's 2xx")
	unblock()
	require.NoError(t, <-pending)
	require.NoError(t, <-pending)
	p.TrackExposureEvent("f", "u", "treatment", nil, nil)
	assert.Len(t, bufferedEvents(p), 1, "released by its own batch's 2xx")
}

// Logs never contain identifiers, trait values or response content.
func TestEventProcessorLogsNoPersonalData(t *testing.T) {
	const email, trait = "user@example.com", "trait@example.com"
	server := newEventsServer(t, statuses(http.StatusBadRequest))
	logs := &recordingHandler{}
	p := newProc(t.Context(), server.URL, 100, 0, withLogs(logs))
	traits := map[string]interface{}{"email": trait}
	p.TrackExposureEvent("checkout_cta", email, "treatment", traits, nil)
	p.TrackExposureEvent("checkout_cta", email, "treatment", traits, nil)
	p.TrackEvent("purchase", &EventOptions{Identifier: email, Traits: traits})
	require.Error(t, p.Flush(t.Context()))
	require.NotEmpty(t, logs.matching(slog.LevelDebug, "skipping duplicate exposure"))
	require.NotEmpty(t, logs.matching(slog.LevelWarn, "events API rejected batch; dropping it"))
	logs.assertAbsent(t, email, trait)
	for _, body := range []string{
		`{"detail": "Invalid batch.", "events": [{"identifier": "user@example.com", "traits": {"email": "trait@example.com"}}]}`,
		`bad request for user@example.com with trait trait@example.com`,
		`{"identifier": "user@example.com", "traits": {"email": "trait@example.com"}}`,
	} {
		logs := &recordingHandler{}
		p := newProc(t.Context(), newEventsServer(t, withBody(http.StatusBadRequest, body)).URL, 100, 0, withLogs(logs))
		track(p, "purchase")
		require.Error(t, p.Flush(t.Context()))
		assert.Equal(t, []map[string]interface{}{{"count": int64(1), "status": int64(http.StatusBadRequest), "body_bytes": int64(len(body))}},
			logs.matching(slog.LevelWarn, "events API rejected batch; dropping it"))
		logs.assertAbsent(t, "Invalid batch.", email, trait)
	}
}

// Events that cannot be encoded are dropped and counted when tracked.
func TestEventProcessorDropsUnencodableEvents(t *testing.T) {
	server := newEventsServer(t, nil)
	logs := &recordingHandler{}
	p := newProc(t.Context(), server.URL, 100, 0, withLogs(logs))
	p.TrackEvent("bad", &EventOptions{Metadata: map[string]interface{}{"ratio": math.NaN()}})
	assert.Equal(t, int64(1), p.DroppedEvents())
	assert.Empty(t, bufferedEvents(p))
	dropped := logs.matching(slog.LevelWarn, "event could not be encoded as JSON; dropping it")
	require.Len(t, dropped, 1)
	assert.NotContains(t, dropped[0], "error")
	logs.assertAbsent(t, "NaN")
	require.NoError(t, p.Flush(t.Context()))
	assert.Zero(t, server.count())
	track(p, "good")
	p.TrackEvent("bad", &EventOptions{Metadata: map[string]interface{}{"ratio": math.Inf(1)}})
	p.TrackExposureEvent("f", "u", "v", map[string]interface{}{"ch": make(chan int)}, nil)
	require.NoError(t, p.Flush(t.Context()))
	assert.Equal(t, 1, server.count())
	assert.Equal(t, []string{"good"}, sentNames(t, server))
	assert.Equal(t, int64(3), p.DroppedEvents())
	assert.Empty(t, bufferedEvents(p))
}

// Traits and metadata are captured at track time; later mutation cannot race or change them.
func TestEventProcessorSnapshotsNestedData(t *testing.T) {
	server := newEventsServer(t, nil)
	p := newProc(t.Context(), server.URL, 100, 0)
	nestedTrait, traitList := map[string]interface{}{"tier": "gold"}, []interface{}{"a", "b"}
	nestedMeta, metaList := map[string]interface{}{"source": "checkout"}, []interface{}{1.0, 2.0}
	p.TrackEvent("purchase", &EventOptions{
		Identifier: "user-1",
		Traits:     map[string]interface{}{"profile": nestedTrait, "tags": traitList},
		Metadata:   map[string]interface{}{"context": nestedMeta, "items": metaList},
	})
	stop, mutated := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(mutated)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			nestedTrait["tier"], nestedTrait[fmt.Sprint("k", i%10)], traitList[0] = fmt.Sprint("changed-", i), i, i
			nestedMeta["source"], nestedMeta[fmt.Sprint("k", i%10)], metaList[1] = fmt.Sprint("changed-", i), i, float64(i)
		}
	}()
	for i := 0; i < 20; i++ {
		require.NoError(t, p.Flush(t.Context()))
		track(p, fmt.Sprint("next-", i))
	}
	close(stop)
	<-mutated
	require.NoError(t, p.Flush(t.Context()))
	events := server.events(t)
	require.NotEmpty(t, events)
	assert.Equal(t, "purchase", events[0]["event"])
	assert.Equal(t, map[string]interface{}{"profile": map[string]interface{}{"tier": "gold"}, "tags": []interface{}{"a", "b"}}, events[0]["traits"])
	metadata := events[0]["metadata"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"source": "checkout"}, metadata["context"])
	assert.Equal(t, []interface{}{1.0, 2.0}, metadata["items"])
}

// Cancelling the context flushes once, retrying only within the request timeout.
func TestEventProcessorFinalFlush(t *testing.T) {
	server := newEventsServer(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	p := newProc(ctx, server.URL, 100, time.Hour)
	track(p, "purchase")
	assert.Zero(t, server.count())
	cancel()
	waitStopped(t, p)
	assert.Equal(t, 1, server.count())
	assert.Len(t, server.events(t), 1, "sends the buffer and stops")
	server = newEventsServer(t, slow(30*time.Millisecond, http.StatusAccepted))
	ctx, cancel = context.WithCancel(t.Context())
	p = NewEventProcessor(ctx, resty.New(), server.URL+"/", 1, 0, time.Second, createLogger())
	track(p, "a")
	eventually(t, func() bool { return server.count() == 1 })
	cancel()
	waitStopped(t, p)
	assert.Equal(t, 1, server.done(), "does not abort a batch in flight")
	for _, tt := range []struct {
		name             string
		respond          responder
		timeout, backoff time.Duration
		wantRequests     int
		wantDropped      int64
	}{
		{"does not sleep past the timeout", statuses(503), 100 * time.Millisecond, time.Second, 1, 1},
		{"makes the retries that fit, then drops", statuses(503), 500 * time.Millisecond, time.Millisecond, 3, 1},
		{"delivers on a retry that succeeds", statuses(503, http.StatusAccepted), 500 * time.Millisecond, time.Millisecond, 2, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newEventsServer(t, tt.respond)
			ctx, cancel := context.WithCancel(t.Context())
			p := newProc(ctx, server.URL, 100, 0, withTimeout(tt.timeout), withBackoff(tt.backoff), noJitter)
			track(p, "a")
			start := time.Now()
			cancel()
			waitStopped(t, p)
			assert.Less(t, time.Since(start), 5*tt.timeout)
			assert.Equal(t, tt.wantRequests, server.count())
			assert.Empty(t, bufferedEvents(p))
			assert.Equal(t, tt.wantDropped, p.DroppedEvents())
		})
	}
}

// Shutdown cuts every batch in flight at one request timeout and counts what it drops.
func TestEventProcessorShutdown(t *testing.T) {
	const timeout = 100 * time.Millisecond
	stalled := func(t *testing.T, maxBufferSize int, opts ...option) (*EventProcessor, *eventsServer, context.CancelFunc) {
		server := newEventsServer(t, stall)
		ctx, cancel := context.WithCancel(t.Context())
		return newProc(ctx, server.URL, maxBufferSize, 0, append([]option{withTimeout(timeout), withBackoff(time.Millisecond)}, opts...)...), server, cancel
	}
	t.Run("cuts a buffer-full batch", func(t *testing.T) {
		p, server, cancel := stalled(t, 5)
		track(p, "e0", "e1", "e2", "e3", "e4")
		eventually(t, func() bool { return server.count() == 1 })
		start := time.Now()
		cancel()
		waitStopped(t, p)
		assert.Less(t, time.Since(start), timeout+150*time.Millisecond)
		assert.Equal(t, int64(5), p.DroppedEvents())
		assert.Zero(t, inFlightCount(p))
		assert.Empty(t, bufferedEvents(p))
		time.Sleep(50 * time.Millisecond)
		sent := server.count()
		time.Sleep(3 * timeout)
		assert.Equal(t, sent, server.count(), "no attempt starts after exit; one aborted at the deadline may land just after")
		track(p, "late")
		assert.Empty(t, bufferedEvents(p))
		assert.Equal(t, int64(6), p.DroppedEvents(), "tracking after shutdown is counted")
		eventually(t, func() bool { return len(processorGoroutines()) == 0 })
		eventually(t, func() bool { return server.active() == 0 })
	})
	t.Run("drops the final batch cut at the deadline", func(t *testing.T) {
		p, _, cancel := stalled(t, 3)
		track(p, "a", "b", "c", "d", "e")
		eventually(t, func() bool { return inFlightCount(p) == 1 })
		cancel()
		waitStopped(t, p)
		assert.Equal(t, int64(5), p.DroppedEvents())
		assert.Zero(t, inFlightCount(p))
		assert.Empty(t, bufferedEvents(p))
	})
	t.Run("cuts an explicit Flush on a context that never ends", func(t *testing.T) {
		p, server, cancel := stalled(t, 100, withBackoff(timeout), noJitter)
		track(p, "a")
		flushed := goFlush(context.Background(), p)
		eventually(t, func() bool { return server.active() == 1 })
		start := time.Now()
		cancel()
		waitStopped(t, p)
		assert.Less(t, time.Since(start), timeout+80*time.Millisecond)
		assert.Error(t, receive(t, flushed, "explicit Flush kept running after shutdown"))
		assert.Less(t, time.Since(start), timeout+150*time.Millisecond, "unbounded, the retries would take about 600ms")
		assert.Equal(t, int64(1), p.DroppedEvents())
		assert.Empty(t, bufferedEvents(p))
		assert.Zero(t, inFlightCount(p))
		eventually(t, func() bool { return server.active() == 0 })
	})
}

func TestEventsContextHelpers(t *testing.T) {
	p := newProc(t.Context(), noServer, 100, 0)
	caller, cancelCaller := context.WithTimeout(t.Context(), time.Minute)
	defer cancelCaller()
	merged, cancel := p.flushContext(caller)
	defer cancel()
	want, _ := caller.Deadline()
	got, ok := merged.Deadline()
	require.True(t, ok)
	assert.Equal(t, want, got, "flushContext keeps the caller's deadline")
	cancelCaller()
	receive(t, merged.Done(), "cancelling the caller's context did not end the flush context")
	ctx, cancelShort := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancelShort()
	start := time.Now()
	assert.ErrorIs(t, sleepContext(ctx, time.Second), context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Millisecond, "sleepContext fails at once when the deadline is too close")
	assert.NoError(t, sleepContext(t.Context(), time.Millisecond))
}

// A panicking log handler never reaches the caller or kills the worker.
func TestEventProcessorPanickingLogger(t *testing.T) {
	server := newEventsServer(t, statuses(http.StatusBadRequest))
	p := newProc(t.Context(), server.URL, 100, 0, withLogs(panickingHandler{}))
	assert.NotPanics(t, func() {
		p.TrackExposureEvent("f", "u", "v", nil, nil)
		p.TrackExposureEvent("f", "u", "v", nil, nil)
		p.TrackExposureEvent("f", "", "v", nil, nil)
		assert.Error(t, p.Flush(t.Context()))
	})
	assert.Equal(t, 1, server.count())
	server = newEventsServer(t, statuses(http.StatusBadRequest))
	p = newProc(t.Context(), server.URL, 1, 0, withLogs(panickingHandler{}))
	track(p, "a")
	eventually(t, func() bool { return server.done() == 1 && inFlightCount(p) == 0 })
	track(p, "b")
	eventually(t, func() bool { return server.done() == 2 })
	assert.NoError(t, p.Flush(t.Context()))
}
