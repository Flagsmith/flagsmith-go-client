package environments

import (
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/projects"
)

type EnvironmentModel struct {
	ID                int                           `json:"id"`
	APIKey            string                        `json:"api_key"`
	Project           *projects.ProjectModel        `json:"project"`
	FeatureStates     []*features.FeatureStateModel `json:"feature_states"`
	IdentityOverrides []*identities.IdentityModel   `json:"identity_overrides"`
}
