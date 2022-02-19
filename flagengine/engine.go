package flagengine

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

func GetIdentityFeatureStates(
	environment *environments.EnvironmentModel,
	identity *identities.IdentityModel,
	overrideTraits []*traits.TraitModel,
) []*features.FeatureStateModel {
	return nil
}
