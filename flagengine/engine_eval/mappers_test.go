package engine_eval

import (
	"testing"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/projects"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
)

func TestMapEnvironmentDocumentToEvaluationContext(t *testing.T) {
	// Create test data
	env := &environments.EnvironmentModel{
		ID:     1,
		APIKey: "test-api-key",
		Project: &projects.ProjectModel{
			ID:   1,
			Name: "Test Project",
			Segments: []*segments.SegmentModel{
				{
					ID:   1,
					Name: "test-segment",
					Rules: []*segments.SegmentRuleModel{
						{
							Type: segments.All,
							Conditions: []*segments.SegmentConditionModel{
								{
									Operator: "EQUAL",
									Property: "test_property",
									Value:    "test_value",
								},
							},
						},
					},
					FeatureStates: []*features.FeatureStateModel{
						{
							Enabled: true,
							Feature: &features.FeatureModel{
								ID:   1,
								Name: "segment-override-feature",
							},
							RawValue: "segment-value",
						},
					},
				},
			},
		},
		FeatureStates: []*features.FeatureStateModel{
			{
				Enabled: true,
				Feature: &features.FeatureModel{
					ID:   1,
					Name: "test-feature",
				},
				RawValue:         "test-value",
				FeatureStateUUID: "test-uuid",
				DjangoID:         123,
			},
			{
				Enabled: false,
				Feature: &features.FeatureModel{
					ID:   2,
					Name: "disabled-feature",
				},
				RawValue: nil,
			},
		},
		UpdatedAt: time.Now(),
	}

	// Test the mapping function
	result := MapEnvironmentDocumentToEvaluationContext(env)

	// Test Environment mapping
	if result.Environment.Key != "test-api-key" {
		t.Errorf("Expected Environment.Key to be 'test-api-key', got %v", result.Environment.Key)
	}
	if result.Environment.Name != "Test Project" {
		t.Errorf("Expected Environment.Name to be 'Test Project', got %v", result.Environment.Name)
	}

	// Test Features mapping
	if len(result.Features) != 2 {
		t.Errorf("Expected 2 features, got %d", len(result.Features))
	}

	// Test first feature
	testFeature, exists := result.Features["test-feature"]
	if !exists {
		t.Error("Expected 'test-feature' to exist in Features map")
	} else {
		if !testFeature.Enabled {
			t.Error("Expected test-feature to be enabled")
		}
		if testFeature.FeatureKey != "1" {
			t.Errorf("Expected FeatureKey to be '1' (feature ID), got %v", testFeature.FeatureKey)
		}
		if testFeature.Name != "test-feature" {
			t.Errorf("Expected Name to be 'test-feature', got %v", testFeature.Name)
		}
		if testFeature.Key != "123" {
			t.Errorf("Expected Key to be '123' (from DjangoID), got %v", testFeature.Key)
		}
		if testFeature.Value == nil || testFeature.Value.String == nil || *testFeature.Value.String != "test-value" {
			t.Errorf("Expected Value.String to be 'test-value', got %v", testFeature.Value)
		}
	}

	// Test second feature (disabled with nil value)
	disabledFeature, exists := result.Features["disabled-feature"]
	if !exists {
		t.Error("Expected 'disabled-feature' to exist in Features map")
	} else {
		if disabledFeature.Enabled {
			t.Error("Expected disabled-feature to be disabled")
		}
		if disabledFeature.Value != nil {
			t.Errorf("Expected Value to be nil for disabled feature, got %v", disabledFeature.Value)
		}
	}

	// Test Segments mapping
	if len(result.Segments) != 1 {
		t.Errorf("Expected 1 segment, got %d", len(result.Segments))
	}

	testSegment, exists := result.Segments["1"]
	if !exists {
		t.Error("Expected segment with key '1' to exist in Segments map")
	} else {
		if testSegment.Name != "test-segment" {
			t.Errorf("Expected segment name to be 'test-segment', got %v", testSegment.Name)
		}
		if testSegment.Key != "1" {
			t.Errorf("Expected segment key to be '1', got %v", testSegment.Key)
		}

		// Test segment rules
		if len(testSegment.Rules) != 1 {
			t.Errorf("Expected 1 rule in segment, got %d", len(testSegment.Rules))
		} else {
			rule := testSegment.Rules[0]
			if rule.Type != All {
				t.Errorf("Expected rule type to be All, got %v", rule.Type)
			}
			if len(rule.Conditions) != 1 {
				t.Errorf("Expected 1 condition in rule, got %d", len(rule.Conditions))
			} else {
				condition := rule.Conditions[0]
				if condition.Operator != "EQUAL" {
					t.Errorf("Expected condition operator to be 'EQUAL', got %v", condition.Operator)
				}
				if condition.Property != "test_property" {
					t.Errorf("Expected condition property to be 'test_property', got %v", condition.Property)
				}
				if condition.Value == nil || condition.Value.String == nil || *condition.Value.String != "test_value" {
					t.Errorf("Expected condition value to be 'test_value', got %v", condition.Value)
				}
			}
		}

		// Test segment overrides
		if len(testSegment.Overrides) != 1 {
			t.Errorf("Expected 1 override in segment, got %d", len(testSegment.Overrides))
		} else {
			override := testSegment.Overrides[0]
			if override.FeatureKey != "1" {
				t.Errorf("Expected override feature key to be '1' (feature ID), got %v", override.FeatureKey)
			}
			if !override.Enabled {
				t.Error("Expected segment override to be enabled")
			}
			if override.Value == nil || override.Value.String == nil || *override.Value.String != "segment-value" {
				t.Errorf("Expected override value to be 'segment-value', got %v", override.Value)
			}
		}
	}
}

func TestMapEnvironmentDocumentToEvaluationContextWithNilProject(t *testing.T) {
	env := &environments.EnvironmentModel{
		ID:            1,
		APIKey:        "test-api-key",
		Project:       nil,
		FeatureStates: []*features.FeatureStateModel{},
		UpdatedAt:     time.Now(),
	}

	result := MapEnvironmentDocumentToEvaluationContext(env)

	// When project is nil, name should default to APIKey
	if result.Environment.Name != "test-api-key" {
		t.Errorf("Expected Environment.Name to default to APIKey 'test-api-key', got %v", result.Environment.Name)
	}

	// Should have no segments when project is nil
	if len(result.Segments) != 0 {
		t.Errorf("Expected 0 segments when project is nil, got %d", len(result.Segments))
	}
}

func TestMapEnvironmentDocumentToEvaluationContextWithEmptyFeatureStates(t *testing.T) {
	env := &environments.EnvironmentModel{
		ID:     1,
		APIKey: "test-api-key",
		Project: &projects.ProjectModel{
			ID:       1,
			Name:     "Test Project",
			Segments: []*segments.SegmentModel{},
		},
		FeatureStates: []*features.FeatureStateModel{},
		UpdatedAt:     time.Now(),
	}

	result := MapEnvironmentDocumentToEvaluationContext(env)

	// Should have no features when FeatureStates is empty
	if len(result.Features) != 0 {
		t.Errorf("Expected 0 features when FeatureStates is empty, got %d", len(result.Features))
	}

	// Should have no segments when project segments is empty
	if len(result.Segments) != 0 {
		t.Errorf("Expected 0 segments when project segments is empty, got %d", len(result.Segments))
	}
}
