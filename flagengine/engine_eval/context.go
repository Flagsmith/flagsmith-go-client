package engine_eval

import (
	"encoding/json"
	"fmt"
)

// A context object containing the necessary information to evaluate Flagsmith feature flags.
type EngineEvaluationContext struct {
	// Environment context required for evaluation.
	Environment EnvironmentContext `json:"environment"`
	// Features to be evaluated in the context.
	Features map[string]FeatureContext `json:"features,omitempty"`
	// Identity context used for identity-based evaluation.
	Identity *IdentityContext `json:"identity,omitempty"`
	// Segments applicable to the evaluation context.
	Segments map[string]SegmentContext `json:"segments,omitempty"`
}

// Environment context required for evaluation.
//
// Represents an environment context for feature flag evaluation.
type EnvironmentContext struct {
	// An environment's unique identifier.
	Key string `json:"key"`
	// An environment's human-readable name.
	Name string `json:"name"`
}

// Represents a feature context for feature flag evaluation.
type FeatureContext struct {
	// Indicates whether the feature is enabled in the environment.
	Enabled bool `json:"enabled"`
	// Unique feature identifier.
	FeatureKey string `json:"feature_key"`
	// Key used when selecting a value for a multivariate feature. Set to an internal identifier
	// or a UUID, depending on Flagsmith implementation.
	Key string `json:"key"`
	// Feature name.
	Name string `json:"name"`
	// Priority of the feature context. Lower values indicate a higher priority when multiple
	// contexts apply to the same feature.
	Priority *float64 `json:"priority,omitempty"`
	// A default environment value for the feature. If the feature is multivariate, this will be
	// the control value.
	Value *Value `json:"value"`
	// An array of environment default values associated with the feature. Contains a single
	// value for standard features, or multiple values for multivariate features.
	Variants []FeatureValue `json:"variants,omitempty"`
}

// Represents a multivariate value for a feature flag.
type FeatureValue struct {
	// The value of the feature.
	Value *Value `json:"value"`
	// The weight of the feature value variant, as a percentage number (i.e. 100.0).
	Weight float64 `json:"weight"`
}

// FlexibleString is a type that can unmarshal from either string or number JSON values.
type FlexibleString string

// UnmarshalJSON implements custom JSON unmarshaling for FlexibleString.
func (f *FlexibleString) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*f = FlexibleString(str)
		return nil
	}

	// Try to unmarshal as a number
	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*f = FlexibleString(num.String())
		return nil
	}

	// Try to unmarshal as any type and convert to string
	var val interface{}
	if err := json.Unmarshal(data, &val); err == nil {
		*f = FlexibleString(fmt.Sprintf("%v", val))
		return nil
	}

	return fmt.Errorf("unable to unmarshal FlexibleString from %s", string(data))
}

type IdentityContext struct {
	// A unique identifier for an identity, used for segment and multivariate feature flag
	// targeting, and displayed in the Flagsmith UI.
	Identifier string `json:"identifier"`
	// Key used when selecting a value for a multivariate feature, or for % split segmentation.
	// Set to an internal identifier or a composite value based on the environment key and
	// identifier, depending on Flagsmith implementation.
	Key string `json:"key"`
	// A map of traits associated with the identity, where the key is the trait name and the
	// value is the trait value.
	Traits map[string]*Value `json:"traits,omitempty"`
}

// Represents a segment context for feature flag evaluation.
type SegmentContext struct {
	// Key used for % split segmentation.
	Key string `json:"key"`
	// The name of the segment.
	Name string `json:"name"`
	// Feature overrides for the segment.
	Overrides []FeatureContext `json:"overrides,omitempty"`
	// Rules that define the segment.
	Rules []SegmentRule `json:"rules"`
}

// Represents a rule within a segment for feature flag evaluation.
type SegmentRule struct {
	// Conditions that must be met for the rule to apply.
	Conditions []Condition `json:"conditions,omitempty"`
	// Sub-rules nested within the segment rule.
	Rules []SegmentRule `json:"rules,omitempty"`
	// Segment rule type. Represents a logical quantifier for the conditions and sub-rules.
	Type Type `json:"type"`
}

// Represents a condition within a segment rule for feature flag evaluation.
//
// Represents an IN condition within a segment rule for feature flag evaluation.
type Condition struct {
	// The operator to use for evaluating the condition.
	Operator Operator `json:"operator"`
	// A reference to the identity trait or value in the evaluation context.
	Property string `json:"property"`
	// The value to compare against the trait or context value.
	//
	// The values to compare against the trait or context value.
	Value *ValueUnion `json:"value"`
}

// The operator to use for evaluating the condition.
type Operator string

const (
	Contains             Operator = "CONTAINS"
	Equal                Operator = "EQUAL"
	GreaterThan          Operator = "GREATER_THAN"
	GreaterThanInclusive Operator = "GREATER_THAN_INCLUSIVE"
	In                   Operator = "IN"
	IsNotSet             Operator = "IS_NOT_SET"
	IsSet                Operator = "IS_SET"
	LessThan             Operator = "LESS_THAN"
	LessThanInclusive    Operator = "LESS_THAN_INCLUSIVE"
	Modulo               Operator = "MODULO"
	NotContains          Operator = "NOT_CONTAINS"
	NotEqual             Operator = "NOT_EQUAL"
	PercentageSplit      Operator = "PERCENTAGE_SPLIT"
	Regex                Operator = "REGEX"
)

// Segment rule type. Represents a logical quantifier for the conditions and sub-rules.
type Type string

const (
	All  Type = "ALL"
	Any  Type = "ANY"
	None Type = "NONE"
)

// A default environment value for the feature. If the feature is multivariate, this will be
// the control value.
//
// The value of the feature.
type Value struct {
	Bool   *bool
	Double *float64
	String *string
}

type ValueUnion struct {
	String      *string
	StringArray []string
}

// UnmarshalJSON implements custom JSON unmarshaling for Value.
func (v *Value) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as null first
	if string(data) == "null" {
		return nil
	}

	// Try to unmarshal as a structured object
	var structured struct {
		Bool   *bool    `json:"bool"`
		Double *float64 `json:"double"`
		String *string  `json:"string"`
	}
	if err := json.Unmarshal(data, &structured); err == nil && (structured.Bool != nil || structured.Double != nil || structured.String != nil) {
		v.Bool = structured.Bool
		v.Double = structured.Double
		v.String = structured.String
		return nil
	}

	// Try to unmarshal as a raw value
	var rawValue interface{}
	if err := json.Unmarshal(data, &rawValue); err != nil {
		return err
	}

	switch val := rawValue.(type) {
	case bool:
		v.Bool = &val
	case float64:
		v.Double = &val
	case string:
		v.String = &val
	case nil:
		// Already handled above, but just in case
		return nil
	default:
		// If it's not a basic type, convert to string
		str := fmt.Sprintf("%v", val)
		v.String = &str
	}

	return nil
}

// UnmarshalJSON implements custom JSON unmarshaling for ValueUnion.
func (v *ValueUnion) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as null first
	if string(data) == "null" {
		return nil
	}

	// Try to unmarshal as a string array
	var strArray []string
	if err := json.Unmarshal(data, &strArray); err == nil {
		v.StringArray = strArray
		return nil
	}

	// Try to unmarshal as a single string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		v.String = &str
		return nil
	}

	// Try to unmarshal as a structured object
	var structured struct {
		String      *string  `json:"string"`
		StringArray []string `json:"stringArray"`
	}
	if err := json.Unmarshal(data, &structured); err == nil {
		v.String = structured.String
		v.StringArray = structured.StringArray
		return nil
	}

	return fmt.Errorf("unable to unmarshal ValueUnion from %s", string(data))
}

// UnmarshalJSON implements custom JSON unmarshaling for IdentityContext.
func (ic *IdentityContext) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid recursion
	type Alias IdentityContext
	aux := struct {
		Key FlexibleString `json:"key"`
		*Alias
	}{
		Alias: (*Alias)(ic),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	ic.Key = string(aux.Key)
	return nil
}

// ContextValue represents allowed types: nil, int, float64, bool, string.
type ContextValue interface{}
