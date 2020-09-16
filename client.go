package bullettrain

import (
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

func (c *BulletTrainClient) GetFeatures() ([]Flag, error) {
	flags := make([]Flag, 0)
	_, err := c.client.NewRequest().
		SetResult(&flags).
		SetQueryParams(map[string]string{
			"page": "1",
		}).
		Get(c.config.BaseURI + "flags/")

	return flags, err
}

func (c *BulletTrainClient) GetUserFeatures(user User) ([]Flag, error) {
	flags := make([]Flag, 0)
	_, err := c.client.NewRequest().
		SetResult(&flags).
		Get(c.config.BaseURI + "flags/" + user.Identifier + "/")

	return flags, err
}

func (c *BulletTrainClient) HasFeature(name string) (bool, error) {
	flags, err := c.GetFeatures()
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, name), nil
}

func (c *BulletTrainClient) HasUserFeature(user User, name string) (bool, error) {
	flags, err := c.GetUserFeatures(user)
	if err != nil {
		return false, err
	}
	return hasFeatureFlag(flags, name), nil
}

func (c *BulletTrainClient) FeatureEnabled(name string) (bool, error) {
	flags, err := c.GetFeatures()
	if err != nil {
		return false, err
	}
	flag := findFeatureFlag(flags, name)
	return flag != nil && flag.Enabled, nil
}

func (c *BulletTrainClient) UserFeatureEnabled(user User, name string) (bool, error) {
	flags, err := c.GetUserFeatures(user)
	if err != nil {
		return false, err
	}
	flag := findFeatureFlag(flags, name)
	return flag != nil && flag.Enabled, nil
}

func (c *BulletTrainClient) GetValue(name string) (string, error) {
	flags, err := c.GetFeatures()
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, name)
	if flag != nil {
		return flag.StateValue, nil
	}

	return "", fmt.Errorf("feature flag '%s' not found", name)
}

func (c *BulletTrainClient) GetUserValue(user User, name string) (string, error) {
	flags, err := c.GetUserFeatures(user)
	if err != nil {
		return "", err
	}
	flag := findFeatureFlag(flags, name)
	if flag != nil {
		return flag.StateValue, nil
	}
	return "", fmt.Errorf("feature flag '%s' not found", name)
}

func (c *BulletTrainClient) GetTrait(user User, key string) (*Trait, error) {
	traits, err := c.GetTraits(user, key)
	if err != nil {
		return nil, err
	} else if len(traits) == 0 {
		return nil, fmt.Errorf("trait '%s' not found", key)
	}
	t := traits[0]
	t.Identity = user
	return t, nil
}

func (c *BulletTrainClient) GetTraits(user User, keys ...string) ([]*Trait, error) {
	resp := struct {
		Flags  []interface{} `json:"flags"`
		Traits []*Trait      `json:"traits"`
	}{}
	_, err := c.client.NewRequest().
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

func (c *BulletTrainClient) UpdateTrait(user User, toUpdate *Trait) (*Trait, error) {
	toUpdate.Identity = user

	trait := new(Trait)
	_, err := c.client.NewRequest().
		SetBody(toUpdate).
		SetResult(trait).
		Post(c.config.BaseURI + "traits/")

	return trait, err
}

func findFeatureFlag(flags []Flag, name string) *Flag {
	for _, flag := range flags {
		if flag.Feature.Name == name {
			return &flag
		}
	}
	return nil
}

func hasFeatureFlag(flags []Flag, name string) bool {
	flag := findFeatureFlag(flags, name)
	return flag != nil
}
