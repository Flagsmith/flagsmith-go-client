package bullettrain_test

import (
	"testing"

	bullettrain "github.com/BulletTrainHQ/bullet-train-go-client"
	"github.com/stretchr/testify/assert"
)

var (
	apiKey           = "MgfUaRCvvZMznuQyqjnQKt"
	testUser         = bullettrain.User{Identifier: "test_user"}
	differentUser    = bullettrain.User{Identifier: "different_user"}
	testFeatureName  = "test_feature"
	testFeatureValue = "sample feature value"
	testTraitName    = "test_trait"
	testTraitValue   = "sample trait value"
	testTrait2Name   = "another_trait"
	testTrait2Value  = "yet another sample trait value"
)

func TestGetFeatureFlags(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	flags, err := c.GetFeatures()

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
	flags, err := c.GetUserFeatures(testUser)

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
	val, err := c.GetValue(testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testFeatureValue, val)
}

func TestGetUserFeatureFlagValue(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	val, err := c.GetUserValue(testUser, testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testFeatureValue, val)
}

func TestHasFeature(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	enabled, err := c.HasFeature(testFeatureName)

	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestHasUserFeature(t *testing.T) {
	c := bullettrain.DefaultBulletTrainClient(apiKey)
	enabled, err := c.HasUserFeature(testUser, testFeatureName)

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
	assert.NoError(t, err)
	assert.Equal(t, newValue, trait.Value)

	trait.Value = "old value"
	_, err = c.UpdateTrait(differentUser, trait)
	assert.NoError(t, err)
}
