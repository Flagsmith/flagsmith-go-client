package flagsmith

type EvaluationContext struct {
	Environment *EnvironmentEvaluationContext `json:"environment,omitempty"`
	Feature     *FeatureEvaluationContext     `json:"feature,omitempty"`
	Identity    *IdentityEvaluationContext    `json:"identity,omitempty"`
}

type EnvironmentEvaluationContext struct {
	APIKey string `json:"api_key"`
}

type FeatureEvaluationContext struct {
	Name string `json:"name"`
}

type IdentityEvaluationContext struct {
	Identifier string                             `json:"identifier,omitempty"`
	Traits     map[string]*TraitEvaluationContext `json:"traits,omitempty"`
	Transient  bool                               `json:"transient,omitempty"`
}

type TraitEvaluationContext struct {
	Transient bool        `json:"transient,omitempty"`
	Value     interface{} `json:"value"`
}
