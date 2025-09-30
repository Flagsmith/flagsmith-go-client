package flagsmith

import (
	"testing"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/engine_eval"
)

func TestMakeFlagFromEngineEvaluationFlagResult(t *testing.T) {
	tests := []struct {
		name     string
		input    *engine_eval.FlagResult
		expected Flag
	}{
		{
			name: "flag with string value",
			input: &engine_eval.FlagResult{
				Enabled:    true,
				FeatureKey: "test_feature_key",
				Name:       "test_feature",
				Value: &engine_eval.Value{
					String: stringPtr("test_value"),
				},
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
				Enabled:    false,
				FeatureKey: "bool_feature_key",
				Name:       "bool_feature",
				Value: &engine_eval.Value{
					Bool: boolPtr(true),
				},
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
				Enabled:    true,
				FeatureKey: "double_feature_key",
				Name:       "double_feature",
				Value: &engine_eval.Value{
					Double: float64Ptr(42.5),
				},
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
				Enabled:    true,
				FeatureKey: "nil_feature_key",
				Name:       "nil_feature",
				Value:      nil,
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
			name: "flag with empty value struct",
			input: &engine_eval.FlagResult{
				Enabled:    false,
				FeatureKey: "empty_feature_key",
				Name:       "empty_feature",
				Value:      &engine_eval.Value{},
			},
			expected: Flag{
				Enabled:     false,
				Value:       nil,
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "empty_feature",
			},
		},
		{
			name: "flag with zero values",
			input: &engine_eval.FlagResult{
				Enabled:    false,
				FeatureKey: "",
				Name:       "",
				Value: &engine_eval.Value{
					String: stringPtr(""),
				},
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
			name: "flag with reason field (should be ignored in conversion)",
			input: &engine_eval.FlagResult{
				Enabled:    true,
				FeatureKey: "reason_feature_key",
				Name:       "reason_feature",
				Reason:     stringPtr("TARGETING_MATCH"),
				Value: &engine_eval.Value{
					String: stringPtr("reason_value"),
				},
			},
			expected: Flag{
				Enabled:     true,
				Value:       "reason_value",
				IsDefault:   false,
				FeatureID:   0,
				FeatureName: "reason_feature",
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
				Context: engine_eval.EngineEvaluationContext{},
				Flags: []engine_eval.FlagResult{
					{
						Enabled:    true,
						FeatureKey: "feature1_key",
						Name:       "feature1",
						Value: &engine_eval.Value{
							String: stringPtr("value1"),
						},
					},
					{
						Enabled:    false,
						FeatureKey: "feature2_key",
						Name:       "feature2",
						Value: &engine_eval.Value{
							Bool: boolPtr(true),
						},
					},
					{
						Enabled:    true,
						FeatureKey: "feature3_key",
						Name:       "feature3",
						Value: &engine_eval.Value{
							Double: float64Ptr(123.45),
						},
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
				Context:  engine_eval.EngineEvaluationContext{},
				Flags:    []engine_eval.FlagResult{},
				Segments: []engine_eval.SegmentResult{},
			},
			expected: []Flag{},
		},
		{
			name: "evaluation result with single flag",
			input: &engine_eval.EvaluationResult{
				Context: engine_eval.EngineEvaluationContext{},
				Flags: []engine_eval.FlagResult{
					{
						Enabled:    true,
						FeatureKey: "single_feature_key",
						Name:       "single_feature",
						Value: &engine_eval.Value{
							String: stringPtr("single_value"),
						},
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

			for i, expectedFlag := range tt.expected {
				actualFlag := result.flags[i]

				if actualFlag.Enabled != expectedFlag.Enabled {
					t.Errorf("Flag %d: Expected Enabled %v, got %v", i, expectedFlag.Enabled, actualFlag.Enabled)
				}
				if actualFlag.Value != expectedFlag.Value {
					t.Errorf("Flag %d: Expected Value %v, got %v", i, expectedFlag.Value, actualFlag.Value)
				}
				if actualFlag.IsDefault != expectedFlag.IsDefault {
					t.Errorf("Flag %d: Expected IsDefault %v, got %v", i, expectedFlag.IsDefault, actualFlag.IsDefault)
				}
				if actualFlag.FeatureID != expectedFlag.FeatureID {
					t.Errorf("Flag %d: Expected FeatureID %v, got %v", i, expectedFlag.FeatureID, actualFlag.FeatureID)
				}
				if actualFlag.FeatureName != expectedFlag.FeatureName {
					t.Errorf("Flag %d: Expected FeatureName %v, got %v", i, expectedFlag.FeatureName, actualFlag.FeatureName)
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
		Context: engine_eval.EngineEvaluationContext{},
		Flags: []engine_eval.FlagResult{
			{
				Enabled:    true,
				FeatureKey: "test_feature_key",
				Name:       "test_feature",
				Value: &engine_eval.Value{
					String: stringPtr("test_value"),
				},
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

// Helper functions for creating pointers.
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func float64Ptr(f float64) *float64 {
	return &f
}
