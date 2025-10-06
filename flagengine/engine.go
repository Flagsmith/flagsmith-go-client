package flagengine

import (
	"fmt"
	"math"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

// featureContextWithSegmentName holds a feature context along with the segment name it came from.
type featureContextWithSegmentName struct {
	featureContext *engine_eval.FeatureContext
	segmentName    string
}

// getPriorityOrDefault returns the priority value if it exists, otherwise returns the default priority.
func getPriorityOrDefault(priority *float64) float64 {
	if priority != nil {
		return *priority
	}
	return math.Inf(1) // Weakest possible priority
}

func processSegments(ec *engine_eval.EngineEvaluationContext) ([]engine_eval.SegmentResult, map[string]featureContextWithSegmentName) {
	segments := []engine_eval.SegmentResult{}
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

				overridePriority := getPriorityOrDefault(override.Priority)

				// Check if we should update the segment feature context
				shouldUpdate := false
				if existing, exists := segmentFeatureContexts[featureKey]; !exists {
					shouldUpdate = true
				} else {
					existingPriority := getPriorityOrDefault(existing.featureContext.Priority)
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

	return segments, segmentFeatureContexts
}

func processFeatures(ec *engine_eval.EngineEvaluationContext, segmentFeatureContexts map[string]featureContextWithSegmentName) map[string]*engine_eval.FlagResult {
	flags := make(map[string]*engine_eval.FlagResult)

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
				flags[featureContext.Name] = &engine_eval.FlagResult{
					Enabled:    fc.Enabled,
					FeatureKey: fc.FeatureKey,
					Name:       fc.Name,
					Reason:     &reason,
					Value:      fc.Value,
				}
			} else {
				// Use default feature context
				flagResult := getFlagResultFromFeatureContext(&featureContext, identityKey)
				flags[featureContext.Name] = &flagResult
			}
		}
	}

	return flags
}

// GetEvaluationResult computes flags and matched segments.
func GetEvaluationResult(ec *engine_eval.EngineEvaluationContext) engine_eval.EvaluationResult {
	// Process segments
	segments, segmentFeatureContexts := processSegments(ec)

	// Process features
	flags := processFeatures(ec, segmentFeatureContexts)

	return engine_eval.EvaluationResult{
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
				reason = fmt.Sprintf("SPLIT; weight=%.0f", variant.Weight)
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
