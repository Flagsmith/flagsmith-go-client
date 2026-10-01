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
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, fixtures.Feature1Reason, allFlags[0].Reason)
	assert.Empty(t, allFlags[0].Variant)
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
	assert.Equal(t, fixtures.Feature1IdentityReason, allFlags[0].Reason)
	assert.Equal(t, fixtures.Feature1IdentityVariant, allFlags[0].Variant)
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
	assert.Equal(t, "DEFAULT", allFlags[0].Reason)
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

func TestGetEnvironmentFlagsIgnoresSegmentOverrides(t *testing.T) {
	// Given: a document with a truthy segment override
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, fixtures.EnvironmentJsonWithSegmentOverride)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then: should return default value
	assert.NoError(t, err)
	flag, err := flags.GetFlag("feature_1")
	assert.NoError(t, err)
	assert.Equal(t, fixtures.Feature1Value, flag.Value)
	assert.Equal(t, "some_value", flag.Value)
	assert.Equal(t, "DEFAULT", flag.Reason)
}

func TestGetIdentityFlagsAppliesSegmentOverridesWithReason(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, fixtures.EnvironmentJsonWithSegmentOverride)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags(ctx, "test_identity", nil)

	// Then
	assert.NoError(t, err)
	flag, err := flags.GetFlag(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.Equal(t, "segment_override", flag.Value)
	assert.Equal(t, "TARGETING_MATCH; segment=Test Segment", flag.Reason)
}

func TestGetIdentityFlagsSetsVariantForMultivariateFeature(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, fixtures.EnvironmentJsonWithMultivariateFeature)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags(ctx, "test_identity", nil)

	// Then
	assert.NoError(t, err)
	flag, err := flags.GetFlag(fixtures.MVFeatureName)
	assert.NoError(t, err)
	assert.Equal(t, fixtures.MVFeatureVariantValue, flag.Value)
	assert.Equal(t, fixtures.MVFeatureVariantKey, flag.Variant)
	assert.Equal(t, "SPLIT; weight=100", flag.Reason)
}

func TestGetEnvironmentFlagsHasNoVariantForMultivariateFeature(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, fixtures.EnvironmentJsonWithMultivariateFeature)
	}))
	defer server.Close()

	// When
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))
	err := client.UpdateEnvironment(ctx)
	assert.NoError(t, err)

	flags, err := client.GetEnvironmentFlags(ctx)

	// Then: without an identity there is nothing to bucket, so the control value is
	// served without a variant
	assert.NoError(t, err)
	flag, err := flags.GetFlag(fixtures.MVFeatureName)
	assert.NoError(t, err)
	assert.Equal(t, "control_value", flag.Value)
	assert.Empty(t, flag.Variant)
	assert.Equal(t, "DEFAULT", flag.Reason)
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
	assert.Equal(t, "TARGETING_MATCH; segment=identity_overrides", allFlags[0].Reason)
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

func TestExtractNextPage(t *testing.T) {
	client := flagsmith.NewClient("test-key")

	testCases := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "valid link header with encoded page_id",
			header:   "</api/v1/environment-document/?page_id=" + fixtures.PageIDEncoded + ">; rel=\"next\"",
			expected: fixtures.PageID,
		},
		{
			name:     "empty header returns empty string",
			header:   "",
			expected: "",
		},
		{
			name:     "header without page_id returns empty string",
			header:   "</api/v1/environment-document/>; rel=\"next\"",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.ExtractNextPage(tc.header)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUpdateEnvironmentPaginatesIdentityOverrides(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.PaginatedEnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	// Identity from page 1 should be found
	flags, err := client.GetIdentityFlags(ctx, fixtures.OverriddenIdentifier, nil)
	assert.NoError(t, err)
	enabled, err := flags.IsFeatureEnabled(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.False(t, enabled, "identity from page 1 should have overridden feature disabled")

	// Identity from page 2 should also be found
	flags2, err := client.GetIdentityFlags(ctx, fixtures.OverriddenIdentifierPage2, nil)
	assert.NoError(t, err)
	enabled2, err := flags2.IsFeatureEnabled(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.False(t, enabled2, "identity from page 2 should have overridden feature disabled")
}

func TestUpdateEnvironmentSinglePageNoLinkHeader(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.UpdateEnvironment(ctx)

	// Then — no pagination, identity from page 1 should still work
	assert.NoError(t, err)

	flags, err := client.GetIdentityFlags(ctx, fixtures.OverriddenIdentifier, nil)
	assert.NoError(t, err)
	enabled, err := flags.IsFeatureEnabled(fixtures.Feature1Name)
	assert.NoError(t, err)
	assert.False(t, enabled, "identity override should have feature disabled")
}

func TestUpdateEnvironmentLogsWarningWhenSlowerThanRefreshInterval(t *testing.T) {
	// Given: handler delays the response so the fetch takes longer than the
	// refresh interval; we capture logs to assert the warning is emitted.
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		time.Sleep(20 * time.Millisecond)
		fixtures.EnvironmentDocumentHandler(rw, req)
	}))
	defer server.Close()

	var logOutput strings.Builder
	var logMu sync.Mutex
	slogLogger := slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (n int, err error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logOutput.Write(p)
	}), &slog.HandlerOptions{Level: slog.LevelWarn}))

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithSlogLogger(slogLogger),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithEnvironmentRefreshInterval(1*time.Millisecond))

	// When
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	logMu.Lock()
	logStr := logOutput.String()
	logMu.Unlock()

	assert.Contains(t, logStr, "fetching environment took longer than the configured refresh interval")
	assert.Contains(t, logStr, "refresh_interval=1ms")
	assert.Contains(t, logStr, "elapsed=")
}

func TestUpdateEnvironmentDoesNotLogWarningWhenWithinRefreshInterval(t *testing.T) {
	// Given
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(fixtures.EnvironmentDocumentHandler))
	defer server.Close()

	var logOutput strings.Builder
	var logMu sync.Mutex
	slogLogger := slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (n int, err error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logOutput.Write(p)
	}), &slog.HandlerOptions{Level: slog.LevelWarn}))

	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey,
		flagsmith.WithSlogLogger(slogLogger),
		flagsmith.WithBaseURL(server.URL+"/api/v1/"),
		flagsmith.WithEnvironmentRefreshInterval(10*time.Second))

	// When
	err := client.UpdateEnvironment(ctx)

	// Then
	assert.NoError(t, err)

	logMu.Lock()
	logStr := logOutput.String()
	logMu.Unlock()

	assert.NotContains(t, logStr, "fetching environment took longer")
}

// newExperimentServer serves the experiment identity fixture on the flags API and records
// events on the events API.
func newExperimentServer(t *testing.T, events *fixtures.EventsAPIHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/identities/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(rw, fixtures.IdentityResponseJsonWithExperiment)
	})
	mux.HandleFunc("/api/v1/environment-document/", fixtures.EnvironmentDocumentHandler)
	mux.Handle("/v1/events", events)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newExperimentClient(t *testing.T, server *httptest.Server, opts ...flagsmith.Option) *flagsmith.Client {
	t.Helper()
	opts = append([]flagsmith.Option{
		flagsmith.WithBaseURL(server.URL + "/api/v1/"),
		flagsmith.WithEvents(t.Context()),
		flagsmith.WithEventsBaseURL(server.URL + "/"),
		flagsmith.WithEventsFlushInterval(0),
	}, opts...)
	return flagsmith.NewClient(fixtures.EnvironmentAPIKey, opts...)
}

func TestClientPanicsIfEventsWithOfflineMode(t *testing.T) {
	// Given
	offlineHandler, err := flagsmith.NewLocalFileHandler("./fixtures/environment.json")
	require.NoError(t, err)

	// When, Then
	assert.PanicsWithValue(t, "events cannot be used in offline mode.", func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey,
			flagsmith.WithOfflineHandler(offlineHandler),
			flagsmith.WithOfflineMode(),
			flagsmith.WithEvents(t.Context()),
		)
	})
}

func TestClientPanicsIfEventsConfigInvalid(t *testing.T) {
	const msg = "events buffer size must be positive and flush interval must not be negative."
	assert.PanicsWithValue(t, msg, func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithEvents(t.Context()), flagsmith.WithEventsMaxBufferSize(0))
	})
	assert.PanicsWithValue(t, msg, func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithEvents(t.Context()), flagsmith.WithEventsFlushInterval(-time.Second))
	})
	assert.PanicsWithValue(t, "events retry backoff must not be negative.", func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey, flagsmith.WithEvents(t.Context()), flagsmith.WithEventsRetryBackoff(-time.Second))
	})
}

func TestEventsOptionsWithoutWithEventsAreAccepted(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = flagsmith.NewClient(fixtures.EnvironmentAPIKey,
			flagsmith.WithEventsBaseURL("http://localhost"),
			flagsmith.WithEventsMaxBufferSize(0),
		)
	})
}

func TestEventMethodsWithoutEventsEnabled(t *testing.T) {
	// Given
	client := flagsmith.NewClient(fixtures.EnvironmentAPIKey)
	var clientErr *flagsmith.FlagsmithClientError

	// When, Then
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext("user", nil))
	assert.ErrorAs(t, err, &clientErr)
	assert.ErrorAs(t, client.TrackEvent("purchase", nil), &clientErr)
	assert.ErrorAs(t, client.TrackExposureEvent("f", "user", "v", nil), &clientErr)
	assert.NoError(t, client.FlushEvents(t.Context()))
}

func TestTrackEventRejectsReservedNames(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When
	err := client.TrackEvent(flagsmith.FlagExposureEvent, nil)

	// Then
	var clientErr *flagsmith.FlagsmithClientError
	assert.ErrorAs(t, err, &clientErr)
	assert.ErrorAs(t, client.TrackEvent("$custom", nil), &clientErr)
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Empty(t, events.Requests())
}

func TestTrackEventSendsEvent(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When
	err := client.TrackEvent("purchase", &flagsmith.EventOptions{
		Identifier: "user-123",
		Value:      49.0,
		Traits:     map[string]interface{}{"plan": "premium"},
		Metadata:   map[string]interface{}{"currency": "EUR"},
	})
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))

	// Then
	sent := events.Events()
	require.Len(t, sent, 1)
	assert.Equal(t, "purchase", sent[0]["event"])
	assert.Nil(t, sent[0]["feature_name"])
	assert.Equal(t, "user-123", sent[0]["identifier"])
	assert.Equal(t, "49", sent[0]["value"])
	assert.Equal(t, map[string]interface{}{"plan": "premium"}, sent[0]["traits"])
	metadata := sent[0]["metadata"].(map[string]interface{})
	assert.Equal(t, "EUR", metadata["currency"])
	assert.NotEmpty(t, metadata["sdk_version"])

	r := events.Requests()[0]
	assert.Equal(t, fixtures.EnvironmentAPIKey, r.Header.Get("X-Environment-Key"))
	assert.Regexp(t, `^flagsmith-go-sdk/`, r.Header.Get("Flagsmith-SDK-User-Agent"))
	assert.Regexp(t, `^flagsmith-go-sdk/`, r.Header.Get("User-Agent"))
	assert.Equal(t, "application/json", r.Header.Get("Accept"))
	assert.Regexp(t, `^application/json`, r.Header.Get("Content-Type"))
}

func TestTrackExposureEventWithBlankIdentifierFails(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	for _, identifier := range []string{"", "  "} {
		// When
		err := client.TrackExposureEvent("checkout_cta", identifier, "treatment", nil)

		// Then
		var clientErr *flagsmith.FlagsmithClientError
		assert.ErrorAs(t, err, &clientErr, "%q", identifier)
	}
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Empty(t, events.Requests())
}

func TestTrackExposureEventSendsExposure(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When: Identifier and Value in opts are ignored in favour of the positional arguments
	err := client.TrackExposureEvent("checkout_cta", "user-123", "treatment", &flagsmith.EventOptions{
		Identifier: "ignored",
		Value:      "ignored",
		Traits:     map[string]interface{}{"plan": "premium"},
		Metadata:   map[string]interface{}{"experiment_id": 7},
	})
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))

	// Then
	sent := events.Events()
	require.Len(t, sent, 1)
	assert.Equal(t, flagsmith.FlagExposureEvent, sent[0]["event"])
	assert.Equal(t, "checkout_cta", sent[0]["feature_name"])
	assert.Equal(t, "user-123", sent[0]["identifier"])
	assert.Equal(t, "treatment", sent[0]["value"])
	assert.Equal(t, map[string]interface{}{"plan": "premium"}, sent[0]["traits"])
	assert.Equal(t, 7.0, sent[0]["metadata"].(map[string]interface{})["experiment_id"])
}

func TestGetExperimentFlagRecordsExposureWhenEnrolled(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))
	ec := flagsmith.NewEvaluationContext("user-123", map[string]interface{}{"plan": "premium"})

	// When
	flag, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, ec)

	// Then
	require.NoError(t, err)
	assert.Equal(t, fixtures.ExperimentVariant, flag.Variant)
	assert.Equal(t, &flagsmith.ExperimentMetadata{ID: fixtures.ExperimentID, Name: fixtures.ExperimentName, InExperiment: true}, flag.Experiment)
	require.NoError(t, client.FlushEvents(t.Context()))
	sent := events.Events()
	require.Len(t, sent, 1)
	assert.Equal(t, flagsmith.FlagExposureEvent, sent[0]["event"])
	assert.Equal(t, fixtures.ExperimentFeatureName, sent[0]["feature_name"])
	assert.Equal(t, "user-123", sent[0]["identifier"])
	assert.Equal(t, fixtures.ExperimentVariant, sent[0]["value"])
	assert.Equal(t, map[string]interface{}{"plan": "premium"}, sent[0]["traits"])
	metadata := sent[0]["metadata"].(map[string]interface{})
	assert.Equal(t, float64(fixtures.ExperimentID), metadata["experiment_id"])
	assert.NotContains(t, metadata, "experiment_name")
}

func TestGetExperimentFlagSendsTransientTraits(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))
	identifier := "user-123"
	persistent, transient := flagsmith.NewTraitEvaluationContext("premium", false), flagsmith.NewTraitEvaluationContext("secret", true)
	ec := flagsmith.EvaluationContext{Identity: &flagsmith.IdentityEvaluationContext{
		Identifier: &identifier,
		Traits:     map[string]*flagsmith.TraitEvaluationContext{"plan": &persistent, "session": &transient},
	}}
	onlyTransient := flagsmith.EvaluationContext{Identity: &flagsmith.IdentityEvaluationContext{
		Identifier: &identifier,
		Traits:     map[string]*flagsmith.TraitEvaluationContext{"session": &transient},
	}}

	// When
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, ec)
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))
	_, err = client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, onlyTransient)
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))

	// Then
	sent := events.Events()
	require.Len(t, sent, 2)
	assert.Equal(t, map[string]interface{}{"plan": "premium", "session": "secret"}, sent[0]["traits"])
	assert.Equal(t, map[string]interface{}{"session": "secret"}, sent[1]["traits"])
}

func TestGetExperimentFlagWithoutTraitsSendsNullTraits(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext("user-123", nil))

	// Then
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))
	sent := events.Events()
	require.Len(t, sent, 1)
	assert.Nil(t, sent[0]["traits"])
}

func TestGetExperimentFlagSkipsExposure(t *testing.T) {
	tests := []struct {
		name        string
		feature     string
		opts        []flagsmith.Option
		wantEnabled bool
		wantDefault bool
	}{
		{name: "identity not enrolled", feature: fixtures.NotEnrolledFeatureName, wantEnabled: true},
		{name: "no metadata", feature: fixtures.NoMetadataFeatureName, wantEnabled: true},
		{name: "flag disabled", feature: fixtures.DisabledExperimentFeatureName},
		{
			name:    "missing feature served by the default handler",
			feature: "missing",
			opts: []flagsmith.Option{flagsmith.WithDefaultHandler(func(string) (flagsmith.Flag, error) {
				return flagsmith.Flag{IsDefault: true, Enabled: true, Value: "default"}, nil
			})},
			wantEnabled: true,
			wantDefault: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			events := &fixtures.EventsAPIHandler{}
			client := newExperimentClient(t, newExperimentServer(t, events), tt.opts...)

			// When
			flag, err := client.GetExperimentFlag(t.Context(), tt.feature, flagsmith.NewEvaluationContext("user-123", nil))

			// Then
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, flag.Enabled)
			assert.Equal(t, tt.wantDefault, flag.IsDefault)
			require.NoError(t, client.FlushEvents(t.Context()))
			assert.Empty(t, events.Requests())
		})
	}
}

func TestGetExperimentFlagMissingFeatureWithoutHandler(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When
	_, err := client.GetExperimentFlag(t.Context(), "missing", flagsmith.NewEvaluationContext("user-123", nil))

	// Then
	var clientErr *flagsmith.FlagsmithClientError
	assert.ErrorAs(t, err, &clientErr)
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Empty(t, events.Requests())
}

func TestGetExperimentFlagRequiresIdentity(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))
	empty, blank := "", "  "

	for name, ec := range map[string]flagsmith.EvaluationContext{
		"no identity":         {},
		"no identifier":       {Identity: &flagsmith.IdentityEvaluationContext{}},
		"empty identifier":    {Identity: &flagsmith.IdentityEvaluationContext{Identifier: &empty}},
		"blank identifier":    {Identity: &flagsmith.IdentityEvaluationContext{Identifier: &blank}},
		"environment context": {Environment: &flagsmith.EnvironmentEvaluationContext{APIKey: fixtures.EnvironmentAPIKey}},
	} {
		t.Run(name, func(t *testing.T) {
			// When
			_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, ec)

			// Then
			var clientErr *flagsmith.FlagsmithClientError
			assert.ErrorAs(t, err, &clientErr)
		})
	}
}

func TestGetExperimentFlagWithEnvironmentOverrideFails(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))
	ec := flagsmith.NewEvaluationContext("user-123", nil)
	ec.Environment = &flagsmith.EnvironmentEvaluationContext{APIKey: "other-environment"}

	// When
	flag, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, ec)

	// Then
	var clientErr *flagsmith.FlagsmithClientError
	assert.ErrorAs(t, err, &clientErr)
	assert.Equal(t, flagsmith.Flag{}, flag)
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Empty(t, events.Requests())
}

func TestGetExperimentFlagWithSameEnvironmentRecordsExposure(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))
	ec := flagsmith.NewEvaluationContext("user-123", nil)
	ec.Environment = &flagsmith.EnvironmentEvaluationContext{APIKey: fixtures.EnvironmentAPIKey}

	// When
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, ec)

	// Then
	require.NoError(t, err)
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Len(t, events.Events(), 1)
}

func TestGetExperimentFlagWithLocalEvaluationSkipsExposure(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	server := newExperimentServer(t, events)
	client := newExperimentClient(t, server, flagsmith.WithLocalEvaluation(t.Context()))
	require.NoError(t, client.UpdateEnvironment(t.Context()))

	// When
	flag, err := client.GetExperimentFlag(t.Context(), fixtures.Feature1Name, flagsmith.NewEvaluationContext("user-123", nil))

	// Then
	require.NoError(t, err)
	assert.Equal(t, fixtures.Feature1Value, flag.Value)
	assert.Nil(t, flag.Experiment)
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Empty(t, events.Requests())
}

func TestGetExperimentFlagTwoIdentities(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events))

	// When
	for _, identifier := range []string{"user-1", "user-2"} {
		_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext(identifier, nil))
		require.NoError(t, err)
	}

	// Then
	require.NoError(t, client.FlushEvents(t.Context()))
	sent := events.Events()
	require.Len(t, sent, 2)
	assert.Equal(t, "user-1", sent[0]["identifier"])
	assert.Equal(t, "user-2", sent[1]["identifier"])
}

func TestGetExperimentFlagRequestError(t *testing.T) {
	// Given: a flags API that fails
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newExperimentClient(t, server)

	// When
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext("user-123", nil))

	// Then
	assert.Error(t, err)
}

func TestEventsIgnoreClientRetries(t *testing.T) {
	// Given: an events API that is always unavailable
	events := &fixtures.EventsAPIHandler{Statuses: []int{503, 503, 503, 503, 503, 503, 503, 503, 503, 503}}
	client := newExperimentClient(t, newExperimentServer(t, events),
		flagsmith.WithRetries(3, time.Millisecond),
		flagsmith.WithEventsRetryBackoff(time.Millisecond),
	)
	require.NoError(t, client.TrackEvent("purchase", nil))

	// When
	err := client.FlushEvents(t.Context())

	// Then: three attempts in total, not multiplied by the client's retry count
	assert.Error(t, err)
	assert.Len(t, events.Requests(), 3)
}

func TestEventsRetryBackoff(t *testing.T) {
	// Given: a zero backoff, where the 1s default would make the retries slow
	events := &fixtures.EventsAPIHandler{Statuses: []int{503, 429}}
	client := newExperimentClient(t, newExperimentServer(t, events),
		flagsmith.WithEventsRetryBackoff(0),
	)
	require.NoError(t, client.TrackEvent("purchase", nil))

	// When
	start := time.Now()
	err := client.FlushEvents(t.Context())

	// Then: retried straight away, and delivered on the third attempt
	assert.NoError(t, err)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Len(t, events.Requests(), 3)
	assert.Zero(t, client.DroppedEvents())
}

func TestEventsDoNotSendCustomHeaders(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events),
		flagsmith.WithCustomHeaders(map[string]string{
			"X-Custom": "custom-value", "X-Environment-Key": "other-key", "User-Agent": "custom-agent",
		}),
	)
	require.NoError(t, client.TrackEvent("purchase", nil))

	// When
	require.NoError(t, client.FlushEvents(t.Context()))

	// Then
	require.Len(t, events.Requests(), 1)
	header := events.Requests()[0].Header
	assert.Empty(t, header.Get("X-Custom"))
	assert.Equal(t, fixtures.EnvironmentAPIKey, header.Get("X-Environment-Key"))
	assert.Regexp(t, `^flagsmith-go-sdk/`, header.Get("User-Agent"))
	assert.Regexp(t, `^flagsmith-go-sdk/`, header.Get("Flagsmith-SDK-User-Agent"))
	assert.Equal(t, "application/json", header.Get("Accept"))
}

func TestWithEventsBaseURLWithoutTrailingSlash(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	server := newExperimentServer(t, events)
	client := newExperimentClient(t, server, flagsmith.WithEventsBaseURL(server.URL))
	require.NoError(t, client.TrackEvent("purchase", nil))

	// When
	require.NoError(t, client.FlushEvents(t.Context()))

	// Then
	require.Len(t, events.Requests(), 1)
	assert.Equal(t, "/v1/events", events.Requests()[0].Path)
}

func TestEventsFlushOnMaxBufferSize(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	client := newExperimentClient(t, newExperimentServer(t, events), flagsmith.WithEventsMaxBufferSize(2))

	// When
	require.NoError(t, client.TrackEvent("a", nil))
	require.NoError(t, client.TrackEvent("b", nil))

	// Then
	assert.Eventually(t, func() bool { return len(events.Events()) == 2 }, time.Second, 5*time.Millisecond)
}

func TestEventsFlushedWhenContextCancelled(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{}
	server := newExperimentServer(t, events)
	ctx, cancel := context.WithCancel(t.Context())
	client := newExperimentClient(t, server, flagsmith.WithEvents(ctx), flagsmith.WithEventsFlushInterval(time.Hour))
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext("user-123", nil))
	require.NoError(t, err)

	// When
	cancel()

	// Then
	assert.Eventually(t, func() bool { return len(events.Events()) == 1 }, time.Second, 5*time.Millisecond)
}

func TestDroppedEventsExposedToHost(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{Statuses: []int{http.StatusBadRequest}}
	client := newExperimentClient(t, newExperimentServer(t, events))
	require.NoError(t, client.TrackEvent("a", nil))
	require.NoError(t, client.TrackEvent("b", nil))

	// When
	err := client.FlushEvents(t.Context())

	// Then
	assert.Error(t, err)
	assert.Equal(t, int64(2), client.DroppedEvents())
}

func TestDroppedEventsWithoutEventsEnabled(t *testing.T) {
	assert.Zero(t, flagsmith.NewClient(fixtures.EnvironmentAPIKey).DroppedEvents())
}

func TestEventsDisabledAfterUnauthorised(t *testing.T) {
	// Given
	events := &fixtures.EventsAPIHandler{Statuses: []int{http.StatusUnauthorized}}
	client := newExperimentClient(t, newExperimentServer(t, events))
	require.NoError(t, client.TrackEvent("a", nil))
	require.Error(t, client.FlushEvents(t.Context()))

	// When
	_, err := client.GetExperimentFlag(t.Context(), fixtures.ExperimentFeatureName, flagsmith.NewEvaluationContext("user-123", nil))
	require.NoError(t, err)
	require.NoError(t, client.TrackEvent("b", nil))

	// Then: flags still work, but nothing more is sent
	require.NoError(t, client.FlushEvents(t.Context()))
	assert.Len(t, events.Requests(), 1)
}
