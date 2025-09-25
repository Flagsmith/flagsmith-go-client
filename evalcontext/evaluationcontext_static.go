package evalcontext

import (
//	"encoding/json"
//	"fmt"
	//"strconv"

// feenv "github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	//"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	//"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
)

// func getTraitEvaluationContext(v interface{}) TraitEvaluationContext {
// 	tCtx, ok := v.(TraitEvaluationContext)
// 	if ok {
// 		return tCtx
// 	}
// 	return TraitEvaluationContext{Value: v}
// }

// func NewTraitEvaluationContext(value interface{}, transient bool) TraitEvaluationContext {
// 	return TraitEvaluationContext{Value: value, Transient: &transient}
// }

// func NewEvaluationContext(identifier string, traits map[string]interface{}) EvaluationContext {
// 	ec := EvaluationContext{}
// 	traitsCtx := make(map[string]*TraitEvaluationContext, len(traits))
// 	for tKey, tValue := range traits {
// 		tCtx := getTraitEvaluationContext(tValue)
// 		traitsCtx[tKey] = &tCtx
// 	}
// 	ec.Identity = &IdentityEvaluationContext{
// 		Identifier: &identifier,
// 		Traits:     traitsCtx,
// 	}
// 	return ec
// }

// func NewTransientEvaluationContext(identifier string, traits map[string]interface{}) EvaluationContext {
// 	ec := NewEvaluationContext(identifier, traits)
// 	var transient = true
// 	ec.Identity.Transient = &transient
// 	return ec
// }

// MapEnvironmentDocumentToEvaluationContext maps an environment document JSON
// to the higher-level EvaluationContext representation used for evaluation.
// func MapEnvironmentDocumentToEvaluationContext(envJSON []byte) (EvaluationContext, error) {
// 	var env feenv.EnvironmentModel
// 	if err := json.Unmarshal(envJSON, &env); err != nil {
// 		return EvaluationContext{}, err
// 	}

// 	ctx := EvaluationContext{}

// 	// Environment
// 	// map environment -> EnvironmentEvaluationContext
// 	ctx.Environment = &EnvironmentEvaluationContext{APIKey: env.APIKey}
// 	if env.Project != nil {
// 		ctx.Environment.Name = env.Project.Name
// 	}

// 	// Features (environment defaults)
// 	if len(env.FeatureStates) > 0 {
// 		ctx.Features = make(map[string]FeatureContext, len(env.FeatureStates))
// 		for _, fs := range env.FeatureStates {
// 			fc := mapFeatureStateToFeatureContext(fs)
// 			ctx.Features[fc.FeatureKey] = fc
// 		}
// 	}

// 	// Segments
// 	if env.Project != nil && len(env.Project.Segments) > 0 {
// 		ctx.Segments = make(map[string]SegmentContext, len(env.Project.Segments))
// 		for _, s := range env.Project.Segments {
// 			sc := mapSegmentToSegmentContext(s)
// 			ctx.Segments[sc.Name] = sc
// 		}
// 	}

// 	return ctx, nil
// }

// func mapFeatureStateToFeatureContext(fs *features.FeatureStateModel) FeatureContext {
// 	var key string
// 	if fs.DjangoID != 0 {
// 		key = strconv.Itoa(fs.DjangoID)
// 	} else {
// 		key = fs.FeatureStateUUID
// 	}

// 	fc := FeatureContext{
// 		Enabled:   fs.Enabled,
// 		FeatureKey: fs.Feature.Name,
// 		Key:       key,
// 		Name:      fs.Feature.Name,
// 	}

// 	// Value
// 	if fs.RawValue != nil {
// 		fc.Value = fmt.Sprint(fs.RawValue)
// 	}

// 	// Variants
// 	if len(fs.MultivariateFeatureStateValues) > 0 {
// 		variants := make([]FeatureValue, 0, len(fs.MultivariateFeatureStateValues))
// 		for _, mv := range fs.MultivariateFeatureStateValues {
// 			variants = append(variants, FeatureValue{
// 				Value:  fmt.Sprint(mv.MultivariateFeatureOption.Value),
// 				Weight: mv.PercentageAllocation,
// 			})
// 		}
// 		fc.Variants = variants
// 	}

// 	// Priority (if present via segment override)
// 	if fs.FeatureSegment != nil {
// 		p := float64(fs.FeatureSegment.Priority)
// 		fc.Priority = &p
// 	}

// 	return fc
// }

// func mapSegmentToSegmentContext(s *segments.SegmentModel) SegmentContext {
// 	sc := SegmentContext{
// 		Key:   strconv.Itoa(s.ID),
// 		Name:  s.Name,
// 		Rules: make([]SegmentRule, 0, len(s.Rules)),
// 	}

// 	// Overrides
// 	if len(s.FeatureStates) > 0 {
// 		for _, fs := range s.FeatureStates {
// 			sc.Overrides = append(sc.Overrides, mapFeatureStateToFeatureContext(fs))
// 		}
// 	}

// 	// Rules
// 	for _, r := range s.Rules {
// 		sc.Rules = append(sc.Rules, mapSegmentRuleToRule(r))
// 	}

// 	return sc
// }

// func mapSegmentRuleToRule(r *segments.SegmentRuleModel) SegmentRule {
// 	er := SegmentRule{Type: mapRuleType(r.Type)}
// 	// Conditions
// 	if len(r.Conditions) > 0 {
// 		for _, c := range r.Conditions {
// 			er.Conditions = append(er.Conditions, Condition{
// 				Operator: mapConditionOperator(c.Operator),
// 				Property: c.Property,
// 				Value:    &EvaluationValue{String: &c.Value},
// 			})
// 		}
// 	}
// 	// Nested rules
// 	if len(r.Rules) > 0 {
// 		for _, sr := range r.Rules {
// 			er.Rules = append(er.Rules, mapSegmentRuleToRule(sr))
// 		}
// 	}
// 	return er
// }

// func mapRuleType(t segments.RuleType) Type {
// 	switch t {
// 	case segments.All:
// 		return All
// 	case segments.Any:
// 		return Any
// 	default:
// 		return None
// 	}
// }

// func mapConditionOperator(op segments.ConditionOperator) Operator {
// 	// Normalise NOT EQUAL -> NOT_EQUAL
// 	if op == "NOT EQUAL" {
// 		return NotEqual
// 	}
// 	return Operator(op)
// }
