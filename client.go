package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
	"github.com/go-resty/resty/v2"

	enginetraits "github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
)

type contextKey string

var contextKeyEvaluationContext = contextKey("evaluationContext")

// Client is a Flagsmith client, used to evaluate feature flags. Use [NewClient] to instantiate a client.
//
// Use [Client.GetFlags] to evaluate feature flags.
type Client struct {
	config             config
	defaultFlagHandler func(string) (Flag, error)
	errorHandler       func(handler *FlagsmithAPIError)
	offlineHandler     OfflineHandler
	analyticsProcessor *AnalyticsProcessor
	ctxLocalEval       context.Context
	ctxAnalytics       context.Context

	environment             atomic.Value
	identitiesWithOverrides atomic.Value

	log    *slog.Logger
	client *resty.Client
}

// NewClient creates a Flagsmith [Client] using the environment determined by apiKey.
//
// Feature flags are evaluated remotely by the Flagsmith API over HTTP by default.
// To evaluate flags locally, use [WithLocalEvaluation] and a server-side SDK key. For example:
//
//	func GetDefaultFlag(key string) (Flag, error) {
//		return Flag{
//			FeatureName: key,
//			IsDefault:   true,
//			Value:       `{"colour": "#FFFF00"}`,
//			Enabled:     true,
//		}, nil
//	}
//	flagsmithClient := flagsmith.NewClient(
//		os.Getenv("FLAGSMITH_SDK_KEY"),
//		flagsmith.WithLocalEvaluation(context.Background()),
//		flagsmith.WithDefaultHandler(GetDefaultFlag),
//	)
func NewClient(apiKey string, options ...Option) *Client {
	c := &Client{
		config: defaultConfig(),
		client: resty.New(),
	}

	if c.log == nil {
		c.log = defaultLogger()
	}
	c.client = resty.
		New().
		OnBeforeRequest(newRestyLogRequestMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log))
	c.client.SetLogger(restySlogLogger{c.log})

	c.client.SetHeaders(map[string]string{
		"Accept":             "application/json",
		EnvironmentKeyHeader: apiKey,
	})
	c.client.SetTimeout(c.config.timeout)

	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
	}

	if c.config.offlineMode && c.offlineHandler == nil {
		panic("offline handler must be provided to use offline mode.")
	}
	if c.defaultFlagHandler != nil && c.offlineHandler != nil {
		panic("default flag handler and offline handler cannot be used together.")
	}
	if c.config.localEvaluation && c.offlineHandler != nil {
		panic("local evaluation and offline handler cannot be used together.")
	}
	if c.offlineHandler != nil {
		c.environment.Store(c.offlineHandler.GetEnvironment())
	}

	if c.config.localEvaluation {
		if !strings.HasPrefix(apiKey, "ser.") {
			panic("In order to use local evaluation, please generate a server key in the environment settings page.")
		}
		if c.config.useRealtime {
			go c.startRealtimeUpdates(c.ctxLocalEval)
		} else {
			go c.pollEnvironment(c.ctxLocalEval)
		}
	}
	// Initialise analytics processor
	if c.config.enableAnalytics {
		c.analyticsProcessor = NewAnalyticsProcessor(c.ctxAnalytics, c.client, c.config.baseURL, nil, c.log)
	}
	return c
}

// GetFlags evaluates the feature flags within an [EvaluationContext].
//
// When flag evaluation fails, the return value is determined by the default flag handler from
// [WithDefaultHandler], if one was provided.
//
// Flags are evaluated remotely by the Flagsmith API by default.
// To evaluate flags locally, instantiate a client using [WithLocalEvaluation].
//
// The following example shows how to evaluate flags for an identity with some application-defined traits:
//
//	evalCtx := flagsmith.NewEvaluationContext(
//		"my-user-123",
//		map[string]interface{}{
//			"company": "ACME Corporation",
//		},
//	)
//	userFlags, _ := GetFlags(ctx.Background(), evalCtx)
//	if userFlags.IsFeatureEnabled("my_feature_key") {
//		// Flag is enabled for this evaluation context
//	}
func (c *Client) GetFlags(ctx context.Context, ec EvaluationContext) (f Flags, err error) {
	ctx = withEvaluationContext(ctx, ec)
	if ec.Identity.Identifier != "" || ec.Identity.Transient {
		return c.GetIdentityFlags(ctx, ec.Identity.Identifier, mapIdentityEvaluationContextToTraits(ec.Identity))
	}
	return c.GetEnvironmentFlags(ctx)
}

// UpdateEnvironment fetches the current environment state from the Flagsmith API. It is called periodically when using
// [WithLocalEvaluation], or when [WithRealtime] is enabled and an update event was received.
func (c *Client) UpdateEnvironment(ctx context.Context) error {
	var env environments.EnvironmentModel
	resp, err := c.client.
		OnBeforeRequest(newRestyLogRequestMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log)).
		NewRequest().
		SetContext(ctx).
		SetResult(&env).
		ForceContentType("application/json").
		Get(c.config.baseURL + "environment-document/")

	if err != nil {
		e := &FlagsmithAPIError{Msg: "failed to GET /environment-document", Err: err}
		return c.handleError(e)
	}
	if resp.IsError() {
		msg := fmt.Sprintf("error response from GET /environment-document: %s", resp.Status())
		e := &FlagsmithAPIError{Msg: msg, Err: nil, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
		return c.handleError(e)
	}
	c.environment.Store(&env)
	identitiesWithOverrides := make(map[string]identities.IdentityModel)
	for _, id := range env.IdentityOverrides {
		identitiesWithOverrides[id.Identifier] = *id
	}
	c.identitiesWithOverrides.Store(identitiesWithOverrides)

	c.log.Info("environment updated", "environment", env.APIKey)
	return nil
}

// GetIdentitySegments returns the segments that this evaluation context is a part of. It requires a local environment
// provided by [WithLocalEvaluation] and/or [WithOfflineHandler].
func (c *Client) GetIdentitySegments(ec EvaluationContext) ([]*segments.SegmentModel, error) {
	if env, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
		identity := c.getIdentityModel(ec.Identity.Identifier, env.APIKey, mapIdentityEvaluationContextToTraits(ec.Identity))
		return flagengine.GetIdentitySegments(env, &identity), nil
	}
	return nil, &FlagsmithClientError{msg: "no local environment available to calculate identity segments"}
}

// BulkIdentify can be used to create/overwrite identities(with traits) in bulk
// NOTE: This method only works with Edge API endpoint.
func (c *Client) BulkIdentify(ctx context.Context, batch []*IdentityTraits) error {
	if len(batch) > BulkIdentifyMaxCount {
		return fmt.Errorf("batch size must be less than %d", BulkIdentifyMaxCount)
	}

	body := struct {
		Data []*IdentityTraits `json:"data"`
	}{Data: batch}

	resp, err := c.client.NewRequest().
		SetBody(&body).
		SetContext(ctx).
		ForceContentType("application/json").
		Post(c.config.baseURL + "bulk-identities/")

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("")
	}
	return nil
}

// GetEnvironmentFlags evaluates and returns the feature flags, using the current environment as the evaluation context.
//
// Use [Client.GetFlags] to evaluate the flags for an identity.
func (c *Client) GetEnvironmentFlags(ctx context.Context) (f Flags, err error) {
	if c.config.localEvaluation || c.config.offlineMode {
		if f, err = c.getEnvironmentFlagsFromEnvironment(); err == nil {
			return f, nil
		}
	} else {
		if f, err = c.getEnvironmentFlagsFromAPI(ctx); err == nil {
			return f, nil
		}
	}
	if c.offlineHandler != nil {
		return c.getEnvironmentFlagsFromEnvironment()
	} else if c.defaultFlagHandler != nil {
		return Flags{defaultFlagHandler: c.defaultFlagHandler}, nil
	}
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err)}
}

// GetIdentityFlags evaluates and returns feature flags, using the given identity as the flag evaluation context.
//
// Deprecated: Use [Client.GetFlags] instead.
func (c *Client) GetIdentityFlags(ctx context.Context, identifier string, traits []Trait) (f Flags, err error) {
	if c.config.localEvaluation || c.config.offlineMode {
		if f, err = c.getIdentityFlagsFromEnvironment(identifier, traits); err == nil {
			return f, nil
		}
	} else {
		if f, err = c.getIdentityFlagsFromAPI(ctx, identifier, traits); err == nil {
			return f, nil
		}
	}
	if c.offlineHandler != nil {
		return c.getIdentityFlagsFromEnvironment(identifier, traits)
	} else if c.defaultFlagHandler != nil {
		return Flags{defaultFlagHandler: c.defaultFlagHandler}, nil
	}
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err)}
}

// getEnvironmentFlagsFromAPI tries to contact the Flagsmith API to get the latest environment data.
func (c *Client) getEnvironmentFlagsFromAPI(ctx context.Context) (Flags, error) {
	client := c.client.
		OnBeforeRequest(newRestyLogRequestMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log))
	req := client.NewRequest()
	ec, ok := getEvaluationContext(ctx)
	if ok && ec.Environment.APIKey != "" {
		req.SetHeader(EnvironmentKeyHeader, ec.Environment.APIKey)
	}

	resp, err := req.
		SetContext(ctx).
		ForceContentType("application/json").
		Get(c.config.baseURL + "flags/")

	if err != nil {
		msg := fmt.Sprintf("GET /flags failed: %s", err)
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("unexpected response from GET /flags: %s", resp.Status())
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	return makeFlagsFromAPIFlags(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)
}

// getIdentityFlagsFromAPI tries to contact the Flagsmith API to get the latest identity flags.
// Will return an error in case of failure or unexpected response.
func (c *Client) getIdentityFlagsFromAPI(ctx context.Context, identifier string, traits []Trait) (Flags, error) {
	body := struct {
		Identifier string  `json:"identifier"`
		Traits     []Trait `json:"traits,omitempty"`
		Transient  bool    `json:"transient,omitempty"`
	}{Identifier: identifier, Traits: traits}
	req := c.client.NewRequest()
	ec, ok := getEvaluationContext(ctx)
	if ok {
		if ec.Environment.APIKey != "" {
			req.SetHeader(EnvironmentKeyHeader, ec.Environment.APIKey)
		}
		body.Transient = ec.Identity.Transient
	}
	resp, err := req.
		SetBody(&body).
		SetContext(ctx).
		ForceContentType("application/json").
		Post(c.config.baseURL + "identities/")
	if err != nil {
		msg := fmt.Sprintf("POST /identities failed: %s", err)
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	if !resp.IsSuccess() {
		msg := fmt.Sprintf("unexpected response from POST /identities: %s", resp.Status())
		return Flags{}, &FlagsmithAPIError{Msg: msg, Err: err, ResponseStatusCode: resp.StatusCode(), ResponseStatus: resp.Status()}
	}
	return makeFlagsfromIdentityAPIJson(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)
}

func (c *Client) getEnvironmentFlagsFromEnvironment() (Flags, error) {
	env, ok := c.environment.Load().(*environments.EnvironmentModel)
	if !ok {
		return Flags{}, fmt.Errorf("getEnvironmentFlagsFromEnvironment: no local environment is available")
	}
	return makeFlagsFromFeatureStates(
		env.FeatureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		"",
	), nil
}

func (c *Client) getIdentityFlagsFromEnvironment(identifier string, traits []Trait) (Flags, error) {
	env, ok := c.environment.Load().(*environments.EnvironmentModel)
	if !ok {
		return Flags{}, fmt.Errorf("getIdentityFlagsFromDocument: no local environment is available")
	}
	identity := c.getIdentityModel(identifier, env.APIKey, traits)
	featureStates := flagengine.GetIdentityFeatureStates(env, &identity)
	flags := makeFlagsFromFeatureStates(
		featureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		identifier,
	)
	return flags, nil
}

func (c *Client) pollEnvironment(ctx context.Context) {
	update := func() {
		ctx, cancel := context.WithTimeout(ctx, c.config.envRefreshInterval)
		defer cancel()
		err := c.UpdateEnvironment(ctx)
		if err != nil {
			c.log.Error("pollEnvironment failed", "error", err)
		}
	}
	update()
	ticker := time.NewTicker(c.config.envRefreshInterval)
	for {
		select {
		case <-ticker.C:
			c.log.Debug("polling environment")
			update()
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) getIdentityModel(identifier string, apiKey string, traits []Trait) identities.IdentityModel {
	identityTraits := make([]*enginetraits.TraitModel, len(traits))
	for i, trait := range traits {
		identityTraits[i] = trait.ToTraitModel()
	}

	identitiesWithOverrides, _ := c.identitiesWithOverrides.Load().(map[string]identities.IdentityModel)
	identity, ok := identitiesWithOverrides[identifier]
	if ok {
		identity.IdentityTraits = identityTraits
		return identity
	}

	return identities.IdentityModel{
		Identifier:        identifier,
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: apiKey,
	}
}

func withEvaluationContext(ctx context.Context, ec EvaluationContext) context.Context {
	return context.WithValue(ctx, contextKeyEvaluationContext, ec)
}

func getEvaluationContext(ctx context.Context) (ec EvaluationContext, ok bool) {
	ec, ok = ctx.Value(contextKeyEvaluationContext).(EvaluationContext)
	return ec, ok
}

func (c *Client) handleError(err *FlagsmithAPIError) *FlagsmithAPIError {
	if c.errorHandler != nil {
		c.errorHandler(err)
	}
	return err
}
