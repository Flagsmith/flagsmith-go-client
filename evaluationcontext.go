package flagsmith

// EvaluationContext is contextual data used during feature flag evaluation.
//
// The zero value represents the current Flagsmith environment.
type EvaluationContext struct {
	Environment EnvironmentEvaluationContext `json:"environment,omitempty"`
	Identity    IdentityEvaluationContext    `json:"identity,omitempty"`
}

// EnvironmentEvaluationContext is the EvaluationContext of a Flagsmith
// environment, such as Staging or Production.
//
// APIKey is the environment's client-side SDK key.
type EnvironmentEvaluationContext struct {
	APIKey string `json:"api_key"`
}

// IdentityEvaluationContext is the EvaluationContext of an identity within a
// Flagsmith environment.
//
// Identifier is the identity's targeting key. It is used for identity-specific
// flag overrides, and as the seed for fractional evaluation.
//
// Traits are application-defined key-value attributes.
//
// If Transient, no identity data will be persisted when flags are evaluated
// remotely by the Flagsmith API.
type IdentityEvaluationContext struct {
	Transient  bool                              `json:"transient,omitempty"`
	Identifier string                            `json:"identifier,omitempty"`
	Traits     map[string]TraitEvaluationContext `json:"traits,omitempty"`
}

// TraitEvaluationContext represents a single key-value attribute in an
// IdentityEvaluationContext.
//
// If Transient, this trait will not be persisted when flags are evaluated
// remotely by the Flagsmith API. Other non-transient traits will be persisted.
type TraitEvaluationContext struct {
	Transient bool        `json:"transient,omitempty"`
	Value     interface{} `json:"value"`
}
