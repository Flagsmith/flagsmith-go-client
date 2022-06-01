package environments

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments/integrations"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/projects"
)

type EnvironmentModel struct {
	ID            int                           `json:"id"`
	APIKey        string                        `json:"api_key"`
	Project       *projects.ProjectModel        `json:"project"`
	FeatureStates []*features.FeatureStateModel `json:"feature_states"`

	AmplitudeConfig *integrations.IntegrationModel `json:"amplitude_config"`
	SegmentConfig   *integrations.IntegrationModel `json:"segment_config"`
	MixpanelConfig  *integrations.IntegrationModel `json:"mixpanel_config"`
	HeapConfig      *integrations.IntegrationModel `json:"heap_config"`
}
