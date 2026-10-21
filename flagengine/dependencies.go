package flagengine

import (
	"fmt"
	"slices"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
)

const circularDependencyReason = "ERROR; code=CIRCULAR_DEPENDENCY"

// flagDependencyResolver resolves the flags read by segment conditions on `$.flags`,
// following dependencies transitively and memoising the results for one evaluation.
type flagDependencyResolver struct {
	ec          *engine_eval.EngineEvaluationContext
	identityKey *string

	segmentKeysByFeatureName map[string][]string
	segmentMatches           map[string]bool
	resolving                []string

	// Flag results resolved so far, mapped by feature name.
	resolved map[string]*engine_eval.FlagResult
	// Names of the features found to be in a dependency cycle.
	cyclic map[string]struct{}
	// Incremented whenever a cycle is cut. A segment whose evaluation increments it
	// read a flag that could not be resolved, so it must not match.
	cycleHits int
}

func newFlagDependencyResolver(ec *engine_eval.EngineEvaluationContext) *flagDependencyResolver {
	segmentKeysByFeatureName := make(map[string][]string)
	for _, segmentKey := range getSortedSegmentKeys(ec.Segments) {
		for _, override := range ec.Segments[segmentKey].Overrides {
			segmentKeysByFeatureName[override.Name] = append(segmentKeysByFeatureName[override.Name], segmentKey)
		}
	}
	return &flagDependencyResolver{
		ec:                       ec,
		identityKey:              getIdentityKey(ec),
		segmentKeysByFeatureName: segmentKeysByFeatureName,
		segmentMatches:           make(map[string]bool),
		resolved:                 make(map[string]*engine_eval.FlagResult),
		cyclic:                   make(map[string]struct{}),
	}
}

// ResolveFlag implements engine_eval.FlagResolver.
func (r *flagDependencyResolver) ResolveFlag(featureName string) *engine_eval.FlagResult {
	if _, ok := r.resolved[featureName]; !ok {
		r.resolveFeature(featureName)
	}
	return r.resolved[featureName]
}

func (r *flagDependencyResolver) resolveFeature(featureName string) {
	if cycleStart := slices.Index(r.resolving, featureName); cycleStart >= 0 {
		for _, name := range r.resolving[cycleStart:] {
			r.cyclic[name] = struct{}{}
		}
		r.cycleHits++
		return
	}
	featureContext, ok := r.ec.Features[featureName]
	if !ok {
		return
	}

	r.resolving = append(r.resolving, featureName)
	override, hasOverride := r.getSegmentOverride(featureName)
	r.resolving = r.resolving[:len(r.resolving)-1]

	if _, ok := r.cyclic[featureName]; ok {
		return
	}

	reason := "DEFAULT"
	if hasOverride {
		featureContext = *override.featureContext
		reason = fmt.Sprintf("TARGETING_MATCH; segment=%s", override.segmentName)
	}
	flagResult := getFlagResultFromFeatureContext(featureName, &featureContext, r.identityKey, reason)
	r.resolved[featureName] = &flagResult
}

// matchesSegment reports whether the context matches a segment, resolving the flags it depends on.
func (r *flagDependencyResolver) matchesSegment(segmentKey string) bool {
	if matches, ok := r.segmentMatches[segmentKey]; ok {
		return matches
	}

	cycleHits := r.cycleHits
	segmentContext := r.ec.Segments[segmentKey]
	matches := engine_eval.IsContextInSegmentWithFlags(r.ec, &segmentContext, r)
	if r.cycleHits != cycleHits {
		return false
	}

	r.segmentMatches[segmentKey] = matches
	return matches
}

func (r *flagDependencyResolver) getSegmentOverride(featureName string) (featureContextWithSegmentName, bool) {
	var segmentOverride featureContextWithSegmentName
	found := false

	for _, segmentKey := range r.segmentKeysByFeatureName[featureName] {
		if !r.matchesSegment(segmentKey) {
			continue
		}
		segmentContext := r.ec.Segments[segmentKey]
		for i := range segmentContext.Overrides {
			override := &segmentContext.Overrides[i]
			if override.Name != featureName {
				continue
			}
			if found && !overrideTakesPrecedence(override, segmentOverride.featureContext) {
				continue
			}
			segmentOverride = featureContextWithSegmentName{featureContext: override, segmentName: segmentContext.Name}
			found = true
		}
	}

	return segmentOverride, found
}
