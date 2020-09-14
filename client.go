package bullettrain

import (
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type BulletTrainClient struct {
	apiKey string
	config Config
	client *resty.Client
}

func DefaultBulletTrainClient(apiKey string) *BulletTrainClient {
	return NewBulletTrainClient(apiKey, DefaultConfig())
}

func NewBulletTrainClient(apiKey string, config Config) *BulletTrainClient {
	c := &BulletTrainClient{
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

func (c *BulletTrainClient) GetFeatureFlags() ([]Flag, error) {
	flags := make([]Flag, 0)
	resp, err := c.client.NewRequest().
		SetResult(&flags).
		SetQueryParams(map[string]string{
			"page": "1",
		}).
		Get(c.config.BaseURI + "flags/")

	fmt.Println(string(resp.Body()))

	return flags, err
}

func (c *BulletTrainClient) GetUserFeatureFlags(user FeatureUser) ([]Flag, error) {
	flags := make([]Flag, 0)
	_, err := c.client.NewRequest().
		SetResult(&flags).
		Get(c.config.BaseURI + "flags/" + user.Identifier + "/")

	return flags, err
}

func (c *BulletTrainClient) HasFeatureFlag(featureName string) (bool, error) {
	flags, err := c.GetFeatureFlags()
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, featureName), nil
}

func (c *BulletTrainClient) HasUserFeatureFlag(user FeatureUser, featureName string) (bool, error) {
	flags, err := c.GetUserFeatureFlags(user)
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, featureName), nil
}

func (c *BulletTrainClient) GetFeatureFlagValue(featureName string) (string, error) {
	flags, err := c.GetFeatureFlags()
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, featureName)
	if flag != nil {
		return flag.StateValue, nil
	}

	return "", fmt.Errorf("feature flag '%s' not found", featureName)
}

func (c *BulletTrainClient) GetUserFeatureFlagValue(user FeatureUser, featureName string) (string, error) {
	flags, err := c.GetUserFeatureFlags(user)
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, featureName)
	if flag != nil {
		return flag.StateValue, nil
	}
	return "", fmt.Errorf("feature flag '%s' not found", featureName)
}

func (c *BulletTrainClient) GetTrait(user FeatureUser, key string) (Trait, error) {
	traits, err := c.GetTraits(user, key)
	if err != nil {
		return Trait{}, err
	}
	return traits[0], nil
}

func (c *BulletTrainClient) GetTraits(user FeatureUser, keys ...string) ([]Trait, error) {
	traits := make([]Trait, 0)
	resp, err := c.client.NewRequest().
		SetResult(&traits).
		Get(c.config.BaseURI + "traits/")

	fmt.Println(string(resp.Body()))

	if err != nil {
		return nil, err
	}

	filtered := make([]Trait, 0, len(keys))
	for _, key := range keys {
		for _, t := range traits {
			fmt.Println(t)
			if key == t.Key {
				filtered = append(filtered, t)
				continue
			}
		}
	}
	return filtered, nil
}

func (c *BulletTrainClient) UpdateTrait(user FeatureUser, toUpdate Trait) (Trait, error) {
	return Trait{}, errors.New("not implemented")
}

func findFeatureFlag(flags []Flag, featureName string) *Flag {
	for _, flag := range flags {
		if flag.Feature.Name == featureName {
			return &flag
		}
	}
	return nil
}

func hasFeatureFlag(flags []Flag, featureName string) bool {
	flag := findFeatureFlag(flags, featureName)
	return flag != nil && flag.Enabled
}
