package flagengine

import (
	"strconv"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/utils"
)

// GetIdentityFeatureStates returns a list of feature states for a given identity in a given environment.
func GetIdentityFeatureStates(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits ...*traits.TraitModel,
) []*features.FeatureStateModel {
	featureStatesMap := getIdentityFeatureStatesMap(environment, identity, overrideTraits...)
	featureStates := make([]*features.FeatureStateModel, 0, len(featureStatesMap))
	hideDisabled := environment.Project.HideDisabledFlags
	for _, fs := range featureStatesMap {
		if hideDisabled && !fs.Enabled {
			continue
		}
		featureStates = append(featureStates, fs)
	}

	return featureStates
}

func getIdentitySegments(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits ...*traits.TraitModel,
) []*segments.SegmentModel {
	var list []*segments.SegmentModel

	for _, s := range environment.Project.Segments {
		if evaluateIdentityInSegment(identity, s, overrideTraits...) {
			list = append(list, s)
		}
	}

	return list
}

func evaluateIdentityInSegment(
	identity *identities.IdentityModel,
	segment *segments.SegmentModel,
	overrideTraits ...*traits.TraitModel,
) bool {
	if len(segment.Rules) == 0 {
		return false
	}

	traits := identity.IdentityTraits
	if len(overrideTraits) > 0 {
		traits = overrideTraits
	}

	for _, rule := range segment.Rules {
		if !traitsMatchSegmentRule(traits, rule, segment.ID, identity.CompositeKey()) {
			return false
		}
	}

	return true
}

func traitsMatchSegmentRule(
	identityTraits []*traits.TraitModel,
	rule *segments.SegmentRuleModel,
	segmentID int,
	identityID string,
) bool {
	condtions := make([]bool, len(rule.Conditions))
	for i, c := range rule.Conditions {
		condtions[i] = traitsMatchSegmentCondition(identityTraits, c, segmentID, identityID)
	}
	matchesCondtions := rule.MatchingFunction()(condtions)

	rules := make([]bool, len(rule.Rules))
	for i, r := range rule.Rules {
		rules[i] = traitsMatchSegmentRule(identityTraits, r, segmentID, identityID)
	}

	return matchesCondtions && utils.All(rules)
}

func traitsMatchSegmentCondition(
	identityTraits []*traits.TraitModel,
	condition *segments.SegmentConditionModel,
	segmentID int,
	identityID string,
) bool {
	if condition.Operator == segments.PercentageSplit {
		floatValue, _ := strconv.ParseFloat(condition.Value, 64)
		return utils.GetHashedPercentageForObjectIds([]string{strconv.Itoa(segmentID), identityID}, 1) > floatValue
	}

	for _, trait := range identityTraits {
		if trait.TraitKey == condition.Property {
			return condition.MatchesTraitValue(trait.TraitValue)
		}
	}
	return false
}

func getIdentityFeatureStatesMap(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits ...*traits.TraitModel,
) map[*features.FeatureModel]*features.FeatureStateModel {
	featureStates := make(map[*features.FeatureModel]*features.FeatureStateModel)
	for _, fs := range environment.FeatureStates {
		featureStates[fs.Feature] = fs
	}

	identitySegments := getIdentitySegments(environment, identity, overrideTraits...)
	for _, segment := range identitySegments {
		for _, fs := range segment.FeatureStates {
			featureStates[fs.Feature] = fs
		}
	}

	for _, fs := range identity.IdentityFeatures {
		if _, ok := featureStates[fs.Feature]; ok {
			featureStates[fs.Feature] = fs
		}
	}

	return featureStates
}
