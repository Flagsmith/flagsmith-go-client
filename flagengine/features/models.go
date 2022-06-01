package features

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/utils"
)

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
	RawValue                       string                                `json:"feature_state_value"`
}

func (fs *FeatureStateModel) UnmarshalJSON(bytes []byte) error {
	var obj struct {
		Feature                        *FeatureModel                         `json:"feature"`
		Enabled                        bool                                  `json:"enabled"`
		DjangoID                       int                                   `json:"django_id"`
		FeatureStateUUID               string                                `json:"featurestate_uuid"`
		MultivariateFeatureStateValues []*MultivariateFeatureStateValueModel `json:"multivariate_feature_state_values"`
		RawValue                       json.RawMessage                       `json:"feature_state_value"`
	}

	err := json.Unmarshal(bytes, &obj)
	if err != nil {
		return err
	}

	fs.Feature = obj.Feature
	fs.Enabled = obj.Enabled
	fs.DjangoID = obj.DjangoID
	fs.FeatureStateUUID = obj.FeatureStateUUID
	fs.MultivariateFeatureStateValues = obj.MultivariateFeatureStateValues
	fs.RawValue = strings.Trim(string(obj.RawValue), `"`)
	return nil
}

type MultivariateFeatureOptionModel struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

type MultivariateFeatureStateValueModel struct {
	ID                        *int                            `json:"ID"`
	MultivariateFeatureOption *MultivariateFeatureOptionModel `json:"multivariate_feature_option"`
	PercentageAllocation      float64                         `json:"percentage_allocation"`
	MVFSValueUUID             string                          `json:"mv_fs_value_uuid"`
}

func (mfsv *MultivariateFeatureStateValueModel) Key() string {
	if mfsv.ID != nil {
		return strconv.Itoa(*mfsv.ID)
	}
	return mfsv.MVFSValueUUID
}

func (fs *FeatureStateModel) Value(identityID string) interface{} {
	if identityID != "" && len(fs.MultivariateFeatureStateValues) > 0 {
		return fs.multivariateValue(identityID)
	}
	return fs.RawValue
}

func (fs *FeatureStateModel) multivariateValue(identityID string) interface{} {
	var uuid string
	if fs.DjangoID != 0 {
		uuid = strconv.Itoa(fs.DjangoID)
	} else {
		uuid = fs.FeatureStateUUID
	}
	percentageValue := utils.GetHashedPercentageForObjectIds([]string{uuid, identityID}, 1)
	startPercentage := 0.0

	values := make([]*MultivariateFeatureStateValueModel, len(fs.MultivariateFeatureStateValues))
	copy(values, fs.MultivariateFeatureStateValues)
	sort.Slice(values, func(i, j int) bool {
		return values[i].Key() < values[j].Key()
	})

	for _, value := range values {
		limit := value.PercentageAllocation + startPercentage
		if startPercentage <= percentageValue && percentageValue < limit {
			return value.MultivariateFeatureOption.Value
		}
		startPercentage = limit
	}

	return fs.RawValue
}
