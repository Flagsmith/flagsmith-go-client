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

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v3"
	"github.com/Flagsmith/flagsmith-go-client/v3/fixtures"
	"github.com/stretchr/testify/assert"
)

func TestClientErrorsIfLocalEvaluationWithNonServerSideKey(t *testing.T) {
	// When, Then
	assert.Panics(t, func() {
		_ = flagsmith.NewClient("key", flagsmith.WithLocalEvaluation(context.Background()))
	})
}

func TestClientErrorsIfOfflineModeWithoutOfflineHandler(t *testing.T) {
	// When
	defer func() {
		if r := recover(); r != nil {
			// Then
			errMsg := fmt.Sprintf("%v", r)
			expectedErrMsg := "offline handler must be provided to use offline mode."
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.NewClient("key", flagsmith.WithOfflineMode())
}

func TestClientErrorsIfDefaultHandlerAndOfflineHandlerAreBothSet(t *testing.T) {
	// Given
	envJsonPath := "./fixtures/environment.json"
	offlineHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)
	assert.NoError(t, err)

	// When
	defer func() {
		if r := recover(); r != nil {
			// Then
			errMsg := fmt.Sprintf("%v", r)
			expectedErrMsg := "default flag handler and offline handler cannot be used together."
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.NewClient("key",
		flagsmith.WithOfflineHandler(offlineHandler),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))
}
func TestClientErrorsIfLocalEvaluationModeAndOfflineHandlerAreBothSet(t *testing.T) {
	// Given
	envJsonPath := "./fixtures/environment.json"
	offlineHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)
	assert.NoError(t, err)

	// When
	defer func() {
		if r := recover(); r != nil {
			// Then
			errMsg := fmt.Sprintf("%v", r)
			expectedErrMsg := "local evaluation and offline handler cannot be used together."
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.NewClient("key",
		flagsmith.WithOfflineHandler(offlineHandler),
		flagsmith.WithLocalEvaluation(context.Background()))
}

func TestClientUpdatesEnvironmentOnStartForLocalEvaluation(t *testing.T) {
	// Given
	ctx := context.Background()
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

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.EnvironmentJson)
		if err != nil {
			panic(err)
		}
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// Sleep to ensure that the server has time to update the environment
	time.Sleep(10 * time.Millisecond)

	// Then
	requestReceived.mu.Lock()
	assert.True(t, requestReceived.isRequestReceived)
}

func TestClientUpdatesEnvironmentOnEachRefresh(t *testing.T) {
	// Given
	ctx := context.Background()
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

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.EnvironmentJson)
		if err != nil {
			panic(err)
		}
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags(ctx)
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetEnvironmentFlagsCallsAPIWhenLocalEnvironmentNotAvailable(t *testing.T) {
	// Given
	ctx := context.Background()
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

	flags, err := client.GetEnvironmentFlags(ctx)

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
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()
	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags(ctx, "test_identity", nil)

	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetIdentityFlagsUseslocalOverridesWhenAvailable(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()
	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags(ctx, "overridden-id", nil)

	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1OverriddenValue, allFlags[0].Value)
}

func TestGetIdentityFlagsCallsAPIWhenLocalEnvironmentNotAvailableWithTraits(t *testing.T) {
	// Given
	ctx := context.Background()
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

	flags, err := client.GetIdentityFlags(ctx, "test_identity", traits)

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
	ctx := context.Background()
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

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag("feature_that_does_not_exist")
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestDefaultHandlerIsUsedWhenTimeout(t *testing.T) {
	// Given
	ctx := context.Background()
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

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestDefaultHandlerIsUsedWhenRequestFails(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.FlagsAPIHandlerWithInternalServerError))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{IsDefault: true}, nil
		}))

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then
	assert.NoError(t, err)

	flag, err := flags.GetFlag("feature_that_does_not_exist")
	assert.NoError(t, err)
	assert.True(t, flag.IsDefault)
}

func TestFlagsmithAPIErrorIsReturnedIfRequestFailsWithoutDefaultHandler(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.FlagsAPIHandlerWithInternalServerError))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetEnvironmentFlags(ctx)
	assert.Error(t, err)
	var flagErr *flagsmith.FlagsmithClientError
	assert.True(t, errors.As(err, &flagErr))
}

func TestGetIdentitySegmentsNoTraits(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	segments, err := client.GetIdentitySegments("test_identity", nil)
	assert.NoError(t, err)

	assert.Equal(t, 0, len(segments))
}

func TestGetIdentitySegmentsWithTraits(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	// lifted from fixtures/EnvironmentJson
	trait_key := "foo"
	trait_value := "bar"

	trait := flagsmith.Trait{TraitKey: trait_key, TraitValue: trait_value}

	traits := []*flagsmith.Trait{&trait}

	// When
	segments, err := client.GetIdentitySegments("test_identity", traits)

	// Then
	assert.NoError(t, err)

	assert.Equal(t, 1, len(segments))
	assert.Equal(t, "Test Segment", segments[0].Name)
}

func TestBulkIdentifyReturnsErrorIfBatchSizeIsTooLargeToProcess(t *testing.T) {
	// Given
	ctx := context.Background()
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
	err := client.BulkIdentify(ctx, data)

	// Then
	assert.Error(t, err)
	assert.Equal(t, "flagsmith: batch size must be less than 100", err.Error())
}

func TestBulkIdentifyReturnsErrorIfServerReturns404(t *testing.T) {
	// Given
	ctx := context.Background()
	data := []*flagsmith.IdentityTraits{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(ctx, data)

	// Then
	assert.Error(t, err)
	assert.Equal(t, "flagsmith: Bulk identify endpoint not found; Please make sure you are using Edge API endpoint", err.Error())
}

func TestBulkIdentify(t *testing.T) {
	// Given
	ctx := context.Background()
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
	err := client.BulkIdentify(ctx, data)

	// Then
	assert.NoError(t, err)
}

func TestWithProxyClientOption(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx), flagsmith.WithProxy(server.URL),
		flagsmith.WithBaseURL("http://some-other-url-that-should-not-be-used/api/v1/"))

	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags(ctx)
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestOfflineMode(t *testing.T) {
	// Given
	ctx := context.Background()

	envJsonPath := "./fixtures/environment.json"
	offlineHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)
	assert.NoError(t, err)

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithOfflineMode(), flagsmith.WithOfflineHandler(offlineHandler))

	// Then
	flags, err := client.GetEnvironmentFlags(ctx)
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)

	// And GetIdentityFlags works as well
	flags, err = client.GetIdentityFlags(ctx, "test_identity", nil)
	assert.NoError(t, err)

	allFlags = flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestOfflineHandlerIsUsedWhenRequestFails(t *testing.T) {
	// Given
	ctx := context.Background()

	envJsonPath := "./fixtures/environment.json"
	offlineHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithOfflineHandler(offlineHandler),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// Then
	flags, err := client.GetEnvironmentFlags(ctx)
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)

	// And GetIdentityFlags works as well
	flags, err = client.GetIdentityFlags(ctx, "test_identity", nil)
	assert.NoError(t, err)

	allFlags = flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestPollErrorHandlerIsUsedWhenPollFails(t *testing.T) {
	// Given
	ctx := context.Background()
	var capturedError error
	var statusCode int
	var status string

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithEnvironmentRefreshInterval(time.Duration(2)*time.Second),
		flagsmith.WithErrorHandler(func(handler flagsmith.FlagsmithErrorHandler) {
			capturedError = handler.Err
			statusCode = handler.ResponseStatusCode
			status = handler.ResponseStatus
		}),
	)

	// when
	_ = client.UpdateEnvironment(ctx)

	// Then
	assert.Equal(t, capturedError, nil)
	assert.Equal(t, statusCode, 500)
	assert.Equal(t, status, "500 Internal Server Error")
}
