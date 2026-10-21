package flagengine

import (
	"fmt"
	"math"
	"sort"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

const controlVariantKey = "control"

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

func getMatchingSegmentsAndOverrides(ec *engine_eval.EngineEvaluationContext, resolver *flagDependencyResolver) ([]engine_eval.SegmentResult, map[string]featureContextWithSegmentName) {
	segmentResults := []engine_eval.SegmentResult{}
	featureOverrides := make(map[string]featureContextWithSegmentName)

	// Process segments in deterministic order (sorted by key)
	for _, segmentKey := range getSortedSegmentKeys(ec.Segments) {
		if !resolver.matchesSegment(segmentKey) {
			continue
		}
		segmentContext := ec.Segments[segmentKey]

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

// getSortedSegmentKeys returns segment keys sorted for deterministic ordering.
func getSortedSegmentKeys(segments map[string]engine_eval.SegmentContext) []string {
	keys := make([]string, 0, len(segments))
	for key := range segments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// applySegmentOverrides updates the feature overrides map with this segment's overrides,
// only replacing existing overrides if the new one has equal or higher priority.
func applySegmentOverrides(segment *engine_eval.SegmentContext, featureOverrides map[string]featureContextWithSegmentName) {
	for i := range segment.Overrides {
		override := &segment.Overrides[i]
		if existing, exists := featureOverrides[override.Name]; exists && !overrideTakesPrecedence(override, existing.featureContext) {
			continue
		}
		featureOverrides[override.Name] = featureContextWithSegmentName{
			featureContext: override,
			segmentName:    segment.Name,
		}
	}
}

// overrideTakesPrecedence reports whether a segment override replaces the one held so far,
// i.e. whether it has equal or higher priority.
func overrideTakesPrecedence(candidate, incumbent *engine_eval.FeatureContext) bool {
	return getPriorityOrDefault(candidate.Priority) <= getPriorityOrDefault(incumbent.Priority)
}

// getIdentityKey returns the identity key used for multivariate bucketing, or nil without an identity.
func getIdentityKey(ec *engine_eval.EngineEvaluationContext) *string {
	if ec.Identity == nil {
		return nil
	}
	// If identity key is not provided, construct it from environment key and identifier
	if ec.Identity.Key == "" {
		constructedKey := ec.Environment.Key + "_" + ec.Identity.Identifier
		return &constructedKey
	}
	return &ec.Identity.Key
}

func getFlagResults(ec *engine_eval.EngineEvaluationContext, featureOverrides map[string]featureContextWithSegmentName) map[string]*engine_eval.FlagResult {
	flags := make(map[string]*engine_eval.FlagResult)
	identityKey := getIdentityKey(ec)

	if ec.Features != nil {
		for featureName, featureContext := range ec.Features {
			reason := "DEFAULT"
			// Check if there's an override for this feature
			if override, ok := featureOverrides[featureName]; ok {
				featureContext = *override.featureContext
				reason = fmt.Sprintf("TARGETING_MATCH; segment=%s", override.segmentName)
			}
			flagResult := getFlagResultFromFeatureContext(featureName, &featureContext, identityKey, reason)
			flags[featureName] = &flagResult
		}
	}

	return flags
}

// GetEvaluationResult computes flags and matched segments.
func GetEvaluationResult(ec *engine_eval.EngineEvaluationContext) engine_eval.EvaluationResult {
	resolver := newFlagDependencyResolver(ec)

	// Process segments and get overrides
	segmentResults, featureOverrides := getMatchingSegmentsAndOverrides(ec, resolver)

	// Get flag results
	flags := getFlagResults(ec, featureOverrides)

	// Flags read by segment conditions were resolved with the cycle guard, so take precedence
	for featureName, flagResult := range resolver.resolved {
		flags[featureName] = flagResult
	}
	for featureName := range resolver.cyclic {
		if flagResult, ok := flags[featureName]; ok {
			flagResult.Reason = circularDependencyReason
		}
	}

	return engine_eval.EvaluationResult{
		Flags:    flags,
		Segments: segmentResults,
	}
}

// getFlagResultFromFeatureContext creates a FlagResult from a FeatureContext.
func getFlagResultFromFeatureContext(featureName string, featureContext *engine_eval.FeatureContext, identityKey *string, reason string) engine_eval.FlagResult {
	value := featureContext.Value
	variantKey := ""

	// Handle multivariate features
	if len(featureContext.Variants) > 0 && identityKey != nil && featureContext.Key != "" {
		// Default to the control bucket; a selected variant overrides this
		variantKey = controlVariantKey

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
				variantKey = variant.Key
				reason = fmt.Sprintf("SPLIT; weight=%g", variant.Weight)
				break
			}
		}
	}

	flagResult := engine_eval.FlagResult{
		Enabled:  featureContext.Enabled,
		Name:     featureName,
		Value:    value,
		Variant:  variantKey,
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
