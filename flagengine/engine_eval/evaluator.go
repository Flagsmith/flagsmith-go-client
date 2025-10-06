package engine_eval

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
	"github.com/ohler55/ojg/jp"
)

// IsContextInSegment determines if the given evaluation context matches the segment rules.
func IsContextInSegment(ec *EngineEvaluationContext, segmentContext *SegmentContext) bool {
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

func contextMatchesSegmentRule(ec *EngineEvaluationContext, segmentRule *SegmentRule, segmentKey string) bool {
	matchesConditions := true

	if len(segmentRule.Conditions) > 0 {
		switch segmentRule.Type {
		case All:
			// Short-circuit on first false
			for i := range segmentRule.Conditions {
				if !contextMatchesCondition(ec, &segmentRule.Conditions[i], segmentKey) {
					matchesConditions = false
					break
				}
			}
		case Any:
			// Short-circuit on first true
			matchesConditions = false
			for i := range segmentRule.Conditions {
				if contextMatchesCondition(ec, &segmentRule.Conditions[i], segmentKey) {
					matchesConditions = true
					break
				}
			}
		case None:
			// Short-circuit on first true
			for i := range segmentRule.Conditions {
				if contextMatchesCondition(ec, &segmentRule.Conditions[i], segmentKey) {
					matchesConditions = false
					break
				}
			}
		default:
			return false
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

// matchPercentageSplit handles the PercentageSplit operator for segment conditions.
func matchPercentageSplit(ec *EngineEvaluationContext, segmentCondition *Condition, segmentKey string, contextValue ContextValue) bool {
	var objectIds []string

	if contextValue != nil {
		// Try to get string representation of the context value
		var strValue string
		switch v := contextValue.(type) {
		case string:
			strValue = v
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

func contextMatchesCondition(ec *EngineEvaluationContext, segmentCondition *Condition, segmentKey string) bool {
	var contextValue ContextValue
	if segmentCondition.Property != "" {
		contextValue = getContextValue(ec, segmentCondition.Property)
	}
	if segmentCondition.Operator == PercentageSplit {
		return matchPercentageSplit(ec, segmentCondition, segmentKey, contextValue)
	}
	if segmentCondition.Operator == In {
		return matchInOperator(segmentCondition, contextValue)
	}
	if segmentCondition.Operator == IsNotSet {
		return contextValue == nil
	}
	if segmentCondition.Operator == IsSet {
		return contextValue != nil
	}
	if contextValue != nil {
		return parseAndMatch(segmentCondition.Operator, ToString(contextValue), *segmentCondition.Value.String)
	}
	return false
}

// matchInOperator handles the IN operator for segment conditions, supporting both StringArray and comma-separated strings.
func matchInOperator(segmentCondition *Condition, contextValue ContextValue) bool {
	if contextValue == nil {
		return false
	}

	traitValue := ToString(contextValue)

	// First try to use StringArray if available
	if segmentCondition.Value != nil && len(segmentCondition.Value.StringArray) > 0 {
		return slices.Contains(segmentCondition.Value.StringArray, traitValue)
	}

	// Fall back to comma-separated string approach
	if segmentCondition.Value != nil && segmentCondition.Value.String != nil {
		values := strings.Split(*segmentCondition.Value.String, ",")
		return slices.Contains(values, traitValue)
	}

	return false
}

func getContextValue(ec *EngineEvaluationContext, property string) ContextValue {
	if strings.HasPrefix(property, "$.") {
		return getContextValueGetter(property)(ec)
	} else if ec.Identity != nil && ec.Identity.Traits != nil {
		value, exists := ec.Identity.Traits[property]
		if exists {
			return value
		}
	}
	return nil
}

// getContextValueGetter returns a function to retrieve a value from the evaluation context
// using either a JSONPath expression or returning nil if the property is not a valid JSONPath.
func getContextValueGetter(property string) func(ec *EngineEvaluationContext) any {
	// First, try to parse the property as a JSONPath expression.
	p, err := jp.ParseString(property)
	if err == nil {
		// If successful, create a getter for the JSONPath.
		getter := func(evalCtx *EngineEvaluationContext) any {
			results := p.Get(evalCtx)
			if len(results) > 0 {
				return results[0]
			}
			return nil
		}
		return getter
	}

	// If JSONPath parsing fails, return a getter that always returns nil.
	return func(ec *EngineEvaluationContext) any {
		return nil
	}
}

func ToString(contextValue ContextValue) string {
	if s, ok := contextValue.(string); ok {
		return s
	}
	if b, ok := contextValue.(bool); ok {
		return strconv.FormatBool(b)
	}
	if f, ok := contextValue.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if i, ok := contextValue.(int); ok {
		return strconv.Itoa(i)
	}
	return fmt.Sprint(contextValue)
}
