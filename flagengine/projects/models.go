package projects

import (
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/organisations"
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/segments"
)

type ProjectModel struct {
	ID                int                              `json:"id"`
	Name              string                           `json:"name"`
	HideDisabledFlags bool                             `json:"hide_disabled_flags"`
	Organization      *organisations.OrganisationModel `json:"organization"`
	Segments          []*segments.SegmentModel         `json:"segments"`
}
