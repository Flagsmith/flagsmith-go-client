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

	// Process segments in deterministic order (sorted by key)
	for _, segmentContext := range getSortedSegments(ec.Segments) {
		if !engine_eval.IsContextInSegment(ec, &segmentContext) {
			continue
		}

		// Record matched segment
		segmentResults = append(segmentResults, engine_eval.SegmentResult{
			Name:     segmentContext.Name,
			Metadata: segmentContext.Metadata,
		})

		// Apply segment's feature overrides (respecting priority)
		applySegmentOverrides(&segmentContext, featureOverrides)
	}

	return segmentResults, featureOverrides
}

// getSortedSegments returns segments sorted by their keys for deterministic ordering.
func getSortedSegments(segments map[string]engine_eval.SegmentContext) []engine_eval.SegmentContext {
	keys := make([]string, 0, len(segments))
	for key := range segments {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sorted := make([]engine_eval.SegmentContext, 0, len(keys))
	for _, key := range keys {
		sorted = append(sorted, segments[key])
	}
	return sorted
}

// applySegmentOverrides updates the feature overrides map with this segment's overrides,
// only replacing existing overrides if the new one has equal or higher priority.
func applySegmentOverrides(segment *engine_eval.SegmentContext, featureOverrides map[string]featureContextWithSegmentName) {
	for i := range segment.Overrides {
		override := &segment.Overrides[i]
		newPriority := getPriorityOrDefault(override.Priority)

		// Check if we should use this override
		if existing, exists := featureOverrides[override.Name]; exists {
			existingPriority := getPriorityOrDefault(existing.featureContext.Priority)
			if newPriority > existingPriority {
				continue // Existing override has higher priority
			}
		}

		// Use this override (either it's new or has equal/higher priority)
		featureOverrides[override.Name] = featureContextWithSegmentName{
			featureContext: override,
			segmentName:    segment.Name,
		}
	}
}

func getFlagResults(ec *engine_eval.EngineEvaluationContext, featureOverrides map[string]featureContextWithSegmentName) map[string]*engine_eval.FlagResult {
	flags := make(map[string]*engine_eval.FlagResult)

	// Get identity key if identity exists
	var identityKey *string
	if ec.Identity != nil {
		// If identity key is not provided, construct it from environment key and identifier
		if ec.Identity.Key == "" {
			constructedKey := ec.Environment.Key + "_" + ec.Identity.Identifier
			identityKey = &constructedKey
		} else {
			identityKey = &ec.Identity.Key
		}
	}

	if ec.Features != nil {
		for featureName, featureContext := range ec.Features {
			// Check if there's an override for this feature
			if override, ok := featureOverrides[featureName]; ok {
				flags[featureName] = &engine_eval.FlagResult{
					Enabled:  override.featureContext.Enabled,
					Name:     featureName,
					Reason:   fmt.Sprintf("TARGETING_MATCH; segment=%s", override.segmentName),
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
				reason = fmt.Sprintf("SPLIT; weight=%g", variant.Weight)
				break
			}
		}
	}

	flagResult := engine_eval.FlagResult{
		Enabled:  featureContext.Enabled,
		Name:     featureName,
		Value:    value,
		Reason:   reason,
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
