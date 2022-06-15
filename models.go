package flagsmith

import (
	"encoding/json"
	"fmt"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

type Flag struct {
	Enabled     bool
	Value       interface{}
	IsDefault   bool
	FeatureID   int
	FeatureName string
}

type Trait struct {
	TraitKey string `json:"trait_key"`
	TraitValue interface{} `json:"trait_value"`
}

func (t *Trait) ToTraitModel() *traits.TraitModel {
	return &traits.TraitModel{
		TraitKey: t.TraitKey,
		TraitValue: fmt.Sprint(t.TraitValue),
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

type DefaultFlagHandlerType *func(string) Flag
type Flags struct {
	flags              []Flag
	analyticsProcessor *AnalyticsProcessor
	defaultFlagHandler func(featureName string) Flag
}

func MakeFlagsFromFeatureStates(featureStates []*features.FeatureStateModel,
	analyticsProcessor *AnalyticsProcessor,
	defaultFlagHandler func(featureName string) Flag,
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
func MakeFlagsFromAPIFlags(flagsJson []byte, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler func(string) Flag) (Flags, error) {
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
	return Flags{
		flags:              flags,
		analyticsProcessor: analyticsProcessor,
		defaultFlagHandler: defaultFlagHandler,
	}, err
}
func makeFlagsfromIdentityAPIJson(jsonResponse []byte, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler func(string) Flag) (Flags, error) {
	resonse := struct {
		Flags interface{} `json:"flags"`
	}{}
	err := json.Unmarshal(jsonResponse, &resonse)
	if err != nil {
		return Flags{}, err
	}
	b, err := json.Marshal(resonse.Flags)
	if err != nil {
		return Flags{}, err
	}
	return MakeFlagsFromAPIFlags(b, analyticsProcessor, defaultFlagHandler)
}
// Returns an array of all flag objects
func (f *Flags) AllFlags() []Flag {
	return f.flags
}
// Returns the value of a particular flag
func (f *Flags) GetFeatureValue(featureName string) (interface{}, error){
	flag, err := f.GetFlag(featureName)
	if err != nil {
		return nil, err
	}
	return flag.Value, nil

}

func (f *Flags) IsFeatureEnabled(featureName string) (bool, error) {
	flag, err := f.GetFlag(featureName)
	if err != nil {
		return false, err
	}
	return flag.Enabled, nil
}

func (f *Flags) GetFlag(featureName string) (Flag, error) {
	var resultFlag Flag
	for _, flag := range f.flags {
		fmt.Println("GetFlag", flag.FeatureName, featureName)
		if flag.FeatureName == featureName {
			resultFlag = flag
		}
	}
	if resultFlag.FeatureID  == 0 {
		if f.defaultFlagHandler != nil {
			return f.defaultFlagHandler(featureName), nil
		}
		return resultFlag, fmt.Errorf("No feature found with name %s", featureName)
	}
	if f.analyticsProcessor != nil{
		f.analyticsProcessor.TrackFeature(resultFlag.FeatureID)
	}
	fmt.Println("Getting flag for feature", featureName, resultFlag)
	return resultFlag, nil
}
