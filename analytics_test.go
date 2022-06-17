package flagsmith

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
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
		actualRequestBodyRaw, err := ioutil.ReadAll(req.Body)
		assert.NoError(t, err)
		actualRequestBody.mu.Lock()
		actualRequestBody.body = string(actualRequestBodyRaw)
		actualRequestBody.mu.Unlock()
		assert.Equal(t, "/api/v1/analytics/flags/", req.URL.Path)
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	expectedRequstBody := "{\"1\":1,\"2\":2}"
	analyticsTimer := 10

	// and, the http client
	client := resty.New()
	client.SetHeader("X-Environment-Key", EnvironmentAPIKey)

	// Now let's create the processor
	processor := NewAnalyticsProcessor(context.Background(), client, server.URL+"/api/v1/", &analyticsTimer)

	// and, track some features
	processor.TrackFeature(1)
	processor.TrackFeature(2)
	processor.TrackFeature(2)

	// Next, let's sleep a little to let the processor flush the data
	time.Sleep(50 * time.Millisecond)

	// Finally, let's make sure correct data was sent to the API
	actualRequestBody.mu.Lock()
	assert.Equal(t, expectedRequstBody, actualRequestBody.body)

	// and, that the data was cleared
	processor.store.mu.Lock()
	assert.Equal(t, 0, len(processor.store.data))

}
