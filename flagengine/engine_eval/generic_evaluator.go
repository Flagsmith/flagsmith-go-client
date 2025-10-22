package engine_eval

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"
)

// Comparable defines types that can be compared using standard operators.
// This includes all numeric types, strings, and booleans.
type Comparable interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 |
		~string | ~bool
}

// Ordered defines types that support ordering operations (>, <, >=, <=).
// Note that bool is excluded as it doesn't support ordering.
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 |
		~string
}

// Generic comparison functions - one per operator

// evaluateEqualGeneric implements the EQUAL operator for comparable types.
func evaluateEqualGeneric[T Comparable](v1, v2 T) bool {
	return v1 == v2
}

// evaluateNotEqualGeneric implements the NOT_EQUAL operator for comparable types.
func evaluateNotEqualGeneric[T Comparable](v1, v2 T) bool {
	return v1 != v2
}

// evaluateGreaterThanGeneric implements the GREATER_THAN operator for ordered types.
func evaluateGreaterThanGeneric[T Ordered](v1, v2 T) bool {
	return v1 > v2
}

// evaluateLessThanGeneric implements the LESS_THAN operator for ordered types.
func evaluateLessThanGeneric[T Ordered](v1, v2 T) bool {
	return v1 < v2
}

// evaluateGreaterThanInclusiveGeneric implements the GREATER_THAN_INCLUSIVE operator for ordered types.
func evaluateGreaterThanInclusiveGeneric[T Ordered](v1, v2 T) bool {
	return v1 >= v2
}

// evaluateLessThanInclusiveGeneric implements the LESS_THAN_INCLUSIVE operator for ordered types.
func evaluateLessThanInclusiveGeneric[T Ordered](v1, v2 T) bool {
	return v1 <= v2
}

// evaluateContainsGeneric implements the CONTAINS operator for strings.
func evaluateContainsGeneric(v1, v2 string) bool {
	return strings.Contains(v1, v2)
}

// evaluateNotContainsGeneric implements the NOT_CONTAINS operator for strings.
func evaluateNotContainsGeneric(v1, v2 string) bool {
	return !strings.Contains(v1, v2)
}

// dispatchOperator dispatches the operator to the appropriate generic function.
func dispatchOperator[T Ordered](operator Operator, v1, v2 T) bool {
	switch operator {
	case Equal:
		return evaluateEqualGeneric(v1, v2)
	case NotEqual:
		return evaluateNotEqualGeneric(v1, v2)
	case GreaterThan:
		return evaluateGreaterThanGeneric(v1, v2)
	case LessThan:
		return evaluateLessThanGeneric(v1, v2)
	case GreaterThanInclusive:
		return evaluateGreaterThanInclusiveGeneric(v1, v2)
	case LessThanInclusive:
		return evaluateLessThanInclusiveGeneric(v1, v2)
	}
	return false
}

// dispatchComparableOperator dispatches equality operators for comparable types (including bool).
func dispatchComparableOperator[T Comparable](operator Operator, v1, v2 T) bool {
	switch operator {
	case Equal:
		return evaluateEqualGeneric(v1, v2)
	case NotEqual:
		return evaluateNotEqualGeneric(v1, v2)
	}
	return false
}

// parseAndMatch attempts to parse string values into specific types and compare them using generics.
func parseAndMatch(operator Operator, traitValue, conditionValue string) bool {
	// Handle special operators first
	switch operator {
	case Modulo:
		return evaluateModuloGeneric(traitValue, conditionValue)
	case Regex:
		return evaluateRegexGeneric(traitValue, conditionValue)
	case Contains:
		return evaluateContainsGeneric(traitValue, conditionValue)
	case NotContains:
		return evaluateNotContainsGeneric(traitValue, conditionValue)
	}

	// Handle semver comparison
	if strings.HasSuffix(conditionValue, ":semver") {
		conditionVersion, err := semver.Make(conditionValue[:len(conditionValue)-7])
		if err != nil {
			return false
		}
		return evaluateSemverGeneric(operator, traitValue, conditionVersion)
	}

	// Try boolean parsing
	if b1, e1 := strconv.ParseBool(traitValue); e1 == nil {
		if b2, e2 := strconv.ParseBool(conditionValue); e2 == nil {
			return dispatchComparableOperator(operator, b1, b2)
		}
	}

	// Try integer parsing
	if i1, e1 := strconv.ParseInt(traitValue, 10, 64); e1 == nil {
		if i2, e2 := strconv.ParseInt(conditionValue, 10, 64); e2 == nil {
			return dispatchOperator(operator, i1, i2)
		}
	}

	// Try float parsing
	if f1, e1 := strconv.ParseFloat(traitValue, 64); e1 == nil {
		if f2, e2 := strconv.ParseFloat(conditionValue, 64); e2 == nil {
			return dispatchOperator(operator, f1, f2)
		}
	}

	// Fall back to string comparison
	return dispatchOperator(operator, traitValue, conditionValue)
}

// evaluateRegexGeneric performs regex matching on trait values.
func evaluateRegexGeneric(traitValue, conditionValue string) bool {
	match, err := regexp.Match(conditionValue, []byte(traitValue))
	if err != nil {
		return false
	}
	return match
}

// evaluateModuloGeneric performs modulo operation matching on trait values.
func evaluateModuloGeneric(traitValue, conditionValue string) bool {
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

// evaluateSemverGeneric handles semantic version comparisons.
func evaluateSemverGeneric(operator Operator, traitValue string, conditionVersion semver.Version) bool {
	traitVersion, err := semver.Make(traitValue)
	if err != nil {
		return false
	}

	switch operator {
	case Equal:
		return traitVersion.EQ(conditionVersion)
	case NotEqual:
		return traitVersion.NE(conditionVersion)
	case GreaterThan:
		return traitVersion.GT(conditionVersion)
	case LessThan:
		return traitVersion.LT(conditionVersion)
	case GreaterThanInclusive:
		return traitVersion.GE(conditionVersion)
	case LessThanInclusive:
		return traitVersion.LTE(conditionVersion)
	}
	return false
}
