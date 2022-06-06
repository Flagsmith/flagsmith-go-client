package flagengine

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/segments"
)

// GetEnvironmentFeatureStates returns a list of feature states for a given environment.
func GetEnvironmentFeatureStates(environment *environments.EnvironmentModel) []*features.FeatureStateModel {
	if environment.Project.HideDisabledFlags {
		var featureStates []*features.FeatureStateModel
		for _, fs := range environment.FeatureStates {
			if fs.Enabled {
				featureStates = append(featureStates, fs)
			}
		}
		return featureStates
	}
	return environment.FeatureStates
}

// GetEnvironmentFeatureState returns a specific feature state for a given featureName in a given environment, or nil feature state is not found.
func GetEnvironmentFeatureState(environment *environments.EnvironmentModel, featureName string) *features.FeatureStateModel {
	for _, fs := range environment.FeatureStates {
		if fs.Feature.Name == featureName {
			return fs
		}
	}
	return nil
}

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

	for _, featureState := range featureStates {
		if featureState.Feature.Name == featureName {
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
) map[int]*features.FeatureStateModel {
	featureStates := make(map[int]*features.FeatureStateModel)
	for _, fs := range environment.FeatureStates {
		featureStates[fs.Feature.ID] = fs
	}

	identitySegments := getIdentitySegments(environment, identity, overrideTraits...)
	for _, segment := range identitySegments {
		for _, fs := range segment.FeatureStates {
			existing_fs, exists := featureStates[fs.Feature.ID]
			if exists && existing_fs.IsHigherSegmentPriority(fs) {
				continue
			}

			featureStates[fs.Feature.ID] = fs
		}
	}

	for _, fs := range identity.GetIdentityFeatures() {
		if _, ok := featureStates[fs.Feature.ID]; ok {
			featureStates[fs.Feature.ID] = fs
		}
	}

	return featureStates
}
