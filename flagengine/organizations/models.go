package organizations

import "strconv"

type OrganisationModel struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	FeatureAnalytics bool   `json:"feature_analytics"`
	StopServingFlags bool   `json:"stop_serving_flags"`
	PersistTraitData bool   `json:"persist_trait_data"`
}

func (om *OrganisationModel) UniqueSlug() string {
	return strconv.Itoa(om.ID) + "-" + om.Name
}
