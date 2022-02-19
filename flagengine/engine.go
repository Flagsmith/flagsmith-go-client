package flagengine

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/segments"
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

func GetIdentitySegments(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits ...*traits.TraitModel,
) []*segments.SegmentModel {
	return nil
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

	identitySegments := GetIdentitySegments(environment, identity, overrideTraits...)
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
