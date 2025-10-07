package flagengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
)

const TestDataDir = "./engine-test-data/test_cases"

func TestEngine(t *testing.T) {
	t.Parallel()

	// Read all test case files from the test_cases directory
	files, err := filepath.Glob(filepath.Join(TestDataDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "No test case files found in %s", TestDataDir)

	for _, testFile := range files {
		testFile := testFile // Capture range variable
		testName := strings.TrimSuffix(filepath.Base(testFile), ".json")

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			// Read the test case file
			testSpec, err := os.ReadFile(testFile)
			require.NoError(t, err)
			require.NotEmpty(t, testSpec)

			// Parse the test case
			var testCase struct {
				Context engine_eval.EngineEvaluationContext `json:"context"`
				Result  engine_eval.EvaluationResult        `json:"result"`
			}

			err = json.Unmarshal(testSpec, &testCase)
			require.NoError(t, err)

			// Run the evaluation
			actual := flagengine.GetEvaluationResult(&testCase.Context)
			expected := testCase.Result

			// Compare the results
			assert.Equal(t, expected.Flags, actual.Flags)
		})
	}
}
