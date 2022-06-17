package segments

import (
	"log"
	"regexp"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v2/flagengine/utils"
)

type SegmentConditionModel struct {
	Operator ConditionOperator `json:"operator"`
	Value    string            `json:"value"`
	Property string            `json:"property_"`
}

func (m *SegmentConditionModel) MatchesTraitValue(traitValue string) bool {
	switch m.Operator {
	case NotContains:
		return !strings.Contains(traitValue, m.Value)
	case Regex:
		return m.regex(traitValue)
	default:
		return match(m.Operator, traitValue, m.Value)
	}
}

func (m *SegmentConditionModel) regex(traitValue string) bool {
	match, err := regexp.Match(m.Value, []byte(traitValue))
	if err != nil {
		// TODO: better logging
		log.Printf("WARNING: Invalid regex expression %v", m.Value)
		return false
	}
	return match
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
