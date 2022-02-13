package identities

import (
	"time"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

type IdentityModel struct {
	DjangoID          int                           `json:"django_id"`
	Identifier        string                        `json:"identifier"`
	EnvironmentAPIKey string                        `json:"environment_api_key"`
	CreatedDate       time.Time                     `json:"created_date"`
	IdentityUUID      string                        `json:"identity_uuid"`
	IdentityTraits    []*traits.TraitModel          `json:"identity_traits"`
	IdentityFeatures  []*features.FeatureStateModel `json:"identity_features"` // TODO(tzdybal): for some reason it's Set (not list) in Java impl
	CompositeKey      string                        `json:"composite_key"`
}
