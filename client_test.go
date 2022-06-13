package flagsmith_test

import (
	//	"context"
	"testing"
	"time"
	"context"
	//"io/ioutil"
	"io"
	flagsmith "github.com/Flagsmith/flagsmith-go-client"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/identities/traits"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"
const Feature1Value = "some_value"
const Feature1Name = "feature_1"
const Feature1ID = 1

const EnvironmentJson  = `
{
            "api_key": "B62qaMZNwfiqT76p38ggrQ",
            "project": {
                "name": "Test project",
                "organisation": {
                    "feature_analytics": false,
                    "name": "Test Org",
                    "id": 1,
                    "persist_trait_data": true,
                    "stop_serving_flags": false
                },
                "id": 1,
                "hide_disabled_flags": false,
                "segments": [
                    {
                        "id": 1,
                        "name": "Test Segment",
                        "feature_states":[],
                        "rules": [
                            {
                                "type": "ALL",
                                "conditions": [],
                                "rules": [
                                    {
                                        "type": "ALL",
                                        "rules": [],
                                        "conditions": [
                                            {
                                                "operator": "EQUAL",
                                                "property_": "foo",
                                                "value": "bar"
                                            }
                                        ]
                                    }
                                ]
                            }
                        ]
                    }
                ]
            },
            "segment_overrides": [],
            "id": 1,
            "feature_states": [
                {
                    "multivariate_feature_state_values": [],
                    "feature_state_value": "some_value",
                    "id": 1,
                    "featurestate_uuid": "40eb539d-3713-4720-bbd4-829dbef10d51",
                    "feature": {
                        "name": "feature_1",
                        "type": "STANDARD",
                        "id": 1
                    },
                    "segment_id": null,
                    "enabled": true
                }
            ]
    }

`

const FlagsJson = `
        [
                {
                    "id": 1,
                    "feature": {
                        "id": 1,
                        "name": "feature_1",
                        "created_date": "2019-08-27T14:53:45.698555Z",
                        "initial_value": null,
                        "description": null,
                        "default_enabled": false,
                        "type": "STANDARD",
                        "project": 1
                    },
                    "feature_state_value": "some_value",
                    "enabled": true,
                    "environment": 1,
                    "identity": null,
                    "feature_segment": null
                }
    ]

`

func TestClientUpdatesEnvironmentOnStartForLocalEvaluation(t *testing.T) {
	// Given
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		requestReceived = true
		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// Sleep to ensure that the server has time to update the environment
	time.Sleep(10 * time.Millisecond)

	// Then
	assert.True(t, requestReceived)
}

func TestClientUpdatesEnvironmentOnEachRefresh(t *testing.T) {
	// Given
	actualEnvironmentRefreshCount := 0
	expectedEnvironmentRefreshCount := 3
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		actualEnvironmentRefreshCount++
		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithEnvironmentRefreshInterval(100*time.Millisecond),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	time.Sleep(250 * time.Millisecond)

	// Then
	// We should have called refresh environment 3 times
	// one when the client starts and 2
	// for each time the refresh interval expires
	assert.Equal(t, expectedEnvironmentRefreshCount, actualEnvironmentRefreshCount)

}

func TestGetEnvironmentFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, EnvironmentJson)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags()
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, Feature1Value, allFlags[0].Value)

}

func TestGetEnvironmentFlagsCallsAPIWhenLocalEnvironmentNotAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/flags/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, FlagsJson)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(EnvironmentAPIKey,flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, Feature1Value, allFlags[0].Value)


}
func TestGetIdentityFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, EnvironmentJson)
	}))
	defer server.Close()
	// When
	client := flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	trait := traits.TraitModel{TraitKey: "trait_key", TraitValue: "trait_value"}
	traits := []*traits.TraitModel{&trait}
	flags, err := client.GetIdentityFlags("test_identity", traits)

	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, Feature1Value, allFlags[0].Value)


}
