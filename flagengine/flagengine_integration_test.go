package flagengine_test

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/engine_eval"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
)

const TestData = "./engine-test-data/data/environment_n9fbf9h3v4fFgH3U3ngWhb.json"

func TestEngine(t *testing.T) {
	t.Parallel()
	var testData struct {
		Environment environments.EnvironmentModel `json:"environment"`
		TestCases   []struct {
			EvaluationContext engine_eval.EngineEvaluationContext `json:"context"`
			EvaluationResult  engine_eval.EvaluationResult        `json:"result"`
		} `json:"test_cases"`
	}

	testSpec, err := os.ReadFile(TestData)
	require.NoError(t, err)
	require.NotEmpty(t, testSpec)

	err = json.Unmarshal(testSpec, &testData)
	require.NoError(t, err)

	for i, c := range testData.TestCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			actual := flagengine.GetEvaluationResult(&c.EvaluationContext)
			expected := c.EvaluationResult

			sort.Slice(actual.Flags, func(i, j int) bool {
				return actual.Flags[i].FeatureKey < actual.Flags[j].FeatureKey
			})
			sort.Slice(expected.Flags, func(i, j int) bool {
				return expected.Flags[i].FeatureKey < expected.Flags[j].FeatureKey
			})

			require.Len(actual.Flags, len(expected.Flags))
			for i := range expected.Flags {
				assert.Equal(expected.Flags[i].Value, actual.Flags[i].Value)
				assert.Equal(expected.Flags[i].Enabled, actual.Flags[i].Enabled)
				assert.Equal(expected.Flags[i].FeatureKey, actual.Flags[i].FeatureKey)
			}
		})
	}
}
