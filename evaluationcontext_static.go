package flagsmith

func getTraitEvaluationContext(v interface{}) TraitEvaluationContext {
	tCtx, ok := v.(TraitEvaluationContext)
	if ok {
		return tCtx
	}
	return TraitEvaluationContext{Value: v}
}

func NewTraitEvaluationContext(value interface{}, transient bool) TraitEvaluationContext {
	return TraitEvaluationContext{Value: value, Transient: &transient}
}

func NewEvaluationContext(identifier string, traits map[string]interface{}) EvaluationContext {
	ec := EvaluationContext{}
	traitsCtx := make(map[string]*TraitEvaluationContext, len(traits))
	for tKey, tValue := range traits {
		tCtx := getTraitEvaluationContext(tValue)
		traitsCtx[tKey] = &tCtx
	}
	ec.Identity = &IdentityEvaluationContext{
		Identifier: &identifier,
		Traits:     traitsCtx,
	}
	return ec
}

func NewTransientEvaluationContext(identifier string, traits map[string]interface{}) EvaluationContext {
	ec := NewEvaluationContext(identifier, traits)
	var transient = true
	ec.Identity.Transient = &transient
	return ec
}
