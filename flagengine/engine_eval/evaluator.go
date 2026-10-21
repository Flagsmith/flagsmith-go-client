package engine_eval

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
	"github.com/ohler55/ojg/jp"
)

const flagsContextKey = "flags"

// FlagResolver resolves the flags read by segment conditions on `$.flags.<feature name>`.
type FlagResolver interface {
	// ResolveFlag returns the flag result for the named feature, or nil if it cannot be resolved.
	ResolveFlag(featureName string) *FlagResult
}

// IsContextInSegment determines if the given evaluation context matches the segment rules.
// Conditions on `$.flags` never resolve; use IsContextInSegmentWithFlags to evaluate them.
func IsContextInSegment(ec *EngineEvaluationContext, segmentContext *SegmentContext) bool {
	return IsContextInSegmentWithFlags(ec, segmentContext, nil)
}

// IsContextInSegmentWithFlags determines if the given evaluation context matches the segment rules,
// reading conditions on `$.flags.<feature name>` from flags.
func IsContextInSegmentWithFlags(ec *EngineEvaluationContext, segmentContext *SegmentContext, flags FlagResolver) bool {
	if len(segmentContext.Rules) == 0 {
		return false
	}
	for i := range segmentContext.Rules {
		if !contextMatchesSegmentRule(ec, flags, &segmentContext.Rules[i], segmentContext.Key) {
			return false
		}
	}
	return true
}

// Returns true if conditions match according to the rule type.
func matchesConditionsByRuleType(ec *EngineEvaluationContext, flags FlagResolver, conditions []Condition, ruleType Type, segmentKey string) bool {
	for i := range conditions {
		conditionMatches := contextMatchesCondition(ec, flags, &conditions[i], segmentKey)

		switch ruleType {
		case All:
			if !conditionMatches {
				return false // Short-circuit: ALL requires all conditions to match
			}
		case None:
			if conditionMatches {
				return false // Short-circuit: NONE requires no conditions to match
			}
		case Any:
			if conditionMatches {
				return true // Short-circuit: ANY requires at least one condition to match
			}
		default:
			return false
		}
	}

	// If we reach here: ALL/NONE passed all checks, ANY found no matches
	return ruleType != Any
}

func contextMatchesSegmentRule(ec *EngineEvaluationContext, flags FlagResolver, segmentRule *SegmentRule, segmentKey string) bool {
	if len(segmentRule.Conditions) > 0 {
		if !matchesConditionsByRuleType(ec, flags, segmentRule.Conditions, segmentRule.Type, segmentKey) {
			return false
		}
	}

	return matchesSubRulesByRuleType(ec, flags, segmentRule.Rules, segmentRule.Type, segmentKey)
}

// matchesSubRulesByRuleType evaluates sub-rules using the parent rule's type (ALL/ANY/NONE).
func matchesSubRulesByRuleType(ec *EngineEvaluationContext, flags FlagResolver, subRules []SegmentRule, ruleType Type, segmentKey string) bool {
	if len(subRules) == 0 {
		return true
	}
	for i := range subRules {
		subRuleMatches := contextMatchesSegmentRule(ec, flags, &subRules[i], segmentKey)
		switch ruleType {
		case All:
			if !subRuleMatches {
				return false
			}
		case None:
			if subRuleMatches {
				return false
			}
		case Any:
			if subRuleMatches {
				return true
			}
		default:
			return false
		}
	}
	return ruleType != Any
}

func matchPercentageSplit(ec *EngineEvaluationContext, segmentCondition *Condition, segmentKey string, contextValue ContextValue) bool {
	var objectIds []string

	if contextValue != nil {
		strValue := ToString(contextValue)
		objectIds = []string{segmentKey, strValue}
	} else if ec.Identity != nil {
		objectIds = []string{segmentKey, ec.Identity.Key}
	} else {
		return false
	}

	if segmentCondition.Value != nil {
		if strValue, ok := segmentCondition.Value.(string); ok {
			floatValue, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return false
			}
			return utils.GetHashedPercentageForObjectIds(objectIds, 1) <= floatValue
		}
	}
	return false
}

func contextMatchesCondition(ec *EngineEvaluationContext, flags FlagResolver, segmentCondition *Condition, segmentKey string) bool {
	var contextValue ContextValue
	if segmentCondition.Property != "" {
		contextValue = getContextValue(ec, flags, segmentCondition.Property)
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
	if contextValue != nil && segmentCondition.Value != nil {
		if strValue, ok := segmentCondition.Value.(string); ok {
			return parseAndMatch(segmentCondition.Operator, ToString(contextValue), strValue)
		}
	}
	return false
}

// matchInOperator handles the IN operator for segment conditions, supporting both StringArray and comma-separated strings.
func matchInOperator(segmentCondition *Condition, contextValue ContextValue) bool {
	if contextValue == nil {
		return false
	}

	traitValue := ToString(contextValue)

	if segmentCondition.Value == nil {
		return false
	}

	// First try to use []string if available
	if strArray, ok := segmentCondition.Value.([]string); ok {
		return slices.Contains(strArray, traitValue)
	}

	// Convert []interface{} to []string (happens during JSON unmarshaling)
	if ifaceArray, ok := segmentCondition.Value.([]interface{}); ok {
		for _, v := range ifaceArray {
			if str, ok := v.(string); ok && str == traitValue {
				return true
			}
		}
		return false
	}

	// Fall back to string - try JSON parsing first, then comma-separated
	if strValue, ok := segmentCondition.Value.(string); ok {
		// Try to parse as JSON array first
		var jsonArray []string
		if err := json.Unmarshal([]byte(strValue), &jsonArray); err == nil {
			return slices.Contains(jsonArray, traitValue)
		}

		// Fall back to comma-separated string
		values := strings.Split(strValue, ",")
		return slices.Contains(values, traitValue)
	}

	return false
}

func getContextValue(ec *EngineEvaluationContext, flags FlagResolver, property string) ContextValue {
	if strings.HasPrefix(property, "$.") {
		value := getContextValueGetter(property, flags)(ec)
		// Only use JSONPath result if it's a primitive value (not an object/array/map)
		if value != nil && isPrimitive(value) {
			return value
		}
		// If JSONPath returned non-primitive or nil, fall back to checking traits by exact key name
	}

	// Check traits by property name (handles both regular traits and invalid JSONPath strings)
	if ec.Identity != nil && ec.Identity.Traits != nil {
		value, exists := ec.Identity.Traits[property]
		if exists {
			return value
		}
	}
	return nil
}

// isPrimitive checks if a value is a primitive type (string, number, bool, nil)
// Objects, arrays, and maps are not considered primitive.
func isPrimitive(value any) bool {
	if value == nil {
		return true
	}
	switch value.(type) {
	case string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

// getContextValueGetter returns a function to retrieve a value from the evaluation context
// using either a JSONPath expression or returning nil if the property is not a valid JSONPath.
// Expressions selecting `$.flags.<feature name>` read the flag result resolved by flags.
func getContextValueGetter(property string, flags FlagResolver) func(ec *EngineEvaluationContext) any {
	// First, try to parse the property as a JSONPath expression.
	p, err := jp.ParseString(property)
	if featureName, ok := getFlagDependency(p); err == nil && ok {
		return func(*EngineEvaluationContext) any {
			if flags == nil {
				return nil
			}
			flag := flags.ResolveFlag(featureName)
			if flag == nil {
				return nil
			}
			results := append(jp.R(), p[3:]...).Get(flagResultNode(flag))
			if len(results) > 0 {
				return results[0]
			}
			return nil
		}
	}
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

// getFlagDependency returns the feature name a JSONPath expression selects under `$.flags`, if any.
func getFlagDependency(p jp.Expr) (string, bool) {
	if len(p) < 3 || p[1] != jp.Child(flagsContextKey) {
		return "", false
	}
	featureName, ok := p[2].(jp.Child)
	return string(featureName), ok
}

// flagResultNode returns the JSON representation of a flag result, as read by `$.flags` conditions.
func flagResultNode(flag *FlagResult) map[string]any {
	var variant any
	if flag.Variant != "" {
		variant = flag.Variant
	}
	return map[string]any{
		"enabled": flag.Enabled,
		"name":    flag.Name,
		"reason":  flag.Reason,
		"value":   flag.Value,
		"variant": variant,
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
