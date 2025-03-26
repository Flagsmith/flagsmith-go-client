package traits

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TraitModel is a flagsmith.Trait with a serialised Value.
type TraitModel struct {
	Key   string `json:"trait_key"`
	Value string `json:"trait_value"`
}

// NewTrait serialises value into a TraitModel using fmt.Sprint.
func NewTrait(key string, value interface{}) *TraitModel {
	return &TraitModel{key, fmt.Sprint(value)}
}

func (t *TraitModel) UnmarshalJSON(bytes []byte) error {
	var obj struct {
		Key string          `json:"trait_key"`
		Val json.RawMessage `json:"trait_value"`
	}

	err := json.Unmarshal(bytes, &obj)
	if err != nil {
		return err
	}

	t.Key = obj.Key
	t.Value = strings.Trim(string(obj.Val), `"`)
	return nil
}
