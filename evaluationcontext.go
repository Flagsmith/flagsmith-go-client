package flagsmith

// EvaluationContext is contextual data used during feature flag evaluation.
//
// The zero value represents the current Flagsmith environment.
type EvaluationContext struct {
	identifier string
	traits     map[string]interface{}
}

// NewEvaluationContext creates a flag evaluation context for an identity.
func NewEvaluationContext(identifier string, traits map[string]interface{}) (ec EvaluationContext) {
	ec.identifier = identifier
	ec.traits = traits
	// Store a copy of the trait map
	ec.traits = make(map[string]interface{}, len(traits))
	for k, v := range traits {
		ec.traits[k] = v
	}
	return ec
}

// NewTransientEvaluationContext is equivalent to NewEvaluationContext("", traits).
func NewTransientEvaluationContext(traits map[string]interface{}) (ec EvaluationContext) {
	return NewEvaluationContext("", traits)
}
