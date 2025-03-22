package flagsmith

// NewEvaluationContext creates an evaluation context for an identity.
func NewEvaluationContext(identifier string, traits map[string]interface{}) (ec EvaluationContext) {
	traitsCtx := make(map[string]TraitEvaluationContext, len(traits))
	for tKey, tValue := range traits {
		tCtx := getTraitEvaluationContext(tValue)
		traitsCtx[tKey] = tCtx
	}
	ec.Identity = IdentityEvaluationContext{
		Identifier: identifier,
		Traits:     traitsCtx,
	}
	return ec
}

// NewTransientEvaluationContext creates an evaluation context for an identity.
// If this context is used to evaluate flags remotely, Flagsmith will not persist the traits.
func NewTransientEvaluationContext(identifier string, traits map[string]interface{}) EvaluationContext {
	ec := NewEvaluationContext("", traits)
	ec.Identity.Transient = true
	return ec
}

func getTraitEvaluationContext(v interface{}) TraitEvaluationContext {
	tCtx, ok := v.(TraitEvaluationContext)
	if ok {
		return tCtx
	}
	return TraitEvaluationContext{Value: v}
}
