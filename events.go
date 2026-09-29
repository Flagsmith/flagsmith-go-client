package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// DefaultEventsBaseURL is the base URL of Flagsmith's events API.
	DefaultEventsBaseURL = "https://events.api.flagsmith.com/"
	// DefaultEventsFlushInterval is how often buffered events are sent.
	DefaultEventsFlushInterval = 10 * time.Second
	// DefaultEventsMaxBufferSize is the number of buffered events that triggers a flush.
	DefaultEventsMaxBufferSize = 1000
	// FlagExposureEvent is the event recorded when an identity is exposed to an experiment
	// variant. It is the only "$"-prefixed event name an SDK may send.
	FlagExposureEvent = "$flag_exposure"

	eventsEndpoint = "v1/events"
	// maxEventsRetryBackoff caps the default wait before retrying a failed batch.
	maxEventsRetryBackoff = time.Second
	// maxEventsAttempts is how many times a batch is posted before it is dropped.
	maxEventsAttempts = 2
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

// eventBatch is a set of events taken from the buffer and being posted. done is closed
// once the batch has been delivered or dropped.
type eventBatch struct {
	events []event
	done   chan struct{}
}

// EventProcessor buffers experimentation events and ships them in batches.
//
// Events are sent every flush interval, as soon as the buffer holds maxBufferSize events,
// or when Flush is called. Exposure events are deduplicated within a flush window. A
// batch that cannot be posted is retried once and then dropped; failures are logged and
// never returned to the code that tracked the event.
type EventProcessor struct {
	client        *resty.Client
	endpoint      string
	maxBufferSize int
	flushInterval time.Duration
	timeout       time.Duration
	retryBackoff  time.Duration
	log           *slog.Logger

	mu       sync.Mutex
	buffer   []event
	seen     map[string]struct{}      // exposure dedupe keys for the current flush window
	inFlight map[*eventBatch]struct{} // batches being posted
	flushCh  chan struct{}            // buffer-full signal, capacity 1
	stopped  chan struct{}            // closed when the worker goroutine exits
}

// NewEventProcessor creates an EventProcessor that posts to eventsBaseURL and starts its
// worker goroutine.
//
// The worker exits when ctx is done, after one final flush bounded by timeout. timeout also
// bounds each POST attempt, and the wait before a retry is min(timeout, 1s). A
// flushInterval of 0 disables the timer; buffer-full and manual flushes still work.
func NewEventProcessor(ctx context.Context, client *resty.Client, eventsBaseURL string, maxBufferSize int, flushInterval time.Duration, timeout time.Duration, log *slog.Logger) *EventProcessor {
	return newEventProcessor(ctx, client, eventsBaseURL, maxBufferSize, flushInterval, timeout, defaultEventsRetryBackoff(timeout), log)
}

func defaultEventsRetryBackoff(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return maxEventsRetryBackoff
	}
	return min(timeout, maxEventsRetryBackoff)
}

func newEventProcessor(ctx context.Context, client *resty.Client, eventsBaseURL string, maxBufferSize int, flushInterval, timeout, retryBackoff time.Duration, log *slog.Logger) *EventProcessor {
	if !strings.HasSuffix(eventsBaseURL, "/") {
		eventsBaseURL += "/"
	}
	if maxBufferSize < 1 {
		maxBufferSize = DefaultEventsMaxBufferSize
	}
	if flushInterval < 0 {
		flushInterval = DefaultEventsFlushInterval
	}
	if log == nil {
		log = createLogger()
	}
	p := &EventProcessor{
		client:        client,
		endpoint:      eventsBaseURL + eventsEndpoint,
		maxBufferSize: maxBufferSize,
		flushInterval: flushInterval,
		timeout:       timeout,
		retryBackoff:  max(retryBackoff, 0),
		log:           log,
		seen:          make(map[string]struct{}),
		inFlight:      make(map[*eventBatch]struct{}),
		flushCh:       make(chan struct{}, 1),
		stopped:       make(chan struct{}),
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
// exposures equal to one already buffered in the current flush window, are discarded.
func (p *EventProcessor) TrackExposureEvent(featureName string, identifier string, value interface{}, traits map[string]interface{}, metadata map[string]interface{}) {
	if identifier == "" {
		p.debug("not buffering exposure: an exposure requires an identifier", "feature", featureName)
		return
	}
	p.bufferEvent(FlagExposureEvent, &featureName, identifier, value, traits, metadata)
}

// Flush sends the buffered events now. It returns once that batch, and every batch that
// was already in flight when Flush was called, has been delivered or dropped. Batches
// started after the call are not waited for, so sustained traffic cannot hold it up.
//
// The returned error is the outcome of the batch sent by this call, or ctx's error if ctx
// ends while waiting. Failures of other batches are logged, not returned.
func (p *EventProcessor) Flush(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: flushing events failed: %v", r)
		}
	}()
	batch, waiting := p.takeBatch(true)
	if batch != nil {
		err = p.post(ctx, batch)
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
	if p.flushInterval > 0 {
		ticker := time.NewTicker(p.flushInterval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case <-tick:
			p.dispatch(ctx)
		case <-p.flushCh:
			p.dispatch(ctx)
		case <-ctx.Done():
			// The final flush must not be aborted by the cancellation that triggered it.
			timeout := p.timeout
			if timeout <= 0 {
				timeout = DefaultTimeout
			}
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			defer cancel()
			if err := p.Flush(final); err != nil {
				p.warn("final events flush failed", "error", err)
			}
			p.debug("event processor stopped")
			return
		}
	}
}

// dispatch takes the buffered events and posts them in the background, so the worker is
// always free to pick up the next signal. The POST outlives ctx's cancellation; the
// final flush waits for it.
func (p *EventProcessor) dispatch(ctx context.Context) {
	batch, _ := p.takeBatch(false)
	if batch == nil {
		return
	}
	go func() {
		_ = p.post(context.WithoutCancel(ctx), batch)
	}()
}

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

	buffered, full := p.append(e)
	if !buffered {
		p.debug("skipping duplicate exposure in this flush window", "feature", deref(featureName), "identifier", identifier)
		return
	}
	if full {
		select {
		case p.flushCh <- struct{}{}:
		default:
		}
	}
}

// append adds e to the buffer unless it is a duplicate exposure, and reports whether the
// buffer is now full.
func (p *EventProcessor) append(e event) (buffered bool, full bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e.Event == FlagExposureEvent {
		key := exposureKey(e)
		if _, dup := p.seen[key]; dup {
			return false, false
		}
		p.seen[key] = struct{}{}
	}
	p.buffer = append(p.buffer, e)
	return true, len(p.buffer) >= p.maxBufferSize
}

// takeBatch swaps the buffer and dedupe window for fresh ones and registers the taken
// events as an in-flight batch. With snapshot, it also returns the batches that were
// already in flight, so the caller can wait for exactly those.
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
	p.seen = make(map[string]struct{})
	p.inFlight[batch] = struct{}{}
	return batch, waiting
}

// post sends a batch and marks it done whatever the outcome, including a panic.
func (p *EventProcessor) post(ctx context.Context, batch *eventBatch) (err error) {
	defer p.finish(batch)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flagsmith: sending events failed: %v", r)
		}
	}()
	return p.send(ctx, batch.events)
}

func (p *EventProcessor) finish(batch *eventBatch) {
	p.mu.Lock()
	delete(p.inFlight, batch)
	p.mu.Unlock()
	close(batch.done)
}

// send posts events, retrying once on a transport error or a 5xx response, then drops
// them. A 4xx response is dropped without a retry. A failed batch is never put back into
// the buffer: a permanently rejected batch would otherwise be retried forever.
func (p *EventProcessor) send(ctx context.Context, events []event) error {
	body := eventsRequest{Events: events}
	var err error
	for attempt := 1; attempt <= maxEventsAttempts; attempt++ {
		if attempt > 1 {
			if waitErr := sleepContext(ctx, p.retryBackoff); waitErr != nil {
				break
			}
		}
		var retry bool
		retry, err = p.attempt(ctx, body)
		if err == nil {
			return nil
		}
		if !retry {
			return err
		}
	}
	p.warn("failed to send events; dropping them", "count", len(events), "error", err)
	return err
}

// attempt posts body once, and reports whether a failure is worth retrying.
func (p *EventProcessor) attempt(ctx context.Context, body eventsRequest) (retry bool, err error) {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	resp, err := p.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Flagsmith-SDK-User-Agent", getUserAgent()).
		SetBody(body).
		Post(p.endpoint)
	if err != nil {
		return true, &FlagsmithAPIError{Msg: fmt.Sprintf("flagsmith: error sending events: %s", err), Err: err}
	}
	if resp.IsSuccess() {
		return false, nil
	}
	apiErr := &FlagsmithAPIError{
		Msg:                fmt.Sprintf("flagsmith: unexpected response from events API: %s", resp.Status()),
		ResponseStatusCode: resp.StatusCode(),
		ResponseStatus:     resp.Status(),
	}
	if resp.StatusCode() >= http.StatusInternalServerError {
		return true, apiErr
	}
	p.warn("events API rejected batch; dropping it",
		"count", len(body.Events),
		"status", resp.StatusCode(),
		"body", resp.String(),
	)
	return false, apiErr
}

func sleepContext(ctx context.Context, d time.Duration) error {
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
