package flagsmith

import (
	"encoding/json"
	"fmt"

	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/features"

	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/identities/traits"
)

type Flag struct {
	Enabled     bool
	Value       interface{}
	IsDefault   bool
	FeatureID   int
	FeatureName string
}

type Trait struct {
	TraitKey   string      `json:"trait_key"`
	TraitValue interface{} `json:"trait_value"`
}

type IdentityTraits struct {
	Identifier string   `json:"identifier"`
	Traits     []*Trait `json:"traits"`
}

func (t *Trait) ToTraitModel() *traits.TraitModel {
	return &traits.TraitModel{
		TraitKey:   t.TraitKey,
		TraitValue: fmt.Sprint(t.TraitValue),
	}
}

func makeFlagFromFeatureState(featureState *features.FeatureStateModel, identityID string) Flag {
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
	defaultFlagHandler func(featureName string) (Flag, error)
}

func makeFlagsFromFeatureStates(featureStates []*features.FeatureStateModel,
	analyticsProcessor *AnalyticsProcessor,
	defaultFlagHandler func(featureName string) (Flag, error),
	identityID string) Flags {
	flags := make([]Flag, len(featureStates))
	for i, featureState := range featureStates {
		flags[i] = makeFlagFromFeatureState(featureState, identityID)
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
func makeFlagsFromAPIFlags(flagsJson []byte, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler func(string) (Flag, error)) (Flags, error) {
	var jsonflags []jsonFlag
	err := json.Unmarshal(flagsJson, &jsonflags)
	if err != nil {
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
func makeFlagsfromIdentityAPIJson(jsonResponse []byte, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler func(string) (Flag, error)) (Flags, error) {
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
	return makeFlagsFromAPIFlags(b, analyticsProcessor, defaultFlagHandler)
}

// Returns an array of all flag objects.
func (f *Flags) AllFlags() []Flag {
	return f.flags
}

// Returns the value of a particular flag.
func (f *Flags) GetFeatureValue(featureName string) (interface{}, error) {
	flag, err := f.GetFlag(featureName)
	if err != nil {
		return nil, err
	}
	return flag.Value, nil
}

// Returns a boolean indicating whether a particular flag is enabled.
func (f *Flags) IsFeatureEnabled(featureName string) (bool, error) {
	flag, err := f.GetFlag(featureName)
	if err != nil {
		return false, err
	}
	return flag.Enabled, nil
}

// Returns a specific flag given the name of the feature.
func (f *Flags) GetFlag(featureName string) (Flag, error) {
	var resultFlag Flag
	for _, flag := range f.flags {
		if flag.FeatureName == featureName {
			resultFlag = flag
		}
	}
	if resultFlag.FeatureID == 0 {
		if f.defaultFlagHandler != nil {
			return f.defaultFlagHandler(featureName)
		}
		return resultFlag, FlagsmithClientError(fmt.Errorf("flagsmith: No feature found with name %q", featureName))
	}
	if f.analyticsProcessor != nil {
		f.analyticsProcessor.TrackFeature(resultFlag.FeatureName)
	}
	return resultFlag, nil
}
