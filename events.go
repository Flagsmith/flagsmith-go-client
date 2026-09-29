package flagsmith

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
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
	// DefaultEventsMaxBufferSize is the number of buffered events that triggers a flush.
	// It is also the most events the buffer holds; beyond it the oldest are dropped.
	DefaultEventsMaxBufferSize = 1000
	// FlagExposureEvent is the event recorded when an identity is exposed to an experiment
	// variant. It is the only "$"-prefixed event name an SDK may send.
	FlagExposureEvent = "$flag_exposure"

	// DefaultEventsRetryBackoff is the backoff before the first retry of a failed batch. It
	// doubles before each further retry, up to 10 seconds, and every wait is a random
	// duration between zero and the backoff.
	DefaultEventsRetryBackoff = time.Second

	eventsEndpoint = "v1/events"
	// maxEventsRetryBackoff caps the backoff between two attempts.
	maxEventsRetryBackoff = 10 * time.Second
	// maxEventsAttempts is how many times a batch is posted before it is given up on:
	// one attempt and two retries.
	maxEventsAttempts = 3
)

// EventOptions carries the optional fields of an event.
type EventOptions struct {
	// Identifier is the identity the event relates to. Empty means none.
	Identifier string
	// Value is stringified before sending: nil is sent as null, strings as-is, and
	// anything else formatted with fmt's %v verb.
	Value interface{}
	// Traits is a flat map of trait values recorded with the event.
	Traits map[string]interface{}
	// Metadata is merged with the SDK version, which takes precedence.
	Metadata map[string]interface{}
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

type eventsRequest struct {
	Events []event `json:"events"`
}

// eventsResponse is the body of a 202 from the events API.
type eventsResponse struct {
	Accepted int `json:"accepted"`
	Rejected []struct {
		Index int         `json:"index"`
		Error interface{} `json:"error"`
	} `json:"rejected"`
}

// eventBatch is a set of events taken from the buffer and being posted. done is closed
// once the batch has been delivered, re-queued or dropped.
type eventBatch struct {
	events []event
	done   chan struct{}
	// auto marks a batch started by the timer or a full buffer; at most one is in flight.
	auto bool
}

// sendOutcome classifies the result of one POST attempt.
type sendOutcome int

const (
	outcomeDelivered    sendOutcome = iota // 2xx
	outcomeRetryable                       // transport error, timeout, 408, 429, 502, 503 or 504
	outcomeRejected                        // any other status: dropped without a retry
	outcomeUnauthorised                    // 401 or 403: the processor stops
)

// eventProcessorConfig holds the settings of an EventProcessor.
type eventProcessorConfig struct {
	baseURL       string
	maxBufferSize int
	flushInterval time.Duration
	// timeout bounds each POST attempt and the final flush.
	timeout time.Duration
	// retryBackoff is the backoff before the first retry; it doubles for the next one,
	// up to maxEventsRetryBackoff.
	retryBackoff time.Duration
	log          *slog.Logger
	// jitter turns a backoff into the duration to wait. Defaults to fullJitter.
	jitter func(time.Duration) time.Duration
	// sleep waits for d, or fails when ctx cannot wait that long. Defaults to sleepContext.
	sleep func(ctx context.Context, d time.Duration) error
}

// EventProcessor buffers experimentation events and ships them in batches.
//
// Events are sent every flush interval, as soon as the buffer holds maxBufferSize events,
// or when Flush is called. Failures are logged and never returned to the code that
// tracked the event:
//
//   - A network error, a timeout, or a 408, 429, 502, 503 or 504 response is retried. A
//     batch is posted up to three times in total. The backoff starts at the configured
//     retry backoff and doubles, up to 10 seconds, and each wait is a random duration
//     between zero and the backoff. A batch that still fails is put back at the head of
//     the buffer and waits for the next flush.
//   - Any other error status, including 500, drops the batch without a retry.
//   - A 401 or 403 stops the processor for good: the timer stops, the buffer is dropped,
//     tracking becomes a no-op, and one warning is logged.
//   - When a 202 lists rejected events, each is logged at warn and never sent again.
//   - The buffer never holds more than maxBufferSize events. At most one timer or
//     buffer-full send is in flight at a time; while it is pending, a full buffer drops
//     its oldest events. Batches put back after a failure also drop the oldest to fit.
//
// Every lost event is counted by DroppedEvents. Exposure events are deduplicated until
// a 2xx response, including a partial 202, clears the dedupe set.
type EventProcessor struct {
	client   *resty.Client
	endpoint string
	cfg      eventProcessorConfig
	log      *slog.Logger
	// sendCtx is the context of timer and buffer-full batches. It is the worker's context
	// without its cancellation, so a batch is not aborted the moment shutdown starts, but
	// cancelSend cuts it at the shutdown deadline.
	sendCtx    context.Context
	cancelSend context.CancelFunc

	mu       sync.Mutex
	buffer   []event
	seen     map[string]struct{}      // exposure dedupe keys, cleared after a 2xx
	inFlight map[*eventBatch]struct{} // batches being posted
	disabled bool                     // set on a 401 or 403
	stopping bool                     // set once shutdown starts; nothing is re-queued after
	// autoPending is set while a timer or buffer-full batch is in flight. Until it
	// clears, a full buffer drops its oldest events instead of starting another send.
	autoPending bool
	// held is set when a failed batch is put back in the buffer. Until the next timer tick
	// or an explicit Flush, a full buffer drops its oldest events instead of resending it.
	held bool
	// overflowLogged limits the buffer-full warning to one per pending send.
	overflowLogged bool

	dropped     atomic.Int64
	disableOnce sync.Once
	disabledCh  chan struct{} // closed on a 401 or 403
	stopped     chan struct{} // closed when the worker goroutine exits
}

// NewEventProcessor creates an EventProcessor that posts to eventsBaseURL and starts its
// worker goroutine.
//
// The worker exits when ctx is done, after one final flush. That flush may retry with
// backoff, but all of it, including batches already in flight, must fit within timeout; a
// batch that still fails is dropped and counted, and so is anything left in the buffer.
// Tracking after that is a no-op, counted as dropped. timeout also bounds each POST
// attempt, and the retry backoff starts at DefaultEventsRetryBackoff. A flushInterval of 0
// disables the timer; buffer-full and manual flushes still work.
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
		log:        cfg.log,
		sendCtx:    sendCtx,
		cancelSend: cancelSend,
		seen:       make(map[string]struct{}),
		inFlight:   make(map[*eventBatch]struct{}),
		disabledCh: make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	p.debug("event processor starting")
	go p.start(ctx)
	return p
}

// TrackEvent buffers a custom event. opts may be nil. Once a 401 or 403 or shutdown has
// stopped the processor it is a no-op, counted by DroppedEvents. It never blocks on the
// network and never panics.
func (p *EventProcessor) TrackEvent(name string, opts *EventOptions) {
	if opts == nil {
		opts = &EventOptions{}
	}
	p.bufferEvent(name, nil, opts.Identifier, opts.Value, opts.Traits, opts.Metadata)
}

// TrackExposureEvent buffers a $flag_exposure event. Exposures without an identifier, and
// exposures equal to one buffered or sent since the last 2xx response, are discarded.
// Once a 401 or 403 or shutdown has stopped the processor it is a no-op, counted by
// DroppedEvents. It never blocks on the network and never panics.
func (p *EventProcessor) TrackExposureEvent(featureName string, identifier string, value interface{}, traits map[string]interface{}, metadata map[string]interface{}) {
	if identifier == "" {
		p.debug("not buffering exposure: an exposure requires an identifier", "feature", featureName)
		return
	}
	p.bufferEvent(FlagExposureEvent, &featureName, identifier, value, traits, metadata)
}

// Flush sends the buffered events now. It returns once that batch, and every batch that
// was already in flight when Flush was called, has been delivered, re-queued or dropped.
// Batches started after the call are not waited for, so sustained traffic cannot hold it
// up. The batch goes through the same retries as any other; one that still fails with a
// retryable error is put back in the buffer for the next flush, and Flush returns the
// error rather than trying again. A retry whose wait would pass ctx's deadline is not
// attempted.
//
// The returned error is the outcome of the batch sent by this call, or ctx's error if ctx
// ends while waiting. Failures of other batches are logged, not returned. Flush never
// panics.
func (p *EventProcessor) Flush(ctx context.Context) error {
	return p.flush(ctx, false)
}

// DroppedEvents returns how many events have been lost so far. It only ever increases.
// It counts events dropped from a full buffer, whether because a send was already in
// flight or a failed batch was held for the next tick; batches dropped on a non-retryable
// status; events that cannot be encoded as JSON; the buffer and batches discarded on a
// 401 or 403, and events tracked afterwards; events listed as rejected in a 202 response;
// and, at shutdown, batches that fail or are cut at the deadline, events left in the
// buffer, and events tracked afterwards.
func (p *EventProcessor) DroppedEvents() int64 {
	return p.dropped.Load()
}

// flush implements Flush. With final, a batch that fails is dropped instead of re-queued,
// since nothing will flush the buffer again.
func (p *EventProcessor) flush(ctx context.Context, final bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: flushing events failed: %v", r)
		}
	}()
	p.mu.Lock()
	p.held = false
	p.mu.Unlock()
	batch, waiting := p.takeBatch(true)
	if batch != nil {
		err = p.post(ctx, batch, final)
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
			p.debug("event processor stopped: the environment key was rejected")
			return
		case <-ctx.Done():
			p.shutdown(ctx)
			p.debug("event processor stopped")
			return
		}
	}
}

// shutdown performs the final flush when the worker's context is done. One deadline, the
// request timeout, bounds all of it: the final batch, and the timer and buffer-full batches
// already in flight, whose retries are cut when it passes. From the start of shutdown a
// batch that fails is dropped and counted, never re-queued, and tracking is a no-op. Once
// the in-flight batches have finished, anything left in the buffer is dropped and counted.
func (p *EventProcessor) shutdown(ctx context.Context) {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()

	timeout := p.cfg.timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	// The final flush must not be aborted by the cancellation that triggered it.
	final, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	stopAfter := context.AfterFunc(final, p.cancelSend)
	defer stopAfter()

	if err := p.flush(final, true); err != nil {
		p.warn("final events flush failed", "error", err)
	}

	// Cut whatever automatic batch is still retrying, and wait for it to give up, which is
	// immediate once its context is cancelled. A batch started by an explicit Flush runs on
	// its caller's context: it is only waited for until the same deadline.
	p.cancelSend()
	for _, b := range p.inFlightBatches() {
		if b.auto {
			<-b.done
			continue
		}
		select {
		case <-b.done:
		case <-final.Done():
			p.warn("events batch from an explicit flush still in flight after shutdown")
		}
	}

	p.mu.Lock()
	left := len(p.buffer)
	p.buffer = nil
	p.seen = make(map[string]struct{})
	p.mu.Unlock()
	if left > 0 {
		p.dropped.Add(int64(left))
		p.warn("events left unsent at shutdown; dropping them", "count", left)
	}
}

func (p *EventProcessor) inFlightBatches() []*eventBatch {
	p.mu.Lock()
	defer p.mu.Unlock()
	batches := make([]*eventBatch, 0, len(p.inFlight))
	for b := range p.inFlight {
		batches = append(batches, b)
	}
	return batches
}

// onTick releases a batch held after a failure and sends the buffer.
func (p *EventProcessor) onTick() {
	p.mu.Lock()
	p.held = false
	p.mu.Unlock()
	p.dispatch()
}

// dispatch takes the buffered events and posts them in the background, unless a timer or
// buffer-full send is already in flight: that caps the goroutines and batches a stalled
// events API can accumulate. The POST outlives the worker context's cancellation; the
// final flush waits for it. When the batch is delivered and the buffer has filled again
// meanwhile, the next batch is sent straight away, unless a failed batch is held: that
// waits for the timer or an explicit Flush.
func (p *EventProcessor) dispatch() {
	if batch := p.takeAutoBatch(); batch != nil {
		p.launch(batch)
	}
}

// launch posts a timer or buffer-full batch in the background.
func (p *EventProcessor) launch(batch *eventBatch) {
	go func() {
		if p.post(p.sendCtx, batch, false) == nil && p.bufferFull() {
			p.dispatch()
		}
	}()
}

// takeAutoBatch takes the buffer as a timer or buffer-full batch, or returns nil when the
// buffer is empty, such a batch is already in flight, or a failed batch is held.
func (p *EventProcessor) takeAutoBatch() *eventBatch {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.autoPending || p.held || p.stopping || len(p.buffer) == 0 {
		return nil
	}
	return p.takeAutoLocked()
}

func (p *EventProcessor) takeAutoLocked() *eventBatch {
	batch := p.takeLocked()
	batch.auto = true
	p.autoPending = true
	return batch
}

func (p *EventProcessor) bufferFull() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buffer) >= p.cfg.maxBufferSize
}

type appendResult int

const (
	appendBuffered appendResult = iota
	appendDuplicate
	appendDisabled
)

func (p *EventProcessor) bufferEvent(name string, featureName *string, identifier string, value interface{}, traits, metadata map[string]interface{}) {
	defer func() {
		// Tracking an event must never panic into the calling application.
		_ = recover()
	}()
	e := event{
		Event:       name,
		FeatureName: featureName,
		Value:       stringifyValue(value),
		Traits:      copyNonEmpty(traits),
		Metadata:    eventMetadata(metadata),
		Timestamp:   time.Now().UnixMilli(),
	}
	if identifier != "" {
		e.Identifier = &identifier
	}

	result, batch, logOverflow := p.append(e)
	if logOverflow {
		p.warn("events buffer full while a send is in flight; dropping the oldest events")
	}
	if result == appendDuplicate {
		p.debug("skipping duplicate exposure", "feature", deref(featureName))
	}
	if batch != nil {
		p.launch(batch)
	}
}

// append adds e to the buffer unless it is a duplicate exposure or the processor is
// disabled. When the buffer is full it is taken as a buffer-full batch for the caller
// to launch, in the same critical section, so a reachable events API never costs an
// event. If such a send is already in flight, or a failed batch is held for the next
// tick, the oldest event is dropped instead. logOverflow reports the first drop since
// the last timer or buffer-full send finished.
func (p *EventProcessor) append(e event) (result appendResult, batch *eventBatch, logOverflow bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled || p.stopping {
		// Nothing would ever send it.
		p.dropped.Add(1)
		return appendDisabled, nil, false
	}
	if e.Event == FlagExposureEvent {
		key := exposureKey(e)
		if _, dup := p.seen[key]; dup {
			return appendDuplicate, nil, false
		}
		p.seen[key] = struct{}{}
	}
	if len(p.buffer) >= p.cfg.maxBufferSize {
		// Only reachable after a failed batch was put back, or while a send is pending.
		if !p.autoPending && !p.held {
			batch = p.takeAutoLocked()
		} else {
			p.dropOldestLocked(len(p.buffer) + 1 - p.cfg.maxBufferSize)
			logOverflow = !p.overflowLogged
			p.overflowLogged = true
		}
	}
	p.buffer = append(p.buffer, e)
	if batch == nil && !p.autoPending && !p.held && len(p.buffer) >= p.cfg.maxBufferSize {
		batch = p.takeAutoLocked()
	}
	return appendBuffered, batch, logOverflow
}

// dropOldestLocked drops the n oldest buffered events and counts them. Their exposure
// dedupe keys are released, since no copy of them is left to send.
func (p *EventProcessor) dropOldestLocked(n int) {
	p.releaseKeysLocked(p.buffer[:n])
	p.buffer = p.buffer[n:]
	p.dropped.Add(int64(n))
}

// takeBatch swaps the buffer for a fresh one and registers the taken events as an
// in-flight batch. With snapshot, it also returns the batches that were already in
// flight, so the caller can wait for exactly those. The dedupe set is left alone: it is
// only cleared once events have been delivered.
func (p *EventProcessor) takeBatch(snapshot bool) (*eventBatch, []*eventBatch) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var waiting []*eventBatch
	if snapshot {
		waiting = make([]*eventBatch, 0, len(p.inFlight))
		for b := range p.inFlight {
			waiting = append(waiting, b)
		}
	}
	if len(p.buffer) == 0 {
		return nil, waiting
	}
	return p.takeLocked(), waiting
}

// takeLocked swaps the non-empty buffer for a fresh one and registers it as in flight.
func (p *EventProcessor) takeLocked() *eventBatch {
	batch := &eventBatch{events: p.buffer, done: make(chan struct{})}
	p.buffer = nil
	p.inFlight[batch] = struct{}{}
	return batch
}

// post sends a batch and marks it done whatever the outcome, including a panic.
func (p *EventProcessor) post(ctx context.Context, batch *eventBatch, final bool) (err error) {
	defer p.finish(batch)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: sending events failed: %v", r)
		}
	}()
	return p.send(ctx, batch.events, final)
}

func (p *EventProcessor) finish(batch *eventBatch) {
	p.mu.Lock()
	delete(p.inFlight, batch)
	if batch.auto {
		p.autoPending = false
		p.overflowLogged = false
	}
	p.mu.Unlock()
	close(batch.done)
}

// send posts events up to maxEventsAttempts times while the failure is retryable, waiting
// with capped exponential backoff and full jitter in between. A batch that still fails is
// put back at the head of the buffer, or dropped when final. Non-retryable failures drop
// the batch straight away; a 401 or 403 also disables the processor. Events that cannot
// be encoded as JSON are dropped before sending.
func (p *EventProcessor) send(ctx context.Context, events []event, final bool) error {
	events, payload, err := p.encode(events)
	if err != nil {
		return err
	}
	b := newBackoffWithJitter(p.cfg.retryBackoff, maxEventsRetryBackoff, func(d time.Duration) time.Duration {
		return p.cfg.jitter(min(d, maxEventsRetryBackoff))
	})
	for attempt := 1; ; attempt++ {
		var outcome sendOutcome
		outcome, err = p.attempt(ctx, payload, events)
		switch outcome {
		case outcomeDelivered:
			p.clearSeen()
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
		if p.cfg.sleep(ctx, b.next()) != nil {
			break
		}
	}
	if final {
		p.dropped.Add(int64(len(events)))
		p.warn("failed to send events; dropping them", "count", len(events), "error", err)
		return err
	}
	if p.requeue(events) {
		p.warn("failed to send events; kept them for the next flush", "count", len(events), "error", err)
	} else {
		p.warn("failed to send events; dropping them", "count", len(events), "error", err)
	}
	return err
}

// encode marshals events into a request body. Events that cannot be encoded, e.g.
// because a trait or metadata value is a channel or an infinite float, would fail every
// retry, so they are dropped and counted, and the rest are sent. It fails only when no
// event is left.
func (p *EventProcessor) encode(events []event) ([]event, []byte, error) {
	payload, err := json.Marshal(eventsRequest{Events: events})
	if err == nil {
		return events, payload, nil
	}
	valid := make([]event, 0, len(events))
	var invalid []event
	for _, e := range events {
		if _, eErr := json.Marshal(e); eErr != nil {
			invalid = append(invalid, e)
			continue
		}
		valid = append(valid, e)
	}
	p.drop(invalid)
	p.warn("events could not be encoded as JSON; dropping them", "count", len(invalid), "error", err)
	if len(valid) == 0 {
		return nil, nil, fmt.Errorf("flagsmith: encoding events: %w", err)
	}
	payload, err = json.Marshal(eventsRequest{Events: valid})
	if err != nil {
		p.drop(valid)
		return nil, nil, fmt.Errorf("flagsmith: encoding events: %w", err)
	}
	return valid, payload, nil
}

// drop counts events as dropped and releases their exposure dedupe keys, since no copy
// of them is left to send.
func (p *EventProcessor) drop(events []event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releaseKeysLocked(events)
	p.dropped.Add(int64(len(events)))
}

func (p *EventProcessor) releaseKeysLocked(events []event) {
	for _, e := range events {
		if e.Event == FlagExposureEvent {
			delete(p.seen, exposureKey(e))
		}
	}
}

// attempt posts payload, the encoding of events, once and classifies the result.
func (p *EventProcessor) attempt(ctx context.Context, payload []byte, events []event) (sendOutcome, error) {
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
		p.logRejected(resp.Body(), events)
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
	// The body is untrusted and may echo the events sent, so none of it is logged.
	p.warn("events API rejected batch; dropping it",
		"count", len(events),
		"status", resp.StatusCode(),
		"body_bytes", len(resp.Body()),
	)
	return outcomeRejected, apiErr
}

// maxLoggedRejectionError is the most characters of a per-event rejection error that are
// logged.
const maxLoggedRejectionError = 200

// logRejected logs every event the events API accepted the request for but rejected on
// its own. Those events are never sent again.
func (p *EventProcessor) logRejected(respBody []byte, events []event) {
	var parsed eventsResponse
	if len(respBody) == 0 || json.Unmarshal(respBody, &parsed) != nil {
		return
	}
	counted := make(map[int]struct{}, len(parsed.Rejected))
	for _, r := range parsed.Rejected {
		if r.Index < 0 || r.Index >= len(events) {
			p.warn("events API rejected an event outside the batch", "index", r.Index)
			continue
		}
		if _, dup := counted[r.Index]; dup {
			continue
		}
		counted[r.Index] = struct{}{}
		e := events[r.Index]
		// Identifiers, traits and anything but a plain error message are never logged.
		args := []any{"index", r.Index, "event", e.Event, "feature", deref(e.FeatureName)}
		if msg, ok := r.Error.(string); ok {
			if runes := []rune(msg); len(runes) > maxLoggedRejectionError {
				msg = string(runes[:maxLoggedRejectionError])
			}
			args = append(args, "error", msg)
		}
		p.warn("events API rejected event", args...)
	}
	p.dropped.Add(int64(len(counted)))
}

// requeue puts events that could not be sent back at the head of the buffer, ahead of
// anything tracked since, and holds them for the next timer tick or explicit Flush. The
// buffer stays within maxBufferSize by dropping the oldest.
//
// It reports false when the processor is disabled or shutting down: nothing would flush
// the buffer again, so the events are dropped and counted instead.
func (p *EventProcessor) requeue(events []event) bool {
	kept, overflow := p.requeueLocked(events)
	if overflow > 0 {
		p.warn("events buffer full; dropped the oldest events", "count", overflow)
	}
	return kept
}

// requeueLocked does the work of requeue under the lock, and returns whether the events
// were kept and how many were dropped to make room.
func (p *EventProcessor) requeueLocked(events []event) (kept bool, overflow int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled || p.stopping {
		p.dropped.Add(int64(len(events)))
		return false, 0
	}
	merged := make([]event, 0, len(events)+len(p.buffer))
	merged = append(merged, events...)
	merged = append(merged, p.buffer...)
	p.buffer = merged
	p.held = true
	overflow = len(merged) - p.cfg.maxBufferSize
	if overflow <= 0 {
		return true, 0
	}
	p.dropOldestLocked(overflow)
	return true, overflow
}

func (p *EventProcessor) clearSeen() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = make(map[string]struct{})
}

// disable stops the processor after a 401 or 403: the key will not be accepted, so
// nothing more is buffered or sent until the client is re-created. It logs one warning,
// however many batches hit it.
func (p *EventProcessor) disable(batchSize int, err error) {
	p.mu.Lock()
	p.disabled = true
	discarded := batchSize + len(p.buffer)
	p.buffer = nil
	p.seen = make(map[string]struct{})
	p.mu.Unlock()
	p.dropped.Add(int64(discarded))

	p.disableOnce.Do(func() {
		close(p.disabledCh)
		p.warn("events API rejected the environment key; event tracking is disabled", "error", err)
	})
}

// sleepContext waits for d. It fails straight away when ctx is done, or when ctx's
// deadline is too close to wait that long: a retry after the deadline could not succeed.
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

// warn logs at warn level. A misbehaving log handler must not break event delivery.
func (p *EventProcessor) warn(msg string, args ...any) {
	defer func() { _ = recover() }()
	p.log.Warn(msg, args...)
}

// debug logs at debug level. A misbehaving log handler must not break event delivery.
func (p *EventProcessor) debug(msg string, args ...any) {
	defer func() { _ = recover() }()
	p.log.Debug(msg, args...)
}

// stringifyValue renders an event value the way the events API expects: nil stays nil,
// strings are used as-is, floats are written in plain decimal notation, so 49.0 becomes
// "49" and 1.5e6 becomes "1500000", and anything else is formatted with %v.
func stringifyValue(v interface{}) *string {
	switch value := v.(type) {
	case nil:
		return nil
	case string:
		return &value
	case float64:
		s := strconv.FormatFloat(value, 'f', -1, 64)
		return &s
	case float32:
		s := strconv.FormatFloat(float64(value), 'f', -1, 32)
		return &s
	default:
		s := fmt.Sprintf("%v", value)
		return &s
	}
}

// eventMetadata copies the caller's metadata and adds the SDK version, which wins.
func eventMetadata(metadata map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(metadata)+1)
	maps.Copy(m, metadata)
	m["sdk_version"] = getSDKVersion()
	return m
}

func copyNonEmpty(m map[string]interface{}) map[string]interface{} {
	if len(m) == 0 {
		return nil
	}
	return maps.Clone(m)
}

func exposureKey(e event) string {
	experimentID := ""
	if id, ok := e.Metadata["experiment_id"]; ok && id != nil {
		experimentID = fmt.Sprint(id)
	}
	return strings.Join([]string{
		e.Event,
		deref(e.FeatureName),
		deref(e.Identifier),
		deref(e.Value),
		experimentID,
	}, "\x00")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
