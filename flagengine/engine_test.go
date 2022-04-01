package flagengine_test

import (
	"encoding/json"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/utils/fixtures"
	"io/ioutil"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Flagsmith/flagsmith-go-client/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

const TestData = "./engine-test-data/data/environment_n9fbf9h3v4fFgH3U3ngWhb.json"

func TestEngine(t *testing.T) {
	t.Parallel()
	var testData struct {
		Environment environments.EnvironmentModel `json:"environment"`
		TestCases   []struct {
			Identity identities.IdentityModel `json:"identity"`
			Response struct {
				Traits []traits.TraitModel          `json:"traits"`
				Flags  []features.FeatureStateModel `json:"flags"`
			} `json:"response"`
		} `json:"identities_and_responses"`
	}

	testSpec, err := ioutil.ReadFile(TestData)
	require.NoError(t, err)
	require.NotEmpty(t, testSpec)

	err = json.Unmarshal(testSpec, &testData)
	require.NoError(t, err)

	for i, c := range testData.TestCases {
		t.Run(strconv.Itoa(i)+":"+c.Identity.CompositeKey(), func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			actual := flagengine.GetIdentityFeatureStates(&testData.Environment, &c.Identity)
			expected := c.Response.Flags

			sort.Slice(actual, func(i, j int) bool {
				return actual[i].Feature.Name < actual[j].Feature.Name
			})
			sort.Slice(expected, func(i, j int) bool {
				return expected[i].Feature.Name < expected[j].Feature.Name
			})

			require.Len(actual, len(expected))
			for i := range expected {
				id := strconv.Itoa(c.Identity.DjangoID)
				assert.Equal(expected[i].Value(id), actual[i].Value(id))
				assert.Equal(expected[i].Enabled, actual[i].Enabled)
			}
		})
	}
}

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
	t.Parallel()

	feature1, _, segment, env, identity := fixtures.GetFixtures()

	envWithSegmentOverride := fixtures.EnvironmentWithSegmentOverride(env, fixtures.SegmentOverrideFs(segment, feature1), segment)

	traitModels := []*traits.TraitModel{
		{TraitKey: fixtures.SegmentConditionProperty, TraitValue: fixtures.SegmentConditionStringValaue},
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

func getEnvironmentFeatureStateForFeature(env *environments.EnvironmentModel, feature *features.FeatureModel) *features.FeatureStateModel {
	for _, fs := range env.FeatureStates {
		if fs.Feature == feature {
			return fs
		}
	}
	return nil
}

func getEnvironmentFeatureStateForFeatureByName(env *environments.EnvironmentModel, featureName string) *features.FeatureStateModel {
	for _, fs := range env.FeatureStates {
		if fs.Feature.Name == featureName {
			return fs
		}
	}
	return nil
}
