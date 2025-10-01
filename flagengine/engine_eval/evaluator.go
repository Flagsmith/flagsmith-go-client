package engine_eval

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
	"github.com/blang/semver/v4"
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
		conditions := make([]bool, len(segmentRule.Conditions))
		for i := range segmentRule.Conditions {
			conditions[i] = contextMatchesCondition(ec, &segmentRule.Conditions[i], segmentKey)
		}
		switch segmentRule.Type {
		case All:
			matchesConditions = utils.All(conditions)
		case Any:
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

func contextMatchesCondition(ec *EngineEvaluationContext, segmentCondition *Condition, segmentKey string) bool {
	var contextValue ContextValue
	if segmentCondition.Property != "" {
		contextValue = getContextValue(ec, segmentCondition.Property)
	}
	if segmentCondition.Operator == PercentageSplit {
		var objectIds []string
		if contextValue != nil {
			// Try to get string representation of the context value
			var strValue string
			switch v := contextValue.(type) {
			case string:
				strValue = v
			case *Value:
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
	if segmentCondition.Operator == IsNotSet {
		return contextValue == nil
	}
	if segmentCondition.Operator == IsSet {
		return contextValue != nil
	}
	if contextValue != nil {
		return match(segmentCondition.Operator, ToString(contextValue), *segmentCondition.Value.String)
	}
	return false
}

func getContextValue(ec *EngineEvaluationContext, property string) ContextValue {
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
	// Handle *Value type
	if v, ok := contextValue.(*Value); ok && v != nil {
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

func match(c Operator, traitValue, conditionValue string) bool {
	// Handle special operators first
	switch c {
	case Modulo:
		return matchModulo(traitValue, conditionValue)
	case Regex:
		return matchRegex(traitValue, conditionValue)
	}

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

func matchSemver(c Operator, traitValue string, conditionVersion semver.Version) bool {
	traitVersion, err := semver.Make(traitValue)
	if err != nil {
		return false
	}
	switch c {
	case Equal:
		return traitVersion.EQ(conditionVersion)
	case GreaterThan:
		return traitVersion.GT(conditionVersion)
	case LessThan:
		return traitVersion.LT(conditionVersion)
	case LessThanInclusive:
		return traitVersion.LTE(conditionVersion)
	case GreaterThanInclusive:
		return traitVersion.GE(conditionVersion)
	case NotEqual:
		return traitVersion.NE(conditionVersion)
	}
	return false
}

func matchBool(c Operator, v1, v2 bool) bool {
	var i1, i2 int64
	if v1 {
		i1 = 1
	}
	if v2 {
		i2 = 1
	}
	return matchInt(c, i1, i2)
}

func matchInt(c Operator, v1, v2 int64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchFloat(c Operator, v1, v2 float64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchString(c Operator, v1, v2 string) bool {
	switch c {
	case Contains:
		return strings.Contains(v1, v2)
	case NotContains:
		return !strings.Contains(v1, v2)
	case In:
		return slices.Contains(strings.Split(v2, ","), v1)
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

// matchRegex performs regex matching on trait values.
func matchRegex(traitValue, conditionValue string) bool {
	match, err := regexp.Match(conditionValue, []byte(traitValue))
	if err != nil {
		return false
	}
	return match
}

// matchModulo performs modulo operation matching on trait values.
func matchModulo(traitValue, conditionValue string) bool {
	values := strings.Split(conditionValue, "|")
	if len(values) != 2 {
		return false
	}

	divisor, err := strconv.ParseFloat(values[0], 64)
	if err != nil {
		return false
	}

	remainder, err := strconv.ParseFloat(values[1], 64)
	if err != nil {
		return false
	}

	traitValueFloat, err := strconv.ParseFloat(traitValue, 64)
	if err != nil {
		return false
	}

	return math.Mod(traitValueFloat, divisor) == remainder
}
