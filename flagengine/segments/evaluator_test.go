package segments_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/utils"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/utils/fixtures"
)

const (
	trait_key_1   = "email"
	trait_value_1 = "user@example.com"

	trait_key_2   = "num_purchase"
	trait_value_2 = "12"

	trait_key_3   = "date_joined"
	trait_value_3 = "2021-01-01"
)

var (
	empty_segment            = &segments.SegmentModel{ID: 1, Name: "empty_segment"}
	segment_single_condition = &segments.SegmentModel{
		ID:   2,
		Name: "segment_one_condition",
		Rules: []*segments.SegmentRuleModel{
			{
				Type: segments.All,
				Conditions: []*segments.SegmentConditionModel{
					{
						Operator: segments.Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
				},
			},
		},
	}
	segment_multiple_conditions_all = &segments.SegmentModel{
		ID:   3,
		Name: "segment_multiple_conditions_all",
		Rules: []*segments.SegmentRuleModel{
			{
				Type: segments.All,
				Conditions: []*segments.SegmentConditionModel{
					{
						Operator: segments.Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
					{
						Operator: segments.Equal,
						Property: trait_key_2,
						Value:    trait_value_2,
					},
				},
			},
		},
	}
	segment_multiple_conditions_any = &segments.SegmentModel{
		ID:   4,
		Name: "segment_multiple_conditions_all",
		Rules: []*segments.SegmentRuleModel{
			{
				Type: segments.Any,
				Conditions: []*segments.SegmentConditionModel{{
					Operator: segments.Equal,
					Property: trait_key_1,
					Value:    trait_value_1,
				},
					{
						Operator: segments.Equal,
						Property: trait_key_2,
						Value:    trait_value_2,
					},
				},
			},
		},
	}
	segment_nested_rules = &segments.SegmentModel{
		ID:   5,
		Name: "segment_nested_rules_all",
		Rules: []*segments.SegmentRuleModel{
			{
				Type: segments.All,
				Rules: []*segments.SegmentRuleModel{
					{
						Type: segments.All,
						Conditions: []*segments.SegmentConditionModel{
							{
								Operator: segments.Equal,
								Property: trait_key_1,
								Value:    trait_value_1,
							},
							{
								Operator: segments.Equal,
								Property: trait_key_2,
								Value:    trait_value_2,
							},
						},
					},
					{
						Type: segments.All,
						Conditions: []*segments.SegmentConditionModel{
							{
								Operator: segments.Equal,
								Property: trait_key_3,
								Value:    trait_value_3,
							},
						},
					},
				},
			},
		},
	}
	segment_conditions_and_nested_rules = &segments.SegmentModel{
		ID:   6,
		Name: "segment_multiple_conditions_all_and_nested_rules",
		Rules: []*segments.SegmentRuleModel{
			{
				Type: segments.All,
				Conditions: []*segments.SegmentConditionModel{
					{
						Operator: segments.Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
				},
				Rules: []*segments.SegmentRuleModel{
					{
						Type: segments.All,
						Conditions: []*segments.SegmentConditionModel{
							{
								Operator: segments.Equal,
								Property: trait_key_2,
								Value:    trait_value_2,
							},
						},
					},
					{
						Type: segments.All,
						Conditions: []*segments.SegmentConditionModel{
							{
								Operator: segments.Equal,
								Property: trait_key_3,
								Value:    trait_value_3,
							},
						},
					},
				},
			},
		},
	}
)

func TestIdentityInSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		segment        *segments.SegmentModel
		identityTraits []*traits.TraitModel
		expected       bool
	}{
		{empty_segment, nil, false},
		{segment_single_condition, nil, false},
		{
			segment_single_condition,
			[]*traits.TraitModel{{TraitKey: trait_key_1, TraitValue: trait_value_1}},
			true,
		},
		{segment_multiple_conditions_all, nil, false},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{{TraitKey: trait_key_1, TraitValue: trait_value_1}},
			false,
		},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
				{TraitKey: trait_key_2, TraitValue: trait_value_2},
			},
			true,
		},
		{segment_multiple_conditions_any, nil, false},
		{
			segment_multiple_conditions_any,
			[]*traits.TraitModel{{TraitKey: trait_key_1, TraitValue: trait_value_1}},
			true,
		},
		{
			segment_multiple_conditions_any,
			[]*traits.TraitModel{{TraitKey: trait_key_2, TraitValue: trait_value_2}},
			true,
		},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
				{TraitKey: trait_key_2, TraitValue: trait_value_2},
			},
			true,
		},
		{segment_nested_rules, nil, false},
		{
			segment_nested_rules,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
			},
			false,
		},
		{
			segment_nested_rules,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
				{TraitKey: trait_key_2, TraitValue: trait_value_2},
				{TraitKey: trait_key_3, TraitValue: trait_value_3},
			},
			true,
		},
		{segment_conditions_and_nested_rules, nil, false},
		{
			segment_conditions_and_nested_rules,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
			},
			false,
		},
		{
			segment_conditions_and_nested_rules,
			[]*traits.TraitModel{
				{TraitKey: trait_key_1, TraitValue: trait_value_1},
				{TraitKey: trait_key_2, TraitValue: trait_value_2},
				{TraitKey: trait_key_3, TraitValue: trait_value_3},
			},
			true,
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			doTestIdentityInSegment(t, c.segment, c.identityTraits, c.expected)
		})
	}
}

func doTestIdentityInSegment(t *testing.T, segment *segments.SegmentModel, identityTraits []*traits.TraitModel, expected bool) {
	t.Helper()

	identity := &identities.IdentityModel{
		Identifier:        "foo",
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: "api-key",
	}

	assert.Equal(t, expected, segments.EvaluateIdentityInSegment(identity, segment))
}

func TestIdentityInSegmentPercentageSplit(t *testing.T) {
	cases := []struct {
		segmentSplitValue        int
		identityHashedPercentage int
		expectedResult           bool
	}{
		{10, 1, true},
		{100, 50, true},
		{0, 1, false},
		{10, 20, false},
	}

	_, _, _, _, identity := fixtures.GetFixtures()

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			cond := &segments.SegmentConditionModel{
				Operator: segments.PercentageSplit,
				Value:    strconv.Itoa(c.segmentSplitValue),
			}
			rule := &segments.SegmentRuleModel{
				Type:       segments.All,
				Conditions: []*segments.SegmentConditionModel{cond},
			}
			segment := &segments.SegmentModel{ID: 1, Name: "% split", Rules: []*segments.SegmentRuleModel{rule}}

			utils.MockSetHashedPercentageForObjectIds(func(_ []string, _ int) float64 {
				return float64(c.identityHashedPercentage)
			})
			result := segments.EvaluateIdentityInSegment(identity, segment)

			assert.Equal(t, c.expectedResult, result)
		})
	}
	utils.MockSetHashedPercentageForObjectIds(utils.GetHashedPercentageForObjectIds)
}

func TestIdentityInSegmentIsSetAndIsNotSet(t *testing.T) {
	cases := []struct {
		operator       segments.ConditionOperator
		property       string
		identityTraits []*traits.TraitModel
		expectedResult bool
	}{
		{segments.IsSet, "foo", []*traits.TraitModel{{TraitKey: "foo", TraitValue: "bar"}}, true},
		{segments.IsSet, "foo", []*traits.TraitModel{{TraitKey: "not_foo", TraitValue: "bar"}}, false},
		{segments.IsSet, "foo", []*traits.TraitModel{}, false},
		{segments.IsNotSet, "foo", []*traits.TraitModel{}, true},
		{segments.IsNotSet, "foo", []*traits.TraitModel{{TraitKey: "foo", TraitValue: "bar"}}, false},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			cond := &segments.SegmentConditionModel{
				Operator: c.operator,
				Property: c.property,
			}
			rule := &segments.SegmentRuleModel{
				Type:       segments.All,
				Conditions: []*segments.SegmentConditionModel{cond},
			}
			segment := &segments.SegmentModel{ID: 1, Name: "IsSet or IsNot", Rules: []*segments.SegmentRuleModel{rule}}
			doTestIdentityInSegment(t, segment, c.identityTraits, c.expectedResult)
		})
	}
}

func TestSegmentConditionMatchesTraitValue(t *testing.T) {
	cases := []struct {
		operator       segments.ConditionOperator
		traitValue     interface{}
		conditionValue string
		expectedResult bool
	}{
		{segments.Equal, "bar", "bar", true},
		{segments.Equal, "bar", "baz", false},
		{segments.Equal, 1, "1", true},
		{segments.Equal, 1, "2", false},
		{segments.Equal, true, "true", true},
		{segments.Equal, false, "false", true},
		{segments.Equal, false, "true", false},
		{segments.Equal, true, "false", false},
		{segments.Equal, 1.23, "1.23", true},
		{segments.Equal, 1.23, "4.56", false},
		{segments.GreaterThan, 2, "1", true},
		{segments.GreaterThan, 1, "1", false},
		{segments.GreaterThan, 0, "1", false},
		{segments.GreaterThan, 2.1, "2.0", true},
		{segments.GreaterThan, 2.1, "2.1", false},
		{segments.GreaterThan, 2.0, "2.1", false},
		{segments.GreaterThanInclusive, 2, "1", true},
		{segments.GreaterThanInclusive, 1, "1", true},
		{segments.GreaterThanInclusive, 0, "1", false},
		{segments.GreaterThanInclusive, 2.1, "2.0", true},
		{segments.GreaterThanInclusive, 2.1, "2.1", true},
		{segments.GreaterThanInclusive, 2.0, "2.1", false},
		{segments.LessThan, 1, "2", true},
		{segments.LessThan, 1, "1", false},
		{segments.LessThan, 1, "0", false},
		{segments.LessThan, 2.0, "2.1", true},
		{segments.LessThan, 2.1, "2.1", false},
		{segments.LessThan, 2.1, "2.0", false},
		{segments.LessThanInclusive, 1, "2", true},
		{segments.LessThanInclusive, 1, "1", true},
		{segments.LessThanInclusive, 1, "0", false},
		{segments.LessThanInclusive, 2.0, "2.1", true},
		{segments.LessThanInclusive, 2.1, "2.1", true},
		{segments.LessThanInclusive, 2.1, "2.0", false},
		{segments.NotEqual, "bar", "baz", true},
		{segments.NotEqual, "bar", "bar", false},
		{segments.NotEqual, 1, "2", true},
		{segments.NotEqual, 1, "1", false},
		{segments.NotEqual, true, "false", true},
		{segments.NotEqual, false, "true", true},
		{segments.NotEqual, false, "false", false},
		{segments.NotEqual, true, "true", false},
		{segments.Contains, "bar", "b", true},
		{segments.Contains, "bar", "bar", true},
		{segments.Contains, "bar", "baz", false},
		{segments.NotContains, "bar", "b", false},
		{segments.NotContains, "bar", "bar", false},
		{segments.NotContains, "bar", "baz", true},
		{segments.Regex, "foo", "[a-z]+", true},
		{segments.Regex, "FOO", "[a-z]+", false},

		// Semver
		{segments.Equal, "1.2.3", "1.2.3:semver", true},
		{segments.Equal, "1.2.4", "1.2.3:semver", false},
		{segments.Equal, "not_a_semver", "1.2.3:semver", false},

		{segments.NotEqual, "1.0.0", "1.0.0:semver", false},
		{segments.NotEqual, "1.0.1", "1.0.0:semver", true},

		{segments.GreaterThan, "1.0.1", "1.0.0:semver", true},
		{segments.GreaterThan, "1.0.1", "1.1.0:semver", false},
		{segments.GreaterThan, "1.0.1", "1.0.1:semver", false},
		{segments.GreaterThan, "1.2.4", "1.2.3-pre.2+build.4:semver", true},

		{segments.LessThan, "1.0.1", "1.0.0:semver", false},
		{segments.LessThan, "1.0.1", "1.1.0:semver", true},
		{segments.LessThan, "1.0.1", "1.0.1:semver", false},
		{segments.LessThan, "1.2.4", "1.2.3-pre.2+build.4:semver", false},

		{segments.GreaterThanInclusive, "1.0.1", "1.0.0:semver", true},
		{segments.GreaterThanInclusive, "1.0.1", "1.2.0:semver", false},
		{segments.GreaterThanInclusive, "1.0.1", "1.0.1:semver", true},
		{segments.LessThanInclusive, "1.0.0", "1.0.1:semver", true},
		{segments.LessThanInclusive, "1.0.0", "1.0.0:semver", true},
		{segments.LessThanInclusive, "1.0.1", "1.0.0:semver", false},

		// Modulo
		{segments.Modulo, 1, "2|0", false},
		{segments.Modulo, 2, "2|0", true},
		{segments.Modulo, 1.1, "2.1|1.1", true},
		{segments.Modulo, 3, "2|0", false},
		{segments.Modulo, 34.2, "4|3", false},
		{segments.Modulo, 35.0, "4|3", true},
		{segments.Modulo, "foo", "4|3", false},
		{segments.Modulo, "1.0.0", "4|3", false},
		{segments.Modulo, false, "4|3", false},
	}

	for _, c := range cases {
		trStr := fmt.Sprint(c.traitValue)
		t.Run(trStr+" "+string(c.operator)+" "+c.conditionValue, func(t *testing.T) {
			cond := &segments.SegmentConditionModel{
				Operator: c.operator,
				Property: "foo",
				Value:    c.conditionValue,
			}
			assert.Equal(t, c.expectedResult, cond.MatchesTraitValue(trStr))
		})
	}
}

func TestSegmentRuleNone(t *testing.T) {
	cases := []struct {
		iterable       []bool
		expectedResult bool
	}{
		{[]bool{}, true},
		{[]bool{false}, true},
		{[]bool{false, false}, true},
		{[]bool{false, true}, false},
		{[]bool{true, true}, false},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, c.expectedResult, utils.None(c.iterable))
		})
	}
}
