package identities

import (
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/utils"
)

type IdentityModel struct {
	DjangoID          int                           `json:"django_id"`
	Identifier        string                        `json:"identifier"`
	EnvironmentAPIKey string                        `json:"environment_api_key"`
	CreatedDate       utils.ISOTime                 `json:"created_date"`
	IdentityUUID      string                        `json:"identity_uuid"`
	IdentityTraits    []*traits.Trait               `json:"identity_traits"`
	IdentityFeatures  []*features.FeatureStateModel `json:"identity_features"`
	CompositeKey_     string                        `json:"composite_key"`
}

func (i *IdentityModel) CompositeKey() string {
	if i.CompositeKey_ == "" {
		i.CompositeKey_ = i.EnvironmentAPIKey + "_" + i.Identifier
	}
	return i.CompositeKey_
}
