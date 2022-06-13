package flagsmith

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
)

type Flag struct {
	Enabled     bool        `json:"enabled"`
	Value       interface{} `json:"value"`
	IsDefault   bool        `json:"is_default"`
	FeatureID   int         `json:"feature_id"`
	FeatureName string      `json:"feature_name"`
}

type DefaultFlagHandlerType func(FeatureName string) Flag


func MakeFlagFromFeatureState(featureState *features.FeatureStateModel, identityID string) Flag {
	return Flag{
		Enabled:     featureState.Enabled,
		Value:       featureState.Value(identityID),
		IsDefault:   false,
		FeatureID:   featureState.Feature.ID,
		FeatureName: featureState.Feature.Name,
	}
}


type Flags struct {
	flags              []Flag
	analyticsProcessor *AnalyticsProcessor
	defaultFlagHandler *DefaultFlagHandlerType
}

func MakeFlagsFromFeatureStates(featureStates []*features.FeatureStateModel,
	analyticsProcessor *AnalyticsProcessor,
	defaultFlagHandler *DefaultFlagHandlerType,
	identityID string) Flags{

	flags := make([]Flag, len(featureStates))
	for i, featureState := range featureStates {
		flags[i] = MakeFlagFromFeatureState(featureState, identityID)
	}

	return Flags{
		flags:              flags,
		analyticsProcessor: analyticsProcessor,
		defaultFlagHandler: defaultFlagHandler,
	}

}
