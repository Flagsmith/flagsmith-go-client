package segments

import "github.com/Flagsmith/flagsmith-go-client/flagengine/features"

type SegmentConditionModel struct {
	Operator ConditionOperator `json:"operator"`
	Value    string            `json:"value"`
	Property string            `json:"property_"`
}

type SegmentRuleModel struct {
	Type       string `json:"string"`
	Rules      []*SegmentRuleModel
	Conditions []*SegmentConditionModel
}

type SegmentModel struct {
	ID            int                           `json:"id"`
	Name          string                        `json:"name"`
	Rules         []*SegmentRuleModel           `json:"rules"`
	FeatureStates []*features.FeatureStateModel `json:"feature_states"`
}
