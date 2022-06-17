package identities

import (
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/utils"
)

type IdentityModel struct {
	DjangoID          int                           `json:"django_id"`
	Identifier        string                        `json:"identifier"`
	EnvironmentAPIKey string                        `json:"environment_api_key"`
	CreatedDate       utils.ISOTime                 `json:"created_date"`
	IdentityUUID      string                        `json:"identity_uuid"`
	IdentityTraits    []*traits.TraitModel          `json:"identity_traits"`
	IdentityFeatures  []*features.FeatureStateModel `json:"identity_features"` // TODO(tzdybal): for some reason it's Set (not list) in Java impl
	CompositeKey_     string                        `json:"composite_key"`
}

func (i *IdentityModel) CompositeKey() string {
	if i.CompositeKey_ == "" {
		i.CompositeKey_ = i.EnvironmentAPIKey + "_" + i.Identifier
	}
	return i.CompositeKey_
}

func (i *IdentityModel) GetIdentityFeatures() []*features.FeatureStateModel {
	unique := make(map[int]*features.FeatureStateModel)
	for _, feat := range i.IdentityFeatures {
		if _, ok := unique[feat.Feature.ID]; ok {
			panic("hell")
		} else {
			unique[feat.Feature.ID] = feat
		}
	}

	return i.IdentityFeatures
}
