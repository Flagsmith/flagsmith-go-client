package engine_eval

import (
	"testing"
)

func TestMatchGeneric(t *testing.T) {
	tests := []struct {
		name     string
		operator Operator
		v1       any
		v2       any
		expected bool
	}{
		// Boolean tests
		{"bool equal true", Equal, true, true, true},
		{"bool equal false", Equal, false, false, true},
		{"bool not equal", Equal, true, false, false},
		{"bool not equal operator", NotEqual, true, false, true},

		// Integer tests
		{"int equal", Equal, int64(42), int64(42), true},
		{"int not equal", Equal, int64(42), int64(43), false},
		{"int greater than", GreaterThan, int64(43), int64(42), true},
		{"int less than", LessThan, int64(42), int64(43), true},
		{"int greater than equal", GreaterThanInclusive, int64(42), int64(42), true},
		{"int less than equal", LessThanInclusive, int64(42), int64(42), true},

		// Float tests
		{"float equal", Equal, 42.5, 42.5, true},
		{"float not equal", Equal, 42.5, 42.6, false},
		{"float greater than", GreaterThan, 42.6, 42.5, true},
		{"float less than", LessThan, 42.5, 42.6, true},

		// String tests
		{"string equal", Equal, "hello", "hello", true},
		{"string not equal", Equal, "hello", "world", false},
		{"string greater than", GreaterThan, "world", "hello", true},
		{"string less than", LessThan, "hello", "world", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			switch v1 := tt.v1.(type) {
			case bool:
				if v2, ok := tt.v2.(bool); ok {
					switch tt.operator {
					case Equal:
						result = evaluateEqualGeneric(v1, v2)
					case NotEqual:
						result = evaluateNotEqualGeneric(v1, v2)
					}
				}
			case int64:
				if v2, ok := tt.v2.(int64); ok {
					switch tt.operator {
					case Equal:
						result = evaluateEqualGeneric(v1, v2)
					case NotEqual:
						result = evaluateNotEqualGeneric(v1, v2)
					case GreaterThan:
						result = evaluateGreaterThanGeneric(v1, v2)
					case LessThan:
						result = evaluateLessThanGeneric(v1, v2)
					case GreaterThanInclusive:
						result = evaluateGreaterThanInclusiveGeneric(v1, v2)
					case LessThanInclusive:
						result = evaluateLessThanInclusiveGeneric(v1, v2)
					}
				}
			case float64:
				if v2, ok := tt.v2.(float64); ok {
					switch tt.operator {
					case Equal:
						result = evaluateEqualGeneric(v1, v2)
					case NotEqual:
						result = evaluateNotEqualGeneric(v1, v2)
					case GreaterThan:
						result = evaluateGreaterThanGeneric(v1, v2)
					case LessThan:
						result = evaluateLessThanGeneric(v1, v2)
					}
				}
			case string:
				if v2, ok := tt.v2.(string); ok {
					switch tt.operator {
					case Equal:
						result = evaluateEqualGeneric(v1, v2)
					case NotEqual:
						result = evaluateNotEqualGeneric(v1, v2)
					case GreaterThan:
						result = evaluateGreaterThanGeneric(v1, v2)
					case LessThan:
						result = evaluateLessThanGeneric(v1, v2)
					}
				}
			}

			if result != tt.expected {
				t.Errorf("evaluateGeneric(%v, %v, %v) = %v, want %v", tt.operator, tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestParseAndMatch(t *testing.T) {
	tests := []struct {
		name           string
		operator       Operator
		traitValue     string
		conditionValue string
		expected       bool
	}{
		// Boolean parsing and comparison
		{"parse bool equal true", Equal, "true", "true", true},
		{"parse bool equal false", Equal, "false", "false", true},
		{"parse bool not equal", Equal, "true", "false", false},
		{"parse bool not equal operator", NotEqual, "true", "false", true},

		// Integer parsing and comparison
		{"parse int equal", Equal, "42", "42", true},
		{"parse int not equal", Equal, "42", "43", false},
		{"parse int greater than", GreaterThan, "43", "42", true},
		{"parse int less than", LessThan, "42", "43", true},

		// Float parsing and comparison
		{"parse float equal", Equal, "42.5", "42.5", true},
		{"parse float not equal", Equal, "42.5", "42.6", false},
		{"parse float greater than", GreaterThan, "42.6", "42.5", true},

		// String comparison (when parsing fails)
		{"string equal", Equal, "hello", "hello", true},
		{"string not equal", Equal, "hello", "world", false},
		{"string contains", Contains, "hello world", "world", true},
		{"string not contains", NotContains, "hello world", "xyz", true},

		// Mixed type parsing (should fall back to string)
		{"mixed types", Equal, "42", "hello", false},
		{"mixed bool and int", Equal, "true", "1", true}, // Both parse as bool: true == true

		// Semver comparison
		{"semver equal", Equal, "1.2.3", "1.2.3:semver", true},
		{"semver greater than", GreaterThan, "1.2.4", "1.2.3:semver", true},
		{"semver less than", LessThan, "1.2.2", "1.2.3:semver", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndMatch(tt.operator, tt.traitValue, tt.conditionValue)
			if result != tt.expected {
				t.Errorf("parseAndMatch(%v, %q, %q) = %v, want %v", tt.operator, tt.traitValue, tt.conditionValue, result, tt.expected)
			}
		})
	}
}
