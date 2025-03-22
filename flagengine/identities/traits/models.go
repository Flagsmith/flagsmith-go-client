package traits

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Trait is a flagsmith.Trait with a serialised Value.
type Trait struct {
	Key   string `json:"trait_key"`
	Value string `json:"trait_value"`
}

// NewTrait serialises value into a Trait using fmt.Sprint.
func NewTrait(key string, value interface{}) *Trait {
	return &Trait{key, fmt.Sprint(value)}
}

func (t *Trait) UnmarshalJSON(bytes []byte) error {
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
