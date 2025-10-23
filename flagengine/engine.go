package flagengine

import (
	"fmt"
	"math"
	"sort"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

func getPriorityOrDefault(priority *float64) float64 {
	if priority != nil {
		return *priority
	}
	return math.Inf(1) // Weakest possible priority
}

func getMatchingSegments(ec *engine_eval.EngineEvaluationContext) ([]engine_eval.SegmentResult, []engine_eval.SegmentContext) {
	segmentResults := []engine_eval.SegmentResult{}
	matchedSegments := []engine_eval.SegmentContext{}

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

		// Keep track of matched segments with their overrides
		matchedSegments = append(matchedSegments, segmentContext)
	}

	return segmentResults, matchedSegments
}

func getFlagResults(ec *engine_eval.EngineEvaluationContext, matchedSegments []engine_eval.SegmentContext) map[string]*engine_eval.FlagResult {
	flags := make(map[string]*engine_eval.FlagResult)

	// Get identity key if identity exists
	var identityKey *string
	if ec.Identity != nil {
		identityKey = &ec.Identity.Key
	}

	if ec.Features != nil {
		for featureName, featureContext := range ec.Features {
			// Find the best override for this feature from matched segments
			var bestOverride *engine_eval.FeatureContext
			var bestPriority = math.Inf(1)
			var segmentName string

			for _, segment := range matchedSegments {
				for i := range segment.Overrides {
					override := &segment.Overrides[i]
					// Match by feature name (use the map key, not featureContext.Name)
					if override.Name == featureName {
						priority := getPriorityOrDefault(override.Priority)
						if priority <= bestPriority {
							bestOverride = override
							bestPriority = priority
							segmentName = segment.Name
						}
					}
				}
			}

			// Use override if found, otherwise use default
			if bestOverride != nil {
				reason := fmt.Sprintf("TARGETING_MATCH; segment=%s", segmentName)
				flags[featureName] = &engine_eval.FlagResult{
					Enabled:  bestOverride.Enabled,
					Name:     featureName,
					Reason:   &reason,
					Value:    bestOverride.Value,
					Metadata: bestOverride.Metadata,
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
	// Process segments
	segmentResults, matchedSegments := getMatchingSegments(ec)

	// Get flag results
	flags := getFlagResults(ec, matchedSegments)

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
