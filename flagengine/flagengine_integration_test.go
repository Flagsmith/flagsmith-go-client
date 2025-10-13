package flagengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/hujson"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
)

const TestDataDir = "./engine-test-data/test_cases"

func TestEngine(t *testing.T) {
	t.Parallel()

	// Read all test case files from the test_cases directory (both .json and .jsonc)
	jsonFiles, err := filepath.Glob(filepath.Join(TestDataDir, "*.json"))
	require.NoError(t, err)

	jsoncFiles, err := filepath.Glob(filepath.Join(TestDataDir, "*.jsonc"))
	require.NoError(t, err)

	files := append(jsonFiles, jsoncFiles...)
	require.NotEmpty(t, files, "No test case files found in %s", TestDataDir)

	for _, testFile := range files {
		testFile := testFile // Capture range variable

		// Get test name by removing extension
		testName := filepath.Base(testFile)
		testName = strings.TrimSuffix(testName, filepath.Ext(testName))

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			// Read the test case file
			testSpec, err := os.ReadFile(testFile)
			require.NoError(t, err)
			require.NotEmpty(t, testSpec)

			// Standardise .jsonc files to standard JSON
			if strings.HasSuffix(testFile, ".jsonc") {
				ast, err := hujson.Parse(testSpec)
				require.NoError(t, err)
				ast.Standardize()
				testSpec = ast.Pack()
			}

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
			assert.Equal(t, expected.Flags, actual.Flags, "Flags should match")

			// Compare segments - check key and name only since metadata is an implementation detail
			if len(expected.Segments) > 0 {
				require.Len(t, actual.Segments, len(expected.Segments), "Segment count should match")
				for i, expectedSeg := range expected.Segments {
					assert.Equal(t, expectedSeg.Key, actual.Segments[i].Key, "Segment key should match")
					assert.Equal(t, expectedSeg.Name, actual.Segments[i].Name, "Segment name should match")
				}
			}
		})
	}
}
