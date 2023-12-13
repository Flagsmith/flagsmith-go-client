package flagsmith

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v3/internal/flaghttp"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"

func TestAnalytics(t *testing.T) {
	// First, we need to create a test server
	// to capture the requests made to the API
	actualRequestBody := struct {
		mu   sync.Mutex
		body string
	}{}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		actualRequestBodyRaw, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		actualRequestBody.mu.Lock()
		actualRequestBody.body = string(actualRequestBodyRaw)
		actualRequestBody.mu.Unlock()
		assert.Equal(t, "/api/v1/analytics/flags/", req.URL.Path)
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	expectedRequstBody := "{\"feature_1\":1,\"feature_2\":2}"
	analyticsTimer := 10

	// and, the http client
	client := flaghttp.NewClient()
	client.SetHeader("X-Environment-Key", EnvironmentAPIKey)

	// Now let's create the processor
	processor := NewAnalyticsProcessor(context.Background(), client, server.URL+"/api/v1/", &analyticsTimer, createLogger())

	// and, track some features
	processor.TrackFeature("feature_1")
	processor.TrackFeature("feature_2")
	processor.TrackFeature("feature_2")

	// Next, let's sleep a little to let the processor flush the data
	time.Sleep(50 * time.Millisecond)

	// Finally, let's make sure correct data was sent to the API
	actualRequestBody.mu.Lock()
	assert.Equal(t, expectedRequstBody, actualRequestBody.body)

	// and, that the data was cleared
	processor.store.mu.Lock()
	assert.Equal(t, 0, len(processor.store.data))
}
