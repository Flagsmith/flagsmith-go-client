package flagengine_test

import (
	"encoding/json"
	"io/ioutil"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Flagsmith/flagsmith-go-client/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
)

const TestData = "./engine-test-data/data/environment_n9fbf9h3v4fFgH3U3ngWhb.json"

func TestEngine(t *testing.T) {
	var testData struct {
		Environment environments.EnvironmentModel `json:"environment"`
		TestCases   []struct {
			Identity identities.IdentityModel `json:"identity"`
			Response struct {
				Traits []traits.TraitModel          `json:"traits"`
				Flags  []features.FeatureStateModel `json:"flags"`
			} `json:"response"`
		} `json:"identities_and_responses"`
	}

	testSpec, err := ioutil.ReadFile(TestData)
	require.NoError(t, err)
	require.NotEmpty(t, testSpec)

	err = json.Unmarshal(testSpec, &testData)
	require.NoError(t, err)

	for i, c := range testData.TestCases {
		t.Run(strconv.Itoa(i)+":"+c.Identity.CompositeKey(), func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			actual := flagengine.GetIdentityFeatureStates(&testData.Environment, &c.Identity, nil)
			expected := c.Response.Flags

			sort.Slice(actual, func(i, j int) bool {
				return actual[i].Feature.Name < actual[j].Feature.Name
			})
			sort.Slice(expected, func(i, j int) bool {
				return expected[i].Feature.Name < expected[j].Feature.Name
			})

			require.Len(actual, len(expected))
			for i := range expected {
				id := strconv.Itoa(c.Identity.DjangoID)
				assert.Equal(expected[i].Value(id), actual[i].Value(id))
				assert.Equal(expected[i].Enabled, actual[i].Enabled)
			}
		})
	}
}
