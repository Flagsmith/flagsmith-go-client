package flagengine

import (
	"fmt"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils"
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

func GetIdentitySegments(
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

	identitySegments := GetIdentitySegments(environment, identity, overrideTraits...)
	for _, segment := range identitySegments {
		for _, fs := range segment.FeatureStates {
			existing_fs, exists := featureStates[fs.Feature.ID]
			if exists && existing_fs.IsHigherSegmentPriority(fs) {
				continue
			}

			featureStates[fs.Feature.ID] = fs
		}
	}

	for _, fs := range identity.IdentityFeatures {
		if _, ok := featureStates[fs.Feature.ID]; ok {
			featureStates[fs.Feature.ID] = fs
		}
	}

	return featureStates
}

// featureContextWithSegmentName holds a feature context along with the segment name it came from.
type featureContextWithSegmentName struct {
	featureContext *engine_eval.FeatureContext
	segmentName    string
}

// GetEvaluationResult computes flags and matched segments given a context and a segment matcher.
// The matcher should return true when the provided segment applies to the provided context.
func GetEvaluationResult(ec *engine_eval.EngineEvaluationContext) engine_eval.EvaluationResult {
	const defaultPriority = 0.0

	segments := []engine_eval.SegmentResult{}
	flags := []engine_eval.FlagResult{}
	segmentFeatureContexts := make(map[string]featureContextWithSegmentName)

	// Process segments
	for _, segmentContext := range ec.Segments {
		if !engine_eval.IsContextInSegment(ec, &segmentContext) {
			continue
		}

		// Add segment to results
		segments = append(segments, engine_eval.SegmentResult{
			Key:  segmentContext.Key,
			Name: segmentContext.Name,
		})

		// Process segment overrides
		if segmentContext.Overrides != nil {
			for i := range segmentContext.Overrides {
				override := &segmentContext.Overrides[i]
				featureKey := override.FeatureKey

				// Get priority, defaulting to 0 if not set
				overridePriority := defaultPriority
				if override.Priority != nil {
					overridePriority = *override.Priority
				}

				// Check if we should update the segment feature context
				shouldUpdate := false
				if existing, exists := segmentFeatureContexts[featureKey]; !exists {
					shouldUpdate = true
				} else {
					existingPriority := defaultPriority
					if existing.featureContext.Priority != nil {
						existingPriority = *existing.featureContext.Priority
					}
					if overridePriority < existingPriority {
						shouldUpdate = true
					}
				}

				if shouldUpdate {
					segmentFeatureContexts[featureKey] = featureContextWithSegmentName{
						featureContext: override,
						segmentName:    segmentContext.Name,
					}
				}
			}
		}
	}

	// Get identity key if identity exists
	var identityKey *string
	if ec.Identity != nil {
		identityKey = &ec.Identity.Key
	}

	// Process features
	if ec.Features != nil {
		for _, featureContext := range ec.Features {
			// Check if we have a segment override for this feature
			if segmentFeatureCtx, exists := segmentFeatureContexts[featureContext.FeatureKey]; exists {
				// Use segment override
				fc := segmentFeatureCtx.featureContext
				reason := fmt.Sprintf("TARGETING_MATCH; segment=%s", segmentFeatureCtx.segmentName)
				flags = append(flags, engine_eval.FlagResult{
					Enabled:    fc.Enabled,
					FeatureKey: fc.FeatureKey,
					Name:       fc.Name,
					Reason:     &reason,
					Value:      fc.Value,
				})
			} else {
				// Use default feature context
				flagResult := getFlagResultFromFeatureContext(&featureContext, identityKey)
				flags = append(flags, flagResult)
			}
		}
	}

	return engine_eval.EvaluationResult{
		Context:  *ec,
		Flags:    flags,
		Segments: segments,
	}
}

// getFlagResultFromFeatureContext creates a FlagResult from a FeatureContext.
func getFlagResultFromFeatureContext(featureContext *engine_eval.FeatureContext, identityKey *string) engine_eval.FlagResult {
	reason := "DEFAULT"
	value := featureContext.Value

	// Handle multivariate features
	if len(featureContext.Variants) > 0 && identityKey != nil && featureContext.Key != "" {
		// Calculate hash percentage for the identity and feature combination
		objectIds := []string{featureContext.Key, *identityKey}
		hashPercentage := utils.GetHashedPercentageForObjectIds(objectIds, 1)

		// Select variant based on weighted distribution
		cumulativeWeight := 0.0
		for _, variant := range featureContext.Variants {
			cumulativeWeight += variant.Weight
			if hashPercentage <= cumulativeWeight {
				value = variant.Value
				break
			}
		}
	}

	flagResult := engine_eval.FlagResult{
		Enabled:    featureContext.Enabled,
		FeatureKey: featureContext.FeatureKey,
		Name:       featureContext.Name,
		Value:      value,
		Reason:     &reason,
	}

	return flagResult
}
