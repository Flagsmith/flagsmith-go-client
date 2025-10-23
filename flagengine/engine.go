package flagengine

import (
	"fmt"
	"math"
	"sort"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

type featureContextWithSegmentName struct {
	featureContext *engine_eval.FeatureContext
	segmentName    string
}

func getPriorityOrDefault(priority *float64) float64 {
	if priority != nil {
		return *priority
	}
	return math.Inf(1) // Weakest possible priority
}

func getMatchingSegmentsAndOverrides(ec *engine_eval.EngineEvaluationContext) ([]engine_eval.SegmentResult, map[string]featureContextWithSegmentName) {
	segmentResults := []engine_eval.SegmentResult{}
	featureOverrides := make(map[string]featureContextWithSegmentName)

	// Get sorted segment keys for deterministic ordering
	segmentKeys := make([]string, 0, len(ec.Segments))
	for key := range ec.Segments {
		segmentKeys = append(segmentKeys, key)
	}
	sort.Strings(segmentKeys)

	// Process segments in sorted order
	for _, key := range segmentKeys {
		segmentContext := ec.Segments[key]
		if !engine_eval.IsContextInSegment(ec, &segmentContext) {
			continue
		}

		// Add segment to results
		segmentResults = append(segmentResults, engine_eval.SegmentResult{
			Name:     segmentContext.Name,
			Metadata: segmentContext.Metadata,
		})

		// Process feature overrides for this segment
		for i := range segmentContext.Overrides {
			override := &segmentContext.Overrides[i]
			priority := getPriorityOrDefault(override.Priority)

			// Check if this override is better than what we have
			if existing, ok := featureOverrides[override.Name]; ok {
				existingPriority := getPriorityOrDefault(existing.featureContext.Priority)
				if priority <= existingPriority {
					featureOverrides[override.Name] = featureContextWithSegmentName{
						featureContext: override,
						segmentName:    segmentContext.Name,
					}
				}
			} else {
				featureOverrides[override.Name] = featureContextWithSegmentName{
					featureContext: override,
					segmentName:    segmentContext.Name,
				}
			}
		}
	}

	return segmentResults, featureOverrides
}

func getFlagResults(ec *engine_eval.EngineEvaluationContext, featureOverrides map[string]featureContextWithSegmentName) map[string]*engine_eval.FlagResult {
	flags := make(map[string]*engine_eval.FlagResult)

	// Get identity key if identity exists
	var identityKey *string
	if ec.Identity != nil {
		identityKey = &ec.Identity.Key
	}

	if ec.Features != nil {
		for featureName, featureContext := range ec.Features {
			// Check if there's an override for this feature (O(1) lookup)
			if override, ok := featureOverrides[featureName]; ok {
				reason := fmt.Sprintf("TARGETING_MATCH; segment=%s", override.segmentName)
				flags[featureName] = &engine_eval.FlagResult{
					Enabled:  override.featureContext.Enabled,
					Name:     featureName,
					Reason:   &reason,
					Value:    override.featureContext.Value,
					Metadata: override.featureContext.Metadata,
				}
			} else {
				// Use default feature context
				flagResult := getFlagResultFromFeatureContext(featureName, &featureContext, identityKey)
				flags[featureName] = &flagResult
			}
		}
	}

	return flags
}

// GetEvaluationResult computes flags and matched segments.
func GetEvaluationResult(ec *engine_eval.EngineEvaluationContext) engine_eval.EvaluationResult {
	// Process segments and get overrides
	segmentResults, featureOverrides := getMatchingSegmentsAndOverrides(ec)

	// Get flag results
	flags := getFlagResults(ec, featureOverrides)

	return engine_eval.EvaluationResult{
		Flags:    flags,
		Segments: segmentResults,
	}
}

// getFlagResultFromFeatureContext creates a FlagResult from a FeatureContext.
func getFlagResultFromFeatureContext(featureName string, featureContext *engine_eval.FeatureContext, identityKey *string) engine_eval.FlagResult {
	reason := "DEFAULT"
	value := featureContext.Value

	// Handle multivariate features
	if len(featureContext.Variants) > 0 && identityKey != nil && featureContext.Key != "" {
		// Sort variants by priority (lower priority value = higher priority)
		sortedVariants := getSortedVariantsByPriority(featureContext.Variants)

		// Calculate hash percentage for the identity and feature combination
		objectIds := []string{featureContext.Key, *identityKey}
		hashPercentage := utils.GetHashedPercentageForObjectIds(objectIds, 1)

		// Select variant based on weighted distribution
		cumulativeWeight := 0.0
		for _, variant := range sortedVariants {
			cumulativeWeight += variant.Weight
			if hashPercentage <= cumulativeWeight {
				value = variant.Value
				reason = fmt.Sprintf("SPLIT; weight=%.0f", variant.Weight)
				break
			}
		}
	}

	flagResult := engine_eval.FlagResult{
		Enabled:  featureContext.Enabled,
		Name:     featureName,
		Value:    value,
		Reason:   &reason,
		Metadata: featureContext.Metadata,
	}

	return flagResult
}

// getSortedVariantsByPriority returns a copy of variants sorted by priority (lower priority number = higher priority).
// Variants without priority are treated as having the weakest priority (placed at the end).
func getSortedVariantsByPriority(variants []engine_eval.FeatureValue) []engine_eval.FeatureValue {
	// Create a copy to avoid modifying the original slice
	sortedVariants := make([]engine_eval.FeatureValue, len(variants))
	copy(sortedVariants, variants)

	// Sort by priority (lower number = higher priority)
	sort.SliceStable(sortedVariants, func(i, j int) bool {
		// Use big.Int Cmp: returns -1 if i < j (i has higher priority)
		return sortedVariants[i].Priority.Cmp(&sortedVariants[j].Priority) < 0
	})

	return sortedVariants
}
