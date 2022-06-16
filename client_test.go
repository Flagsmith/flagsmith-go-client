package flagsmith_test

import (
	//	"context"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client"
	"github.com/stretchr/testify/assert"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"
const Feature1Value = "some_value"
const Feature1Name = "feature_1"
const Feature1ID = 1

const EnvironmentJson = `
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
		"segments": [{
			"id": 1,
			"name": "Test Segment",
			"feature_states": [],
			"rules": [{
				"type": "ALL",
				"conditions": [],
				"rules": [{
					"type": "ALL",
					"rules": [],
					"conditions": [{
						"operator": "EQUAL",
						"property_": "foo",
						"value": "bar"
					}]
				}]
			}]
		}]
	},
	"segment_overrides": [],
	"id": 1,
	"feature_states": [{
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
	}]
}
`

const FlagsJson = `
[{
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
}]
`
const IdentityResponseJson = `
{
	"flags": [{
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
	}],
	"traits": [{
		"trait_key": "foo",
		"trait_value": "bar"
	}]
}

`

func TestClientUpdatesEnvironmentOnStartForLocalEvaluation(t *testing.T) {
	// Given
	requestReceived := struct {
		mu                sync.Mutex
		isRequestReceived bool
	}{}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		requestReceived.mu.Lock()
		requestReceived.isRequestReceived = true
		requestReceived.mu.Unlock()
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
	requestReceived.mu.Lock()
	assert.True(t, requestReceived.isRequestReceived)
}

func TestClientUpdatesEnvironmentOnEachRefresh(t *testing.T) {
	// Given
	actualEnvironmentRefreshCounter := struct {
		mu    sync.Mutex
		count int
	}{}
	expectedEnvironmentRefreshCount := 3
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		actualEnvironmentRefreshCounter.mu.Lock()
		actualEnvironmentRefreshCounter.count++
		actualEnvironmentRefreshCounter.mu.Unlock()
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

	actualEnvironmentRefreshCounter.mu.Lock()
	assert.Equal(t, expectedEnvironmentRefreshCount, actualEnvironmentRefreshCounter.count)

}

func TestGetEnvironmentFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, EnvironmentJson)
		assert.NoError(t, err)
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
		_, err := io.WriteString(rw, FlagsJson)

		assert.NoError(t, err)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) flagsmith.Flag {
			return flagsmith.Flag{IsDefault: true}
		}))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))
	flag, err := flags.GetFlag(Feature1Name)

	assert.NoError(t, err)
	assert.Equal(t, Feature1Name, flag.FeatureName)
	assert.Equal(t, Feature1ID, flag.FeatureID)
	assert.Equal(t, Feature1Value, flag.Value)
	assert.False(t, flag.IsDefault)

	isEnabled, err := flags.IsFeatureEnabled(Feature1Name)

	assert.NoError(t, err)
	assert.True(t, isEnabled)

	value, err := flags.GetFeatureValue(Feature1Name)
	assert.NoError(t, err)
	assert.Equal(t, Feature1Value, value)

}
func TestGetIdentityFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, EnvironmentJson)

		assert.NoError(t, err)
	}))
	defer server.Close()
	// When
	client := flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags("test_identity", nil)

	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, Feature1Value, allFlags[0].Value)

}

func TestGetIdentityFlagsCallsAPIWhenLocalEnvironmentNotAvailableWithTraits(t *testing.T) {
	// Given
	expectedRequestBody := `{"identifier":"test_identity","traits":[{"trait_key":"stringTrait","trait_value":"trait_value"},` +
		`{"trait_key":"intTrait","trait_value":1},` +
		`{"trait_key":"floatTrait","trait_value":1.11},` +
		`{"trait_key":"boolTrait","trait_value":true}]}`

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/identities/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		// Test that we sent the correct body
		rawBody, err := ioutil.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, expectedRequestBody, string(rawBody))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err = io.WriteString(rw, IdentityResponseJson)

		assert.NoError(t, err)
	}))
	defer server.Close()
	// When
	client := flagsmith.NewClient(EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	stringTrait := flagsmith.Trait{TraitKey: "stringTrait", TraitValue: "trait_value"}
	intTrait := flagsmith.Trait{TraitKey: "intTrait", TraitValue: 1}
	floatTrait := flagsmith.Trait{TraitKey: "floatTrait", TraitValue: 1.11}
	boolTrait := flagsmith.Trait{TraitKey: "boolTrait", TraitValue: true}

	traits := []*flagsmith.Trait{&stringTrait, &intTrait, &floatTrait, &boolTrait}
	// When

	flags, err := client.GetIdentityFlags("test_identity", traits)

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, Feature1Value, allFlags[0].Value)

}

func TestDefaultHandlerIsUsedWhenNoMatchingEnvironmentFlagReturned(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		assert.Equal(t, req.URL.Path, "/api/v1/flags/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, FlagsJson)

		assert.NoError(t, err)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) flagsmith.Flag {
			return flagsmith.Flag{IsDefault: true}
		}))

	flags, err := client.GetEnvironmentFlags()
	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag("feature_that_does_not_exist")
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}
