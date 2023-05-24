package utils_test

import (
	"fmt"
	"sort"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v3/flagengine/utils"
)

func TestGetHashedPercentageForObjectIds(t *testing.T) {
	cases := []struct {
		name       string
		ids        []string
		iterations int
		expected   float64
	}{
		{
			name:       "foobar",
			ids:        []string{"foo", "bar"},
			iterations: 3,
			expected:   85.37707541508301, // this was generated using REPL and flagsmith-engine code
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := utils.GetHashedPercentageForObjectIds(c.ids, c.iterations)
			assert.InEpsilon(t, c.expected, actual, 1e-6)
		})
	}
}

func TestGetHashedPercentageForObjectIdsIsNumberBetween0incAnd100Exc(t *testing.T) {
	cases := []struct {
		objectsIds []string
	}{
		{[]string{"12", "93"}},
		{[]string{uuid.Must(uuid.NewUUID()).String(), "99"}},
		{[]string{"99", uuid.Must(uuid.NewUUID()).String()}},
		{[]string{uuid.Must(uuid.NewUUID()).String(), uuid.Must(uuid.NewUUID()).String()}},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c.objectsIds), func(t *testing.T) {
			val := utils.GetHashedPercentageForObjectIds(c.objectsIds, 1)
			assert.GreaterOrEqual(t, val, 0.0)
			assert.Less(t, val, 100.0)
		})
	}
}

func TestGetHashedPercentageForObjectIdsIsSameEachTime(t *testing.T) {
	cases := []struct {
		objectsIds []string
	}{
		{[]string{"12", "93"}},
		{[]string{uuid.Must(uuid.NewUUID()).String(), "99"}},
		{[]string{"99", uuid.Must(uuid.NewUUID()).String()}},
		{[]string{uuid.Must(uuid.NewUUID()).String(), uuid.Must(uuid.NewUUID()).String()}},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c.objectsIds), func(t *testing.T) {
			val1 := utils.GetHashedPercentageForObjectIds(c.objectsIds, 1)
			val2 := utils.GetHashedPercentageForObjectIds(c.objectsIds, 1)
			assert.InEpsilon(t, val1, val2, 1e-6)
		})
	}
}

func TestPercentageValueIsUniqueForDifferentIdentities(t *testing.T) {
	first := []string{"14", "106"}
	second := []string{"53", "200"}

	val1 := utils.GetHashedPercentageForObjectIds(first, 1)
	val2 := utils.GetHashedPercentageForObjectIds(second, 1)

	assert.NotEqual(t, val1, val2)
}

func TestPercentageIsEvenlyDistributed(t *testing.T) {
	testSamples := 500
	testBuckets := 50
	testBucketSize := testSamples / testBuckets
	errorFactor := -0.1

	pairs := make([][2]string, 0, testSamples*testSamples)
	for i := 0; i < testSamples; i++ {
		for j := 0; j < testSamples; j++ {
			pairs = append(pairs, [2]string{strconv.Itoa(i), strconv.Itoa(j)})
		}
	}

	values := make([]float64, len(pairs))
	for i, pair := range pairs {
		values[i] = utils.GetHashedPercentageForObjectIds(pair[:], 1)
	}
	sort.Float64s(values)

	for i := 0; i < testBuckets; i++ {
		bucketStart := i * testBucketSize
		bucketEnd := (i + 1) * testBucketSize
		bucketValueLimit := float64(i+1)/float64(testBuckets) + errorFactor*(float64(i+1)/float64(testBuckets))
		if bucketValueLimit > 1.0 {
			bucketValueLimit = 1.0
		}

		for _, value := range values[bucketStart:bucketEnd] {
			assert.Less(t, value, bucketValueLimit)
		}
	}
}
