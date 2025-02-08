package flagsmith

import (
	"context"
	"fmt"
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

// Client provides various methods to query Flagsmith API.
type Client struct {
	apiKey string
	config config

	environment             atomic.Value
	identitiesWithOverrides atomic.Value

	analyticsProcessor *AnalyticsProcessor
	defaultFlagHandler func(string) (Flag, error)

	client         *resty.Client
	ctxLocalEval   context.Context
	ctxAnalytics   context.Context
	log            Logger
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

// NewClient creates instance of Client with given configuration.
func NewClient(apiKey string, options ...Option) *Client {
	c := &Client{
		apiKey: apiKey,
		config: defaultConfig(),
		client: resty.New(),
	}

	c.client.SetHeaders(map[string]string{
		"Accept":             "application/json",
		EnvironmentKeyHeader: c.apiKey,
	})
	c.client.SetTimeout(c.config.timeout)
	c.log = createLogger()

	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
	}
	c.client.SetLogger(c.log)

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
		}
		go c.pollEnvironment(c.ctxLocalEval)
	}
	// Initialize analytics processor
	if c.config.enableAnalytics {
		c.analyticsProcessor = NewAnalyticsProcessor(c.ctxAnalytics, c.client, c.config.baseURL, nil, c.log)
	}
	return c
}

// Returns `Flags` struct holding all the flags for the current environment.
//
// Provide `EvaluationContext` to evaluate flags for a specific environment or identity.
//
// If local evaluation is enabled this function will not call the Flagsmith API
// directly, but instead read the asynchronously updated local environment or
// use the default flag handler in case it has not yet been updated.
//
// Notes:
//
// * `EvaluationContext.Environment` is ignored in local evaluation mode.
//
// * `EvaluationContext.Feature` is not yet supported.
func (c *Client) GetFlags(ctx context.Context, ec *EvaluationContext) (f Flags, err error) {
	if ec != nil {
		ctx = WithEvaluationContext(ctx, *ec)
		if ec.Identity != nil {
			return c.GetIdentityFlags(ctx, *ec.Identity.Identifier, mapIdentityEvaluationContextToTraits(*ec.Identity))
		}
	}
	return c.GetEnvironmentFlags(ctx)
}

// Returns `Flags` struct holding all the flags for the current environment.
//
// If local evaluation is enabled this function will not call the Flagsmith API
// directly, but instead read the asynchronously updated local environment or
// use the default flag handler in case it has not yet been updated.
//
// Deprecated: Use `GetFlags` instead.
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
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err)}
}

// Returns `Flags` struct holding all the flags for the current environment for
// a given identity.
//
// If local evaluation is disabled it will also upsert all traits to the
// Flagsmith API for future evaluations. Providing a trait with a value of nil
// will remove the trait from the identity if it exists.
//
// If local evaluation is enabled this function will not call the Flagsmith API
// directly, but instead read the asynchronously updated local environment or
// use the default flag handler in case it has not yet been updated.
//
// Deprecated: Use `GetFlags` providing `EvaluationContext.Identity` instead.
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
	return Flags{}, &FlagsmithClientError{msg: fmt.Sprintf("Failed to fetch flags with error: %s", err)}
}

// Returns an array of segments that the given identity is part of.
func (c *Client) GetIdentitySegments(identifier string, traits []*Trait) ([]*segments.SegmentModel, error) {
	if env, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
		identity := c.getIdentityModel(identifier, env.APIKey, traits)
		return flagengine.GetIdentitySegments(env, &identity), nil
	}
	return nil, &FlagsmithClientError{msg: "flagsmith: Local evaluation required to obtain identity segments"}
}

// BulkIdentify can be used to create/overwrite identities(with traits) in bulk
// NOTE: This method only works with Edge API endpoint.
func (c *Client) BulkIdentify(ctx context.Context, batch []*IdentityTraits) error {
	if len(batch) > bulkIdentifyMaxCount {
		msg := fmt.Sprintf("flagsmith: batch size must be less than %d", bulkIdentifyMaxCount)
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
	env, ok := c.environment.Load().(*environments.EnvironmentModel)
	if !ok {
		return Flags{}, fmt.Errorf("flagsmith: local environment has not yet been updated")
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

func (c *Client) getEnvironmentFlagsFromEnvironment() (Flags, error) {
	env, ok := c.environment.Load().(*environments.EnvironmentModel)
	if !ok {
		return Flags{}, fmt.Errorf("flagsmith: local environment has not yet been updated")
	}
	return makeFlagsFromFeatureStates(
		env.FeatureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		"",
	), nil
}

func (c *Client) pollEnvironment(ctx context.Context) {
	update := func() {
		ctx, cancel := context.WithTimeout(ctx, c.config.envRefreshInterval)
		defer cancel()
		err := c.UpdateEnvironment(ctx)
		if err != nil {
			c.log.Errorf("Failed to update environment: %v", err)
		}
	}
	update()
	ticker := time.NewTicker(c.config.envRefreshInterval)
	for {
		select {
		case <-ticker.C:
			update()
		case <-ctx.Done():
			return
		}
	}
}
func (c *Client) UpdateEnvironment(ctx context.Context) error {
	var env environments.EnvironmentModel
	resp, err := c.client.NewRequest().
		SetContext(ctx).
		SetResult(&env).
		ForceContentType("application/json").
		Get(c.config.baseURL + "environment-document/")

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
	c.environment.Store(&env)
	identitiesWithOverrides := make(map[string]identities.IdentityModel)
	for _, id := range env.IdentityOverrides {
		identitiesWithOverrides[id.Identifier] = *id
	}
	c.identitiesWithOverrides.Store(identitiesWithOverrides)

	return nil
}

func (c *Client) getIdentityModel(identifier string, apiKey string, traits []*Trait) identities.IdentityModel {
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
