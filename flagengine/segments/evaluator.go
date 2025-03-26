package segments

import (
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils"
	"github.com/blang/semver/v4"
	"golang.org/x/exp/slices"
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
		if trait.Key == condition.Property {
			matchedTraitValue = &trait.Value
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

func match(c ConditionOperator, traitValue, conditionValue string) bool {
	b1, e1 := strconv.ParseBool(traitValue)
	b2, e2 := strconv.ParseBool(conditionValue)
	if e1 == nil && e2 == nil {
		return matchBool(c, b1, b2)
	}

	i1, e1 := strconv.ParseInt(traitValue, 10, 64)
	i2, e2 := strconv.ParseInt(conditionValue, 10, 64)
	if e1 == nil && e2 == nil {
		return matchInt(c, i1, i2)
	}

	f1, e1 := strconv.ParseFloat(traitValue, 64)
	f2, e2 := strconv.ParseFloat(conditionValue, 64)
	if e1 == nil && e2 == nil {
		return matchFloat(c, f1, f2)
	}
	if strings.HasSuffix(conditionValue, ":semver") {
		conditionVersion, err := semver.Make(conditionValue[:len(conditionValue)-7])
		if err != nil {
			return false
		}
		return matchSemver(c, traitValue, conditionVersion)
	}

	return matchString(c, traitValue, conditionValue)
}

func matchSemver(c ConditionOperator, traitValue string, conditionVersion semver.Version) bool {
	traitVersion, err := semver.Make(traitValue)
	if err != nil {
		return false
	}
	switch c {
	case Equal:
		return traitVersion.EQ(conditionVersion)
	case GreaterThan:
		return traitVersion.GT(conditionVersion)
	case LessThan:
		return traitVersion.LT(conditionVersion)
	case LessThanInclusive:
		return traitVersion.LTE(conditionVersion)
	case GreaterThanInclusive:
		return traitVersion.GE(conditionVersion)
	case NotEqual:
		return traitVersion.NE(conditionVersion)
	}
	return false
}

func matchBool(c ConditionOperator, v1, v2 bool) bool {
	var i1, i2 int64
	if v1 {
		i1 = 1
	}
	if v2 {
		i2 = 1
	}
	return matchInt(c, i1, i2)
}

func matchInt(c ConditionOperator, v1, v2 int64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchFloat(c ConditionOperator, v1, v2 float64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchString(c ConditionOperator, v1, v2 string) bool {
	switch c {
	case Contains:
		return strings.Contains(v1, v2)
	case NotContains:
		return !strings.Contains(v1, v2)
	case In:
		return slices.Contains(strings.Split(v2, ","), v1)
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}
