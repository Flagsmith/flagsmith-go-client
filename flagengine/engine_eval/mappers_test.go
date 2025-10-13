package engine_eval

import (
	"math"
	"testing"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/projects"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
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
		if testFeature.Value == nil {
			t.Error("Expected Value to be set")
		} else if valueStr, ok := testFeature.Value.(string); !ok || valueStr != "test-value" {
			t.Errorf("Expected Value to be 'test-value', got %v", testFeature.Value)
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
			if override.Value == nil {
				t.Error("Expected override Value to be set")
			} else if valueStr, ok := override.Value.(string); !ok || valueStr != "segment-value" {
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

func TestMapEnvironmentDocumentToEvaluationContextWithIdentityOverrides(t *testing.T) {
	env := &environments.EnvironmentModel{
		ID:     1,
		APIKey: "test-api-key",
		Project: &projects.ProjectModel{
			ID:       1,
			Name:     "Test Project",
			Segments: []*segments.SegmentModel{},
		},
		FeatureStates: []*features.FeatureStateModel{},
		IdentityOverrides: []*identities.IdentityModel{
			{
				Identifier:        "user1",
				EnvironmentAPIKey: "test-api-key",
				CreatedDate:       utils.ISOTime{Time: time.Now()},
				IdentityUUID:      "uuid-1",
				IdentityFeatures: []*features.FeatureStateModel{
					{
						Enabled: true,
						Feature: &features.FeatureModel{
							ID:   1,
							Name: "feature_1",
						},
						RawValue: "override_value_1",
					},
					{
						Enabled: false,
						Feature: &features.FeatureModel{
							ID:   2,
							Name: "feature_2",
						},
						RawValue: "override_value_2",
					},
				},
			},
			{
				Identifier:        "user2",
				EnvironmentAPIKey: "test-api-key",
				CreatedDate:       utils.ISOTime{Time: time.Now()},
				IdentityUUID:      "uuid-2",
				IdentityFeatures: []*features.FeatureStateModel{
					{
						Enabled: true,
						Feature: &features.FeatureModel{
							ID:   1,
							Name: "feature_1",
						},
						RawValue: "override_value_1",
					},
					{
						Enabled: false,
						Feature: &features.FeatureModel{
							ID:   2,
							Name: "feature_2",
						},
						RawValue: "override_value_2",
					},
				},
			},
			{
				Identifier:        "user3",
				EnvironmentAPIKey: "test-api-key",
				CreatedDate:       utils.ISOTime{Time: time.Now()},
				IdentityUUID:      "uuid-3",
				IdentityFeatures: []*features.FeatureStateModel{
					{
						Enabled: false,
						Feature: &features.FeatureModel{
							ID:   1,
							Name: "feature_1",
						},
						RawValue: "different_value",
					},
				},
			},
		},
		UpdatedAt: time.Now(),
	}

	result := MapEnvironmentDocumentToEvaluationContext(env)

	// Should have created segments from identity overrides
	if len(result.Segments) != 2 {
		t.Errorf("Expected 2 segments (one for user1+user2 with same overrides, one for user3), got %d", len(result.Segments))
	}

	// Check that segments have the correct structure
	foundIdentitySegments := 0
	for _, segment := range result.Segments {
		if segment.Name == "identity_overrides" {
			foundIdentitySegments++

			// Should have one rule of type All
			if len(segment.Rules) != 1 {
				t.Errorf("Expected 1 rule in identity override segment, got %d", len(segment.Rules))
			} else {
				rule := segment.Rules[0]
				if rule.Type != All {
					t.Errorf("Expected rule type to be All, got %v", rule.Type)
				}

				// Should have one condition for identity identifier
				if len(rule.Conditions) != 1 {
					t.Errorf("Expected 1 condition in rule, got %d", len(rule.Conditions))
				} else {
					condition := rule.Conditions[0]
					if condition.Operator != "IN" {
						t.Errorf("Expected condition operator to be 'IN', got %v", condition.Operator)
					}
					if condition.Property != "$.identity.identifier" {
						t.Errorf("Expected condition property to be '$.identity.identifier', got %v", condition.Property)
					}
					if condition.Value == nil || condition.Value.String == nil {
						t.Error("Expected condition value to have String")
					}
				}
			}

			// Should have feature overrides
			if len(segment.Overrides) == 0 {
				t.Error("Expected identity override segment to have feature overrides")
			}

			// Check override priorities are set to negative infinity
			for _, override := range segment.Overrides {
				if override.Priority == nil {
					t.Error("Expected feature override to have priority set")
				} else if *override.Priority != math.Inf(-1) {
					t.Errorf("Expected priority to be negative infinity, got %v", *override.Priority)
				}
			}
		}
	}

	if foundIdentitySegments != 2 {
		t.Errorf("Expected to find 2 identity override segments, found %d", foundIdentitySegments)
	}
}

func TestMapContextAndIdentityDataToContext(t *testing.T) {
	// Create a base context
	baseContext := EngineEvaluationContext{
		Environment: EnvironmentContext{
			Key:  "test-env-key",
			Name: "Test Environment",
		},
		Features: map[string]FeatureContext{
			"test-feature": {
				Enabled:    true,
				FeatureKey: "1",
				Name:       "test-feature",
			},
		},
	}

	// Test with different trait value types
	traitList := []*Trait{
		{TraitKey: "string_trait", TraitValue: "string_value"},
		{TraitKey: "int_trait", TraitValue: 42},
		{TraitKey: "float_trait", TraitValue: 3.14},
		{TraitKey: "bool_true_trait", TraitValue: true},
		{TraitKey: "bool_false_trait", TraitValue: false},
		{TraitKey: "string_number_trait", TraitValue: "99"},
		{TraitKey: "string_bool_trait", TraitValue: "true"},
		{TraitKey: "empty_trait", TraitValue: ""},
	}

	result := MapContextAndIdentityDataToContext(baseContext, "test-user", traitList)

	// Check that the original context is preserved
	if result.Environment.Key != "test-env-key" {
		t.Errorf("Expected environment key to be preserved, got %v", result.Environment.Key)
	}
	if result.Environment.Name != "Test Environment" {
		t.Errorf("Expected environment name to be preserved, got %v", result.Environment.Name)
	}
	if len(result.Features) != 1 {
		t.Errorf("Expected features to be preserved, got %d features", len(result.Features))
	}

	// Check identity context
	if result.Identity == nil {
		t.Fatal("Expected identity to be set")
	}

	identity := result.Identity
	if identity.Identifier != "test-user" {
		t.Errorf("Expected identifier to be 'test-user', got %v", identity.Identifier)
	}
	if identity.Key != "test-env-key_test-user" {
		t.Errorf("Expected key to be 'test-env-key_test-user', got %v", identity.Key)
	}

	// Check traits
	if identity.Traits == nil {
		t.Fatal("Expected traits to be set")
	}

	// Test string trait
	if stringTrait, exists := identity.Traits["string_trait"]; !exists {
		t.Error("Expected string_trait to exist")
	} else if stringTrait != "string_value" {
		t.Errorf("Expected string_trait to be 'string_value', got %v", stringTrait)
	}

	// Test int trait
	if intTrait, exists := identity.Traits["int_trait"]; !exists {
		t.Error("Expected int_trait to exist")
	} else if intTrait != 42 {
		t.Errorf("Expected int_trait to be 42, got %v", intTrait)
	}

	// Test float trait (float64 3.14)
	if floatTrait, exists := identity.Traits["float_trait"]; !exists {
		t.Error("Expected float_trait to exist")
	} else if floatTrait != 3.14 {
		t.Errorf("Expected float_trait to be 3.14, got %v", floatTrait)
	}

	// Test bool true trait (bool true)
	if boolTrueTrait, exists := identity.Traits["bool_true_trait"]; !exists {
		t.Error("Expected bool_true_trait to exist")
	} else if boolTrueTrait != true {
		t.Errorf("Expected bool_true_trait to be true, got %v", boolTrueTrait)
	}

	// Test bool false trait (bool false)
	if boolFalseTrait, exists := identity.Traits["bool_false_trait"]; !exists {
		t.Error("Expected bool_false_trait to exist")
	} else if boolFalseTrait != false {
		t.Errorf("Expected bool_false_trait to be false, got %v", boolFalseTrait)
	}

	// Test string number trait (string "99" parsed as float64)
	if stringNumberTrait, exists := identity.Traits["string_number_trait"]; !exists {
		t.Error("Expected string_number_trait to exist")
	} else if stringNumberTrait != "99" {
		t.Errorf("Expected string_number_trait to be 99.0, got %v", stringNumberTrait)
	}

	// Test string bool trait (string "true" parsed as bool)
	if stringBoolTrait, exists := identity.Traits["string_bool_trait"]; !exists {
		t.Error("Expected string_bool_trait to exist")
	} else if stringBoolTrait != "true" {
		t.Errorf("Expected string_bool_trait to be true, got %v", stringBoolTrait)
	}

	// Test empty trait (should be included as empty string is a valid value)
	if emptyTrait, exists := identity.Traits["empty_trait"]; !exists {
		t.Error("Expected empty_trait to be included")
	} else if emptyStr, ok := emptyTrait.(string); !ok || emptyStr != "" {
		t.Errorf("Expected empty_trait to be empty string, got %v", emptyTrait)
	}
}

func TestMapContextAndIdentityDataToContextWithNilTraits(t *testing.T) {
	baseContext := EngineEvaluationContext{
		Environment: EnvironmentContext{
			Key:  "test-env-key",
			Name: "Test Environment",
		},
	}

	result := MapContextAndIdentityDataToContext(baseContext, "test-user", nil)

	// Check identity context
	if result.Identity == nil {
		t.Fatal("Expected identity to be set")
	}

	identity := result.Identity
	if identity.Identifier != "test-user" {
		t.Errorf("Expected identifier to be 'test-user', got %v", identity.Identifier)
	}
	if identity.Key != "test-env-key_test-user" {
		t.Errorf("Expected key to be 'test-env-key_test-user', got %v", identity.Key)
	}

	// Should have empty traits map when nil traits passed
	if len(identity.Traits) != 0 {
		t.Errorf("Expected empty traits map, got %d traits", len(identity.Traits))
	}
}

func TestMapEvaluationResultSegmentsToSegmentModels(t *testing.T) {
	// Create a test evaluation result with segments
	result := EvaluationResult{
		Segments: []SegmentResult{
			{
				Key:  "1",
				Name: "test-segment",
				Metadata: &SegmentMetadata{
					SegmentID: 1,
					Source:    SegmentSourceAPI,
				},
			},
			{
				Key:  "42",
				Name: "another-segment",
				Metadata: &SegmentMetadata{
					SegmentID: 42,
					Source:    SegmentSourceAPI,
				},
			},
			{
				Key:  "",
				Name: "identity-override-segment",
				Metadata: &SegmentMetadata{
					SegmentID: 0,
					Source:    SegmentSourceIdentityOverride,
				},
			},
		},
	}

	// Test the mapper
	segmentModels := MapEvaluationResultSegmentsToSegmentModels(&result)

	// Assertions - should only include API segments (2), not identity overrides
	if len(segmentModels) != 2 {
		t.Errorf("Expected 2 segment models, got %d", len(segmentModels))
	}

	// First segment
	segment1 := segmentModels[0]
	if segment1.ID != 1 {
		t.Errorf("Expected segment ID to be 1, got %d", segment1.ID)
	}

	if segment1.Name != "test-segment" {
		t.Errorf("Expected segment name to be 'test-segment', got %s", segment1.Name)
	}

	// Rules and FeatureStates should be nil/empty since we only populate ID and Name
	if segment1.Rules != nil {
		t.Errorf("Expected Rules to be nil, got %v", segment1.Rules)
	}

	if segment1.FeatureStates != nil {
		t.Errorf("Expected FeatureStates to be nil, got %v", segment1.FeatureStates)
	}

	// Second segment
	segment2 := segmentModels[1]
	if segment2.ID != 42 {
		t.Errorf("Expected segment ID to be 42, got %d", segment2.ID)
	}

	if segment2.Name != "another-segment" {
		t.Errorf("Expected segment name to be 'another-segment', got %s", segment2.Name)
	}
}

func TestMapEvaluationResultSegmentsToSegmentModelsEmpty(t *testing.T) {
	// Test with empty segments
	result := EvaluationResult{
		Segments: []SegmentResult{},
	}

	segmentModels := MapEvaluationResultSegmentsToSegmentModels(&result)

	if segmentModels != nil {
		t.Errorf("Expected nil for empty segments, got %v", segmentModels)
	}
}

func TestMapEvaluationResultSegmentsToSegmentModelsInvalidKey(t *testing.T) {
	// Test with segment result that has no metadata (should be filtered out)
	result := EvaluationResult{
		Segments: []SegmentResult{
			{
				Key:  "invalid-key",
				Name: "segment-without-metadata",
			},
		},
	}

	segmentModels := MapEvaluationResultSegmentsToSegmentModels(&result)

	// Segments without metadata should be filtered out
	if len(segmentModels) != 0 {
		t.Errorf("Expected 0 segment models (no metadata), got %d", len(segmentModels))
	}
}
