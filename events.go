package flagsmith

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// DefaultEventsBaseURL is the base URL of Flagsmith's events API.
	DefaultEventsBaseURL = "https://events.api.flagsmith.com/"
	// DefaultEventsFlushInterval is how often buffered events are sent.
	DefaultEventsFlushInterval = 10 * time.Second
	// DefaultEventsMaxBufferSize is the buffer size that triggers a send, and its limit.
	DefaultEventsMaxBufferSize = 1000
	// DefaultEventsRetryBackoff is the backoff before the first retry of a failed batch.
	DefaultEventsRetryBackoff = time.Second
	// FlagExposureEvent is the event recorded when an identity is exposed to a variant.
	FlagExposureEvent     = "$flag_exposure"
	eventsEndpoint        = "v1/events"
	maxEventsRetryBackoff = 10 * time.Second
	maxEventsAttempts     = 3
)

// EventOptions carries the optional fields of an event. Values are captured when tracked.
type EventOptions struct {
	Identifier string
	Value      interface{}
	Traits     map[string]interface{}
	Metadata   map[string]interface{}
}

type event struct {
	Event       string                 `json:"event"`
	FeatureName *string                `json:"feature_name"`
	Identifier  *string                `json:"identifier"`
	Value       *string                `json:"value"`
	Traits      map[string]interface{} `json:"traits"`
	Metadata    map[string]interface{} `json:"metadata"`
	Timestamp   int64                  `json:"timestamp"`
}

// bufferedEvent is an event encoded at track time, with its exposure dedupe key.
type bufferedEvent struct {
	raw json.RawMessage
	key string
}

type eventBatch struct {
	events []bufferedEvent
	done   chan struct{}
	auto   bool // started by the timer or a full buffer
}

type sendOutcome int

const (
	outcomeDelivered sendOutcome = iota
	outcomeRetryable
	outcomeRejected
	outcomeUnauthorised
)

type eventProcessorConfig struct {
	baseURL       string
	maxBufferSize int
	flushInterval time.Duration
	timeout       time.Duration
	retryBackoff  time.Duration
	log           *slog.Logger
	jitter        func(time.Duration) time.Duration
	sleep         func(ctx context.Context, d time.Duration) error
}

var errEventsDisabled = &FlagsmithAPIError{Msg: "flagsmith: events API rejected the environment key; event tracking is disabled"}

// EventProcessor buffers experimentation events and sends them in batches. See the
// README's Experimentation section for delivery and failure handling.
type EventProcessor struct {
	client     *resty.Client
	endpoint   string
	cfg        eventProcessorConfig
	sendCtx    context.Context // cancelled at the shutdown deadline
	cancelSend context.CancelFunc

	mu          sync.Mutex
	buffer      []bufferedEvent
	seen        map[string]struct{}
	inFlight    map[*eventBatch]struct{}
	stopping    bool // shutdown has started
	autoPending bool // a timer or buffer-full batch is in flight
	held        bool // a failed batch waits for the next tick or Flush

	disabled    atomic.Bool // a 401 or 403 was received
	dropped     atomic.Int64
	disableOnce sync.Once
	disabledCh  chan struct{}
	stopped     chan struct{}
}

// NewEventProcessor starts an EventProcessor whose worker exits when ctx is done.
func NewEventProcessor(ctx context.Context, client *resty.Client, eventsBaseURL string, maxBufferSize int, flushInterval time.Duration, timeout time.Duration, log *slog.Logger) *EventProcessor {
	return newEventProcessor(ctx, client, eventProcessorConfig{
		baseURL:       eventsBaseURL,
		maxBufferSize: maxBufferSize,
		flushInterval: flushInterval,
		timeout:       timeout,
		retryBackoff:  DefaultEventsRetryBackoff,
		log:           log,
	})
}

func newEventProcessor(ctx context.Context, client *resty.Client, cfg eventProcessorConfig) *EventProcessor {
	if !strings.HasSuffix(cfg.baseURL, "/") {
		cfg.baseURL += "/"
	}
	if cfg.maxBufferSize < 1 {
		cfg.maxBufferSize = DefaultEventsMaxBufferSize
	}
	if cfg.flushInterval < 0 {
		cfg.flushInterval = DefaultEventsFlushInterval
	}
	cfg.retryBackoff = max(cfg.retryBackoff, 0)
	if cfg.log == nil {
		cfg.log = createLogger()
	}
	if cfg.jitter == nil {
		cfg.jitter = fullJitter
	}
	if cfg.sleep == nil {
		cfg.sleep = sleepContext
	}
	sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
	p := &EventProcessor{
		client:     client,
		endpoint:   cfg.baseURL + eventsEndpoint,
		cfg:        cfg,
		sendCtx:    sendCtx,
		cancelSend: cancelSend,
		seen:       make(map[string]struct{}),
		inFlight:   make(map[*eventBatch]struct{}),
		disabledCh: make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	go p.start(ctx)
	return p
}

// TrackEvent buffers a custom event. opts may be nil.
func (p *EventProcessor) TrackEvent(name string, opts *EventOptions) {
	if opts == nil {
		opts = &EventOptions{}
	}
	p.bufferEvent(name, nil, opts.Identifier, opts.Value, opts.Traits, opts.Metadata)
}

// TrackExposureEvent buffers a $flag_exposure event; it is ignored without an identifier.
func (p *EventProcessor) TrackExposureEvent(featureName string, identifier string, value interface{}, traits map[string]interface{}, metadata map[string]interface{}) {
	if identifier == "" {
		p.logf(slog.LevelDebug, "not buffering exposure: an exposure requires an identifier", "feature", featureName)
		return
	}
	p.bufferEvent(FlagExposureEvent, &featureName, identifier, value, traits, metadata)
}

// Flush sends buffered events and waits for the batches in flight when it was called.
func (p *EventProcessor) Flush(ctx context.Context) error {
	return p.flush(ctx, false)
}

// DroppedEvents returns how many events have been lost so far.
func (p *EventProcessor) DroppedEvents() int64 {
	return p.dropped.Load()
}

func (p *EventProcessor) flush(ctx context.Context, final bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: flushing events failed: %v", r)
		}
	}()
	p.mu.Lock()
	p.held = false
	waiting := slices.Collect(maps.Keys(p.inFlight))
	var batch *eventBatch
	if len(p.buffer) > 0 {
		batch = p.takeLocked(false)
	}
	p.mu.Unlock()
	if batch != nil {
		sendCtx := ctx
		if !final {
			var cancel context.CancelFunc
			sendCtx, cancel = p.flushContext(ctx)
			defer cancel()
		}
		err = p.post(sendCtx, batch, final)
	}
	for _, other := range waiting {
		select {
		case <-other.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// flushContext ends with the caller's ctx, keeping its deadline, or at the shutdown deadline.
func (p *EventProcessor) flushContext(ctx context.Context) (context.Context, context.CancelFunc) {
	var merged context.Context
	var cancel context.CancelFunc
	if deadline, ok := ctx.Deadline(); ok {
		merged, cancel = context.WithDeadline(p.sendCtx, deadline)
	} else {
		merged, cancel = context.WithCancel(p.sendCtx)
	}
	stop := context.AfterFunc(ctx, cancel)
	return merged, func() { stop(); cancel() }
}

func (p *EventProcessor) start(ctx context.Context) {
	defer close(p.stopped)
	defer p.cancelSend()
	var tick <-chan time.Time
	if p.cfg.flushInterval > 0 {
		ticker := time.NewTicker(p.cfg.flushInterval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case <-tick:
			p.onTick()
		case <-p.disabledCh:
			return
		case <-ctx.Done():
			p.shutdown(ctx)
			return
		}
	}
}

// shutdown sends the buffer and cuts every in-flight batch at one request timeout.
func (p *EventProcessor) shutdown(ctx context.Context) {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()
	timeout := p.cfg.timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	final, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	stopAfter := context.AfterFunc(final, p.cancelSend)
	defer stopAfter()
	if err := p.flush(final, true); err != nil {
		p.logf(slog.LevelWarn, "final events flush failed", "error", err)
	}

	p.cancelSend()
	p.mu.Lock()
	batches := slices.Collect(maps.Keys(p.inFlight))
	p.mu.Unlock()
	for _, b := range batches {
		<-b.done
	}

	p.mu.Lock()
	left := len(p.buffer)
	p.dropLocked(p.buffer)
	p.buffer = nil
	p.mu.Unlock()
	if left > 0 {
		p.logf(slog.LevelWarn, "events left unsent at shutdown; dropping them", "count", left)
	}
}

func (p *EventProcessor) onTick() {
	p.mu.Lock()
	p.held = false
	p.mu.Unlock()
	p.dispatch(false)
}

// dispatch starts a timer or buffer-full batch unless one is in flight or a batch is held.
func (p *EventProcessor) dispatch(onlyIfFull bool) {
	p.mu.Lock()
	var batch *eventBatch
	ready := len(p.buffer) > 0 && (!onlyIfFull || len(p.buffer) >= p.cfg.maxBufferSize)
	if ready && !p.autoPending && !p.held && !p.stopping {
		batch = p.takeLocked(true)
	}
	p.mu.Unlock()
	if batch != nil {
		p.launch(batch)
	}
}

func (p *EventProcessor) launch(batch *eventBatch) {
	go func() {
		if p.post(p.sendCtx, batch, false) == nil {
			p.dispatch(true)
		}
	}()
}

// takeLocked swaps the buffer out as an in-flight batch.
func (p *EventProcessor) takeLocked(auto bool) *eventBatch {
	batch := &eventBatch{events: p.buffer, done: make(chan struct{}), auto: auto}
	p.buffer = nil
	p.inFlight[batch] = struct{}{}
	if auto {
		p.autoPending = true
	}
	return batch
}

// bufferEvent encodes the event now and buffers it; unencodable events are dropped.
func (p *EventProcessor) bufferEvent(name string, featureName *string, identifier string, value interface{}, traits, metadata map[string]interface{}) {
	defer func() { _ = recover() }()
	e := event{
		Event:       name,
		FeatureName: featureName,
		Value:       stringifyValue(value),
		Metadata:    map[string]interface{}{},
		Timestamp:   time.Now().UnixMilli(),
	}
	if len(traits) > 0 {
		e.Traits = maps.Clone(traits)
	}
	maps.Copy(e.Metadata, metadata)
	e.Metadata["sdk_version"] = getSDKVersion()
	if identifier != "" {
		e.Identifier = &identifier
	}
	raw, err := json.Marshal(e)
	if err != nil {
		p.dropped.Add(1)
		// The error text may quote the value.
		p.logf(slog.LevelWarn, "event could not be encoded as JSON; dropping it", "error_type", fmt.Sprintf("%T", err))
		return
	}
	be := bufferedEvent{raw: raw}
	if name == FlagExposureEvent {
		be.key = exposureKey(e)
	}
	if batch := p.append(be, deref(featureName)); batch != nil {
		p.launch(batch)
	}
}

// append buffers e, returning a batch to launch once the buffer is full.
func (p *EventProcessor) append(e bufferedEvent, feature string) *eventBatch {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled.Load() || p.stopping {
		p.dropped.Add(1)
		return nil
	}
	if e.key != "" {
		if _, dup := p.seen[e.key]; dup {
			p.logf(slog.LevelDebug, "skipping duplicate exposure", "feature", feature)
			return nil
		}
		p.seen[e.key] = struct{}{}
	}
	canSend := !p.autoPending && !p.held
	var batch *eventBatch
	if len(p.buffer) >= p.cfg.maxBufferSize {
		if canSend {
			batch = p.takeLocked(true)
		} else {
			n := len(p.buffer) + 1 - p.cfg.maxBufferSize
			p.dropLocked(p.buffer[:n])
			p.buffer = p.buffer[n:]
			p.logf(slog.LevelDebug, "events buffer full while a send is in flight; dropped the oldest event")
		}
	}
	p.buffer = append(p.buffer, e)
	if batch == nil && canSend && len(p.buffer) >= p.cfg.maxBufferSize {
		batch = p.takeLocked(true)
	}
	return batch
}

func (p *EventProcessor) post(ctx context.Context, batch *eventBatch, final bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: sending events failed: %v", r)
		}
		p.mu.Lock()
		delete(p.inFlight, batch)
		if batch.auto {
			p.autoPending = false
		}
		p.mu.Unlock()
		close(batch.done)
	}()
	return p.send(ctx, batch.events, final)
}

// send posts events with retries, then re-queues them, or drops them when final.
func (p *EventProcessor) send(ctx context.Context, events []bufferedEvent, final bool) error {
	payload := encodeBatch(events)
	b := newBackoffWithJitter(p.cfg.retryBackoff, maxEventsRetryBackoff, func(d time.Duration) time.Duration {
		return p.cfg.jitter(min(d, maxEventsRetryBackoff))
	})
	var err error
	for attempt := 1; ; attempt++ {
		if p.disabled.Load() {
			p.drop(events)
			return errEventsDisabled
		}
		var outcome sendOutcome
		outcome, err = p.attempt(ctx, payload, events)
		switch outcome {
		case outcomeDelivered:
			p.mu.Lock()
			p.releaseLocked(events)
			p.mu.Unlock()
			return nil
		case outcomeUnauthorised:
			p.disable(len(events), err)
			return err
		case outcomeRejected:
			p.drop(events)
			return err
		}
		if attempt == maxEventsAttempts || ctx.Err() != nil {
			break
		}
		if p.disabled.Load() {
			p.drop(events)
			return errEventsDisabled
		}
		if p.cfg.sleep(ctx, b.next()) != nil {
			break
		}
	}
	if !final && p.requeue(events) {
		p.logf(slog.LevelWarn, "failed to send events; kept them for the next flush", "count", len(events), "error", err)
		return err
	}
	if final {
		p.dropped.Add(int64(len(events)))
	}
	p.logf(slog.LevelWarn, "failed to send events; dropping them", "count", len(events), "error", err)
	return err
}

func encodeBatch(events []bufferedEvent) []byte {
	var body bytes.Buffer
	body.WriteString(`{"events":[`)
	for i, e := range events {
		if i > 0 {
			body.WriteByte(',')
		}
		body.Write(e.raw)
	}
	body.WriteString("]}")
	return body.Bytes()
}

func (p *EventProcessor) drop(events []bufferedEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropLocked(events)
}

// dropLocked counts events as dropped and releases their dedupe keys.
func (p *EventProcessor) dropLocked(events []bufferedEvent) {
	p.releaseLocked(events)
	p.dropped.Add(int64(len(events)))
}

// releaseLocked releases the dedupe keys of events that are no longer pending.
func (p *EventProcessor) releaseLocked(events []bufferedEvent) {
	for _, e := range events {
		if e.key != "" {
			delete(p.seen, e.key)
		}
	}
}

// attempt posts payload once and classifies the result.
func (p *EventProcessor) attempt(ctx context.Context, payload []byte, events []bufferedEvent) (sendOutcome, error) {
	if p.cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.timeout)
		defer cancel()
	}
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Flagsmith-SDK-User-Agent", getUserAgent()).
		SetBody(payload).
		Post(p.endpoint)
	if err != nil {
		return outcomeRetryable, &FlagsmithAPIError{Msg: fmt.Sprintf("flagsmith: error sending events: %s", err), Err: err}
	}
	if resp.IsSuccess() {
		p.logRejected(resp.Body(), len(events))
		return outcomeDelivered, nil
	}
	apiErr := &FlagsmithAPIError{
		Msg:                fmt.Sprintf("flagsmith: unexpected response from events API: %s", resp.Status()),
		ResponseStatusCode: resp.StatusCode(),
		ResponseStatus:     resp.Status(),
	}
	switch resp.StatusCode() {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return outcomeRetryable, apiErr
	case http.StatusUnauthorized, http.StatusForbidden:
		return outcomeUnauthorised, apiErr
	}
	// The body is untrusted and may echo the events, so none of it is logged.
	p.logf(slog.LevelWarn, "events API rejected batch; dropping it",
		"count", len(events), "status", resp.StatusCode(), "body_bytes", len(resp.Body()))
	return outcomeRejected, apiErr
}

// logRejected logs and counts the rejected entries of a 202 by index only.
func (p *EventProcessor) logRejected(respBody []byte, batchSize int) {
	var parsed struct {
		Rejected []struct {
			Index int `json:"index"`
		} `json:"rejected"`
	}
	if json.Unmarshal(respBody, &parsed) != nil {
		return
	}
	counted := make(map[int]struct{}, len(parsed.Rejected))
	for _, r := range parsed.Rejected {
		if r.Index < 0 || r.Index >= batchSize {
			p.logf(slog.LevelWarn, "events API rejected an event outside the batch", "index", r.Index)
			continue
		}
		if _, dup := counted[r.Index]; !dup {
			counted[r.Index] = struct{}{}
			p.logf(slog.LevelWarn, "events API rejected event", "index", r.Index)
		}
	}
	if len(counted) > 0 {
		p.logf(slog.LevelWarn, "events API rejected events in an accepted batch; dropping them", "count", len(counted))
		p.dropped.Add(int64(len(counted)))
	}
}

// requeue puts events back at the head of the buffer and holds them for the next tick.
func (p *EventProcessor) requeue(events []bufferedEvent) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled.Load() || p.stopping {
		p.dropped.Add(int64(len(events)))
		return false
	}
	p.buffer = append(append(make([]bufferedEvent, 0, len(events)+len(p.buffer)), events...), p.buffer...)
	p.held = true
	if overflow := len(p.buffer) - p.cfg.maxBufferSize; overflow > 0 {
		p.dropLocked(p.buffer[:overflow])
		p.buffer = p.buffer[overflow:]
		p.logf(slog.LevelWarn, "events buffer full; dropped the oldest events", "count", overflow)
	}
	return true
}

// disable stops the processor after a 401 or 403 and logs once.
func (p *EventProcessor) disable(batchSize int, err error) {
	p.mu.Lock()
	p.disabled.Store(true)
	discarded := batchSize + len(p.buffer)
	p.buffer = nil
	p.seen = make(map[string]struct{})
	p.mu.Unlock()
	p.dropped.Add(int64(discarded))
	p.disableOnce.Do(func() {
		close(p.disabledCh)
		p.logf(slog.LevelWarn, "events API rejected the environment key; event tracking is disabled", "error", err)
	})
}

// sleepContext waits for d, failing at once if ctx ends first or its deadline is too close.
func sleepContext(ctx context.Context, d time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < d {
		return context.DeadlineExceeded
	}
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// logf logs through the configured logger; a panicking handler is ignored.
func (p *EventProcessor) logf(level slog.Level, msg string, args ...any) {
	defer func() { _ = recover() }()
	if level == slog.LevelDebug {
		p.cfg.log.Debug(msg, args...)
	} else {
		p.cfg.log.Warn(msg, args...)
	}
}

// stringifyValue renders a value as the events API expects: floats in plain decimals.
func stringifyValue(v interface{}) *string {
	var s string
	switch value := v.(type) {
	case nil:
		return nil
	case string:
		s = value
	case float64:
		s = strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(value), 'f', -1, 32)
	default:
		s = fmt.Sprintf("%v", value)
	}
	return &s
}

func exposureKey(e event) string {
	experimentID := ""
	if id, ok := e.Metadata["experiment_id"]; ok && id != nil {
		experimentID = fmt.Sprint(id)
	}
	return strings.Join([]string{e.Event, deref(e.FeatureName), deref(e.Identifier), deref(e.Value), experimentID}, "\x00")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
