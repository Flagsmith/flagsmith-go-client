package flagsmith_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v4"
	"github.com/Flagsmith/flagsmith-go-client/v4/fixtures"
	"github.com/stretchr/testify/assert"
)

func getTestHttpServer(t *testing.T, expectedPath string, expectedEnvKey string, expectedRequestBody *string, responseFixture string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, expectedPath)
		assert.Equal(t, expectedEnvKey, req.Header.Get("X-Environment-Key"))

		if expectedRequestBody != nil {
			// Test that we sent the correct body
			rawBody, err := io.ReadAll(req.Body)
			assert.NoError(t, err)

			// Use JSON unmarshaling to compare structures instead of direct string comparison
			var expectedJSON, actualJSON map[string]interface{}
			err = json.Unmarshal([]byte(*expectedRequestBody), &expectedJSON)
			assert.NoError(t, err)

			err = json.Unmarshal(rawBody, &actualJSON)
			assert.NoError(t, err)

			assert.Equal(t, expectedJSON["identifier"], actualJSON["identifier"])

			expectedTraits, expectedHasTraits := expectedJSON["traits"]
			actualTraits, actualHasTraits := actualJSON["traits"]

			assert.Equal(t, expectedHasTraits, actualHasTraits)

			if expectedHasTraits && actualHasTraits {
				// Compare traits if they exist
				assert.Equal(t, expectedTraits, actualTraits)
			}
		}

		rw.Header().Set("Content-Type", "application/json")

		_, err := io.WriteString(rw, responseFixture)

		assert.NoError(t, err)
	}))
}

func TestClientErrorsIfLocalEvaluationWithNonServerSideKey(t *testing.T) {
	// When, Then
	assert.Panics(t, func() {
		_ = flagsmith.MustNewClient("key", flagsmith.WithLocalEvaluation(context.Background()))
	})
}

func TestClientErrorsIfOfflineModeWithoutOfflineHandler(t *testing.T) {
	// When
	defer func() {
		if r := recover(); r != nil {
			// Then
			errMsg := fmt.Sprintf("%v", r)
			expectedErrMsg := "offline handler must be provided to use offline mode"
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.MustNewClient("key", flagsmith.WithOfflineMode())
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
			expectedErrMsg := "default flag handler and offline handler cannot be used together"
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.MustNewClient("key",
		flagsmith.WithOfflineHandler(offlineHandler),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{}, nil
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
			expectedErrMsg := "local evaluation and offline handler cannot be used together"
			assert.Equal(t, expectedErrMsg, errMsg, "Unexpected error message")
		}
	}()

	// Trigger panic
	_ = flagsmith.MustNewClient("key",
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
	_ = flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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
	_ = flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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

func TestGetFlags(t *testing.T) {
	// Given
	server := getTestHttpServer(t, "/api/v1/flags/", fixtures.EnvironmentAPIKey, nil, fixtures.FlagsJson)
	defer server.Close()

	// When
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetEnvironmentFlags(context.Background())

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetEnvironmentFlags(t *testing.T) {
	// Given
	ctx := context.Background()
	expectedEnvKey := "different"
	server := getTestHttpServer(t, "/api/v1/flags/", expectedEnvKey, nil, fixtures.FlagsJson)
	defer server.Close()

	// When
	client := flagsmith.MustNewClient(expectedEnvKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetEnvironmentFlags(ctx)
	assert.NoError(t, err)
}

func TestGetFlagsEnvironmentEvaluationContextIdentity(t *testing.T) {
	// Given
	expectedEnvKey := "different"
	server := getTestHttpServer(t, "/api/v1/identities/", expectedEnvKey, nil, fixtures.IdentityResponseJson)
	defer server.Close()

	// When
	client := flagsmith.MustNewClient(expectedEnvKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetFlags(
		context.Background(),
		flagsmith.NewEvaluationContext("test_identity", map[string]interface{}{}),
	)

	// Then
	assert.NoError(t, err)
}

func TestGetEnvironmentFlagsUseslocalEnvironmentWhenAvailable(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	// When
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{}, nil
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
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
	ctx := context.Background()

	traits := map[string]interface{}{
		"stringTrait": "trait_value",
		"intTrait":    float64(1),
		"floatTrait":  1.11,
		"boolTrait":   true,
		"NoneTrait":   nil,
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, req.URL.Path, "/api/v1/identities/")
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))

		// Test that we sent the correct body
		rawBody, err := io.ReadAll(req.Body)
		assert.NoError(t, err)

		// Parse the actual JSON instead of comparing strings directly
		var actualBody map[string]interface{}
		err = json.Unmarshal(rawBody, &actualBody)
		assert.NoError(t, err)

		assert.Equal(t, "test_identity", actualBody["identifier"])

		// Check that all expected traits are present with correct values
		traitsArray, _ := actualBody["traits"].([]interface{})
		assert.Equal(t, 5, len(traitsArray))

		for _, trait := range traitsArray {
			traitObj := trait.(map[string]interface{})
			k := traitObj["trait_key"].(string)
			v := traitObj["trait_value"]
			assert.Equal(t, traits[k], v)
		}

		rw.Header().Set("Content-Type", "application/json")

		rw.WriteHeader(http.StatusOK)
		_, err = io.WriteString(rw, fixtures.IdentityResponseJson)
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, flagsmith.NewEvaluationContext("test_identity", traits))
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{}, nil
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithRequestTimeout(10*time.Millisecond),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{}, nil
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithDefaultHandler(func(featureName string) (flagsmith.Flag, error) {
			return flagsmith.Flag{}, nil
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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

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

	client := flagsmith.MustNewClient(
		fixtures.EnvironmentAPIKey,
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
	)

	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	ec := flagsmith.NewEvaluationContext("test_identity", nil)
	segments, err := client.GetIdentitySegments(ec)
	assert.NoError(t, err)

	assert.Equal(t, 0, len(segments))
}

func TestGetIdentitySegmentsWithTraits(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	// lifted from fixtures/EnvironmentJson
	ec := flagsmith.NewEvaluationContext("test_identity", map[string]interface{}{
		"foo": "bar",
	})

	// When
	segments, err := client.GetIdentitySegments(ec)

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
	trait := flagsmith.Trait{Key: traitKey, Value: traitValue}
	data := []*flagsmith.IdentityTraits{}

	// A batch with more than 100 identities
	for i := 0; i < 102; i++ {
		data = append(data, &flagsmith.IdentityTraits{Traits: []*flagsmith.Trait{&trait}, Identifier: "test_identity"})
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

	}))
	defer server.Close()
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(ctx, data)

	// Then
	assert.Error(t, err)
	assert.Equal(t, "batch size must be less than 100", err.Error())
}

func TestBulkIdentifyReturnsErrorIfServerReturns404(t *testing.T) {
	// Given
	ctx := context.Background()
	data := []*flagsmith.IdentityTraits{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(ctx, data)

	// Then
	assert.Error(t, err)
}

func TestBulkIdentify(t *testing.T) {
	// Given
	ctx := context.Background()
	traitKey := "foo"
	traitValue := "bar"
	identifierOne := "test_identity_1"
	identifierTwo := "test_identity_2"

	trait := flagsmith.Trait{Key: traitKey, Value: traitValue}
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

	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

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

	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx), flagsmith.WithProxy(server.URL),
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

	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey, flagsmith.WithOfflineMode(), flagsmith.WithOfflineHandler(offlineHandler))

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
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithErrorHandler(func(handler *flagsmith.FlagsmithAPIError) {
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

func TestRealtime(t *testing.T) {
	// Given
	mux := http.NewServeMux()
	requestCount := struct {
		mu    sync.Mutex
		count int
	}{}

	mux.HandleFunc("/api/v1/environment-document/", func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
		requestCount.mu.Lock()
		requestCount.count++
		requestCount.mu.Unlock()

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.EnvironmentJson)
		if err != nil {
			panic(err)
		}
		assert.NoError(t, err)
	})
	mux.HandleFunc(fmt.Sprintf("/sse/environments/%s/stream", fixtures.ClientAPIKey), func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "GET", req.Method)

		// Set the necessary headers for SSE
		rw.Header().Set("Content-Type", "text/event-stream")
		rw.Header().Set("Cache-Control", "no-cache")
		rw.Header().Set("Connection", "keep-alive")

		// Flush headers to the client
		flusher, _ := rw.(http.Flusher)
		flusher.Flush()

		// Use an `updated_at` value that is older than the `updated_at` set on the environment document
		// to ensure an older timestamp does not trigger an update.
		sendUpdatedAtSSEEvent(rw, flusher, 1640995200.079725)
		time.Sleep(10 * time.Millisecond)

		// Update the `updated_at`(to trigger the environment update)
		sendUpdatedAtSSEEvent(rw, flusher, 1733480514.079725)
		time.Sleep(10 * time.Millisecond)
	})

	ctx := context.Background()

	server := httptest.NewServer(mux)
	defer server.Close()

	// When
	client := flagsmith.MustNewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithRealtime(),
		flagsmith.WithRealtimeBaseURL(server.URL+"/"),
	)
	// Sleep to ensure that the server has time to update the environment
	time.Sleep(10 * time.Millisecond)

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)

	// Sleep to ensure that the server has time to update the environment
	// (After the second sse event)
	time.Sleep(10 * time.Millisecond)

	requestCount.mu.Lock()
	assert.Equal(t, 2, requestCount.count)
}
func sendUpdatedAtSSEEvent(rw http.ResponseWriter, flusher http.Flusher, updatedAt float64) {
	// Format the SSE event with the provided updatedAt value
	sseEvent := fmt.Sprintf(`event: environment_updated
data: {"updated_at": %f}

`, updatedAt)

	// Write the SSE event to the response
	_, err := io.WriteString(rw, sseEvent)
	if err != nil {
		http.Error(rw, "Failed to send SSE event", http.StatusInternalServerError)
		return
	}

	// Flush the event to the client
	flusher.Flush()
}
