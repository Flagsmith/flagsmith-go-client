package flagsmith

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
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

	eventsEndpoint = "v1/events"
	// maxEventsRetryBackoff caps the default initial wait before retrying a failed batch.
	maxEventsRetryBackoff = time.Second
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
}

// sendOutcome classifies the result of one POST attempt.
type sendOutcome int

const (
	outcomeDelivered    sendOutcome = iota // 2xx
	outcomeRetryable                       // transport error or 503
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
	// retryBackoff is the wait before the first retry; it doubles for the next one.
	retryBackoff time.Duration
	log          *slog.Logger
	// jitter turns a backoff into the duration to wait. Defaults to equalJitter.
	jitter func(time.Duration) time.Duration
	// sleep waits for d, or fails when ctx cannot wait that long. Defaults to sleepContext.
	sleep func(ctx context.Context, d time.Duration) error
}

// EventProcessor buffers experimentation events and ships them in batches.
//
// Events are sent every flush interval, as soon as the buffer holds maxBufferSize events,
// or when Flush is called. A batch that fails with a transport error or a 503 is posted
// up to three times with exponential backoff, then put back at the head of the buffer
// for the next flush. Any other error status drops the batch. A 401 or 403 stops the
// processor: nothing more is buffered or sent. The buffer never holds more than
// maxBufferSize events; the oldest are dropped first. Failures are logged and counted
// by DroppedEvents, never returned to the code that tracked the event.
//
// Exposure events are deduplicated until they have been delivered.
type EventProcessor struct {
	client   *resty.Client
	endpoint string
	cfg      eventProcessorConfig
	log      *slog.Logger
	// sendCtx is the worker's context without its cancellation, for batches that must
	// outlive it.
	sendCtx context.Context

	mu       sync.Mutex
	buffer   []event
	seen     map[string]struct{}      // exposure dedupe keys, cleared after a 2xx
	inFlight map[*eventBatch]struct{} // batches being posted
	disabled bool                     // set on a 401 or 403

	dropped     atomic.Int64
	disableOnce sync.Once
	disabledCh  chan struct{} // closed on a 401 or 403
	stopped     chan struct{} // closed when the worker goroutine exits
}

// NewEventProcessor creates an EventProcessor that posts to eventsBaseURL and starts its
// worker goroutine.
//
// The worker exits when ctx is done, after one final flush bounded by timeout. timeout also
// bounds each POST attempt, and the wait before the first retry is min(timeout, 1s). A
// flushInterval of 0 disables the timer; buffer-full and manual flushes still work.
func NewEventProcessor(ctx context.Context, client *resty.Client, eventsBaseURL string, maxBufferSize int, flushInterval time.Duration, timeout time.Duration, log *slog.Logger) *EventProcessor {
	return newEventProcessor(ctx, client, eventProcessorConfig{
		baseURL:       eventsBaseURL,
		maxBufferSize: maxBufferSize,
		flushInterval: flushInterval,
		timeout:       timeout,
		retryBackoff:  defaultEventsRetryBackoff(timeout),
		log:           log,
	})
}

func defaultEventsRetryBackoff(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return maxEventsRetryBackoff
	}
	return min(timeout, maxEventsRetryBackoff)
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
		cfg.jitter = equalJitter
	}
	if cfg.sleep == nil {
		cfg.sleep = sleepContext
	}
	p := &EventProcessor{
		client:     client,
		endpoint:   cfg.baseURL + eventsEndpoint,
		cfg:        cfg,
		log:        cfg.log,
		sendCtx:    context.WithoutCancel(ctx),
		seen:       make(map[string]struct{}),
		inFlight:   make(map[*eventBatch]struct{}),
		disabledCh: make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	p.debug("event processor starting")
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

// TrackExposureEvent buffers a $flag_exposure event. Exposures without an identifier, and
// exposures equal to one buffered or sent since the last successful delivery, are discarded.
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
// up. A batch that fails with a retryable error is put back in the buffer, and Flush
// returns the error rather than trying again.
//
// The returned error is the outcome of the batch sent by this call, or ctx's error if ctx
// ends while waiting. Failures of other batches are logged, not returned.
func (p *EventProcessor) Flush(ctx context.Context) error {
	return p.flush(ctx, false)
}

// DroppedEvents returns how many events have been lost so far: dropped from a full
// buffer, rejected with a non-retryable status or by the events API's per-event
// validation, discarded on a 401 or 403, or left unsent by the final flush.
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
	var tick <-chan time.Time
	if p.cfg.flushInterval > 0 {
		ticker := time.NewTicker(p.cfg.flushInterval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case <-tick:
			p.dispatch()
		case <-p.disabledCh:
			p.debug("event processor stopped: the environment key was rejected")
			return
		case <-ctx.Done():
			// The final flush must not be aborted by the cancellation that triggered it.
			timeout := p.cfg.timeout
			if timeout <= 0 {
				timeout = DefaultTimeout
			}
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			defer cancel()
			if err := p.flush(final, true); err != nil {
				p.warn("final events flush failed", "error", err)
			}
			p.debug("event processor stopped")
			return
		}
	}
}

// dispatch takes the buffered events and posts them in the background. The POST outlives
// the worker context's cancellation; the final flush waits for it.
func (p *EventProcessor) dispatch() {
	batch, _ := p.takeBatch(false)
	if batch == nil {
		return
	}
	go func() {
		_ = p.post(p.sendCtx, batch, false)
	}()
}

type appendResult int

const (
	appendBuffered appendResult = iota
	appendFull                  // buffered, and the buffer is now full
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

	switch p.append(e) {
	case appendDuplicate:
		p.debug("skipping duplicate exposure", "feature", deref(featureName), "identifier", identifier)
	case appendFull:
		// Sent straight away rather than by the worker, so the buffer never has to drop
		// events while a healthy API is reachable.
		p.dispatch()
	}
}

// append adds e to the buffer unless it is a duplicate exposure or the processor is
// disabled.
func (p *EventProcessor) append(e event) appendResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled {
		return appendDisabled
	}
	if e.Event == FlagExposureEvent {
		key := exposureKey(e)
		if _, dup := p.seen[key]; dup {
			return appendDuplicate
		}
		p.seen[key] = struct{}{}
	}
	p.buffer = append(p.buffer, e)
	if len(p.buffer) >= p.cfg.maxBufferSize {
		return appendFull
	}
	return appendBuffered
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
	batch := &eventBatch{events: p.buffer, done: make(chan struct{})}
	p.buffer = nil
	p.inFlight[batch] = struct{}{}
	return batch, waiting
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
	p.mu.Unlock()
	close(batch.done)
}

// send posts events up to maxEventsAttempts times while the failure is retryable, waiting
// with exponential backoff and jitter in between. A batch that still fails is put back at
// the head of the buffer, or dropped when final. Non-retryable failures drop the batch
// straight away; a 401 or 403 also disables the processor.
func (p *EventProcessor) send(ctx context.Context, events []event, final bool) error {
	body := eventsRequest{Events: events}
	b := newBackoffWithJitter(p.cfg.retryBackoff, maxBackoff, p.cfg.jitter)
	var err error
	for attempt := 1; ; attempt++ {
		var outcome sendOutcome
		outcome, err = p.attempt(ctx, body)
		switch outcome {
		case outcomeDelivered:
			p.clearSeen()
			return nil
		case outcomeUnauthorised:
			p.disable(len(events), err)
			return err
		case outcomeRejected:
			p.dropped.Add(int64(len(events)))
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
	p.requeue(events)
	p.warn("failed to send events; kept them for the next flush", "count", len(events), "error", err)
	return err
}

// attempt posts body once and classifies the result.
func (p *EventProcessor) attempt(ctx context.Context, body eventsRequest) (sendOutcome, error) {
	if p.cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.timeout)
		defer cancel()
	}
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Flagsmith-SDK-User-Agent", getUserAgent()).
		SetBody(body).
		Post(p.endpoint)
	if err != nil {
		return outcomeRetryable, &FlagsmithAPIError{Msg: fmt.Sprintf("flagsmith: error sending events: %s", err), Err: err}
	}
	if resp.IsSuccess() {
		p.logRejected(resp.Body(), body.Events)
		return outcomeDelivered, nil
	}
	apiErr := &FlagsmithAPIError{
		Msg:                fmt.Sprintf("flagsmith: unexpected response from events API: %s", resp.Status()),
		ResponseStatusCode: resp.StatusCode(),
		ResponseStatus:     resp.Status(),
	}
	switch resp.StatusCode() {
	case http.StatusServiceUnavailable:
		return outcomeRetryable, apiErr
	case http.StatusUnauthorized, http.StatusForbidden:
		return outcomeUnauthorised, apiErr
	}
	p.warn("events API rejected batch; dropping it",
		"count", len(body.Events),
		"status", resp.StatusCode(),
		"body", resp.String(),
	)
	return outcomeRejected, apiErr
}

// logRejected logs every event the events API accepted the request for but rejected on
// its own. Those events are never sent again.
func (p *EventProcessor) logRejected(respBody []byte, events []event) {
	var parsed eventsResponse
	if len(respBody) == 0 || json.Unmarshal(respBody, &parsed) != nil {
		return
	}
	for _, r := range parsed.Rejected {
		args := []any{"index", r.Index, "error", r.Error}
		if r.Index >= 0 && r.Index < len(events) {
			e := events[r.Index]
			args = append(args, "event", e.Event, "feature", deref(e.FeatureName), "identifier", deref(e.Identifier))
		}
		p.warn("events API rejected event", args...)
	}
	p.dropped.Add(int64(len(parsed.Rejected)))
}

// requeue puts events that could not be sent back at the head of the buffer, ahead of
// anything tracked since. The buffer stays within maxBufferSize by dropping the oldest.
func (p *EventProcessor) requeue(events []event) {
	if overflow := p.requeueLocked(events); overflow > 0 {
		p.warn("events buffer full; dropped the oldest events", "count", overflow)
	}
}

// requeueLocked does the work of requeue under the lock, and returns how many events
// were dropped to make room.
func (p *EventProcessor) requeueLocked(events []event) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disabled {
		p.dropped.Add(int64(len(events)))
		return 0
	}
	merged := make([]event, 0, len(events)+len(p.buffer))
	merged = append(merged, events...)
	merged = append(merged, p.buffer...)
	overflow := len(merged) - p.cfg.maxBufferSize
	if overflow > 0 {
		p.dropped.Add(int64(overflow))
		merged = merged[overflow:]
	}
	p.buffer = merged
	return max(overflow, 0)
}

func (p *EventProcessor) clearSeen() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = make(map[string]struct{})
}

// disable stops the processor after a 401 or 403: the key will not be accepted, so
// nothing more is buffered or sent. It logs once, however many batches hit it.
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
		p.logError("events API rejected the environment key; event tracking is disabled", "error", err)
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

// logError logs at error level. A misbehaving log handler must not break event delivery.
func (p *EventProcessor) logError(msg string, args ...any) {
	defer func() { _ = recover() }()
	p.log.Error(msg, args...)
}

// debug logs at debug level. A misbehaving log handler must not break event delivery.
func (p *EventProcessor) debug(msg string, args ...any) {
	defer func() { _ = recover() }()
	p.log.Debug(msg, args...)
}

// stringifyValue renders an event value the way the events API expects: nil stays nil,
// strings are used as-is, and anything else is formatted with %v, so 49.0 becomes "49".
func stringifyValue(v interface{}) *string {
	switch value := v.(type) {
	case nil:
		return nil
	case string:
		return &value
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
