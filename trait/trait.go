package trait

import (
	"fmt"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/identities/traits"
)

// Trait represents a trait with key-value pair.
type Trait struct {
	TraitKey   string      `json:"trait_key"`
	TraitValue interface{} `json:"trait_value"`
	Transient  bool        `json:"transient,omitempty"`
}

// ToTraitModel converts a Trait to a TraitModel.
func (t *Trait) ToTraitModel() *traits.TraitModel {
	return &traits.TraitModel{
		TraitKey:   t.TraitKey,
		TraitValue: fmt.Sprint(t.TraitValue),
	}
}
