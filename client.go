package flagsmith

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"log"
	"sync/atomic"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"

	. "github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

// Client provides various methods to query BulletTrain API
type Client struct {
	apiKey string
	config config

	environment atomic.Value

	analyticsProcessor *AnalyticsProcessor
	defaultFlagHandler func(featureName string) Flag

	client *resty.Client
}

// NewClient creates instance of Client with given configuration
func NewClient(apiKey string, options ...Option) *Client {
	c := &Client{
		apiKey: apiKey,
		config: defaultConfig(),
		client: resty.New(),
	}

	c.client.SetHeaders(map[string]string{
		"Accept":            "application/json",
		"X-Environment-Key": c.apiKey,
	})

	for _, opt := range options {
		opt(c)
	}
	if c.config.localEvaluation {
		fmt.Println("local evaluation enabled")
		go c.pollEnvironment(context.TODO())
	}
	fmt.Println("is analytics processor ", c.config.enableAnalytics)
	// Initialize analytics processor
	if c.config.enableAnalytics {
		c.analyticsProcessor = NewAnalyticsProcessor(context.TODO(), c.client, c.config.baseURL, nil)
		fmt.Println("initializing analytics processor")
	}

	return c
}

func (c *Client) GetEnvironmentFlags() (Flags, error) {
	return c.GetEnvironmentFlagsWithContext(context.TODO())
}

func (c *Client) GetIdentityFlags(identifier string, traits []*Trait) (Flags, error) {
	return c.GetIdentityFlagsWithContext(context.TODO(), identifier, traits)
}

func (c *Client) GetEnvironmentFlagsWithContext(ctx context.Context) (Flags, error) {
	if env, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
		return c.GetEnvironmentFlagsFromDocument(ctx, env)

	}
	return c.GetEnvironmentFlagsFromAPI(ctx)
}

func (c *Client) GetEnvironmentFlagsFromAPI(ctx context.Context) (Flags, error) {
	resp, err := c.client.NewRequest().
		SetContext(ctx).
		Get(c.config.baseURL + "flags/")
	if err != nil {
		return Flags{}, err
	}
	fmt.Println("getting environment flags from api", resp)
	if !resp.IsSuccess() {
		return Flags{}, errors.New("Unable to get valid response from Flagsmith API.")
	}
	return MakeFlagsFromAPIFlags(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)

}

func (c *Client) GetIdentityFlagsFromAPI(ctx context.Context, identifer string, traits []*Trait) (Flags, error) {
	body := struct {
		Identifier string   `json:"identifier"`
		Traits     []*Trait `json:"traits,omitempty"`
	}{Identifier: identifer, Traits: traits}
	resp, err := c.client.NewRequest().
		SetBody(&body).
		SetContext(ctx).
		Post(c.config.baseURL + "identities/")
	fmt.Println("getting identity flags from api", resp)
	if err != nil {
		return Flags{}, err
	}
	if !resp.IsSuccess() {
		return Flags{}, errors.New("Unable to get valid response from Flagsmith API.")
	}
	return makeFlagsfromIdentityAPIJson(resp.Body(), c.analyticsProcessor, c.defaultFlagHandler)

}

func (c *Client) GetIdentityFlagsWithContext(ctx context.Context, identifier string, traits []*Trait) (Flags, error) {
	if env, ok := c.environment.Load().(*environments.EnvironmentModel); ok {
		return c.GetIdentityFlagsFromDocument(ctx, env, identifier, traits)

	}
	return c.GetIdentityFlagsFromAPI(ctx, identifier, traits)

}

func (c *Client) GetIdentityFlagsFromDocument(ctx context.Context, env *environments.EnvironmentModel, identifier string, traits []*Trait) (Flags, error) {
	identityTraits := make([]*TraitModel, len(traits))
	for i, trait := range traits{
		identityTraits[i] = trait.ToTraitModel()
	}
	identity := identities.IdentityModel{
		Identifier:        identifier,
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: env.APIKey,
	}
	featureStates := flagengine.GetIdentityFeatureStates(env, &identity)
	flags := MakeFlagsFromFeatureStates(
		featureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		identifier,
	)
	return flags, nil
}
func (c *Client) GetEnvironmentFlagsFromDocument(ctx context.Context, env *environments.EnvironmentModel) (Flags, error) {
	return MakeFlagsFromFeatureStates(
		env.FeatureStates,
		c.analyticsProcessor,
		c.defaultFlagHandler,
		"",
	), nil
}

func (c *Client) pollEnvironment(ctx context.Context) {
	//fmt.Println("polling environment");
	update := func() {
		ctx, cancel := context.WithTimeout(ctx, c.config.envRefreshInterval)
		defer cancel()
		err := c.UpdateEnvironment(ctx)
		if err != nil {
			//	fmt.Println("error updating environment: ", err);
			// TODO(tzdybal): error handling - log vs panic?
			log.Printf("ERROR: failed to update environment: %v", err)
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
	//fmt.Println("updating environment");
	var env environments.EnvironmentModel
	e := make(map[string]string)
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetResult(&env).
		SetError(&e).
		Get(c.config.baseURL + "environment-document/")

	if err != nil {
		return err
	}
	if len(e) > 0 {
		return errors.New(e["detail"])
	}
	//fmt.Println("storing environment ", env.ID);
	c.environment.Store(&env)

	return nil
}
