package flagengine_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/projects"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/segments"
)

func TestGetEvaluationResultDependsOnTheHighestPriorityOverrideOfAnotherFlag(t *testing.T) {
	t.Parallel()

	// Given
	strongPriority, weakPriority := 1.0, 10.0
	prerequisiteEnabled := []engine_eval.SegmentRule{{
		Type:       engine_eval.All,
		Conditions: []engine_eval.Condition{{Operator: engine_eval.Equal, Property: "$.flags.prerequisite.enabled", Value: "true"}},
	}}
	ec := &engine_eval.EngineEvaluationContext{
		Environment: engine_eval.EnvironmentContext{Key: "key", Name: "Environment"},
		Features: map[string]engine_eval.FeatureContext{
			"prerequisite": {Key: "1", Name: "prerequisite", Enabled: true, Value: "default"},
			"colour":       {Key: "2", Name: "colour", Value: "default"},
			"dependent":    {Key: "3", Name: "dependent", Value: "default"},
		},
		Segments: map[string]engine_eval.SegmentContext{
			"1": {
				Key: "1", Name: "strong_colour", Rules: prerequisiteEnabled,
				Overrides: []engine_eval.FeatureContext{{Key: "2", Name: "colour", Enabled: true, Value: "blue", Priority: &strongPriority}},
			},
			"2": {
				Key: "2", Name: "weak_colour", Rules: prerequisiteEnabled,
				Overrides: []engine_eval.FeatureContext{{Key: "2", Name: "colour", Enabled: true, Value: "red", Priority: &weakPriority}},
			},
			"3": {
				Key: "3", Name: "blue_colour",
				Rules: []engine_eval.SegmentRule{{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{{Operator: engine_eval.Equal, Property: "$.flags.colour.value", Value: "blue"}},
				}},
				Overrides: []engine_eval.FeatureContext{{Key: "3", Name: "dependent", Enabled: true, Value: "overridden"}},
			},
		},
	}

	// When
	result := flagengine.GetEvaluationResult(ec)

	// Then
	assert.Equal(t, &engine_eval.FlagResult{
		Name: "colour", Enabled: true, Value: "blue", Reason: "TARGETING_MATCH; segment=strong_colour",
	}, result.Flags["colour"])
	assert.Equal(t, &engine_eval.FlagResult{
		Name: "dependent", Enabled: true, Value: "overridden", Reason: "TARGETING_MATCH; segment=blue_colour",
	}, result.Flags["dependent"])
}

func TestGetEvaluationResultServesTheFirstSegmentInContextAmongOverridesOfEqualPriority(t *testing.T) {
	t.Parallel()

	// Given
	var ec engine_eval.EngineEvaluationContext
	err := json.Unmarshal([]byte(`{
		"environment": {"key": "key", "name": "Environment"},
		"features": {"colour": {"key": "1", "name": "colour", "enabled": false, "value": "default"}},
		"segments": {
			"3": {
				"key": "3", "name": "red_colour",
				"rules": [{"type": "ALL", "conditions": [{"operator": "EQUAL", "property": "$.environment.name", "value": "Environment"}]}],
				"overrides": [{"key": "1", "name": "colour", "enabled": true, "value": "red", "priority": 0}]
			},
			"25": {
				"key": "25", "name": "blue_colour",
				"rules": [{"type": "ALL", "conditions": [{"operator": "EQUAL", "property": "$.environment.name", "value": "Environment"}]}],
				"overrides": [{"key": "1", "name": "colour", "enabled": true, "value": "blue", "priority": 0}]
			}
		}
	}`), &ec)
	assert.NoError(t, err)

	// When
	result := flagengine.GetEvaluationResult(&ec)

	// Then
	assert.Equal(t, &engine_eval.FlagResult{
		Name: "colour", Enabled: true, Value: "red", Reason: "TARGETING_MATCH; segment=red_colour",
	}, result.Flags["colour"])
	assert.Equal(t, []engine_eval.SegmentResult{{Name: "red_colour"}, {Name: "blue_colour"}}, result.Segments)
}

func TestGetEvaluationResultServesTheFirstProjectSegmentAmongOverridesOfEqualPriority(t *testing.T) {
	t.Parallel()

	// Given
	everyone := []*segments.SegmentRuleModel{{
		Type:       segments.All,
		Conditions: []*segments.SegmentConditionModel{{Operator: "EQUAL", Property: "$.environment.name", Value: "Environment"}},
	}}
	colour := &features.FeatureModel{ID: 1, Name: "colour"}
	ec := engine_eval.MapEnvironmentDocumentToEvaluationContext(&environments.EnvironmentModel{
		APIKey:        "key",
		Name:          "Environment",
		FeatureStates: []*features.FeatureStateModel{{Feature: colour, RawValue: "default"}},
		Project: &projects.ProjectModel{Segments: []*segments.SegmentModel{
			{ID: 3, Name: "red_colour", Rules: everyone, FeatureStates: []*features.FeatureStateModel{
				{Feature: colour, Enabled: true, RawValue: "red", FeatureSegment: &features.FeatureSegment{Priority: 0}},
			}},
			{ID: 25, Name: "blue_colour", Rules: everyone, FeatureStates: []*features.FeatureStateModel{
				{Feature: colour, Enabled: true, RawValue: "blue", FeatureSegment: &features.FeatureSegment{Priority: 0}},
			}},
		}},
	})

	// When
	result := flagengine.GetEvaluationResult(&ec)

	// Then
	assert.Equal(t, &engine_eval.FlagResult{
		Name: "colour", Enabled: true, Value: "red", Reason: "TARGETING_MATCH; segment=red_colour",
		Metadata: engine_eval.FeatureMetadata{FeatureID: 1},
	}, result.Flags["colour"])
}

func TestGetEvaluationResultServesSegmentsAddedToAContextAfterItsOwn(t *testing.T) {
	t.Parallel()

	// Given
	var ec engine_eval.EngineEvaluationContext
	err := json.Unmarshal([]byte(`{
		"environment": {"key": "key", "name": "Environment"},
		"features": {"colour": {"key": "1", "name": "colour", "enabled": false, "value": "default"}},
		"segments": {
			"3": {
				"key": "3", "name": "red_colour",
				"rules": [{"type": "ALL", "conditions": [{"operator": "EQUAL", "property": "$.environment.name", "value": "Environment"}]}],
				"overrides": [{"key": "1", "name": "colour", "enabled": true, "value": "red", "priority": 0}]
			},
			"25": {"key": "25", "name": "removed", "rules": []}
		}
	}`), &ec)
	assert.NoError(t, err)
	priority := 0.0
	everyone := []engine_eval.SegmentRule{{
		Type:       engine_eval.All,
		Conditions: []engine_eval.Condition{{Operator: engine_eval.Equal, Property: "$.environment.name", Value: "Environment"}},
	}}
	delete(ec.Segments, "25")
	ec.Segments["1"] = engine_eval.SegmentContext{
		Key: "1", Name: "blue_colour", Rules: everyone,
		Overrides: []engine_eval.FeatureContext{{Key: "1", Name: "colour", Enabled: true, Value: "blue", Priority: &priority}},
	}

	// When
	result := flagengine.GetEvaluationResult(&ec)

	// Then
	assert.Equal(t, &engine_eval.FlagResult{
		Name: "colour", Enabled: true, Value: "red", Reason: "TARGETING_MATCH; segment=red_colour",
	}, result.Flags["colour"])
	assert.Equal(t, []engine_eval.SegmentResult{{Name: "red_colour"}, {Name: "blue_colour"}}, result.Segments)
}

func TestEngineEvaluationContextRejectsMalformedSegments(t *testing.T) {
	t.Parallel()

	for name, data := range map[string]string{
		"segments not an object": `{"environment": {"key": "key", "name": "Environment"}, "segments": []}`,
		"segment not an object":  `{"environment": {"key": "key", "name": "Environment"}, "segments": {"1": 1}}`,
		"context not an object":  `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Given
			var ec engine_eval.EngineEvaluationContext

			// When
			err := json.Unmarshal([]byte(data), &ec)

			// Then
			assert.Error(t, err)
		})
	}
}
