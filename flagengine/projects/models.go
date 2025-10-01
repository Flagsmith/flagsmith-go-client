package projects

import (
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/organisations"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/segments"
)

type ProjectModel struct {
	ID                int                              `json:"id"`
	Name              string                           `json:"name"`
	HideDisabledFlags bool                             `json:"hide_disabled_flags"`
	Organisation      *organisations.OrganisationModel `json:"organisation"`
	Segments          []*segments.SegmentModel         `json:"segments"`
}
