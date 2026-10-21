package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/segments"
	"github.com/go-resty/resty/v2"
)

type contextKey string

var contextKeyEvaluationContext = contextKey("evaluationContext")

// Client provides various methods to query Flagsmith API.
type Client struct {
	apiKey string
	config config

	environment             atomic.Value
	engineEvaluationContext atomic.Value

	analyticsProcessor *AnalyticsProcessor
	eventProcessor     *EventProcessor
	realtime           *realtime
	defaultFlagHandler func(string) (Flag, error)

	client         *resty.Client
	httpClient     *http.Client
	ctxLocalEval   context.Context
	ctxAnalytics   context.Context
	ctxEvents      context.Context
	log            *slog.Logger
	offlineHandler OfflineHandler
	errorHandler   func(handler *FlagsmithAPIError)
}

// Returns context with provided EvaluationContext instance set.
func WithEvaluationContext(ctx context.Context, ec EvaluationContext) context.Context {
	return context.WithValue(ctx, contextKeyEvaluationContext, ec)
}

// Retrieve EvaluationContext instance from context.
func GetEvaluationContextFromCtx(ctx context.Context) (ec EvaluationContext, ok bool) {
	ec, ok = ctx.Value(contextKeyEvaluationContext).(EvaluationContext)
	return ec, ok
}

func getOptionQualifiedName(opt Option) string {
	return runtime.FuncForPC(reflect.ValueOf(opt).Pointer()).Name()
}

func isClientOption(name string) bool {
	return strings.Contains(name, OptionWithHTTPClient) || strings.Contains(name, OptionWithRestyClient)
}

// NewClient creates instance of Client with given configuration.
func NewClient(apiKey string, options ...Option) *Client {
	c := &Client{
		apiKey: apiKey,
		config: defaultConfig(),
	}

	customClientCount := 0
	for _, opt := range options {
		name := getOptionQualifiedName(opt)
		if isClientOption(name) {
			customClientCount = customClientCount + 1
			if customClientCount > 1 {
				panic("Only one client option can be provided")
			}
			opt(c)
		}
	}

	// If a resty custom client has been provided, client is already set - otherwise we use a custom http client or default to a resty
	if c.client == nil {
		if c.httpClient != nil {
			c.client = resty.NewWithClient(c.httpClient)
			c.config.userProvidedClient = true
		} else {
			c.client = resty.New()
		}
	} else {
		c.config.userProvidedClient = true
	}

	c.client.SetHeaders(map[string]string{
		"Accept":             "application/json",
		"User-Agent":         getUserAgent(),
		EnvironmentKeyHeader: c.apiKey,
	})

	if c.client.GetClient().Timeout == 0 {
		c.client.SetTimeout(c.config.timeout)
	}

	c.log = createLogger()

	for _, opt := range options {
		name := getOptionQualifiedName(opt)
		if isClientOption(name) {
			continue
		}
		opt(c)
	}

	c.client = c.client.
		SetLogger(newSlogToRestyAdapter(c.log)).
		OnBeforeRequest(newRestyLogRequestMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log))

	c.log.Info("initialising Flagsmith client",
		"base_url", c.config.baseURL,
		"local_evaluation", c.config.localEvaluation,
		"offline", c.config.offlineMode,
		"analytics", c.config.enableAnalytics,
		"realtime", c.config.useRealtime,
		"realtime_url", c.config.realtimeBaseUrl,
		"events", c.config.enableEvents,
		"events_url", c.config.eventsBaseURL,
		"env_refresh_interval", c.config.envRefreshInterval,
		"timeout", c.config.timeout,
	)

	if c.config.offlineMode && c.offlineHandler == nil {
		panic("offline handler must be provided to use offline mode.")
	}
	if c.defaultFlagHandler != nil && c.offlineHandler != nil {
		panic("default flag handler and offline handler cannot be used together.")
	}
	if c.config.localEvaluation && c.offlineHandler != nil {
		panic("local evaluation and offline handler cannot be used together.")
	}
	if c.config.enableEvents && c.config.offlineMode {
		panic("events cannot be used in offline mode.")
	}
	if c.config.enableEvents && (c.config.eventsMaxBufferSize < 1 || c.config.eventsFlushInterval < 0) {
		panic("events buffer size must be positive and flush interval must not be negative.")
	}
	if c.config.enableEvents && c.config.eventsRetryBackoff != nil && *c.config.eventsRetryBackoff < 0 {
		panic("events retry backoff must not be negative.")
	}
	if c.offlineHandler != nil {
		env := c.offlineHandler.GetEnvironment()
		c.environment.Store(env)
		engineEvalCtx := engine_eval.MapEnvironmentDocumentToEvaluationContext(env)
		c.engineEvaluationContext.Store(&engineEvalCtx)
	}

	if c.config.localEvaluation {
		if !strings.HasPrefix(apiKey, "ser.") {
			panic("In order to use local evaluation, please generate a server key in the environment settings page.")
		}
		if c.config.polling || !c.config.useRealtime {
			// Poll indefinitely
			go c.pollEnvironment(c.ctxLocalEval, true)
		}
		if c.config.useRealtime {
			// Poll until we get the environment once
			go c.pollThenStartRealtime(c.ctxLocalEval)
		}
	}
	// Initialise analytics processor
	if c.config.enableAnalytics {
		c.analyticsProcessor = NewAnalyticsProcessor(
			c.ctxAnalytics,
			c.client,
			c.config.baseURL,
			nil,
			newSlogToLoggerAdapter(
				c.log.With(slog.String("worker", "analytics")),
			),
		)
	}
	if c.config.enableEvents {
		c.eventProcessor = c.newEventProcessor()
	}
	return c
}

// newEventProcessor shares the main client's transport, but not its retries or custom headers.
func (c *Client) newEventProcessor() *EventProcessor {
	log := c.log.With(slog.String("worker", "events"))
	httpClient := c.client.GetClient()
	eventsClient := resty.NewWithClient(httpClient).
		SetLogger(newSlogToRestyAdapter(log)).
		OnBeforeRequest(newRestyLogRequestMiddleware(log)).
		OnAfterResponse(newRestyLogResponseMiddleware(log))
	eventsClient.SetHeaders(map[string]string{
		"Accept":             "application/json",
		"User-Agent":         getUserAgent(),
		EnvironmentKeyHeader: c.apiKey,
	})

	timeout := httpClient.Timeout
	if timeout <= 0 {
		timeout = c.config.timeout
	}
	retryBackoff := DefaultEventsRetryBackoff
	if c.config.eventsRetryBackoff != nil {
		retryBackoff = *c.config.eventsRetryBackoff
	}
	return newEventProcessor(c.ctxEvents, eventsClient, eventProcessorConfig{
		baseURL:       c.config.eventsBaseURL,
		maxBufferSize: c.config.eventsMaxBufferSize,
		flushInterval: c.config.eventsFlushInterval,
		timeout:       timeout,
		retryBackoff:  retryBackoff,
		log:           log,
	})
}

// GetExperimentFlag evaluates one flag for the identity in ec and records an exposure when
// that identity is enrolled in a running experiment, which requires remote evaluation.
// See the README's Experimentation section. Requires WithEvents and an identity in ec, and
// returns an error if ec targets another environment.
func (c *Client) GetExperimentFlag(ctx context.Context, featureName string, ec EvaluationContext) (Flag, error) {
	if c.eventProcessor == nil {
		return Flag{}, &FlagsmithClientError{msg: "flagsmith: events must be enabled (WithEvents) to use experiment flags"}
	}
	if ec.Identity == nil || ec.Identity.Identifier == nil || strings.TrimSpace(*ec.Identity.Identifier) == "" {
		return Flag{}, &FlagsmithClientError{msg: "flagsmith: GetExperimentFlag requires an identity in the evaluation context"}
	}
	if ec.Environment != nil && ec.Environment.APIKey != c.apiKey {
		return Flag{}, &FlagsmithClientError{msg: "flagsmith: GetExperimentFlag cannot evaluate another environment; exposures are recorded for the client's environment key"}
	}
	identifier := *ec.Identity.Identifier

	flags, err := c.GetFlags(ctx, &ec)
	if err != nil {
		return Flag{}, err
	}
	flag, err := flags.GetFlag(featureName)
	if err != nil {
		return flag, err
	}

	skip := func(reason string) (Flag, error) {
		c.log.Info("not recording exposure", "feature", featureName, "reason", reason)
		return flag, nil
	}
	switch {
	case flag.IsDefault:
		return skip("flag served by the default handler")
	case !flag.Enabled:
		return skip("flag is disabled")
	case flag.Experiment == nil || !flag.Experiment.InExperiment:
		return skip("identity is not enrolled in an experiment")
	}

	c.eventProcessor.TrackExposureEvent(featureName, identifier, flag.Variant, traitValues(ec.Identity),
		map[string]interface{}{"experiment_id": flag.Experiment.ID})
	return flag, nil
}

// TrackEvent buffers a custom event, such as a conversion; opts may be nil. Names starting
// with "$" are reserved. Requires WithEvents; see the README's Experimentation section.
func (c *Client) TrackEvent(name string, opts *EventOptions) error {
	if c.eventProcessor == nil {
		return &FlagsmithClientError{msg: "flagsmith: events must be enabled (WithEvents) to track events"}
	}
	if strings.HasPrefix(name, "$") {
		return &FlagsmithClientError{msg: fmt.Sprintf("flagsmith: event names starting with \"$\" are reserved; use TrackExposureEvent to record %q", FlagExposureEvent)}
	}
	c.eventProcessor.TrackEvent(name, opts)
	return nil
}

// TrackExposureEvent buffers a $flag_exposure for a flag evaluated elsewhere. It returns an
// error without an identifier, and reads only Traits and Metadata from opts, which may be nil.
// Requires WithEvents; see the README's Experimentation section.
func (c *Client) TrackExposureEvent(featureName string, identifier string, value interface{}, opts *EventOptions) error {
	if c.eventProcessor == nil {
		return &FlagsmithClientError{msg: "flagsmith: events must be enabled (WithEvents) to track exposure events"}
	}
	if strings.TrimSpace(identifier) == "" {
		return &FlagsmithClientError{msg: "flagsmith: TrackExposureEvent requires an identifier"}
	}
	var traits, metadata map[string]interface{}
	if opts != nil {
		traits, metadata = opts.Traits, opts.Metadata
	}
	c.eventProcessor.TrackExposureEvent(featureName, identifier, value, traits, metadata)
	return nil
}

// FlushEvents sends buffered events and returns once every event tracked before the call
// has been sent, kept for retry or dropped, or when ctx ends. Call it with a deadline
// before a short-lived process exits. Returns nil when events are not enabled.
func (c *Client) FlushEvents(ctx context.Context) error {
	if c.eventProcessor == nil {
		return nil
	}
	return c.eventProcessor.Flush(ctx)
}

// DroppedEvents returns how many experimentation events have been lost so far. The count
// only increases; the README lists what it counts. Returns 0 when events are not enabled.
func (c *Client) DroppedEvents() int64 {
	if c.eventProcessor == nil {
		return 0
	}
	return c.eventProcessor.DroppedEvents()
}

// traitValues flattens the identity's traits, transient ones included, to their values.
func traitValues(ic *IdentityEvaluationContext) map[string]interface{} {
	if ic == nil || len(ic.Traits) == 0 {
		return nil
	}
	values := make(map[string]interface{}, len(ic.Traits))
	for key, trait := range ic.Traits {
		if trait == nil {
			values[key] = nil
			continue
		}
		values[key] = trait.Value
	}
	return values
}

// GetFlags evaluates the feature flags within an EvaluationContext.
//
// When flag evaluation fails, the value of each Flag is determined by the default flag handler
// from WithDefaultHandler, if one was provided.
//
// Flags are evaluated remotely by the Flagsmith API by default.
// To evaluate flags locally, instantiate a client using WithLocalEvaluation.
func (c *Client) GetFlags(ctx context.Context, ec *EvaluationContext) (f Flags, err error) {
	if ec != nil {
		ctx = WithEvaluationContext(ctx, *ec)
		if ec.Identity != nil {
			return c.GetIdentityFlags(ctx, *ec.Identity.Identifier, mapIdentityEvaluationContextToTraits(*ec.Identity))
		}
	}
	return c.GetEnvironmentFlags(ctx)
}

// GetEnvironmentFlags calls GetFlags using the current environment as the EvaluationContext.
// Equivalent to GetFlags(ctx, nil).
func (c *Client) GetEnvironmentFlags(ctx context.Context) (f Flags, err error) {
	if c.config.localEvaluation || c.config.offlineMode {
		if f, err = c.getEnvironmentFlagsFromEnvironment(); err == nil {
			return f, nil
		}
	} else {
		if f, err = c.GetEnvironmentFlagsFromAPI(ctx); err == nil {
			return f, nil
		}
	}
	if c.offlineHandler != nil {
		return c.getEnvironmentFlagsFromEnvironment()
	} else if c.defaultFlagHandler != nil {
		return Flags{defaultFlagHandler: c.defaultFlagHandler}, nil
	}
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err), err: err}
}

// GetIdentityFlags calls GetFlags using this identifier and traits as the EvaluationContext.
func (c *Client) GetIdentityFlags(ctx context.Context, identifier string, traits []*Trait) (f Flags, err error) {
	if c.config.localEvaluation || c.config.offlineMode {
		if f, err = c.getIdentityFlagsFromEnvironment(identifier, traits); err == nil {
			return f, nil
		}
	} else {
		if f, err = c.GetIdentityFlagsFromAPI(ctx, identifier, traits); err == nil {
			return f, nil
		}
	}
	if c.offlineHandler != nil {
		return c.getIdentityFlagsFromEnvironment(identifier, traits)
	} else if c.defaultFlagHandler != nil {
		return Flags{defaultFlagHandler: c.defaultFlagHandler}, nil
	}
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err), err: err}
}

// Returns an array of segments that the given identity is part of.
func (c *Client) GetIdentitySegments(identifier string, traits []*Trait) ([]*segments.SegmentModel, error) {
	if evalCtx, ok := c.engineEvaluationContext.Load().(*engine_eval.EngineEvaluationContext); ok {
		engineEvalCtx := engine_eval.MapContextAndIdentityDataToContext(*evalCtx, identifier, traits)
		result := flagengine.GetEvaluationResult(&engineEvalCtx)
		return engine_eval.MapEvaluationResultSegmentsToSegmentModels(&result), nil
	}
	return nil, &FlagsmithClientError{msg: "flagsmith: Local evaluation required to obtain identity segments"}
}

// BulkIdentify can be used to create/overwrite identities(with traits) in bulk
// NOTE: This method only works with Edge API endpoint.
func (c *Client) BulkIdentify(ctx context.Context, batch []*IdentityTraits) error {
	if len(batch) > BulkIdentifyMaxCount {
		msg := fmt.Sprintf("flagsmith: batch size must be less than %d", BulkIdentifyMaxCount)
		return &FlagsmithAPIError{Msg: msg}
	}

	body := struct {
		Data []*IdentityTraits `json:"data"`
	}{Data: batch}

	resp, err := c.client.NewRequest().
		SetBody(&body).
		SetContext(ctx).
		ForceContentType("application/json").
		Post(c.config.baseURL + "bulk-identities/")
	if resp.StatusCode() == 404 {
		msg := "flagsmith: Bulk identify endpoint not found; Please make sure you are using Edge API endpoint"
		return &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if err != nil {
		msg := fmt.Sprintf("flagsmith: error performing request to Flagsmith API: %s", err)
		return &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("flagsmith: unexpected response from Flagsmith API: %s", resp.Status())
		return &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	return nil
}

// GetEnvironmentFlagsFromAPI tries to contact the Flagsmith API to get the latest environment data.
// Will return an error in case of failure or unexpected response.
func (c *Client) GetEnvironmentFlagsFromAPI(ctx context.Context) (Flags, error) {
	req := c.client.NewRequest()
	ec, ok := GetEvaluationContextFromCtx(ctx)
	if ok {
		envCtx := ec.Environment
		if envCtx != nil {
			req.SetHeader(EnvironmentKeyHeader, envCtx.APIKey)
		}
	}
	resp, err := req.
		SetContext(ctx).
		ForceContentType("application/json").
		Get(c.config.baseURL + "flags/")
	if err != nil {
		msg := fmt.Sprintf("flagsmith: error performing request to Flagsmith API: %s", err)
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("flagsmith: unexpected response from Flagsmith API: %s", resp.Status())
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	return makeFlagsFromAPIFlags(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)
}

// GetIdentityFlagsFromAPI tries to contact the Flagsmith API to get the latest identity flags.
// Will return an error in case of failure or unexpected response.
func (c *Client) GetIdentityFlagsFromAPI(ctx context.Context, identifier string, traits []*Trait) (Flags, error) {
	body := struct {
		Identifier string   `json:"identifier"`
		Traits     []*Trait `json:"traits,omitempty"`
		Transient  *bool    `json:"transient,omitempty"`
	}{Identifier: identifier, Traits: traits}
	req := c.client.NewRequest()
	ec, ok := GetEvaluationContextFromCtx(ctx)
	if ok {
		envCtx := ec.Environment
		if envCtx != nil {
			req.SetHeader(EnvironmentKeyHeader, envCtx.APIKey)
		}
		idCtx := ec.Identity
		if idCtx != nil {
			// `Identifier` and `Traits` had been set by `GetFlags` earlier.
			body.Transient = idCtx.Transient
		}
	}
	resp, err := req.
		SetBody(&body).
		SetContext(ctx).
		ForceContentType("application/json").
		Post(c.config.baseURL + "identities/")
	if err != nil {
		msg := fmt.Sprintf("flagsmith: error performing request to Flagsmith API: %s", err)
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("flagsmith: unexpected response from Flagsmith API: %s", resp.Status())
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	return makeFlagsfromIdentityAPIJson(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)
}

func (c *Client) getIdentityFlagsFromEnvironment(identifier string, traits []*Trait) (Flags, error) {
	evalCtx, ok := c.engineEvaluationContext.Load().(*engine_eval.EngineEvaluationContext)
	if !ok {
		return Flags{}, fmt.Errorf("flagsmith: local environment has not yet been updated")
	}
	engineEvalCtx := engine_eval.MapContextAndIdentityDataToContext(*evalCtx, identifier, traits)
	result := flagengine.GetEvaluationResult(&engineEvalCtx)
	return makeFlagsFromEngineEvaluationResult(&result, c.analyticsProcessor, c.defaultFlagHandler), nil
}

func (c *Client) getEnvironmentFlagsFromEnvironment() (Flags, error) {
	evalCtx, ok := c.engineEvaluationContext.Load().(*engine_eval.EngineEvaluationContext)
	if !ok {
		return Flags{}, fmt.Errorf("flagsmith: local environment has not yet been updated")
	}
	// Clear segments and identity for environment evaluation
	environmentEvalCtx := engine_eval.EngineEvaluationContext{
		Environment: evalCtx.Environment,
		Features:    evalCtx.Features,
		Identity:    nil,
		Segments:    nil,
	}

	result := flagengine.GetEvaluationResult(&environmentEvalCtx)
	return makeFlagsFromEngineEvaluationResult(&result, c.analyticsProcessor, c.defaultFlagHandler), nil
}

func (c *Client) pollEnvironment(ctx context.Context, pollForever bool) {
	log := c.log.With(slog.String("worker", "poll"))
	update := func() {
		log.Debug("polling environment")
		ctx, cancel := context.WithTimeout(ctx, c.config.timeout)
		defer cancel()
		err := c.UpdateEnvironment(ctx)
		if err != nil {
			log.Error("failed to update environment", "error", err)
		}
	}
	update()
	ticker := time.NewTicker(c.config.envRefreshInterval)
	defer func() {
		ticker.Stop()
		log.Info("polling stopped")
	}()
	for {
		select {
		case <-ticker.C:
			if !pollForever {
				// Check if environment was successfully fetched
				if _, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
					if !pollForever {
						c.log.Debug("environment initialised")
						return
					}
				}
			}
			update()
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) pollThenStartRealtime(ctx context.Context) {
	b := newBackoff()
	update := func() {
		c.log.Debug("polling environment")
		ctx, cancel := context.WithTimeout(ctx, c.config.envRefreshInterval)
		defer cancel()
		err := c.UpdateEnvironment(ctx)
		if err != nil {
			c.log.Error("failed to update environment", "error", err)
			b.wait(ctx)
		}
	}
	update()
	defer func() {
		c.log.Info("initial polling stopped")
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// If environment was fetched, start realtime and finish
			if env, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
				streamURL := c.config.realtimeBaseUrl + "sse/environments/" + env.APIKey + "/stream"
				c.log.Debug("environment initialised, starting realtime updates")
				c.realtime = newRealtime(c, ctx, streamURL, env.UpdatedAt)
				go c.realtime.start()
				return
			}
			update()
		}
	}
}

func (c *Client) UpdateEnvironment(ctx context.Context) error {
	start := time.Now()

	var env environments.EnvironmentModel
	nextPage := ""
	pageCount := 0

	for {
		var page environments.EnvironmentModel
		req := c.client.NewRequest().
			SetContext(ctx).
			SetResult(&page).
			ForceContentType("application/json")
		if nextPage != "" {
			req = req.SetQueryParam("page_id", nextPage)
		}

		resp, err := req.Get(c.config.baseURL + "environment-document/")
		if err != nil {
			msg := fmt.Sprintf("flagsmith: error performing request to Flagsmith API: %s", err)
			f := &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
			if c.errorHandler != nil {
				c.errorHandler(f)
			}
			return f
		}
		if resp.StatusCode() != 200 {
			msg := fmt.Sprintf("flagsmith: unexpected response from Flagsmith API: %s", resp.Status())
			f := &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
			if c.errorHandler != nil {
				c.errorHandler(f)
			}
			return f
		}

		pageCount++
		nextPage = c.ExtractNextPage(resp.Header().Get("link"))

		if pageCount == 1 {
			env = page
		} else {
			env.IdentityOverrides = append(env.IdentityOverrides, page.IdentityOverrides...)
		}

		if nextPage == "" {
			break
		}
	}

	isNew := false
	previousEnv := c.environment.Load()
	if previousEnv == nil || env.UpdatedAt.After(previousEnv.(*environments.EnvironmentModel).UpdatedAt) {
		isNew = true
	}
	c.environment.Store(&env)

	engineEvalCtx := engine_eval.MapEnvironmentDocumentToEvaluationContext(&env)
	c.engineEvaluationContext.Store(&engineEvalCtx)

	if isNew {
		c.log.Info("environment updated", "environment", env.APIKey, "updated_at", env.UpdatedAt)
	}

	c.log.Debug("IdentityOverrides", "len", len(env.IdentityOverrides))

	if elapsed := time.Since(start); c.config.envRefreshInterval > 0 && elapsed > c.config.envRefreshInterval {
		c.log.Warn(
			"fetching environment took longer than the configured refresh interval; raise WithEnvironmentRefreshInterval or trim the environment",
			"elapsed", elapsed,
			"refresh_interval", c.config.envRefreshInterval,
		)
	}

	return nil
}

// ExtractNextPage parses the Link header from the environment-document API and
// returns the decoded page_id value when a next page exists, or empty string otherwise.
// Expected format: </api/v1/environment-document/?page_id=xxx>; rel="next".
func (c *Client) ExtractNextPage(linkHeader string) string {
	parts := strings.SplitN(linkHeader, ">", 2)
	if len(parts) == 0 {
		return ""
	}

	u, err := url.Parse(strings.TrimPrefix(parts[0], "<"))
	if err != nil {
		return ""
	}

	pageID := u.Query().Get("page_id")
	c.log.Debug("environment-document next page", "link", linkHeader, "page_id", pageID)

	return pageID
}
