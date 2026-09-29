package flagsmith

import (
	"encoding/json"
	"fmt"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/trait"
)

type Flag struct {
	Enabled     bool
	Value       interface{}
	IsDefault   bool
	FeatureID   int
	FeatureName string
	// Reason is the evaluation reason, e.g. "DEFAULT", "SPLIT; weight=70.0" or
	// "TARGETING_MATCH; segment=...". Empty when none was reported.
	Reason string
	// Variant is the key of the multivariate variant the identity was bucketed into.
	// Empty for standard features and evaluation without an identity.
	Variant string
	// Experiment is the running experiment on this feature. Only populated by remote
	// identity evaluation; nil otherwise.
	Experiment *ExperimentMetadata
}

// ExperimentMetadata describes the running experiment a flag was evaluated under.
// It is only populated by remote identity evaluation.
type ExperimentMetadata struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// InExperiment reports whether this identity is enrolled. Variant alone cannot
	// tell: outside the rollout an identity still gets a variant.
	InExperiment bool `json:"in_experiment"`
}

type Trait = trait.Trait

type IdentityTraits struct {
	Identifier string         `json:"identifier"`
	Traits     []*trait.Trait `json:"traits"`
	Transient  bool           `json:"transient,omitempty"`
}

func makeFlagFromEngineEvaluationFlagResult(flagResult *engine_eval.FlagResult) Flag {
	value := flagResult.Value

	// Get FeatureID from metadata
	featureID := flagResult.Metadata.FeatureID

	return Flag{
		Enabled:     flagResult.Enabled,
		Value:       value,
		IsDefault:   false,
		FeatureID:   featureID,
		FeatureName: flagResult.Name,
		Reason:      flagResult.Reason,
		Variant:     flagResult.Variant,
	}
}

type Flags struct {
	flags              []Flag
	analyticsProcessor *AnalyticsProcessor
	defaultFlagHandler func(featureName string) (Flag, error)
}

func makeFlagsFromEngineEvaluationResult(evaluationResult *engine_eval.EvaluationResult, analyticsProcessor *AnalyticsProcessor, defaultFlagHandler func(string) (Flag, error)) Flags {
	flags := make([]Flag, 0, len(evaluationResult.Flags))
	for _, flagResult := range evaluationResult.Flags {
		flags = append(flags, makeFlagFromEngineEvaluationFlagResult(flagResult))
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

type jsonFlagMetadata struct {
	Experiment *ExperimentMetadata `json:"experiment"`
}

type jsonFlag struct {
	Enabled  bool              `json:"enabled"`
	Value    interface{}       `json:"feature_state_value"`
	Feature  jsonFeature       `json:"feature"`
	Reason   string            `json:"reason"`
	Variant  string            `json:"variant"`
	Metadata *jsonFlagMetadata `json:"metadata"`
}

func (jf *jsonFlag) toFlag() Flag {
	f := Flag{
		Enabled:     jf.Enabled,
		Value:       jf.Value,
		IsDefault:   false,
		FeatureID:   jf.Feature.ID,
		FeatureName: jf.Feature.Name,
		Reason:      jf.Reason,
		Variant:     jf.Variant,
	}
	if jf.Metadata != nil {
		f.Experiment = jf.Metadata.Experiment
	}
	return f
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
		return resultFlag, &FlagsmithClientError{fmt.Sprintf("flagsmith: No feature found with name %q", featureName)}
	}
	if f.analyticsProcessor != nil {
		f.analyticsProcessor.TrackFeature(resultFlag.FeatureName)
	}
	return resultFlag, nil
}
