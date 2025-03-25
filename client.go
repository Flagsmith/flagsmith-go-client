package flagsmith

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
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
	baseURL            string
	timeout            time.Duration
	proxy              string
	localEvaluation    bool
	envRefreshInterval time.Duration
	realtimeBaseUrl    string
	useRealtime        bool
	defaultFlagHandler func(string) (Flag, error)
	errorHandler       func(handler *APIError)
	ctxLocalEval       context.Context
	ctxAnalytics       context.Context

	state environmentState

	analyticsProcessor *AnalyticsProcessor
	log                *slog.Logger
	client             *resty.Client
}

// NewClient creates a Flagsmith [Client] using the environment determined by apiKey.
//
// Feature flags are evaluated remotely by the Flagsmith API over HTTP by default.
// To evaluate flags locally, use [WithLocalEvaluation] and a server-side SDK key.
//
// Example:
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
func NewClient(apiKey string, options ...Option) (*Client, error) {
	// Defaults
	c := &Client{
		log:                slog.Default(),
		baseURL:            DefaultBaseURL,
		timeout:            DefaultTimeout,
		envRefreshInterval: time.Second * 60,
		realtimeBaseUrl:    DefaultRealtimeBaseUrl,
		client:             resty.New(),
	}
	// Apply options
	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
	}

	c.log.Debug("initialising Flagsmith client",
		slog.String("api_url", c.baseURL),
		slog.Bool("local_eval", c.localEvaluation),
		slog.Duration("environment_refresh_interval", c.envRefreshInterval),
		slog.Bool("realtime", c.useRealtime),
		slog.String("realtime_url", c.realtimeBaseUrl),
		slog.Bool("offline", c.state.IsOffline()),
		slog.Duration("timeout", c.timeout),
		slog.String("proxy", c.proxy),
	)

	if c.state.IsOffline() {
		return c, nil
	}
	if c.defaultFlagHandler == nil && apiKey == "" {
		return nil, errors.New("no API key, offline environment or default flag handler was provided")
	}

	c.client = c.client.
		SetTimeout(c.timeout).
		OnBeforeRequest(newRestyLogRequestMiddleware(c.log)).
		OnAfterResponse(newRestyLogResponseMiddleware(c.log)).
		SetLogger(restySlogLogger{c.log}).
		SetHeaders(map[string]string{
			"Accept":             "application/json",
			EnvironmentKeyHeader: apiKey,
		})
	if c.proxy != "" {
		c.client = c.client.SetProxy(c.proxy)
	}

	if c.localEvaluation {
		if !strings.HasPrefix(apiKey, "ser.") {
			return nil, errors.New("local flag evaluation requires a server-side SDK key; got " + apiKey)
		}
		if c.envRefreshInterval == 0 && !c.useRealtime {
			return nil, errors.New("local flag evaluation requires a non-zero refresh interval or enabling real-time updates")
		}

		// Fail fast if we can't fetch the initial environment within the timeout
		ctxWithTimeout, cancel := context.WithTimeout(c.ctxLocalEval, c.timeout)
		defer cancel()
		c.log.Debug("fetching initial environment")
		env, err := c.updateAndReturnEnvironment(ctxWithTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch initial environment: %w", err)
		}

		if c.useRealtime {
			streamPath, err := url.JoinPath(c.realtimeBaseUrl, "sse/environments", env.APIKey, "stream")
			if err != nil {
				return nil, fmt.Errorf("failed to build stream URL: %w", err)
			}
			go c.startRealtimeUpdates(c.ctxLocalEval, streamPath)
		}
		if c.envRefreshInterval > 0 {
			go c.pollEnvironment(c.ctxLocalEval)
		}
	}
	return c, nil
}

// MustNewClient panics if NewClient returns an error.
func MustNewClient(apiKey string, options ...Option) *Client {
	client, err := NewClient(apiKey, options...)
	if err != nil {
		panic(err)
	}
	return client
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
	if ec.identifier == "" {
		f, err = c.GetEnvironmentFlags(ctx)
	} else {
		f, err = c.GetIdentityFlags(ctx, ec.identifier, ec.traits)
	}
	if err == nil {
		return f, nil
	} else if c.defaultFlagHandler != nil {
		return Flags{defaultFlagHandler: c.defaultFlagHandler}, nil
	} else {
		return Flags{}, errors.New("GetFlags failed and no default flag handler was provided")
	}
}

// UpdateEnvironment fetches the current environment state from the Flagsmith API. It is called periodically when using
// [WithLocalEvaluation], or when [WithRealtime] is enabled and an update event was received.
func (c *Client) UpdateEnvironment(ctx context.Context) error {
	_, err := c.updateAndReturnEnvironment(ctx)
	return err
}

func (c *Client) updateAndReturnEnvironment(ctx context.Context) (*environments.EnvironmentModel, error) {
	var env environments.EnvironmentModel
	resp, err := c.client.
		NewRequest().
		SetContext(ctx).
		SetResult(&env).
		ForceContentType("application/json").
		Get(c.baseURL + "environment-document/")

	if err != nil {
		return nil, c.handleError(&APIError{Err: err})
	}
	if resp.IsError() {
		e := &APIError{response: resp.RawResponse}
		return nil, c.handleError(e)
	}
	c.state.SetEnvironment(&env)

	c.log.Info("environment updated", "environment", env.APIKey)
	return &env, nil
}

// GetIdentitySegments returns the segments that this evaluation context is a part of. It requires a local environment
// provided by [WithLocalEvaluation] and/or [WithOfflineEnvironment].
func (c *Client) GetIdentitySegments(ec EvaluationContext) (s []*segments.SegmentModel, err error) {
	env, ok := c.state.GetEnvironment()
	if !ok {
		return s, errors.New("GetIdentitySegments called with no local environment available")
	}
	identity := c.getIdentityModel(ec.identifier, env.APIKey, ec.traits)
	return flagengine.GetIdentitySegments(env, identity), nil
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
		Post(c.baseURL + "bulk-identities/")

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("")
	}
	return nil
}

// GetEnvironmentFlags calls GetFlags for the default EvaluationContext.
func (c *Client) GetEnvironmentFlags(ctx context.Context) (f Flags, err error) {
	if c.state.IsOffline() || c.localEvaluation {
		f, err = c.getEnvironmentFlagsFromEnvironment()
	} else {
		f, err = c.getEnvironmentFlagsFromAPI(ctx)
	}
	return f, err
}

// GetIdentityFlags calls GetFlags using this identifier and traits as the EvaluationContext.
func (c *Client) GetIdentityFlags(ctx context.Context, identifier string, traits map[string]interface{}) (f Flags, err error) {
	if c.state.IsOffline() || c.localEvaluation {
		f, err = c.getIdentityFlagsFromEnvironment(identifier, traits)
	} else {
		f, err = c.getIdentityFlagsFromAPI(ctx, identifier, traits)
	}
	return f, err
}

// getEnvironmentFlagsFromAPI tries to contact the Flagsmith API to get the latest environment data.
func (c *Client) getEnvironmentFlagsFromAPI(ctx context.Context) (Flags, error) {
	req := c.client.NewRequest()
	resp, err := req.
		SetContext(ctx).
		ForceContentType("application/json").
		Get(c.baseURL + "flags/")
	if err != nil {
		return Flags{}, &APIError{Err: err}
	}
	if !resp.IsSuccess() {
		return Flags{}, &APIError{response: resp.RawResponse}
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
		Post(c.baseURL + "identities/")
	if err != nil {
		return Flags{}, &APIError{Err: err}
	}
	if !resp.IsSuccess() {
		return Flags{}, &APIError{response: resp.RawResponse}
	}
	return makeFlagsfromIdentityAPIJson(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)
}

func (c *Client) getEnvironmentFlagsFromEnvironment() (Flags, error) {
	env, ok := c.state.GetEnvironment()
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

func (c *Client) getIdentityFlagsFromEnvironment(identifier string, traits map[string]interface{}) (Flags, error) {
	env, ok := c.state.GetEnvironment()
	if !ok {
		return Flags{}, fmt.Errorf("getIdentityFlagsFromDocument: no local environment is available")
	}
	identity := c.getIdentityModel(identifier, env.APIKey, traits)
	featureStates := flagengine.GetIdentityFeatureStates(env, identity)
	flags := makeFlagsFromFeatureStates(
		featureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		identifier,
	)
	return flags, nil
}

func (c *Client) pollEnvironment(ctx context.Context) {
	ticker := time.NewTicker(c.envRefreshInterval)
	for {
		select {
		case <-ticker.C:
			env, ok := c.state.GetEnvironment()
			if ok && time.Since(env.UpdatedAt) < c.envRefreshInterval {
				c.log.Debug("environment is already fresh, skipping poll")
				continue
			}
			c.log.Debug("polling environment")
			err := c.UpdateEnvironment(ctx)
			if err != nil {
				c.log.Error("pollEnvironment failed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) getIdentityModel(identifier string, apiKey string, traits map[string]interface{}) *identities.IdentityModel {
	identityTraits := make([]*enginetraits.TraitModel, 0, len(traits))
	for k, v := range traits {
		identityTraits = append(identityTraits, enginetraits.NewTrait(k, v))
	}

	if identity, ok := c.state.GetIdentityOverride(identifier); ok {
		identity.IdentityTraits = identityTraits
		return identity
	}

	return &identities.IdentityModel{
		Identifier:        identifier,
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: apiKey,
	}
}

func (c *Client) handleError(err *APIError) *APIError {
	if c.errorHandler != nil {
		c.errorHandler(err)
	}
	return err
}
