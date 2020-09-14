package bullettrain_test

import (
	"testing"

	bullettrain "github.com/BulletTrainHQ/bullet-train-go-client"
	"github.com/stretchr/testify/assert"
)

var (
	apiKey           = "MgfUaRCvvZMznuQyqjnQKt"
	testUser         = bullettrain.FeatureUser{Identifier: "test_user"}
	differentUser    = bullettrain.FeatureUser{Identifier: "different_user"}
	testFeatureName  = "test_feature"
	testFeatureValue = "sample feature value"
	testTraitName    = "test_trait"
	testTraitValue   = "sample trait value"
	testTrait2Name   = "another_trait"
	testTrait2Value  = "yet another sample trait value"
)

func TestGetFeatureFlags(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	flags, err := c.GetFeatureFlags()

	assert.NoError(t, err)
	assert.NotNil(t, flags)
	assert.NotEmpty(t, flags)

	for _, flag := range flags {
		assert.NotNil(t, flag.Feature, "Flag should have feature")
		assert.NotNil(t, flag.Feature.Name)
	}
}

func TestGetUserFeatureFlags(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	flags, err := c.GetUserFeatureFlags(testUser)

	assert.NoError(t, err)
	assert.NotNil(t, flags)
	assert.NotEmpty(t, flags)

	for _, flag := range flags {
		assert.NotNil(t, flag.Feature, "Flag should have feature")
		assert.NotNil(t, flag.Feature.Name)
	}
}

func TestGetFeatureFlagValue(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	val, err := c.GetFeatureFlagValue(testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testFeatureValue, val)
}

func TestGetUserFeatureFlagValue(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	val, err := c.GetUserFeatureFlagValue(testUser, testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testFeatureValue, val)
}

func TestHasFeatureFlag(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	enabled, err := c.HasFeatureFlag(testFeatureName)

	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestHasUserFeatureFlag(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	enabled, err := c.HasUserFeatureFlag(testUser, testFeatureName)

	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestGetTrait(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	trait, err := c.GetTrait(testUser, testTraitName)

	assert.NoError(t, err)
	assert.Equal(t, testTraitValue, trait.Value)
}

func TestGetTraits(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	traits, err := c.GetTraits(testUser)

	assert.NoError(t, err)
	assert.NotNil(t, traits)
	assert.Len(t, traits, 2)
}

func TestUpdateTrait(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	trait, err := c.GetTrait(differentUser, testTraitName)
	assert.NoError(t, err)

	newValue := "new value"

	trait.Value = newValue
	updated, err := c.UpdateTrait(differentUser, trait)
	assert.NoError(t, err)
	assert.Equal(t, trait.Value, updated.Value)

	trait, err = c.GetTrait(differentUser, testTraitName)
	assert.Equal(t, newValue, trait.Value)
}
