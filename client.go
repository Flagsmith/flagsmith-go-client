package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
	"github.com/go-resty/resty/v2"

	enginetraits "github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
)

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

	environment state

	log               *slog.Logger
	logHandlerOptions *slog.HandlerOptions
	client            *resty.Client
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
		c.environment.SetEnvironment(c.offlineHandler.GetEnvironment())
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
// When flag evaluation fails, the value of each Flag is determined by the default flag handler
// from [WithDefaultHandler], if one was provided.
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
	if ec.identifier != "" {
		if c.config.offlineMode && c.offlineHandler != nil || c.config.localEvaluation {
			f, err = c.getIdentityFlagsFromEnvironment(ec.identifier, ec.traits)
		} else {
			f, err = c.getIdentityFlagsFromAPI(ctx, ec.identifier, ec.traits)
		}
		if err != nil && c.defaultFlagHandler != nil {
			f = Flags{defaultFlagHandler: c.defaultFlagHandler}
		}
		return f, err
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
	c.environment.SetEnvironment(&env)

	c.log.Info("environment updated", "environment", env.APIKey)
	return nil
}

// GetIdentitySegments returns the segments that this evaluation context is a part of. It requires a local environment
// provided by [WithLocalEvaluation] and/or [WithOfflineHandler].
func (c *Client) GetIdentitySegments(ec EvaluationContext) ([]*segments.SegmentModel, error) {
	if env := c.environment.GetEnvironment(); env != nil {
		identity := c.getIdentityModel(ec.identifier, env.APIKey, ec.traits)
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

// GetIdentityFlags evaluates and returns the flags for an identity.
//
// Deprecated: Use GetFlags instead.
func (c *Client) GetIdentityFlags(ctx context.Context, identifier string, traits map[string]interface{}) (f Flags, err error) {
	return c.GetFlags(ctx, NewEvaluationContext(identifier, traits))
}

// getEnvironmentFlagsFromAPI tries to contact the Flagsmith API to get the latest environment data.
func (c *Client) getEnvironmentFlagsFromAPI(ctx context.Context) (Flags, error) {
	req := c.client.NewRequest()
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

// getIdentityFlagsFromAPI tries to contact the Flagsmith API to Get the latest identity flags.
// Will return an error in case of failure or unexpected response.
func (c *Client) getIdentityFlagsFromAPI(ctx context.Context, identifier string, traits map[string]interface{}) (Flags, error) {
	tt := make([]Trait, 0, len(traits))
	for k, v := range traits {
		tt = append(tt, Trait{Key: k, Value: v})
	}

	body := struct {
		Identifier string  `json:"identifier"`
		Traits     []Trait `json:"traits,omitempty"`
	}{Identifier: identifier, Traits: tt}
	req := c.client.NewRequest()
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
	env := c.environment.GetEnvironment()
	if env == nil {
		return Flags{}, fmt.Errorf("getEnvironmentFlagsFromEnvironment: no local environment is available")
	}
	return makeFlagsFromFeatureStates(
		env.FeatureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		"",
	), nil
}

func (c *Client) getIdentityFlagsFromEnvironment(identifier string, traits map[string]interface{}) (Flags, error) {
	env := c.environment.GetEnvironment()
	if env := c.environment.GetEnvironment(); env == nil {
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

func (c *Client) getIdentityModel(identifier string, apiKey string, traits map[string]interface{}) identities.IdentityModel {
	identityTraits := make([]*enginetraits.TraitModel, 0, len(traits))
	for k, v := range traits {
		identityTraits = append(identityTraits, enginetraits.NewTrait(k, v))
	}

	if identity, ok := c.environment.GetIdentityOverride(identifier); ok {
		identity.IdentityTraits = identityTraits
		return identity
	}

	return identities.IdentityModel{
		Identifier:        identifier,
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: apiKey,
	}
}

func (c *Client) handleError(err *FlagsmithAPIError) *FlagsmithAPIError {
	if c.errorHandler != nil {
		c.errorHandler(err)
	}
	return err
}

type state struct {
	mu                sync.RWMutex
	environment       *environments.EnvironmentModel
	identityOverrides sync.Map
}

func (es *state) GetEnvironment() *environments.EnvironmentModel {
	es.mu.RLock()
	defer es.mu.RUnlock()
	return es.environment
}

func (es *state) GetIdentityOverride(identifier string) (identities.IdentityModel, bool) {
	i, ok := es.identityOverrides.Load(identifier)
	if ok && i != nil {
		return i.(identities.IdentityModel), true
	}
	return identities.IdentityModel{}, ok
}

func (es *state) SetEnvironment(env *environments.EnvironmentModel) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.environment = env
	for _, id := range env.IdentityOverrides {
		es.identityOverrides.Store(id.Identifier, *id)
	}
}
