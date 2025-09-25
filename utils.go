package flagsmith

import (
	"sort"
)

// func mapIdentityEvaluationContextToTraits(ic IdentityEvaluationContext) []*Trait {
// 	traits := make([]*Trait, len(ic.Traits))
// 	for i, tKey := range sortedKeys(ic.Traits) {
// 		traits[i] = mapTraitEvaluationContextToTrait(tKey, ic.Traits[tKey])
// 	}
// 	return traits
// }

// func mapTraitEvaluationContextToTrait(tKey string, tCtx *TraitEvaluationContext) *Trait {
// 	if tCtx == nil {
// 		return &Trait{TraitKey: tKey, TraitValue: nil}
// 	}
// 	if tCtx.Transient == nil {
// 		return &Trait{TraitKey: tKey, TraitValue: tCtx.Value}
// 	}
// 	return &Trait{TraitKey: tKey, TraitValue: tCtx.Value, Transient: *tCtx.Transient}
// }

func sortedKeys[Map ~map[string]V, V any](m Map) []string {
	keys := make([]string, len(m))
	i := 0
	for tKey := range m {
		keys[i] = tKey
		i++
	}
	sort.Strings(keys)
	return keys
}
