package features_test

import (
	"math/big"
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

func TestMultivariateFeatureStateValueModelPriorityWithID(t *testing.T) {
	t.Parallel()
	id := 42
	mfsv := features.MultivariateFeatureStateValueModel{
		ID: &id,
	}

	priority := mfsv.Priority()
	expected := *big.NewInt(42)

	assert.Equal(t, expected, priority, "Priority should equal the ID value")
}

func TestMultivariateFeatureStateValueModelPriorityWithUUID(t *testing.T) {
	t.Parallel()
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	mfsv := features.MultivariateFeatureStateValueModel{
		MVFSValueUUID: uuid,
	}

	priority := mfsv.Priority()

	// Parse the expected value from the UUID (without hyphens, as hex)
	expectedBigInt := new(big.Int)
	expectedBigInt.SetString("550e8400e29b41d4a716446655440000", 16)

	assert.Equal(t, *expectedBigInt, priority, "Priority should equal the UUID parsed as big int")
}

func TestMultivariateFeatureStateValueModelPriorityIDTakesPrecedenceOverUUID(t *testing.T) {
	t.Parallel()
	id := 100
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	mfsv := features.MultivariateFeatureStateValueModel{
		ID:            &id,
		MVFSValueUUID: uuid,
	}

	priority := mfsv.Priority()
	expected := *big.NewInt(100)

	assert.Equal(t, expected, priority, "Priority should use ID when both ID and UUID are present")
}

func TestMultivariateFeatureStateValueModelPriorityDefaultsToMaxInt64(t *testing.T) {
	t.Parallel()
	mfsv := features.MultivariateFeatureStateValueModel{}

	priority := mfsv.Priority()
	expected := *big.NewInt(9223372036854775807) // math.MaxInt64

	assert.Equal(t, expected, priority, "Priority should default to max int64 when neither ID nor UUID is set")
}

func TestMultivariateFeatureStateValueModelPriorityWithInvalidUUID(t *testing.T) {
	t.Parallel()
	mfsv := features.MultivariateFeatureStateValueModel{
		MVFSValueUUID: "not-a-valid-uuid",
	}

	priority := mfsv.Priority()
	expected := *big.NewInt(9223372036854775807) // Should default to max int64

	assert.Equal(t, expected, priority, "Priority should default to max int64 when UUID is invalid")
}
