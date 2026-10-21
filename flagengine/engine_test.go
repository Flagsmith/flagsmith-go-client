package flagengine_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
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
