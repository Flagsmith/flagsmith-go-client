package flagsmith

import (
	"encoding/json"
	"fmt"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
)

type DefaultFlagHandlerType func(FeatureName string) Flag
type Flag struct {
	Enabled     bool
	Value       interface{}
	IsDefault   bool
	FeatureID   int
	FeatureName string
}
type jsonFeature struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type jsonFlag struct {
	Enabled bool        `json:"enabled"`
	Value   interface{} `json:"feature_state_value"`
	Feature jsonFeature `json:"feature"`
}

func (jf *jsonFlag) toFlag() Flag {
	return Flag{
		Enabled:     jf.Enabled,
		Value:       jf.Value,
		IsDefault:   false,
		FeatureID:   jf.Feature.ID,
		FeatureName: jf.Feature.Name,
	}
}

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
	identityID string) Flags {

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

func MakeFlagsFromAPIFlags(flagsJson []byte, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler *DefaultFlagHandlerType) (Flags, error) {
	fmt.Println("MakeFlagsFromAPIFlags", flagsJson)
	var jsonflags []jsonFlag
	err := json.Unmarshal(flagsJson, &jsonflags)
	if err != nil {
		fmt.Println("MakeFlagsFromAPIFlags error", err)
		return Flags{}, err
	}
	flags := make([]Flag, len(jsonflags))
	for i, jf := range jsonflags {
		flags[i] = jf.toFlag()
	}
	fmt.Println("MakeFlagsFromAPIFlags flags hain ", flags)
	return Flags{
		flags:              flags,
		analyticsProcessor: analyticsProcessor,
		defaultFlagHandler: defaultFlagHandler,
	}, err
}

// Returns an array of all flag objects
func (f *Flags) AllFlags() []Flag {
	return f.flags
}
