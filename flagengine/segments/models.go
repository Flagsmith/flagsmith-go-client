package segments

import (
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

type SegmentConditionModel struct {
	Operator ConditionOperator `json:"operator"`
	Value    string            `json:"value"`
	Property string            `json:"property_"`
}

type SegmentRuleModel struct {
	Type       RuleType `json:"type"`
	Rules      []*SegmentRuleModel
	Conditions []*SegmentConditionModel
}

func (sr *SegmentRuleModel) MatchingFunction() func([]bool) bool {
	switch sr.Type {
	case All:
		return utils.All
	case Any:
		return utils.Any
	default:
		return utils.None
	}
}

type SegmentModel struct {
	ID            int                           `json:"id"`
	Name          string                        `json:"name"`
	Rules         []*SegmentRuleModel           `json:"rules"`
	FeatureStates []*features.FeatureStateModel `json:"feature_states"`
}
