package fixtures

import (
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/organizations"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/projects"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/segments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/utils"
	"time"
)

const (
	SegmentConditionProperty     = "foo"
	SegmentConditionStringValaue = "bar"
)

func SegmentCondition() *segments.SegmentConditionModel {
	return &segments.SegmentConditionModel{
		Operator: segments.Equal,
		Property: SegmentConditionProperty,
		Value:    SegmentConditionStringValaue,
	}
}

func SegmentRule(cond *segments.SegmentConditionModel) *segments.SegmentRuleModel {
	return &segments.SegmentRuleModel{
		Type:       segments.All,
		Conditions: []*segments.SegmentConditionModel{cond},
	}
}

func Segment(rule *segments.SegmentRuleModel) *segments.SegmentModel {
	return &segments.SegmentModel{
		ID:    1,
		Name:  "my_segment",
		Rules: []*segments.SegmentRuleModel{rule},
	}
}

func Organization() *organizations.OrganisationModel {
	return &organizations.OrganisationModel{
		ID:               1,
		Name:             "test Org",
		StopServingFlags: false,
		PersistTraitData: true,
		FeatureAnalytics: true,
	}
}

func Project(org *organizations.OrganisationModel, segment *segments.SegmentModel) *projects.ProjectModel {
	return &projects.ProjectModel{
		ID:                1,
		Name:              "Test Project",
		Organization:      org,
		HideDisabledFlags: false,
		Segments:          []*segments.SegmentModel{segment},
	}
}

func Feature1() *features.FeatureModel {
	return &features.FeatureModel{
		ID:   1,
		Name: "feature_1",
		Type: "STANDARD",
	}
}

func Feature2() *features.FeatureModel {
	return &features.FeatureModel{
		ID:   2,
		Name: "feature_2",
		Type: "STANDARD",
	}
}

func Environment(feature1, feature2 *features.FeatureModel, project *projects.ProjectModel) *environments.EnvironmentModel {
	return &environments.EnvironmentModel{
		ID:      1,
		APIKey:  "api-key",
		Project: project,
		FeatureStates: []*features.FeatureStateModel{
			{DjangoID: 1, Feature: feature1, Enabled: true},
			{DjangoID: 2, Feature: feature2, Enabled: false},
		},
	}
}

func Identity(env *environments.EnvironmentModel) *identities.IdentityModel {
	return &identities.IdentityModel{
		Identifier:        "identity_1",
		EnvironmentAPIKey: env.APIKey,
		CreatedDate:       utils.ISOTime{time.Now()},
	}
}

func TraitMatchingSegment(segCond *segments.SegmentConditionModel) *traits.TraitModel {
	return &traits.TraitModel{
		TraitKey:   segCond.Property,
		TraitValue: segCond.Value,
	}
}

func IdentityInSegment(trait *traits.TraitModel, env *environments.EnvironmentModel) *identities.IdentityModel {
	return &identities.IdentityModel{
		Identifier:        "identity_2",
		EnvironmentAPIKey: env.APIKey,
		IdentityTraits:    []*traits.TraitModel{trait},
	}
}

func SegmentOverrideFs(segment *segments.SegmentModel, feature *features.FeatureModel) *features.FeatureStateModel {
	return &features.FeatureStateModel{
		DjangoID: 4,
		Feature:  feature,
		Enabled:  false,
		RawValue: "segment_override",
	}
}

func MVFeatureStateValue() *features.MultivariateFeatureStateValueModel {
	id := 1
	return &features.MultivariateFeatureStateValueModel{
		ID: &id,
		MultivariateFeatureOption: &features.MultivariateFeatureOptionModel{
			ID:    1,
			Value: "test_value",
		},
		PercentageAllocation: 100,
	}
}

func EnvironmentWithSegmentOverride(
	env *environments.EnvironmentModel,
	featureState *features.FeatureStateModel,
	segment *segments.SegmentModel,
) *environments.EnvironmentModel {
	segment.FeatureStates = append(segment.FeatureStates, featureState)
	env.Project.Segments = append(env.Project.Segments, segment)
	return env
}

func GetFixtures() (*features.FeatureModel, *features.FeatureModel, *segments.SegmentModel, *environments.EnvironmentModel, *identities.IdentityModel) {
	feature1 := Feature1()
	feature2 := Feature2()
	org := Organization()
	cond := SegmentCondition()
	rule := SegmentRule(cond)
	segment := Segment(rule)
	proj := Project(org, segment)
	env := Environment(feature1, feature2, proj)
	identity := Identity(env)
	return feature1, feature2, segment, env, identity
}
