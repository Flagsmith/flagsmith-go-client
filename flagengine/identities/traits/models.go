package traits

import (
	"encoding/json"
	"strings"
)

type TraitModel struct {
	TraitKey   string `json:"trait_key"`
	TraitValue string `json:"trait_value"`
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

	t.TraitKey = obj.Key
	t.TraitValue = strings.Trim(string(obj.Val), `"`)
	return nil
}
