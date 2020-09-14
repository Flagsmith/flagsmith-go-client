package bullettrain

type Feature struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Flag struct {
	Feature    Feature `json:"feature"`
	StateValue string  `json:"feature_state_value"`
	Enabled    bool    `json:"enabled"`
}

type FeatureUser struct {
	Identifier string `json:"identifier"`
}

type Trait struct {
	Identity FeatureUser `json:"identity"`
	Key      string      `json:"trait_key"`
	Value    string      `json:"trait_value"`
}
