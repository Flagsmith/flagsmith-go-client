package bullettrain

import (
	"errors"

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
	_, err := c.client.NewRequest().
		SetResult(&flags).
		SetQueryParams(map[string]string{
			"page": "1",
		}).
		Get(c.config.BaseURI + "flags/")

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
	return "", errors.New("not implemented")
}

func (c *BulletTrainClient) GetUserFeatureFlagValue(user FeatureUser, featureName string) (string, error) {
	return "", errors.New("not implemented")
}

func (c *BulletTrainClient) GetTrait(user FeatureUser, key string) (Trait, error) {
	return Trait{}, errors.New("not implemented")
}

func (c *BulletTrainClient) GetTraits(user FeatureUser, keys ...string) ([]Trait, error) {
	return nil, errors.New("not implemented")
}

func (c *BulletTrainClient) UpdateTrait(user FeatureUser, toUpdate Trait) (Trait, error) {
	return Trait{}, errors.New("not implemented")
}

func hasFeatureFlag(flags []Flag, featureName string) bool {
	for _, flag := range flags {
		if flag.Enabled && flag.Feature.Name == featureName {
			return true
		}
	}
	return false
}
