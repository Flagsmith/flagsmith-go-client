package features

type FeatureModel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type FeatureStateModel struct {
	Feature                        *FeatureModel                         `json:"feature"`
	Enabled                        bool                                  `json:"enabled"`
	DjangoID                       int                                   `json:"django_id"`
	FeatureStateUUID               string                                `json:"featurestate_uuid"`
	MultivariateFeatureStateValues []*MultivariateFeatureStateValueModel `json:"multivariate_feature_state_values"`
	Value                          interface{}                           `json:"feature_state_value"`
}

type MultivariateFeatureOptionModel struct {
	ID    int         `json:"id"`
	Value interface{} `json:"value"`
}

type MultivariateFeatureStateValueModel struct {
	ID                        int                             `json:"ID"`
	MultivariateFeatureOption *MultivariateFeatureOptionModel `json:"multivariate_feature_option"`
	PercentageAllocation      float64                         `json:"percentage_allocation"`
	MVFSValueUUID             string                          `json:"mv_fs_value_uuid"`
}
