package flagsmith_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v5"
	"github.com/Flagsmith/flagsmith-go-client/v5/fixtures"
	"github.com/go-resty/resty/v2"
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

			assert.Equal(t, *expectedRequestBody, string(rawBody))
		}

		rw.Header().Set("Content-Type", "application/json")

		_, err := io.WriteString(rw, responseFixture)

		assert.NoError(t, err)
	}))
}

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

func TestUserAgentHeaderIsSent(t *testing.T) {
	// Given
	userAgentReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		userAgentReceived = req.Header.Get("User-Agent")
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.EnvironmentJson)
		if err != nil {
			panic(err)
		}
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	_, _ = client.GetEnvironmentFlags(context.Background())

	// Then
	// Get the expected User-Agent value from the SDK's getUserAgent() function
	expectedUserAgent := flagsmith.GetUserAgentForTest()

	assert.NotEmpty(t, userAgentReceived, "User-Agent header should be sent")
	assert.Equal(t, expectedUserAgent, userAgentReceived,
		"User-Agent header should match the value returned by getUserAgent()")

	// Verify basic format requirements
	assert.True(t, strings.HasPrefix(userAgentReceived, "flagsmith-go-sdk/"),
		"User-Agent should start with 'flagsmith-go-sdk/', got: %s", userAgentReceived)
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

func TestGetFlags(t *testing.T) {
	// Given
	ctx := context.Background()
	server := getTestHttpServer(t, "/api/v1/flags/", fixtures.EnvironmentAPIKey, nil, fixtures.FlagsJson)
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, nil)

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetFlagsTransientIdentity(t *testing.T) {
	// Given
	identifier := "transient"
	transient := true
	ctx := context.Background()
	expectedRequestBody := `{"identifier":"transient","transient":true}`
	server := getTestHttpServer(t, "/api/v1/identities/", fixtures.EnvironmentAPIKey, &expectedRequestBody, fixtures.IdentityResponseJson)
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, &flagsmith.EvaluationContext{Identity: &flagsmith.IdentityEvaluationContext{Identifier: &identifier, Transient: &transient}})

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetFlagsTransientTraits(t *testing.T) {
	// Given
	identifier := "test_identity"
	transient := true
	ctx := context.Background()
	expectedRequestBody := `{"identifier":"test_identity","traits":` +
		`[{"trait_key":"NullTrait","trait_value":null},` +
		`{"trait_key":"StringTrait","trait_value":"value"},` +
		`{"trait_key":"TransientTrait","trait_value":"value","transient":true}]}`
	server := getTestHttpServer(t, "/api/v1/identities/", fixtures.EnvironmentAPIKey, &expectedRequestBody, fixtures.IdentityResponseJson)
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(
		ctx,
		&flagsmith.EvaluationContext{
			Identity: &flagsmith.IdentityEvaluationContext{
				Identifier: &identifier,
				Traits: map[string]*flagsmith.TraitEvaluationContext{
					"NullTrait":   nil,
					"StringTrait": {Value: "value"},
					"TransientTrait": {
						Value:     "value",
						Transient: &transient,
					},
				},
			},
		})

	// Then
	assert.NoError(t, err)

	allFlags := flags.AllFlags()

	assert.Equal(t, 1, len(allFlags))

	assert.Equal(t, fixtures.Feature1Name, allFlags[0].FeatureName)
	assert.Equal(t, fixtures.Feature1ID, allFlags[0].FeatureID)
	assert.Equal(t, fixtures.Feature1Value, allFlags[0].Value)
}

func TestGetFlagsEnvironmentEvaluationContextFlags(t *testing.T) {
	// Given
	ctx := context.Background()
	expectedEnvKey := "different"
	server := getTestHttpServer(t, "/api/v1/flags/", expectedEnvKey, nil, fixtures.FlagsJson)
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetFlags(
		ctx,
		&flagsmith.EvaluationContext{
			Environment: &flagsmith.EnvironmentEvaluationContext{APIKey: expectedEnvKey},
		})

	// Then
	assert.NoError(t, err)
}

func TestGetFlagsEnvironmentEvaluationContextIdentity(t *testing.T) {
	// Given
	identifier := "test_identity"
	ctx := context.Background()
	expectedEnvKey := "different"
	server := getTestHttpServer(t, "/api/v1/identities/", expectedEnvKey, nil, fixtures.IdentityResponseJson)
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	_, err := client.GetFlags(
		ctx,
		&flagsmith.EvaluationContext{
			Environment: &flagsmith.EnvironmentEvaluationContext{APIKey: expectedEnvKey},
			Identity:    &flagsmith.IdentityEvaluationContext{Identifier: &identifier},
		})

	// Then
	assert.NoError(t, err)
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
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithRealtime(),
		flagsmith.WithRealtimeBaseURL(server.URL+"/"),
	)
	// Sleep to ensure that the server has time to update the environment
	time.Sleep(10 * time.Millisecond)

	flags, err := client.GetFlags(ctx, nil)

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

func TestWithSlogLogger(t *testing.T) {
	// Given
	var logOutput strings.Builder
	slogLogger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// When
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithSlogLogger(slogLogger))

	// Then
	logStr := logOutput.String()
	t.Log(logStr)
	assert.Contains(t, logStr, "initialising Flagsmith client")
}

func TestWithPollingWorksWithRealtime(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	// guard against data race from goroutines logging at the same time
	var logOutput strings.Builder
	var logMu sync.Mutex
	slogLogger := slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (n int, err error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logOutput.Write(p)
	}), &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Given
	_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithSlogLogger(slogLogger),
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithRealtime(),
		flagsmith.WithPolling(),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	time.Sleep(500 * time.Millisecond)

	// Then
	logMu.Lock()
	logStr := logOutput.String()
	logMu.Unlock()
	assert.Contains(t, logStr, "worker=poll")
	assert.Contains(t, logStr, "worker=realtime")
}

// writerFunc implements io.Writer.
type writerFunc func(p []byte) (n int, err error)

func (f writerFunc) Write(p []byte) (n int, err error) {
	return f(p)
}

// Helper function to implement a header interceptor.
func roundTripperWithHeader(key, value string) http.RoundTripper {
	return &injectHeaderTransport{key: key, value: value}
}

type injectHeaderTransport struct {
	key   string
	value string
}

func (t *injectHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(t.key, t.value)
	return http.DefaultTransport.RoundTrip(req)
}

func TestCustomHTTPClientIsUsed(t *testing.T) {
	ctx := context.Background()

	hasCustomHeader := false
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/api/v1/flags/", req.URL.Path)
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("x-Environment-Key"))
		if req.Header.Get("X-Test-Client") == "http" {
			hasCustomHeader = true
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)
		assert.NoError(t, err)
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: roundTripperWithHeader("X-Test-Client", "http"),
	}

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithHTTPClient(customClient),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, nil)
	assert.Equal(t, 1, len(flags.AllFlags()))
	assert.NoError(t, err)
	assert.True(t, hasCustomHeader, "Expected http header")
	flag, err := flags.GetFlag(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.Equal(t, fixtures.Feature1Value, flag.Value)
}

func TestCustomRestyClientIsUsed(t *testing.T) {
	ctx := context.Background()

	hasCustomHeader := false
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Custom-Test-Header") == "resty" {
			hasCustomHeader = true
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)
		assert.NoError(t, err)
	}))
	defer server.Close()

	restyClient := resty.New().
		SetHeader("X-Custom-Test-Header", "resty")

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithRestyClient(restyClient),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(flags.AllFlags()))
	assert.True(t, hasCustomHeader, "Expected custom resty header")
}

func TestRestyClientOverridesHTTPClientShouldPanic(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperWithHeader("X-Test-Client", "http"),
	}

	restyClient := resty.New().
		SetHeader("X-Test-Client", "resty")

	assert.Panics(t, func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey,
			flagsmith.WithHTTPClient(httpClient),
			flagsmith.WithRestyClient(restyClient),
			flagsmith.WithBaseURL("http://example.com/api/v1/"))
	}, "Expected panic when both HTTP and Resty clients are provided")
}

func TestDefaultRestyClientIsUsed(t *testing.T) {
	ctx := context.Background()

	serverCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		serverCalled = true

		assert.Equal(t, "/api/v1/flags/", req.URL.Path)
		assert.Equal(t, fixtures.EnvironmentAPIKey, req.Header.Get("x-Environment-Key"))

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, err := io.WriteString(rw, fixtures.FlagsJson)
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	flags, err := client.GetFlags(ctx, nil)

	assert.NoError(t, err)
	assert.True(t, serverCalled, "Expected server to be")
	assert.Equal(t, 1, len(flags.AllFlags()))
}

func TestCustomClientOptionsShoudPanic(t *testing.T) {
	restyClient := resty.New()

	testCases := []struct {
		name   string
		option flagsmith.Option
	}{
		{
			name:   "WithRequestTimeout",
			option: flagsmith.WithRequestTimeout(5 * time.Second),
		},
		{
			name:   "WithRetries",
			option: flagsmith.WithRetries(3, time.Second),
		},
		{
			name:   "WithCustomHeaders",
			option: flagsmith.WithCustomHeaders(map[string]string{"X-Custom": "value"}),
		},
		{
			name:   "WithProxy",
			option: flagsmith.WithProxy("http://proxy.example.com"),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			assert.Panics(t, func() {
				_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey,
					flagsmith.WithRestyClient(restyClient),
					test.option)
			}, "Expected panic when using %s with custom resty client", test.name)
		})
	}
}
