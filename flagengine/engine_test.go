package flagengine_test

import (
	"testing"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils/fixtures"
	"github.com/stretchr/testify/assert"
)

func TestIdentityGetFeatureStateWithoutAnyOverride(t *testing.T) {
	t.Parallel()
	feature1, _, _, env, identity := fixtures.GetFixtures()

	featureState := flagengine.GetIdentityFeatureState(env, identity, feature1.Name)
	assert.Equal(t, feature1, featureState.Feature)
}

func TestIdentityGetAllFeatureStatesNoSegments(t *testing.T) {
	t.Parallel()
	_, _, _, env, identity := fixtures.GetFixtures()

	overriddenFeature := &features.FeatureModel{ID: 3, Name: "overridden_feature", Type: "STANDARD"}

	// set the state of the feature to false in the environment configuration
	env.FeatureStates = append(env.FeatureStates, &features.FeatureStateModel{
		DjangoID: 3, Feature: overriddenFeature, Enabled: false,
	})

	// but true for the identity
	identity.IdentityFeatures = []*features.FeatureStateModel{
		{DjangoID: 4, Feature: overriddenFeature, Enabled: true},
	}

	allFeatureStates := flagengine.GetIdentityFeatureStates(env, identity)
	assert.Len(t, allFeatureStates, 3)
	for _, fs := range allFeatureStates {
		envFeatureState := getEnvironmentFeatureStateForFeature(env, fs.Feature)

		var expected bool
		if fs.Feature == overriddenFeature {
			expected = true
		} else {
			expected = envFeatureState.Enabled
		}
		assert.Equal(t, expected, fs.Enabled)
	}
}

func TestGetIdentityFeatureStatesHidesDisabledFlagsIfEnabled(t *testing.T) {
	t.Parallel()
	_, _, _, env, identity := fixtures.GetFixtures()
	env.Project.HideDisabledFlags = true

	featureStates := flagengine.GetIdentityFeatureStates(env, identity)

	for _, fs := range featureStates {
		assert.True(t, fs.Enabled)
	}
}

func TestIdentityGetAllFeatureStatesSegmentsOnly(t *testing.T) {
	t.Parallel()
	_, _, segment, env, _ := fixtures.GetFixtures()
	traitMatchingSegment := fixtures.TraitMatchingSegment(fixtures.SegmentCondition())
	identityInSegment := fixtures.IdentityInSegment(traitMatchingSegment, env)

	overriddenFeature := &features.FeatureModel{
		ID:   3,
		Name: "overridden_feature",
		Type: "STANDARD",
	}

	env.FeatureStates = append(env.FeatureStates, &features.FeatureStateModel{
		DjangoID: 3,
		Feature:  overriddenFeature,
		Enabled:  false,
	})

	segment.FeatureStates = append(segment.FeatureStates, &features.FeatureStateModel{
		DjangoID: 4,
		Feature:  overriddenFeature,
		Enabled:  true,
	})

	allFeatureStates := flagengine.GetIdentityFeatureStates(env, identityInSegment)

	assert.Len(t, allFeatureStates, 3)

	for _, fs := range allFeatureStates {
		envFeatureState := getEnvironmentFeatureStateForFeature(env, fs.Feature)
		expected := envFeatureState.Enabled
		if fs.Feature == overriddenFeature {
			expected = true
		}
		assert.Equal(t, expected, fs.Enabled)
	}
}

func TestIdentityGetAllFeatureStatesWithTraits(t *testing.T) {
	feature1, _, segment, env, identity := fixtures.GetFixtures()

	envWithSegmentOverride := fixtures.EnvironmentWithSegmentOverride(env, fixtures.SegmentOverrideFs(segment, feature1), segment)

	traitModels := []*traits.TraitModel{
		{TraitKey: fixtures.SegmentConditionProperty, TraitValue: fixtures.SegmentConditionStringValue},
	}

	allFeatureStates := flagengine.GetIdentityFeatureStates(envWithSegmentOverride, identity, traitModels...)

	assert.Equal(t, "segment_override", allFeatureStates[0].RawValue)
}

func TestEnvironmentGetAllFeatureStates(t *testing.T) {
	t.Parallel()

	_, _, _, env, _ := fixtures.GetFixtures()
	featureStates := flagengine.GetEnvironmentFeatureStates(env)

	assert.Equal(t, env.FeatureStates, featureStates)
}

func TestEnvironmentGetFeatureStatesHidesDisabledFlagsIfEnabled(t *testing.T) {
	t.Parallel()

	_, _, _, env, _ := fixtures.GetFixtures()
	env.Project.HideDisabledFlags = true
	featureStates := flagengine.GetEnvironmentFeatureStates(env)

	assert.NotEqual(t, env.FeatureStates, featureStates)
	for _, fs := range featureStates {
		assert.True(t, fs.Enabled)
	}
}

func TestEnvironmentGetFeatureState(t *testing.T) {
	t.Parallel()

	feature1, _, _, env, _ := fixtures.GetFixtures()
	fs := flagengine.GetEnvironmentFeatureState(env, feature1.Name)

	assert.Equal(t, feature1, fs.Feature)
}

func TestEnvironmentGetFeatureStateFeatureNotFound(t *testing.T) {
	t.Parallel()

	_, _, _, env, _ := fixtures.GetFixtures()
	fs := flagengine.GetEnvironmentFeatureState(env, "not_a_feature_name")
	assert.Nil(t, fs)
}

func getEnvironmentFeatureStateForFeature(env *environments.EnvironmentModel, feature *features.FeatureModel) *features.FeatureStateModel {
	for _, fs := range env.FeatureStates {
		if fs.Feature == feature {
			return fs
		}
	}
	return nil
}
