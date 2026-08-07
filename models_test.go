package flagsmith

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
)

func TestMakeFlagsFromAPIFlagsSetsReasonAndVariant(t *testing.T) {
	// Given
	flagsJson := []byte(`[{
		"enabled": true,
		"feature_state_value": "test-value",
		"feature": {"id": 123, "name": "test_feature"},
		"reason": "TARGETING_MATCH; segment=premium",
		"variant": "treatment"
	}]`)

	// When
	flags, err := makeFlagsFromAPIFlags(flagsJson, nil, nil)

	// Then
	require.NoError(t, err)
	require.Len(t, flags.flags, 1)
	assert.Equal(t, "TARGETING_MATCH; segment=premium", flags.flags[0].Reason)
	assert.Equal(t, "treatment", flags.flags[0].Variant)
}

func TestMakeFlagsFromAPIFlagsNoReasonOrVariantIsEmpty(t *testing.T) {
	// Given: a response from an API that predates the reason and variant fields
	flagsJson := []byte(`[{
		"enabled": true,
		"feature_state_value": "test-value",
		"feature": {"id": 123, "name": "test_feature"}
	}]`)

	// When
	flags, err := makeFlagsFromAPIFlags(flagsJson, nil, nil)

	// Then
	require.NoError(t, err)
	require.Len(t, flags.flags, 1)
	assert.Empty(t, flags.flags[0].Reason)
	assert.Empty(t, flags.flags[0].Variant)
}

func TestMakeFlagsFromAPIFlagsNullVariantIsEmpty(t *testing.T) {
	// Given: a standard feature, for which the API reports a null variant
	flagsJson := []byte(`[{
		"enabled": true,
		"feature_state_value": "test-value",
		"feature": {"id": 123, "name": "test_feature"},
		"reason": "DEFAULT",
		"variant": null
	}]`)

	// When
	flags, err := makeFlagsFromAPIFlags(flagsJson, nil, nil)

	// Then
	require.NoError(t, err)
	require.Len(t, flags.flags, 1)
	assert.Empty(t, flags.flags[0].Variant)
}

func TestMakeFlagFromEngineEvaluationFlagResult(t *testing.T) {
	tests := []struct {
		name     string
		input    *engine_eval.FlagResult
		expected Flag
	}{
		{
			name: "flag with string value",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "test_feature",
				Value:   "test_value",
			},
			expected: Flag{
				Enabled:     true,
				Value:       "test_value",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "test_feature",
			},
		},
		{
			name: "flag with boolean value",
			input: &engine_eval.FlagResult{
				Enabled: false,
				Name:    "bool_feature",
				Value:   true,
			},
			expected: Flag{
				Enabled:     false,
				Value:       true,
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "bool_feature",
			},
		},
		{
			name: "flag with double value",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "double_feature",
				Value:   42.5,
			},
			expected: Flag{
				Enabled:     true,
				Value:       42.5,
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "double_feature",
			},
		},
		{
			name: "flag with nil value",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "nil_feature",
				Value:   nil,
			},
			expected: Flag{
				Enabled:     true,
				Value:       nil,
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "nil_feature",
			},
		},
		{
			name: "flag with zero values",
			input: &engine_eval.FlagResult{
				Enabled: false,
				Name:    "",
				Value:   "",
			},
			expected: Flag{
				Enabled:     false,
				Value:       "",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "",
			},
		},
		{
			name: "flag with reason field",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "reason_feature",
				Reason:  "TARGETING_MATCH; segment=premium_segment",
				Value:   "reason_value",
			},
			expected: Flag{
				Enabled:     true,
				Value:       "reason_value",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "reason_feature",
				Reason:      "TARGETING_MATCH; segment=premium_segment",
			},
		},
		{
			name: "multivariate flag with selected variant",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "mv_feature",
				Reason:  "SPLIT; weight=30",
				Value:   "variant_value",
				Variant: "treatment",
			},
			expected: Flag{
				Enabled:     true,
				Value:       "variant_value",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "mv_feature",
				Reason:      "SPLIT; weight=30",
				Variant:     "treatment",
			},
		},
		{
			name: "multivariate flag in the control bucket",
			input: &engine_eval.FlagResult{
				Enabled: true,
				Name:    "mv_feature",
				Reason:  "DEFAULT",
				Value:   "control_value",
				Variant: "control",
			},
			expected: Flag{
				Enabled:     true,
				Value:       "control_value",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "mv_feature",
				Reason:      "DEFAULT",
				Variant:     "control",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeFlagFromEngineEvaluationFlagResult(tt.input)

			if result.Enabled != tt.expected.Enabled {
				t.Errorf("Expected Enabled %v, got %v", tt.expected.Enabled, result.Enabled)
			}
			if result.Value != tt.expected.Value {
				t.Errorf("Expected Value %v, got %v", tt.expected.Value, result.Value)
			}
			if result.IsDefault != tt.expected.IsDefault {
				t.Errorf("Expected IsDefault %v, got %v", tt.expected.IsDefault, result.IsDefault)
			}
			if result.FeatureID != tt.expected.FeatureID {
				t.Errorf("Expected FeatureID %v, got %v", tt.expected.FeatureID, result.FeatureID)
			}
			if result.FeatureName != tt.expected.FeatureName {
				t.Errorf("Expected FeatureName %v, got %v", tt.expected.FeatureName, result.FeatureName)
			}
			if result.Reason != tt.expected.Reason {
				t.Errorf("Expected Reason %v, got %v", tt.expected.Reason, result.Reason)
			}
			if result.Variant != tt.expected.Variant {
				t.Errorf("Expected Variant %v, got %v", tt.expected.Variant, result.Variant)
			}
		})
	}
}

func TestMakeFlagsFromEngineEvaluationResult(t *testing.T) {
	tests := []struct {
		name     string
		input    *engine_eval.EvaluationResult
		expected []Flag
	}{
		{
			name: "evaluation result with multiple flags",
			input: &engine_eval.EvaluationResult{
				Flags: map[string]*engine_eval.FlagResult{
					"feature1": {
						Enabled: true,
						Name:    "feature1",
						Value:   "value1",
						Reason:  "DEFAULT",
					},
					"feature2": {
						Enabled: false,
						Name:    "feature2",
						Value:   true,
					},
					"feature3": {
						Enabled: true,
						Name:    "feature3",
						Value:   123.45,
					},
				},
				Segments: []engine_eval.SegmentResult{},
			},
			expected: []Flag{
				{
					Enabled:     true,
					Value:       "value1",
					IsDefault:   false,
					FeatureID:   0,
					FeatureName: "feature1",
					Reason:      "DEFAULT",
				},
				{
					Enabled:     false,
					Value:       true,
					IsDefault:   false,
					FeatureID:   0,
					FeatureName: "feature2",
				},
				{
					Enabled:     true,
					Value:       123.45,
					IsDefault:   false,
					FeatureID:   0,
					FeatureName: "feature3",
				},
			},
		},
		{
			name: "evaluation result with no flags",
			input: &engine_eval.EvaluationResult{
				Flags:    map[string]*engine_eval.FlagResult{},
				Segments: []engine_eval.SegmentResult{},
			},
			expected: []Flag{},
		},
		{
			name: "evaluation result with single flag",
			input: &engine_eval.EvaluationResult{
				Flags: map[string]*engine_eval.FlagResult{
					"single_feature": {
						Enabled: true,
						Name:    "single_feature",
						Value:   "single_value",
					},
				},
				Segments: []engine_eval.SegmentResult{},
			},
			expected: []Flag{
				{
					Enabled:     true,
					Value:       "single_value",
					IsDefault:   false,
					FeatureID:   0,
					FeatureName: "single_feature",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeFlagsFromEngineEvaluationResult(tt.input, nil, nil)

			if len(result.flags) != len(tt.expected) {
				t.Errorf("Expected %d flags, got %d", len(tt.expected), len(result.flags))
				return
			}

			// Create a map of actual flags by feature name for order-independent comparison
			actualFlagsByName := make(map[string]Flag)
			for _, flag := range result.flags {
				actualFlagsByName[flag.FeatureName] = flag
			}

			// Compare each expected flag with the corresponding actual flag
			for _, expectedFlag := range tt.expected {
				actualFlag, exists := actualFlagsByName[expectedFlag.FeatureName]
				if !exists {
					t.Errorf("Expected flag %s not found in actual result", expectedFlag.FeatureName)
					continue
				}

				if actualFlag.Enabled != expectedFlag.Enabled {
					t.Errorf("Flag %s: Expected Enabled %v, got %v", expectedFlag.FeatureName, expectedFlag.Enabled, actualFlag.Enabled)
				}
				if actualFlag.Value != expectedFlag.Value {
					t.Errorf("Flag %s: Expected Value %v, got %v", expectedFlag.FeatureName, expectedFlag.Value, actualFlag.Value)
				}
				if actualFlag.IsDefault != expectedFlag.IsDefault {
					t.Errorf("Flag %s: Expected IsDefault %v, got %v", expectedFlag.FeatureName, expectedFlag.IsDefault, actualFlag.IsDefault)
				}
				if actualFlag.FeatureID != expectedFlag.FeatureID {
					t.Errorf("Flag %s: Expected FeatureID %v, got %v", expectedFlag.FeatureName, expectedFlag.FeatureID, actualFlag.FeatureID)
				}
				if actualFlag.FeatureName != expectedFlag.FeatureName {
					t.Errorf("Flag %s: Expected FeatureName %v, got %v", expectedFlag.FeatureName, expectedFlag.FeatureName, actualFlag.FeatureName)
				}
				if actualFlag.Reason != expectedFlag.Reason {
					t.Errorf("Flag %s: Expected Reason %v, got %v", expectedFlag.FeatureName, expectedFlag.Reason, actualFlag.Reason)
				}
			}

			// Test that analytics processor and default flag handler are set correctly
			if result.analyticsProcessor != nil {
				t.Errorf("Expected analyticsProcessor to be nil, got non-nil value")
			}
			if result.defaultFlagHandler != nil {
				t.Errorf("Expected defaultFlagHandler to be nil, got non-nil function")
			}
		})
	}
}

func TestMakeFlagsFromEngineEvaluationResultWithProcessorAndHandler(t *testing.T) {
	// Mock analytics processor
	mockAnalyticsProcessor := &AnalyticsProcessor{}

	// Mock default flag handler
	mockDefaultFlagHandler := func(featureName string) (Flag, error) {
		return Flag{
			Enabled:     false,
			Value:       "default",
			IsDefault:   true,
			FeatureID:   -1,
			FeatureName: featureName,
		}, nil
	}

	input := &engine_eval.EvaluationResult{
		Flags: map[string]*engine_eval.FlagResult{
			"test_feature": {
				Enabled: true,
				Name:    "test_feature",
				Value:   "test_value",
			},
		},
		Segments: []engine_eval.SegmentResult{},
	}

	result := makeFlagsFromEngineEvaluationResult(input, mockAnalyticsProcessor, mockDefaultFlagHandler)

	// Test that analytics processor and default flag handler are set correctly
	if result.analyticsProcessor != mockAnalyticsProcessor {
		t.Errorf("Expected analyticsProcessor to be set correctly")
	}
	if result.defaultFlagHandler == nil {
		t.Errorf("Expected defaultFlagHandler to be set")
	}

	// Test that the handler works
	if result.defaultFlagHandler != nil {
		flag, err := result.defaultFlagHandler("test")
		if err != nil {
			t.Errorf("Unexpected error from defaultFlagHandler: %v", err)
		}
		if flag.FeatureName != "test" {
			t.Errorf("Expected handler to return flag with name 'test', got %v", flag.FeatureName)
		}
		if !flag.IsDefault {
			t.Errorf("Expected handler to return default flag")
		}
	}
}
