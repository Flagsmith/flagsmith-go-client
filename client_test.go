package flagsmith_test

import (
	"context"
	"testing"

	flagsmith "github.com/Flagsmith/flagsmith-go-client"
	"github.com/stretchr/testify/assert"
)

var (
	apiKey               = "MgfUaRCvvZMznuQyqjnQKt"
	testUser             = flagsmith.User{Identifier: "test_user"}
	differentUser        = flagsmith.User{Identifier: "different_user"}
	testFeatureName      = "test_feature"
	testFeatureValue     = "sample feature value"
	testUserFeatureValue = "user feature value"
	testFlagName         = "test_flag"
	testFlagValue        = true
	testTraitName        = "test_trait"
	testTraitValue       = "sample trait value"
)

func TestGetFeatureFlags(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
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
	c := flagsmith.DefaultClient(apiKey)
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
	c := flagsmith.DefaultClient(apiKey)
	val, err := c.GetValue(testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testFeatureValue, val)
}

func TestGetUserFeatureFlagValue(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	val, err := c.GetUserValue(testUser, testFeatureName)

	assert.NoError(t, err)
	assert.Equal(t, testUserFeatureValue, val)
}

func TestHasFeature(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	enabled, err := c.HasFeature(testFeatureName)

	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestHasUserFeature(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	enabled, err := c.HasUserFeature(testUser, testFeatureName)

	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestGetTrait(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	trait, err := c.GetTrait(testUser, testTraitName)

	assert.NoError(t, err)
	assert.Equal(t, testTraitValue, trait.Value)
}

func TestGetTraits(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	traits, err := c.GetTraits(testUser)

	assert.NoError(t, err)
	assert.NotNil(t, traits)
	assert.Len(t, traits, 2)
}

func TestUpdateTrait(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
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

func updateTraitsAsserts(updated []*flagsmith.Trait, err error, c *flagsmith.Client, t *testing.T) {
	assert.NoError(t, err)
	assert.NotEmpty(t, updated)
	assert.Equal(t, 5, len(updated))
	assert.Equal(t, "true", updated[0].Value)
	assert.Equal(t, "42", updated[1].Value)
	assert.Equal(t, "foo bar baz", updated[2].Value)
	assert.Equal(t, "616", updated[3].Value)
	assert.Equal(t, "3.14", updated[4].Value)

	trait, err := c.GetTrait(differentUser, "boolField")
	assert.NoError(t, err)
	assert.Equal(t, "true", trait.Value)

	trait, err = c.GetTrait(differentUser, "intField")
	assert.NoError(t, err)
	assert.Equal(t, "42", trait.Value)

	trait, err = c.GetTrait(differentUser, "stringField")
	assert.NoError(t, err)
	assert.Equal(t, "foo bar baz", trait.Value)

	trait, err = c.GetTrait(differentUser, "anotherField")
	assert.NoError(t, err)
	assert.Equal(t, "616", trait.Value)

	trait, err = c.GetTrait(differentUser, "floatField")
	assert.NoError(t, err)
	assert.Equal(t, "3.14", trait.Value)
}

func TestBulkUpdateTraitsPointers(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)

	traits := []*flagsmith.Trait{
		{
			Identity: flagsmith.User{},
			Key:      "boolField",
			Value:    "true",
		},
		{
			Identity: flagsmith.User{},
			Key:      "intField",
			Value:    "42",
		},
		{
			Identity: flagsmith.User{},
			Key:      "stringField",
			Value:    "foo bar baz",
		},
		{
			Identity: flagsmith.User{},
			Key:      "anotherField",
			Value:    "616",
		},
		{
			Identity: flagsmith.User{},
			Key:      "floatField",
			Value:    "3.14",
		},
	}
	updated, err := c.UpdateTraits(differentUser, traits)
	updateTraitsAsserts(updated, err, c, t)
}

func TestBulkUpdateTraitsObject(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)

	object := struct {
		boolField    bool
		intField     int
		stringField  string
		anotherField int
		floatField   float64
	}{
		true, 42, "foo bar baz", 616, 3.14,
	}

	updated, err := c.UpdateTraits(differentUser, object)
	updateTraitsAsserts(updated, err, c, t)
}

func TestBulkUpdateTraits(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)

	traits := []flagsmith.Trait{
		{
			Identity: flagsmith.User{},
			Key:      "boolField",
			Value:    "true",
		},
		{
			Identity: flagsmith.User{},
			Key:      "intField",
			Value:    "42",
		},
		{
			Identity: flagsmith.User{},
			Key:      "stringField",
			Value:    "foo bar baz",
		},
		{
			Identity: flagsmith.User{},
			Key:      "anotherField",
			Value:    "616",
		},
		{
			Identity: flagsmith.User{},
			Key:      "floatField",
			Value:    "3.14",
		},
	}

	updated, err := c.UpdateTraits(differentUser, traits)
	updateTraitsAsserts(updated, err, c, t)
}

func TestFeatureEnabled(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	enabled, err := c.FeatureEnabled(testFlagName)
	assert.NoError(t, err)
	assert.Equal(t, testFlagValue, enabled)
}

func TestUserFeatureEnabled(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	enabled, err := c.UserFeatureEnabled(testUser, testFlagName)
	assert.NoError(t, err)
	assert.Equal(t, testFlagValue, enabled)
}

func TestRemoteConfig(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)

	// string
	val, err := c.GetValue(testFeatureName)
	assert.NoError(t, err)
	strVal, ok := val.(string)
	assert.True(t, ok)
	assert.Equal(t, testFeatureValue, strVal)

	// integer
	val, err = c.GetValue("integer_feature")
	assert.NoError(t, err)
	intVal, ok := val.(int)
	assert.True(t, ok)
	assert.Equal(t, 200, intVal)

	// bool
	val, err = c.GetValue("boolean_feature")
	assert.NoError(t, err)
	boolVal, ok := val.(bool)
	assert.True(t, ok)
	assert.Equal(t, true, boolVal)
}

func TestGetTraitsWithContextCancel(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	traits, err := c.GetTraitsWithContext(ctx, testUser)

	assert.Nil(t, traits)
	assert.Error(t, err)
}

func TestGetFeatureFlagsWithContextCancel(t *testing.T) {
	c := flagsmith.DefaultClient(apiKey)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetFeaturesWithContext(ctx)

	assert.Error(t, err)
}
