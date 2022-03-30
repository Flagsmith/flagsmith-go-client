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

func GetIdentityFeatureState(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	featureName string,
	overrideTraits ...*traits.TraitModel,
) *features.FeatureStateModel {
	featureStates := getIdentityFeatureStatesMap(environment, identity, overrideTraits...)

	for feature, featureState := range featureStates {
		if feature.Name == featureName {
			return featureState
		}
	}
	return nil
}

func getIdentitySegments(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits ...*traits.TraitModel,
) []*segments.SegmentModel {
	var list []*segments.SegmentModel

	for _, s := range environment.Project.Segments {
		if segments.EvaluateIdentityInSegment(identity, s, overrideTraits...) {
			list = append(list, s)
		}
	}

	return list
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
