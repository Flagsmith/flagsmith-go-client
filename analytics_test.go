package flagsmith

import (
	"github.com/go-resty/resty/v2"
	"net/http/httptest"
	"testing"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"

func TestAnalytics(t *testing.T) {
	// First, we need to create a test server
	// to capture the requests made to the API
	var actualRequestBody string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		actualRequestBodyRaw, err := ioutil.ReadAll(req.Body)
		assert.NoError(t, err)
		actualRequestBody = string(actualRequestBodyRaw)

		assert.Equal(t, "/api/v1/analytics/flags/", req.URL.Path, )
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	expectedRequstBody := "{\"1\":1,\"2\":2}"
	analyticsTimer := 10

	// and, the http client
	client := resty.New()
	client.SetHeader("X-Environment-Key", EnvironmentAPIKey)

	// Now let's create the processor
	processor := NewAnalyticsProcessor(client, server.URL + "/api/v1/", &analyticsTimer)

	// and, track some features
	processor.TrackFeature(1)
	processor.TrackFeature(2)
	processor.TrackFeature(2)

        // Next, let's sleep a little to let the processor flush the data
	time.Sleep(50* time.Millisecond)

	// Finally, let's make sure correct data was sent to the API
	assert.Equal(t, expectedRequstBody, actualRequestBody)

	// and, that the data was cleared
	assert.Equal(t, 0, len(processor.data))

}
