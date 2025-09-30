package flagengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils"
	"github.com/blang/semver/v4"
	"github.com/ohler55/ojg/jp"
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
		if !isContextInSegment(ec, &segmentContext) {
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
func isContextInSegment(ec *engine_eval.EngineEvaluationContext, segmentContext *engine_eval.SegmentContext) bool {
	if len(segmentContext.Rules) == 0 {
		return false
	}
	for i := range segmentContext.Rules {
		if !contextMatchesSegmentRule(ec, &segmentContext.Rules[i], segmentContext.Key) {
			return false
		}
	}
	return true
}
func contextMatchesCondition(ec *engine_eval.EngineEvaluationContext, segmentCondition *engine_eval.Condition, segmentKey string) bool {
	var contextValue engine_eval.ContextValue
	if segmentCondition.Property != "" {
		contextValue = getContextValue(ec, segmentCondition.Property)
	}
	if segmentCondition.Operator == engine_eval.PercentageSplit {
		var objectIds []string
		if contextValue != nil {
			// Try to get string representation of the context value
			var strValue string
			switch v := contextValue.(type) {
			case string:
				strValue = v
			case *engine_eval.Value:
				if v != nil && v.String != nil {
					strValue = *v.String
				} else {
					return false
				}
			default:
				return false
			}
			objectIds = []string{segmentKey, strValue}
		} else if ec.Identity != nil {
			objectIds = []string{segmentKey, ec.Identity.Key}
		} else {
			return false
		}
		if segmentCondition.Value != nil && segmentCondition.Value.String != nil {
			floatValue, _ := strconv.ParseFloat(*segmentCondition.Value.String, 64)
			return utils.GetHashedPercentageForObjectIds(objectIds, 1) <= floatValue
		}
		return false
	}
	if segmentCondition.Operator == engine_eval.IsNotSet {
		return contextValue == nil
	}
	if segmentCondition.Operator == engine_eval.IsSet {
		return contextValue != nil
	}
	if contextValue != nil {
		return match(segmentCondition.Operator, ToString(contextValue), *segmentCondition.Value.String)
	}
	return false
}

func ToString(contextValue engine_eval.ContextValue) string {
	if s, ok := contextValue.(string); ok {
		return s
	}
	// Handle *engine_eval.Value type
	if v, ok := contextValue.(*engine_eval.Value); ok && v != nil {
		if v.String != nil {
			return *v.String
		}
		if v.Bool != nil {
			return strconv.FormatBool(*v.Bool)
		}
		if v.Double != nil {
			return strconv.FormatFloat(*v.Double, 'f', -1, 64)
		}
	}
	return fmt.Sprint(contextValue)
}

func match(c engine_eval.Operator, traitValue, conditionValue string) bool {
	b1, e1 := strconv.ParseBool(traitValue)
	b2, e2 := strconv.ParseBool(conditionValue)
	if e1 == nil && e2 == nil {
		return matchBool(c, b1, b2)
	}

	i1, e1 := strconv.ParseInt(traitValue, 10, 64)
	i2, e2 := strconv.ParseInt(conditionValue, 10, 64)
	if e1 == nil && e2 == nil {
		return matchInt(c, i1, i2)
	}

	f1, e1 := strconv.ParseFloat(traitValue, 64)
	f2, e2 := strconv.ParseFloat(conditionValue, 64)
	if e1 == nil && e2 == nil {
		return matchFloat(c, f1, f2)
	}
	if strings.HasSuffix(conditionValue, ":semver") {
		conditionVersion, err := semver.Make(conditionValue[:len(conditionValue)-7])
		if err != nil {
			return false
		}
		return matchSemver(c, traitValue, conditionVersion)
	}

	return matchString(c, traitValue, conditionValue)
}
func matchSemver(c engine_eval.Operator, traitValue string, conditionVersion semver.Version) bool {
	traitVersion, err := semver.Make(traitValue)
	if err != nil {
		return false
	}
	switch c {
	case engine_eval.Equal:
		return traitVersion.EQ(conditionVersion)
	case engine_eval.GreaterThan:
		return traitVersion.GT(conditionVersion)
	case engine_eval.LessThan:
		return traitVersion.LT(conditionVersion)
	case engine_eval.LessThanInclusive:
		return traitVersion.LTE(conditionVersion)
	case engine_eval.GreaterThanInclusive:
		return traitVersion.GE(conditionVersion)
	case engine_eval.NotEqual:
		return traitVersion.NE(conditionVersion)
	}
	return false
}

func matchBool(c engine_eval.Operator, v1, v2 bool) bool {
	var i1, i2 int64
	if v1 {
		i1 = 1
	}
	if v2 {
		i2 = 1
	}
	return matchInt(c, i1, i2)
}
func matchInt(c engine_eval.Operator, v1, v2 int64) bool {
	switch c {
	case engine_eval.Equal:
		return v1 == v2
	case engine_eval.GreaterThan:
		return v1 > v2
	case engine_eval.LessThan:
		return v1 < v2
	case engine_eval.LessThanInclusive:
		return v1 <= v2
	case engine_eval.GreaterThanInclusive:
		return v1 >= v2
	case engine_eval.NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchFloat(c engine_eval.Operator, v1, v2 float64) bool {
	switch c {
	case engine_eval.Equal:
		return v1 == v2
	case engine_eval.GreaterThan:
		return v1 > v2
	case engine_eval.LessThan:
		return v1 < v2
	case engine_eval.LessThanInclusive:
		return v1 <= v2
	case engine_eval.GreaterThanInclusive:
		return v1 >= v2
	case engine_eval.NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchString(c engine_eval.Operator, v1, v2 string) bool {
	switch c {
	case engine_eval.Contains:
		return strings.Contains(v1, v2)
	case engine_eval.NotContains:
		return !strings.Contains(v1, v2)
	case engine_eval.In:
		return slices.Contains(strings.Split(v2, ","), v1)
	case engine_eval.Equal:
		return v1 == v2
	case engine_eval.GreaterThan:
		return v1 > v2
	case engine_eval.LessThan:
		return v1 < v2
	case engine_eval.LessThanInclusive:
		return v1 <= v2
	case engine_eval.GreaterThanInclusive:
		return v1 >= v2
	case engine_eval.NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func getContextValue(ec *engine_eval.EngineEvaluationContext, property string) engine_eval.ContextValue {
	if strings.HasPrefix(property, "$.") {
		return getContextValueGetter(property)(ec)
	} else if ec.Identity != nil {
		if ec.Identity.Traits != nil {
			value, exists := ec.Identity.Traits[property]
			if exists {
				return value
			}
		}
	}
	return nil
}

func contextMatchesSegmentRule(ec *engine_eval.EngineEvaluationContext, segmentRule *engine_eval.SegmentRule, segmentKey string) bool {
	matchesConditions := true
	if len(segmentRule.Conditions) > 0 {
		conditions := make([]bool, len(segmentRule.Conditions))
		for i := range segmentRule.Conditions {
			conditions[i] = contextMatchesCondition(ec, &segmentRule.Conditions[i], segmentKey)
		}
		switch segmentRule.Type {
		case engine_eval.All:
			matchesConditions = utils.All(conditions)
		case engine_eval.Any:
			matchesConditions = utils.Any(conditions)
		default:
			matchesConditions = utils.None(conditions)
		}
	}

	if !matchesConditions {
		return false
	}

	for i := range segmentRule.Rules {
		if !contextMatchesSegmentRule(ec, &segmentRule.Rules[i], segmentKey) {
			return false
		}
	}
	return true
}

// getContextValueGetter returns a cached function to retrieve a value from a map[string]any
// using either a JSONPath expression or a fallback trait key.
func getContextValueGetter(property string) func(ec *engine_eval.EngineEvaluationContext) any {
	// First, try to parse the property as a JSONPath expression.
	p, err := jp.ParseString(property)
	if err == nil {
		// If successful, create and cache a getter for the JSONPath.
		getter := func(evalCtx *engine_eval.EngineEvaluationContext) any {
			// Convert the struct to a map for JSONPath evaluation
			data := map[string]interface{}{
				"environment": map[string]interface{}{
					"key":  evalCtx.Environment.Key,
					"name": evalCtx.Environment.Name,
				},
			}

			if evalCtx.Identity != nil {
				identityMap := map[string]interface{}{
					"identifier": evalCtx.Identity.Identifier,
					"key":        evalCtx.Identity.Key,
				}
				if evalCtx.Identity.Traits != nil {
					traits := make(map[string]interface{})
					for k, v := range evalCtx.Identity.Traits {
						if v != nil {
							if v.String != nil {
								traits[k] = *v.String
							} else if v.Bool != nil {
								traits[k] = *v.Bool
							} else if v.Double != nil {
								traits[k] = *v.Double
							}
						}
					}
					identityMap["traits"] = traits
				}
				data["identity"] = identityMap
			}

			results := p.Get(data)
			// jp.Get returns []any - if we have one result, return it
			if len(results) == 1 {
				return results[0]
			} else if len(results) == 0 {
				return nil
			}
			// Return the first result if multiple
			return results[0]
		}
		return getter
	}

	// Fallback: Treat the property as a trait key under $.identity.traits.
	// This handles cases where the property isn't a valid JSONPath.
	fallbackPath := `$.identity.traits["` + escapeDoubleQuotes(property) + `"]`

	p, err = jp.ParseString(fallbackPath)
	if err == nil {
		// Create and cache the fallback getter.
		getter := func(evalCtx *engine_eval.EngineEvaluationContext) any {
			// Convert the struct to a map for JSONPath evaluation
			data := map[string]interface{}{}

			if evalCtx.Identity != nil && evalCtx.Identity.Traits != nil {
				traits := make(map[string]interface{})
				for k, v := range evalCtx.Identity.Traits {
					if v != nil {
						if v.String != nil {
							traits[k] = *v.String
						} else if v.Bool != nil {
							traits[k] = *v.Bool
						} else if v.Double != nil {
							traits[k] = *v.Double
						}
					}
				}
				data["identity"] = map[string]interface{}{
					"traits": traits,
				}
			}

			results := p.Get(data)
			// jp.Get returns []any - if we have one result, return it
			if len(results) == 1 {
				return results[0]
			} else if len(results) == 0 {
				return nil
			}
			// Return the first result if multiple
			return results[0]
		}
		return getter
	}

	// If neither parsing method works, return a function that always returns nil.
	getter := func(evalCtx *engine_eval.EngineEvaluationContext) any {
		return nil
	}
	return getter
}

func escapeDoubleQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}
