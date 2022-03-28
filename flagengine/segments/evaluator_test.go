package segments

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
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
	empty_segment            = &SegmentModel{ID: 1, Name: "empty_segment"}
	segment_single_condition = &SegmentModel{
		ID:   2,
		Name: "segment_one_condition",
		Rules: []*SegmentRuleModel{
			{
				Type: All,
				Conditions: []*SegmentConditionModel{
					{
						Operator: Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
				},
			},
		},
	}
	segment_multiple_conditions_all = &SegmentModel{
		ID:   3,
		Name: "segment_multiple_conditions_all",
		Rules: []*SegmentRuleModel{
			{
				Type: All,
				Conditions: []*SegmentConditionModel{
					{
						Operator: Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
					{
						Operator: Equal,
						Property: trait_key_2,
						Value:    trait_value_2,
					},
				},
			},
		},
	}
	segment_multiple_conditions_any = &SegmentModel{
		ID:   4,
		Name: "segment_multiple_conditions_all",
		Rules: []*SegmentRuleModel{
			{
				Type: Any,
				Conditions: []*SegmentConditionModel{{
					Operator: Equal,
					Property: trait_key_1,
					Value:    trait_value_1,
				},
					{
						Operator: Equal,
						Property: trait_key_2,
						Value:    trait_value_2,
					},
				},
			},
		},
	}
	segment_nested_rules = &SegmentModel{
		ID:   5,
		Name: "segment_nested_rules_all",
		Rules: []*SegmentRuleModel{
			{
				Type: All,
				Rules: []*SegmentRuleModel{
					{
						Type: All,
						Conditions: []*SegmentConditionModel{
							{
								Operator: Equal,
								Property: trait_key_1,
								Value:    trait_value_1,
							},
							{
								Operator: Equal,
								Property: trait_key_2,
								Value:    trait_value_2,
							},
						},
					},
					{
						Type: All,
						Conditions: []*SegmentConditionModel{
							{
								Operator: Equal,
								Property: trait_key_3,
								Value:    trait_value_3,
							},
						},
					},
				},
			},
		},
	}
	segment_conditions_and_nested_rules = &SegmentModel{
		ID:   6,
		Name: "segment_multiple_conditions_all_and_nested_rules",
		Rules: []*SegmentRuleModel{
			{
				Type: All,
				Conditions: []*SegmentConditionModel{
					{
						Operator: Equal,
						Property: trait_key_1,
						Value:    trait_value_1,
					},
				},
				Rules: []*SegmentRuleModel{
					{
						Type: All,
						Conditions: []*SegmentConditionModel{
							{
								Operator: Equal,
								Property: trait_key_2,
								Value:    trait_value_2,
							},
						},
					},
					{
						Type: All,
						Conditions: []*SegmentConditionModel{
							{
								Operator: Equal,
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
		segment        *SegmentModel
		identityTraits []*traits.TraitModel
		expected       bool
	}{
		{empty_segment, nil, false},
		{segment_single_condition, nil, false},
		{
			segment_single_condition,
			[]*traits.TraitModel{{trait_key_1, trait_value_1}},
			true,
		},
		{segment_multiple_conditions_all, nil, false},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{{trait_key_1, trait_value_1}},
			false,
		},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
				{trait_key_2, trait_value_2},
			},
			true,
		},
		{segment_multiple_conditions_any, nil, false},
		{
			segment_multiple_conditions_any,
			[]*traits.TraitModel{{trait_key_1, trait_value_1}},
			true,
		},
		{
			segment_multiple_conditions_any,
			[]*traits.TraitModel{{trait_key_2, trait_value_2}},
			true,
		},
		{
			segment_multiple_conditions_all,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
				{trait_key_2, trait_value_2},
			},
			true,
		},
		{segment_nested_rules, nil, false},
		{
			segment_nested_rules,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
			},
			false,
		},
		{
			segment_nested_rules,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
				{trait_key_2, trait_value_2},
				{trait_key_3, trait_value_3},
			},
			true,
		},
		{segment_conditions_and_nested_rules, nil, false},
		{
			segment_conditions_and_nested_rules,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
			},
			false,
		},
		{
			segment_conditions_and_nested_rules,
			[]*traits.TraitModel{
				{trait_key_1, trait_value_1},
				{trait_key_2, trait_value_2},
				{trait_key_3, trait_value_3},
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

func doTestIdentityInSegment(t *testing.T, segment *SegmentModel, identityTraits []*traits.TraitModel, expected bool) {
	t.Helper()

	identity := &identities.IdentityModel{
		Identifier:        "foo",
		IdentityTraits:    identityTraits,
		EnvironmentAPIKey: "api-key",
	}

	assert.Equal(t, expected, EvaluateIdentityInSegment(identity, segment))
}
