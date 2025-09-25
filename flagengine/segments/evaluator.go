package segments

import (
	"strconv"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils"
)

func EvaluateIdentityInSegment(
	identity *identities.IdentityModel,
	segment *SegmentModel,
	overrideTraits ...*traits.TraitModel,
) bool {
	if len(segment.Rules) == 0 {
		return false
	}

	traits := identity.IdentityTraits
	if len(overrideTraits) > 0 {
		traits = overrideTraits
	}

	identityHashKey := identity.CompositeKey()
	if identity.DjangoID != 0 {
		identityHashKey = strconv.Itoa(identity.DjangoID)
	}
	for _, rule := range segment.Rules {
		if !traitsMatchSegmentRule(traits, rule, segment.ID, identityHashKey) {
			return false
		}
	}

	return true
}

func traitsMatchSegmentRule(
	identityTraits []*traits.TraitModel,
	rule *SegmentRuleModel,
	segmentID int,
	identityID string,
) bool {
	conditions := make([]bool, len(rule.Conditions))
	for i, c := range rule.Conditions {
		conditions[i] = traitsMatchSegmentCondition(identityTraits, c, segmentID, identityID)
	}
	matchesConditions := rule.MatchingFunction()(conditions) || len(rule.Conditions) == 0

	rules := make([]bool, len(rule.Rules))
	for i, r := range rule.Rules {
		rules[i] = traitsMatchSegmentRule(identityTraits, r, segmentID, identityID)
	}

	return matchesConditions && utils.All(rules)
}

func traitsMatchSegmentCondition(
	identityTraits []*traits.TraitModel,
	condition *SegmentConditionModel,
	segmentID int,
	identityID string,
) bool {
	if condition.Operator == PercentageSplit {
		floatValue, _ := strconv.ParseFloat(condition.Value, 64)
		return utils.GetHashedPercentageForObjectIds([]string{strconv.Itoa(segmentID), identityID}, 1) <= floatValue
	}
	var matchedTraitValue *string
	for _, trait := range identityTraits {
		if trait.TraitKey == condition.Property {
			matchedTraitValue = &trait.TraitValue
		}
	}

	if condition.Operator == IsNotSet {
		return matchedTraitValue == nil
	}
	if condition.Operator == IsSet {
		return matchedTraitValue != nil
	}

	if matchedTraitValue != nil {
		return condition.MatchesTraitValue(*matchedTraitValue)
	}
	return false
}
