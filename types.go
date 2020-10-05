package bullettrain

// Feature contains core information about feature
type Feature struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Flag contains information about Feature and it's value
type Flag struct {
	Feature    Feature     `json:"feature"`
	StateValue interface{} `json:"feature_state_value"`
	Enabled    bool        `json:"enabled"`
}

// User holds identity information:w:w
type User struct {
	Identifier string `json:"identifier"`
}

// Trait holds information about User's trait
type Trait struct {
	Identity User   `json:"identity"`
	Key      string `json:"trait_key"`
	Value    string `json:"trait_value"`
}
