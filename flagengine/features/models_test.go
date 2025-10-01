package features_test

import (
	"testing"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/features"
	"github.com/stretchr/testify/assert"
)

func TestFeatureStateIsHigherSegmentPriorityTwoNullFeatureSegment(t *testing.T) {
	t.Parallel()
	featureState1 := features.FeatureStateModel{}
	featureState2 := features.FeatureStateModel{}

	assert.False(t, featureState1.IsHigherSegmentPriority(&featureState2))
	assert.False(t, featureState2.IsHigherSegmentPriority(&featureState1))
}

func TestFeatureStateIsHigherSegmentPriorityOneNullFeatureSegment(t *testing.T) {
	t.Parallel()
	featureSegment := features.FeatureSegment{1}
	featureState1 := features.FeatureStateModel{FeatureSegment: &featureSegment}
	featureState2 := features.FeatureStateModel{}

	assert.True(t, featureState1.IsHigherSegmentPriority(&featureState2))
	assert.False(t, featureState2.IsHigherSegmentPriority(&featureState1))
}

func TestFeatureStateIsHigherSegmentPriority(t *testing.T) {
	t.Parallel()
	featureState1 := features.FeatureStateModel{FeatureSegment: &features.FeatureSegment{0}}
	featureState2 := features.FeatureStateModel{FeatureSegment: &features.FeatureSegment{1}}

	assert.True(t, featureState1.IsHigherSegmentPriority(&featureState2))
	assert.False(t, featureState2.IsHigherSegmentPriority(&featureState1))
}
