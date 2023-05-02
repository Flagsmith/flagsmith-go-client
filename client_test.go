package flagsmith_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v2"
	"github.com/Flagsmith/flagsmith-go-client/v2/fixtures"
	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
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
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
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
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags()
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetEnvironmentFlagsCallsAPIWhenLocalEnvironmentNotAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, "/api/v1/flags/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)

		assert.NoError(t, err)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))
	flag, err := flags.GetFlag(fixtures.Feature1Name)

	assert.NoError(t, err)
	assert.Equal(t, fixtures.Feature1Name, flag.FeatureName)
	assert.Equal(t, fixtures.Feature1ID, flag.FeatureID)
	assert.Equal(t, fixtures.Feature1Value, flag.Value)
	assert.False(t, flag.IsDefault)

	isEnabled, err := flags.IsFeatureEnabled(fixtures.Feature1Name)

	assert.NoError(t, err)
	assert.True(t, isEnabled)

	value, err := flags.GetFeatureValue(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.Equal(t, fixtures.Feature1Value, value)
}

func TestGetIdentityFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()
	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags("test_identity", nil)

	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetIdentityFlagsCallsAPIWhenLocalEnvironmentNotAvailableWithTraits(t *testing.T) {
	// Given
	expectedRequestBody := `{"identifier":"test_identity","traits":[{"trait_key":"stringTrait","trait_value":"trait_value"},` +
		`{"trait_key":"intTrait","trait_value":1},` +
		`{"trait_key":"floatTrait","trait_value":1.11},` +
		`{"trait_key":"boolTrait","trait_value":true},` +
		`{"trait_key":"NoneTrait","trait_value":null}]}`

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, "/api/v1/identities/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		// Test that we sent the correct body
		rawBody, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, expectedRequestBody, string(rawBody))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err = io.WriteString(rw, fixtures.IdentityResponseJson)

		assert.NoError(t, err)
	}))
	defer server.Close()
	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	stringTrait := flagsmith.Trait{TraitKey: "stringTrait", TraitValue: "trait_value"}
	intTrait := flagsmith.Trait{TraitKey: "intTrait", TraitValue: 1}
	floatTrait := flagsmith.Trait{TraitKey: "floatTrait", TraitValue: 1.11}
	boolTrait := flagsmith.Trait{TraitKey: "boolTrait", TraitValue: true}
	nillTrait := flagsmith.Trait{TraitKey: "NoneTrait", TraitValue: nil}

	traits := []*flagsmith.Trait{&stringTrait, &intTrait, &floatTrait, &boolTrait, &nillTrait}
	// When

	flags, err := client.GetIdentityFlags("test_identity", traits)

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestDefaultHandlerIsUsedWhenNoMatchingEnvironmentFlagReturned(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, "/api/v1/flags/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)

		assert.NoError(t, err)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag("feature_that_does_not_exist")
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestDefaultHandlerIsUsedWhenTimeout(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, "/api/v1/flags/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")
		time.Sleep(20 * time.Millisecond)
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)

		assert.NoError(t, err)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithRequestTimeout(10*time.Millisecond),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestDefaultHandlerIsUsedWhenRequestFails(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.FlagsAPIHandlerWithInternalServerError))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))

	flags, err := client.GetEnvironmentFlags()

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag("feature_that_does_not_exist")
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestFlagsmithAPIErrorIsReturnedIfRequestFailsWithoutDefaultHandler(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.FlagsAPIHandlerWithInternalServerError))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetEnvironmentFlags()
	assert.Error(t, err)
	var flagErr *flagsmith.FlagsmithAPIError
	assert.True(t, errors.As(err, &flagErr))
}

func TestGetIdentitySegmentsNoTraits(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	err := client.UpdateEnvironment(context.Background())
	assert.NoError(t, err)

	segments, err := client.GeIdentitySegments("test_identity", nil)
	assert.NoError(t, err)

	assert.Equal(t, 0, len(segments))
}

func TestGetIdentitySegmentsWithTraits(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	err := client.UpdateEnvironment(context.Background())
	assert.NoError(t, err)

	// lifted from fixtures/EnvironmentJson
	trait_key := "foo"
	trait_value := "bar"

	trait := flagsmith.Trait{TraitKey: trait_key, TraitValue: trait_value}

	traits := []*flagsmith.Trait{&trait}

	// When
	segments, err := client.GeIdentitySegments("test_identity", traits)

	// Then
	assert.NoError(t, err)

	assert.Equal(t, 1, len(segments))
	assert.Equal(t, "Test Segment", segments[0].Name)
}

func TestBulkIdentifyReturnsErrorIfBatchSizeIsTooLargeToProcess(t *testing.T) {
	// Given
	traitKey := "foo"
	traitValue := "bar"
	trait := flagsmith.Trait{TraitKey: traitKey, TraitValue: traitValue}
	data := []*flagsmith.IdentityTraits{}

	// A batch with more than 100 identities
	for i := 0; i < 102; i++ {
		data = append(data, &flagsmith.IdentityTraits{Traits: []*flagsmith.Trait{&trait}, Identifier: "test_identity"})
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

	}))
	defer server.Close()
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(data)

	// Then
	assert.Error(t, err)
	assert.Equal(t, "flagsmith: batch size must be less than 100", err.Error())
}

func TestBulkIdentifyReturnsErrorIfServerReturns404(t *testing.T) {
	// Given
	data := []*flagsmith.IdentityTraits{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(data)

	// Then
	assert.Error(t, err)
	assert.Equal(t, "flagsmith: Bulk identify endpoint not found; Please make sure you are using Edge API endpoint", err.Error())
}

func TestBulkIdentify(t *testing.T) {
	// Given
	traitKey := "foo"
	traitValue := "bar"
	identifierOne := "test_identity_1"
	identifierTwo := "test_identity_2"

	trait := flagsmith.Trait{TraitKey: traitKey, TraitValue: traitValue}
	data := []*flagsmith.IdentityTraits{
		{Traits: []*flagsmith.Trait{&trait}, Identifier: identifierOne},
		{Traits: []*flagsmith.Trait{&trait}, Identifier: identifierTwo},
	}

	expectedRequestBody := fmt.Sprintf(`{"data":[{"identifier":"%s","traits":[{"trait_key":"%s","trait_value":"%s"}]},`+
		`{"identifier":"%s","traits":[{"trait_key":"%s","trait_value":"%s"}]}]}`,
		identifierOne, traitKey, traitValue, identifierTwo, traitKey, traitValue)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "POST", req.Method)
		assert.Equal(t, req.URL.Path, "/api/v1/bulk-identities/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		rawBody, err := io.ReadAll(req.Body)
		assert.Equal(t, expectedRequestBody, string(rawBody))
		assert.NoError(t, err)

		rw.Header().Set("Content-Type", "application/json")
	}))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(data)

	// Then
	assert.NoError(t, err)
}

func TestWithProxyClientOption(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(), flagsmith.WithProxy(server.URL),
		flagsmith.WithBaseURL("http://some-other-url-that-should-not-be-used/api/v1/"))

	err := client.UpdateEnvironment(context.Background())

	// Then
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags()
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}
