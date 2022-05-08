package flagsmith

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/go-resty/resty/v2"
)

// Client provides various methods to query BulletTrain API
type Client struct {
	apiKey string
	config Config
	client *resty.Client
}

// DefaultClient returns new Client with default configuration
func DefaultClient(apiKey string) *Client {
	return NewClient(apiKey, DefaultConfig())
}

// NewClient creates instance of Client with given configuration
func NewClient(apiKey string, config Config) *Client {
	c := &Client{
		apiKey: apiKey,
		config: config,
		client: resty.New(),
	}

	c.client.SetHeaders(map[string]string{
		"Accept":            "application/json",
		"X-Environment-Key": c.apiKey,
	})

	return c
}

// NewClient creates instance of Client with given configuration
func New(apiKey string, options ...Option) *Client {
	c := &Client{
		apiKey: apiKey,
		config: DefaultConfig(),
		client: resty.New(),
	}

	c.client.SetHeaders(map[string]string{
		"Accept":            "application/json",
		"X-Environment-Key": c.apiKey,
	})

	for _, opt := range options {
		opt(c)
	}

	return c
}

func (c *Client) SetProxy(proxyUrl string) {
	c.client.SetProxy(proxyUrl)
}
func (c *Client) RemoveProxy() {
	c.client.RemoveProxy()
}

// GetFeatures returns all features available in given environment
func (c *Client) GetFeatures() ([]Flag, error) {
	return c.GetFeaturesWithContext(context.TODO())
}

// GetFeaturesWithContext returns all features available in given environment
func (c *Client) GetFeaturesWithContext(ctx context.Context) ([]Flag, error) {
	flags := make([]Flag, 0)
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetResult(&flags).
		Get(c.config.BaseURI + "flags/")

	return flags, err
}

// GetUserFeatures returns all features as defined for given user
func (c *Client) GetUserFeatures(user User) ([]Flag, error) {
	return c.GetUserFeaturesWithContext(context.TODO(), user)
}

// GetUserFeaturesWithContext returns all features as defined for given user
func (c *Client) GetUserFeaturesWithContext(ctx context.Context, user User) ([]Flag, error) {
	flags := make([]Flag, 0)
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetResult(&flags).
		Get(c.config.BaseURI + "flags/" + user.Identifier + "/")

	return flags, err
}

// HasFeature returns information whether given feature is defined
func (c *Client) HasFeature(name string) (bool, error) {
	return c.HasFeatureWithContext(context.TODO(), name)
}

// HasFeatureWithContext returns information whether given feature is defined
func (c *Client) HasFeatureWithContext(ctx context.Context, name string) (bool, error) {
	flags, err := c.GetFeaturesWithContext(ctx)
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, name), nil
}

// HasUserFeature returns information whether given feature is defined for given user
func (c *Client) HasUserFeature(user User, name string) (bool, error) {
	return c.HasUserFeatureWithContext(context.TODO(), user, name)
}

// HasUserFeatureWithContext returns information whether given feature is defined for given user
func (c *Client) HasUserFeatureWithContext(ctx context.Context, user User, name string) (bool, error) {
	flags, err := c.GetUserFeaturesWithContext(ctx, user)
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, name), nil
}

// FeatureEnabled returns information whether given feature flag is enabled
func (c *Client) FeatureEnabled(name string) (bool, error) {
	return c.FeatureEnabledWithContext(context.TODO(), name)
}

// FeatureEnabledWithContext returns information whether given feature flag is enabled
func (c *Client) FeatureEnabledWithContext(ctx context.Context, name string) (bool, error) {
	flags, err := c.GetFeaturesWithContext(ctx)
	if err != nil {
		return false, err
	}
	flag := findFeatureFlag(flags, name)
	return flag != nil && flag.Enabled, nil
}

// UserFeatureEnabled returns information whether given feature flag is enabled for given user
func (c *Client) UserFeatureEnabled(user User, name string) (bool, error) {
	return c.UserFeatureEnabledWithContext(context.TODO(), user, name)
}

// UserFeatureEnabled returns information whether given feature flag is enabled for given user
func (c *Client) UserFeatureEnabledWithContext(ctx context.Context, user User, name string) (bool, error) {

	flags, err := c.GetUserFeaturesWithContext(ctx, user)
	if err != nil {
		return false, err
	}
	flag := findFeatureFlag(flags, name)
	return flag != nil && flag.Enabled, nil
}

// GetValue returns value of given feature (remote config)
//
// Returned value can have one of following types: bool, int, string
func (c *Client) GetValue(name string) (interface{}, error) {
	return c.GetValueWithContext(context.TODO(), name)
}

// GetValueWithContext returns value of given feature (remote config)
//
// Returned value can have one of following types: bool, int, string
func (c *Client) GetValueWithContext(ctx context.Context, name string) (interface{}, error) {
	flags, err := c.GetFeaturesWithContext(ctx)
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, name)
	if flag != nil {
		return convertValue(flag.StateValue), nil
	}

	return "", fmt.Errorf("feature flag '%s' not found", name)
}

// GetUserValue return value of given feature (remote config) as defined for given user
//
// Returned value can have one of following types: bool, int, string
func (c *Client) GetUserValue(user User, name string) (interface{}, error) {
	return c.GetUserValueWithContext(context.TODO(), user, name)
}

// GetUserValueWithContext return value of given feature (remote config) as defined for given user
//
// Returned value can have one of following types: bool, int, string
func (c *Client) GetUserValueWithContext(ctx context.Context, user User, name string) (interface{}, error) {
	flags, err := c.GetUserFeaturesWithContext(ctx, user)
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, name)
	if flag != nil {
		return convertValue(flag.StateValue), nil
	}
	return "", fmt.Errorf("feature flag '%s' not found", name)
}

// GetTrait returns trait defined for given user
func (c *Client) GetTrait(user User, key string) (*Trait, error) {
	return c.GetTraitWithContext(context.TODO(), user, key)
}

// GetTraitWithContext returns trait defined for given user
func (c *Client) GetTraitWithContext(ctx context.Context, user User, key string) (*Trait, error) {
	traits, err := c.GetTraitsWithContext(ctx, user, key)
	if err != nil {
		return nil, err
	} else if len(traits) == 0 {
		return nil, fmt.Errorf("trait '%s' not found", key)
	}
	t := traits[0]
	t.Identity = user
	return t, nil
}

// GetTraits returns traits defined for given user
//
// If keys are provided, GetTrais returns only corresponding traits,
// otherwise all traits for given user are returned.
func (c *Client) GetTraits(user User, keys ...string) ([]*Trait, error) {
	return c.GetTraitsWithContext(context.TODO(), user, keys...)
}

// GetTraitsWithContext returns traits defined for given user
//
// If keys are provided, GetTrais returns only corresponding traits,
// otherwise all traits for given user are returned.
func (c *Client) GetTraitsWithContext(ctx context.Context, user User, keys ...string) ([]*Trait, error) {
	resp := struct {
		Flags  []interface{} `json:"flags"`
		Traits []*Trait      `json:"traits"`
	}{}
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetResult(&resp).
		SetQueryParam("identifier", user.Identifier).
		Get(c.config.BaseURI + "identities/")

	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return resp.Traits, nil
	}

	filtered := make([]*Trait, 0, len(keys))
	for _, key := range keys {
		for _, t := range resp.Traits {
			if key == t.Key {
				filtered = append(filtered, t)
				continue
			}
		}
	}
	return filtered, nil
}

// UpdateTrait updates trait value
func (c *Client) UpdateTrait(user User, toUpdate *Trait) (*Trait, error) {
	return c.UpdateTraitWithContext(context.TODO(), user, toUpdate)
}

// UpdateTraitWithContext updates trait vaule with context
func (c *Client) UpdateTraitWithContext(ctx context.Context, user User, toUpdate *Trait) (*Trait, error) {
	toUpdate.Identity = user

	trait := new(Trait)
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetBody(toUpdate).
		SetResult(trait).
		Post(c.config.BaseURI + "traits/")

	return trait, err
}

// UpdateTraits updates trait values in bulk.
// The object argument may be a []Trait, []*Trait or a struct.
// If object is []Trait or []*Trait then their Identity field may be mutated.
// If object is a struct then the struct will be inspected
// and each key value pair will be considered as a separate trait to be updated.
func (c *Client) UpdateTraits(user User, object interface{}) ([]*Trait, error) {
	return c.UpdateTraitsWithContext(context.TODO(), user, object)
}

// UpdateTraitsWithContext updates trait values in bulk.
// The object argument may be a []Trait, []*Trait or a struct.
// If object is []Trait or []*Trait then their Identity field may be mutated.
// If object is a struct then the struct will be inspected
// and each key value pair will be considered as a separate trait to be updated.
func (c *Client) UpdateTraitsWithContext(ctx context.Context, user User, object interface{}) ([]*Trait, error) {
	var bulkResp []*Trait = nil
	var bulk []Trait = nil
	switch traits := object.(type) {
	case []*Trait:
		bulk = make([]Trait, len(traits))
		for idx, trait := range traits {
			bulk[idx] = *trait
			bulk[idx].Identity = user
		}
	case []Trait:
		bulk = make([]Trait, len(traits))
		for idx, trait := range traits {
			bulk[idx] = trait
			bulk[idx].Identity = user
		}
	default:
		value := reflect.ValueOf(object)
		typ := reflect.TypeOf(object)
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			toUpdate := Trait{}
			switch field.Kind() {
			case reflect.Bool:
				toUpdate.Value = strconv.FormatBool(field.Bool())
			case reflect.String:
				toUpdate.Value = field.String()
			case reflect.Int:
				toUpdate.Value = strconv.FormatInt(field.Int(), 10)
			case reflect.Float32, reflect.Float64:
				toUpdate.Value = strconv.FormatFloat(field.Float(), 'f', -1, 64)
			default:
				return nil, fmt.Errorf("invalid type of field '%s' (bool, int and string are supported)", typ.Field(i).Name)
			}
			toUpdate.Identity = user
			toUpdate.Key = typ.Field(i).Name
			bulk = append(bulk, toUpdate)

		}
	}
	bulkResp = make([]*Trait, len(bulk))
	_, err := c.client.NewRequest().
		SetContext(ctx).
		SetBody(bulk).
		SetResult(&bulkResp).
		Put(c.config.BaseURI + "traits/bulk/")
	return bulkResp, err
}

func findFeatureFlag(flags []Flag, name string) *Flag {
	for i, flag := range flags {
		if flag.Feature.Name == name {
			return &flags[i]
		}
	}
	return nil
}

func hasFeatureFlag(flags []Flag, name string) bool {
	flag := findFeatureFlag(flags, name)
	return flag != nil
}

// convertValue converts from float64 (default "JSON number" representation in Go) to int
func convertValue(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		return int(v)
	default:
		return value
	}
}
