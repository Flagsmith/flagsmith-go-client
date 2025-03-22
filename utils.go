package flagsmith

import (
	"sort"
)

func mapIdentityEvaluationContextToTraits(ic IdentityEvaluationContext) []Trait {
	traits := make([]Trait, len(ic.Traits))
	for i, tKey := range sortedKeys(ic.Traits) {
		traits[i] = Trait{
			TraitKey:   tKey,
			TraitValue: ic.Traits[tKey].Value,
			Transient:  ic.Traits[tKey].Transient,
		}
	}
	return traits
}

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
