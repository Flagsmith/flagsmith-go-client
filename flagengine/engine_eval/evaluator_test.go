package engine_eval_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

const (
	traitKey1   = "email"
	traitValue1 = "user@example.com"

	traitKey2   = "num_purchase"
	traitValue2 = "12"

	traitKey3   = "date_joined"
	traitValue3 = "2021-01-01"
)

// Helper function to create a string value.
func stringValue(s string) string {
	return s
}

func boolValue(b bool) bool {
	return b
}

func doubleValue(d float64) float64 {
	return d
}

// Helper function to create evaluation context with traits.
func createEvaluationContext(traits map[string]any) *engine_eval.EngineEvaluationContext {
	return &engine_eval.EngineEvaluationContext{
		Environment: engine_eval.EnvironmentContext{
			Key:  "test-env",
			Name: "Test Environment",
		},
		Identity: &engine_eval.IdentityContext{
			Identifier: "test-user",
			Key:        "test-env_test-user",
			Traits:     traits,
		},
	}
}

// Helper function to create segment context.
func createSegmentContext(key, name string, rules []engine_eval.SegmentRule) *engine_eval.SegmentContext {
	// Convert key to int for SegmentID, defaulting to 0 if invalid
	segmentID := 0
	if id, err := strconv.Atoi(key); err == nil {
		segmentID = id
	}

	return &engine_eval.SegmentContext{
		Key:  key,
		Name: name,
		Metadata: &engine_eval.SegmentMetadata{
			SegmentID: segmentID,
			Source:    engine_eval.SegmentSourceAPI,
		},
		Rules: rules,
	}
}

func TestIsContextInSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		segmentContext *engine_eval.SegmentContext
		evalContext    *engine_eval.EngineEvaluationContext
		expected       bool
	}{
		{
			name:           "empty segment rules returns false",
			segmentContext: createSegmentContext("1", "empty_segment", []engine_eval.SegmentRule{}),
			evalContext:    createEvaluationContext(nil),
			expected:       false,
		},
		{
			name: "single condition matches",
			segmentContext: createSegmentContext("2", "single_condition", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Equal,
							Property: traitKey1,
							Value:    traitValue1,
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue(traitValue1),
			}),
			expected: true,
		},
		{
			name: "single condition does not match",
			segmentContext: createSegmentContext("3", "single_condition_no_match", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Equal,
							Property: traitKey1,
							Value:    traitValue1,
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue("different@example.com"),
			}),
			expected: false,
		},
		{
			name: "multiple conditions ALL - all match",
			segmentContext: createSegmentContext("4", "multiple_conditions_all", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Equal,
							Property: traitKey1,
							Value:    traitValue1,
						},
						{
							Operator: engine_eval.Equal,
							Property: traitKey2,
							Value:    traitValue2,
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue(traitValue1),
				traitKey2: stringValue(traitValue2),
			}),
			expected: true,
		},
		{
			name: "multiple conditions ALL - one does not match",
			segmentContext: createSegmentContext("5", "multiple_conditions_all_fail", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Equal,
							Property: traitKey1,
							Value:    traitValue1,
						},
						{
							Operator: engine_eval.Equal,
							Property: traitKey2,
							Value:    traitValue2,
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue(traitValue1),
				traitKey2: stringValue("different_value"),
			}),
			expected: false,
		},
		{
			name: "multiple conditions ANY - one matches",
			segmentContext: createSegmentContext("6", "multiple_conditions_any", []engine_eval.SegmentRule{
				{
					Type: engine_eval.Any,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Equal,
							Property: traitKey1,
							Value:    traitValue1,
						},
						{
							Operator: engine_eval.Equal,
							Property: traitKey2,
							Value:    traitValue2,
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue(traitValue1),
				traitKey2: stringValue("different_value"),
			}),
			expected: true,
		},
		{
			name: "nested rules",
			segmentContext: createSegmentContext("7", "nested_rules", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Rules: []engine_eval.SegmentRule{
						{
							Type: engine_eval.All,
							Conditions: []engine_eval.Condition{
								{
									Operator: engine_eval.Equal,
									Property: traitKey1,
									Value:    traitValue1,
								},
								{
									Operator: engine_eval.Equal,
									Property: traitKey2,
									Value:    traitValue2,
								},
							},
						},
						{
							Type: engine_eval.All,
							Conditions: []engine_eval.Condition{
								{
									Operator: engine_eval.Equal,
									Property: traitKey3,
									Value:    traitValue3,
								},
							},
						},
					},
				},
			}),
			evalContext: createEvaluationContext(map[string]any{
				traitKey1: stringValue(traitValue1),
				traitKey2: stringValue(traitValue2),
				traitKey3: stringValue(traitValue3),
			}),
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := engine_eval.IsContextInSegment(c.evalContext, c.segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestContextMatchesCondition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		operator       engine_eval.Operator
		property       string
		conditionValue string
		traitValue     interface{}
		expected       bool
	}{
		// String comparisons
		{"equal strings match", engine_eval.Equal, traitKey1, "test", "test", true},
		{"equal strings don't match", engine_eval.Equal, traitKey1, "test", "different", false},
		{"not equal strings", engine_eval.NotEqual, traitKey1, "test", "different", true},
		{"not equal same strings", engine_eval.NotEqual, traitKey1, "test", "test", false},

		// Numeric comparisons
		{"greater than int", engine_eval.GreaterThan, traitKey2, "5", "10", true},
		{"greater than int false", engine_eval.GreaterThan, traitKey2, "10", "5", false},
		{"greater than equal", engine_eval.GreaterThan, traitKey2, "10", "10", false},
		{"greater than inclusive", engine_eval.GreaterThanInclusive, traitKey2, "10", "10", true},
		{"less than int", engine_eval.LessThan, traitKey2, "10", "5", true},
		{"less than int false", engine_eval.LessThan, traitKey2, "5", "10", false},
		{"less than inclusive", engine_eval.LessThanInclusive, traitKey2, "10", "10", true},

		// Float comparisons
		{"greater than float", engine_eval.GreaterThan, traitKey2, "5.5", "10.1", true},
		{"less than float", engine_eval.LessThan, traitKey2, "10.1", "5.5", true},

		// Boolean comparisons
		{"equal bool true", engine_eval.Equal, traitKey1, "true", "true", true},
		{"equal bool false", engine_eval.Equal, traitKey1, "false", "false", true},
		{"not equal bool", engine_eval.NotEqual, traitKey1, "true", "false", true},

		// String operations
		{"contains", engine_eval.Contains, traitKey1, "test", "testing", true},
		{"contains false", engine_eval.Contains, traitKey1, "xyz", "testing", false},
		{"not contains", engine_eval.NotContains, traitKey1, "xyz", "testing", true},
		{"not contains false", engine_eval.NotContains, traitKey1, "test", "testing", false},

		// IN operator
		{"in list first", engine_eval.In, traitKey1, "a,b,c", "a", true},
		{"in list middle", engine_eval.In, traitKey1, "a,b,c", "b", true},
		{"in list last", engine_eval.In, traitKey1, "a,b,c", "c", true},
		{"not in list", engine_eval.In, traitKey1, "a,b,c", "d", false},
		{"in single item", engine_eval.In, traitKey1, "test", "test", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition := &engine_eval.Condition{
				Operator: c.operator,
				Property: c.property,
				Value:    c.conditionValue,
			}

			var traitValue any
			switch v := c.traitValue.(type) {
			case string:
				traitValue = stringValue(v)
			case bool:
				traitValue = boolValue(v)
			case float64:
				traitValue = doubleValue(v)
			default:
				traitValue = stringValue(fmt.Sprint(v))
			}

			evalContext := createEvaluationContext(map[string]any{
				c.property: traitValue,
			})

			// We need to access the internal function, so we'll test via IsContextInSegment
			segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
				{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{*condition},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestContextMatchesConditionInOperatorStringArray(t *testing.T) {
	traitKey1 := "trait1"

	cases := []struct {
		name        string
		stringArray []string
		traitValue  string
		expected    bool
	}{
		{"in string array first", []string{"a", "b", "c"}, "a", true},
		{"in string array middle", []string{"a", "b", "c"}, "b", true},
		{"in string array last", []string{"a", "b", "c"}, "c", true},
		{"not in string array", []string{"a", "b", "c"}, "d", false},
		{"in single item array", []string{"test"}, "test", true},
		{"empty string array", []string{}, "test", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition := &engine_eval.Condition{
				Operator: engine_eval.In,
				Property: traitKey1,
				Value:    c.stringArray,
			}

			traitValuePtr := stringValue(c.traitValue)

			evalContext := createEvaluationContext(map[string]any{
				traitKey1: traitValuePtr,
			})

			// Test via IsContextInSegment
			segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
				{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{*condition},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestContextMatchesConditionIsSetAndIsNotSet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		operator       engine_eval.Operator
		property       string
		hasProperty    bool
		expectedResult bool
	}{
		{"IsSet with property", engine_eval.IsSet, "foo", true, true},
		{"IsSet without property", engine_eval.IsSet, "foo", false, false},
		{"IsNotSet with property", engine_eval.IsNotSet, "foo", true, false},
		{"IsNotSet without property", engine_eval.IsNotSet, "foo", false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition := &engine_eval.Condition{
				Operator: c.operator,
				Property: c.property,
			}

			var traits map[string]any
			if c.hasProperty {
				traits = map[string]any{
					c.property: stringValue("some_value"),
				}
			}

			evalContext := createEvaluationContext(traits)

			segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
				{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{*condition},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expectedResult, result)
		})
	}
}

func TestContextMatchesConditionPercentageSplit(t *testing.T) {
	cases := []struct {
		name                     string
		segmentSplitValue        string
		identityHashedPercentage float64
		expectedResult           bool
	}{
		{"10% split, 1% hash - should match", "10", 1.0, true},
		{"100% split, 50% hash - should match", "100", 50.0, true},
		{"0% split, 1% hash - should not match", "0", 1.0, false},
		{"10% split, 20% hash - should not match", "10", 20.0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition := &engine_eval.Condition{
				Operator: engine_eval.PercentageSplit,
				Property: "",
				Value:    c.segmentSplitValue,
			}

			evalContext := createEvaluationContext(nil)

			// Mock the hashing function
			utils.MockSetHashedPercentageForObjectIds(func(_ []string, _ int) float64 {
				return c.identityHashedPercentage
			})
			defer utils.ResetMocks()

			segmentContext := createSegmentContext("test-segment", "test", []engine_eval.SegmentRule{
				{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{*condition},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expectedResult, result)
		})
	}
}

func TestGetContextValueIntegration(t *testing.T) {
	t.Parallel()

	// Test getContextValue indirectly through IsContextInSegment
	// This tests that the function works correctly in the context it's used

	t.Run("simple trait lookup works", func(t *testing.T) {
		evalContext := createEvaluationContext(map[string]any{
			"email": stringValue("test@example.com"),
		})

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "email",
						Value:    "test@example.com",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)
	})

	t.Run("JSONPath identity identifier works", func(t *testing.T) {
		evalContext := createEvaluationContext(nil)

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "$.identity.identifier",
						Value:    "test-user",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)
	})
}

func TestToStringIntegration(t *testing.T) {
	t.Parallel()

	// Test ToString indirectly through IsContextInSegment
	// This tests that the function works correctly in the context it's used

	t.Run("string values work correctly", func(t *testing.T) {
		evalContext := createEvaluationContext(map[string]any{
			"test_prop": stringValue("test_string"),
		})

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "test_prop",
						Value:    "test_string",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)
	})

	t.Run("boolean values work correctly", func(t *testing.T) {
		evalContext := createEvaluationContext(map[string]any{
			"test_prop": boolValue(true),
		})

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "test_prop",
						Value:    "true",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)
	})

	t.Run("numeric values work correctly", func(t *testing.T) {
		evalContext := createEvaluationContext(map[string]any{
			"test_prop": doubleValue(123.45),
		})

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "test_prop",
						Value:    "123.45",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)
	})
}

func TestSemverComparisons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		operator       engine_eval.Operator
		traitValue     string
		conditionValue string
		expected       bool
	}{
		// Equal
		{"semver equal match", engine_eval.Equal, "1.2.3", "1.2.3:semver", true},
		{"semver equal no match", engine_eval.Equal, "1.2.4", "1.2.3:semver", false},
		{"semver equal invalid trait", engine_eval.Equal, "not_a_semver", "1.2.3:semver", false},

		// Not Equal
		{"semver not equal same", engine_eval.NotEqual, "1.0.0", "1.0.0:semver", false},
		{"semver not equal different", engine_eval.NotEqual, "1.0.1", "1.0.0:semver", true},

		// Greater Than
		{"semver greater than true", engine_eval.GreaterThan, "1.0.1", "1.0.0:semver", true},
		{"semver greater than false", engine_eval.GreaterThan, "1.0.1", "1.1.0:semver", false},
		{"semver greater than equal", engine_eval.GreaterThan, "1.0.1", "1.0.1:semver", false},
		{"semver greater than with prerelease", engine_eval.GreaterThan, "1.2.4", "1.2.3-pre.2+build.4:semver", true},

		// Less Than
		{"semver less than false", engine_eval.LessThan, "1.0.1", "1.0.0:semver", false},
		{"semver less than true", engine_eval.LessThan, "1.0.1", "1.1.0:semver", true},
		{"semver less than equal", engine_eval.LessThan, "1.0.1", "1.0.1:semver", false},

		// Greater Than Inclusive
		{"semver gte true", engine_eval.GreaterThanInclusive, "1.0.1", "1.0.0:semver", true},
		{"semver gte false", engine_eval.GreaterThanInclusive, "1.0.1", "1.2.0:semver", false},
		{"semver gte equal", engine_eval.GreaterThanInclusive, "1.0.1", "1.0.1:semver", true},

		// Less Than Inclusive
		{"semver lte true", engine_eval.LessThanInclusive, "1.0.0", "1.0.1:semver", true},
		{"semver lte equal", engine_eval.LessThanInclusive, "1.0.0", "1.0.0:semver", true},
		{"semver lte false", engine_eval.LessThanInclusive, "1.0.1", "1.0.0:semver", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition := &engine_eval.Condition{
				Operator: c.operator,
				Property: "version",
				Value:    c.conditionValue,
			}

			evalContext := createEvaluationContext(map[string]any{
				"version": stringValue(c.traitValue),
			})

			segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
				{
					Type:       engine_eval.All,
					Conditions: []engine_eval.Condition{*condition},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestComplexSegmentRules(t *testing.T) {
	t.Parallel()

	t.Run("conditions and nested rules", func(t *testing.T) {
		// Test a segment with both conditions and nested rules
		segmentContext := createSegmentContext("complex", "complex_segment", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: traitKey1,
						Value:    traitValue1,
					},
				},
				Rules: []engine_eval.SegmentRule{
					{
						Type: engine_eval.All,
						Conditions: []engine_eval.Condition{
							{
								Operator: engine_eval.Equal,
								Property: traitKey2,
								Value:    traitValue2,
							},
						},
					},
					{
						Type: engine_eval.All,
						Conditions: []engine_eval.Condition{
							{
								Operator: engine_eval.Equal,
								Property: traitKey3,
								Value:    traitValue3,
							},
						},
					},
				},
			},
		})

		// Should match when all conditions are met
		evalContext := createEvaluationContext(map[string]any{
			traitKey1: stringValue(traitValue1),
			traitKey2: stringValue(traitValue2),
			traitKey3: stringValue(traitValue3),
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)

		// Should not match when one condition fails
		evalContextPartial := createEvaluationContext(map[string]any{
			traitKey1: stringValue(traitValue1),
			traitKey2: stringValue(traitValue2),
			// Missing traitKey3
		})

		result = engine_eval.IsContextInSegment(evalContextPartial, segmentContext)
		assert.False(t, result)
	})

	t.Run("NONE rule type", func(t *testing.T) {
		segmentContext := createSegmentContext("none_rule", "none_segment", []engine_eval.SegmentRule{
			{
				Type: engine_eval.None,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: traitKey1,
						Value:    traitValue1,
					},
					{
						Operator: engine_eval.Equal,
						Property: traitKey2,
						Value:    traitValue2,
					},
				},
			},
		})

		// Should match when no conditions are met (NONE rule)
		evalContext := createEvaluationContext(map[string]any{
			traitKey1: stringValue("different1"),
			traitKey2: stringValue("different2"),
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result)

		// Should not match when any condition is met
		evalContextWithMatch := createEvaluationContext(map[string]any{
			traitKey1: stringValue(traitValue1), // This matches
			traitKey2: stringValue("different2"),
		})

		result = engine_eval.IsContextInSegment(evalContextWithMatch, segmentContext)
		assert.False(t, result)
	})
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("no identity context", func(t *testing.T) {
		evalContext := &engine_eval.EngineEvaluationContext{
			Environment: engine_eval.EnvironmentContext{
				Key:  "test-env",
				Name: "Test Environment",
			},
			Identity: nil, // No identity
		}

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.Equal,
						Property: "some_trait",
						Value:    "value",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.False(t, result)
	})

	t.Run("empty traits map", func(t *testing.T) {
		evalContext := &engine_eval.EngineEvaluationContext{
			Environment: engine_eval.EnvironmentContext{
				Key:  "test-env",
				Name: "Test Environment",
			},
			Identity: &engine_eval.IdentityContext{
				Identifier: "test-user",
				Key:        "test-env_test-user",
				Traits:     nil, // No traits
			},
		}

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.IsNotSet,
						Property: "missing_trait",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.True(t, result) // IsNotSet should return true for missing trait
	})

	t.Run("percentage split without identity", func(t *testing.T) {
		evalContext := &engine_eval.EngineEvaluationContext{
			Environment: engine_eval.EnvironmentContext{
				Key:  "test-env",
				Name: "Test Environment",
			},
			Identity: nil,
		}

		segmentContext := createSegmentContext("test", "test", []engine_eval.SegmentRule{
			{
				Type: engine_eval.All,
				Conditions: []engine_eval.Condition{
					{
						Operator: engine_eval.PercentageSplit,
						Property: "",
						Value:    "50",
					},
				},
			},
		})

		result := engine_eval.IsContextInSegment(evalContext, segmentContext)
		assert.False(t, result) // Should fail without identity
	})
}

func TestRegexOperator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		traitValue     string
		conditionValue string
		expected       bool
	}{
		{"simple match", "foo", "[a-z]+", true},
		{"no match", "FOO", "[a-z]+", false},
		{"email match", "test@example.com", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, true},
		{"invalid regex", "test", "[", false},
		{"empty values", "", "", true},
		{"number match", "123", `^\d+$`, true},
		{"number no match", "abc", `^\d+$`, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			evalContext := createEvaluationContext(map[string]any{
				"test_trait": stringValue(c.traitValue),
			})

			segmentContext := createSegmentContext("regex_test", "regex_test", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Regex,
							Property: "test_trait",
							Value:    c.conditionValue,
						},
					},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestModuloOperator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		traitValue     string
		conditionValue string
		expected       bool
	}{
		{"simple modulo match", "2", "2|0", true},
		{"simple modulo no match", "1", "2|0", false},
		{"float modulo match", "1.1", "2.1|1.1", true},
		{"float modulo no match", "3", "2|0", false},
		{"large number match", "35.0", "4|3", true},
		{"large number no match", "34.2", "4|3", false},
		{"invalid trait value", "foo", "4|3", false},
		{"invalid condition format", "1", "invalid", false},
		{"invalid divisor", "1", "abc|3", false},
		{"invalid remainder", "1", "4|abc", false},
		{"missing separator", "1", "43", false},
		{"too many parts", "1", "4|3|2", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			evalContext := createEvaluationContext(map[string]any{
				"test_trait": stringValue(c.traitValue),
			})

			segmentContext := createSegmentContext("modulo_test", "modulo_test", []engine_eval.SegmentRule{
				{
					Type: engine_eval.All,
					Conditions: []engine_eval.Condition{
						{
							Operator: engine_eval.Modulo,
							Property: "test_trait",
							Value:    c.conditionValue,
						},
					},
				},
			})

			result := engine_eval.IsContextInSegment(evalContext, segmentContext)
			assert.Equal(t, c.expected, result)
		})
	}
}

func TestMatchWithRegexOperator(t *testing.T) {
	t.Parallel()

	evalContext := createEvaluationContext(map[string]any{
		"email": stringValue("test@example.com"),
	})

	segmentContext := createSegmentContext("regex_test", "regex_test", []engine_eval.SegmentRule{
		{
			Type: engine_eval.All,
			Conditions: []engine_eval.Condition{
				{
					Operator: engine_eval.Regex,
					Property: "email",
					Value:    `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
				},
			},
		},
	})

	result := engine_eval.IsContextInSegment(evalContext, segmentContext)
	assert.True(t, result)
}

func TestMatchWithModuloOperator(t *testing.T) {
	t.Parallel()

	evalContext := createEvaluationContext(map[string]any{
		"user_id": stringValue("35"),
	})

	segmentContext := createSegmentContext("modulo_test", "modulo_test", []engine_eval.SegmentRule{
		{
			Type: engine_eval.All,
			Conditions: []engine_eval.Condition{
				{
					Operator: engine_eval.Modulo,
					Property: "user_id",
					Value:    "4|3",
				},
			},
		},
	})

	result := engine_eval.IsContextInSegment(evalContext, segmentContext)
	assert.True(t, result)
}
