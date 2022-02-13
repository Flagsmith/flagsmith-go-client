package projects

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/organizations"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/segments"
)

type ProjectModel struct {
	ID                int                              `json:"id"`
	Name              string                           `json:"name"`
	HideDisabledFlags bool                             `json:"hide_disabled_flags"`
	Organization      *organizations.OrganisationModel `json:"organization"`
	Segments          []*segments.SegmentModel         `json:"segments"`
}
