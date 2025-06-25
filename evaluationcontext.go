package flagsmith

// EvaluationContext represents a context in which feature flags can be evaluated.
// Flagsmith flags are always evaluated in an EnvironmentEvaluationContext, with an optional IdentityEvaluationContext.
type EvaluationContext struct {
	Environment *EnvironmentEvaluationContext `json:"environment,omitempty"`
	Identity    *IdentityEvaluationContext    `json:"identity,omitempty"`
	Feature     *FeatureEvaluationContext     `json:"feature,omitempty"`
}

// EnvironmentEvaluationContext represents a Flagsmith environment used in an EvaluationContext.
// It is ignored if the evaluating Client was created using WithLocalEvaluation.
type EnvironmentEvaluationContext struct {
	// APIKey is an identifier for this environment. It is also known as the environment ID or client-side SDK key.
	APIKey string `json:"api_key"`
}

// IdentityEvaluationContext represents a Flagsmith identity within a Flagsmith environment, used in an EvaluationContext.
// Traits are application-defined key-value pairs which can be used as part of the flag evaluation context.
// Flagsmith will not persist Transient identities when flags are remotely evaluated.
type IdentityEvaluationContext struct {
	Identifier *string                            `json:"identifier,omitempty"`
	Traits     map[string]*TraitEvaluationContext `json:"traits,omitempty"`
	Transient  *bool                              `json:"transient,omitempty"`
}

// TraitEvaluationContext represents a single trait value used within an IdentityEvaluationContext.
// A Transient trait will not be persisted.
type TraitEvaluationContext struct {
	Transient *bool       `json:"transient,omitempty"`
	Value     interface{} `json:"value"`
}

// FeatureEvaluationContext is not yet implemented.
type FeatureEvaluationContext struct {
	Name string `json:"name"`
}
