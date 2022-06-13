package flagsmith_test

import (
	//	"context"
	"testing"
	"fmt"
	"time"
	//"io/ioutil"
	"net/http"
	"net/http/httptest"
	flagsmith "github.com/Flagsmith/flagsmith-go-client"
	"github.com/stretchr/testify/assert"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"


func TestClientUpdatesEnvironmentOnStartForLocalEvaluation(t *testing.T) {
	// Given
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		fmt.Println("request received")
		requestReceived = true
		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithBaseURL(server.URL + "/api/v1/"))

	// Sleep to ensure that the server has time to update the environment
	time.Sleep(10* time.Millisecond)

	// Then
	assert.True(t, requestReceived)
}

func TestClientUpdatesEnvironmentOnEachRefresh(t *testing.T) {
	// Given
	actualEnvironmentRefreshCount:= 0
	expectedEnvironmentRefreshCount := 3
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		fmt.Println("request received")
		actualEnvironmentRefreshCount++
		assert.Equal(t, req.URL.Path, "/api/v1/environment-document/")
		assert.Equal(t, EnvironmentAPIKey, req.Header.Get("X-Environment-Key"))
	}))
	defer server.Close()

	// When
	_ = flagsmith.NewClient(EnvironmentAPIKey, flagsmith.WithLocalEvaluation(),
		flagsmith.WithEnvironmentRefreshInterval(100*time.Millisecond),
		flagsmith.WithBaseURL(server.URL + "/api/v1/"))


	time.Sleep(250* time.Millisecond)

	// Then
	// We should have called refresh environment 3 times
        // one when the client starts and 2
        // for each time the refresh interval expires
	assert.Equal(t, expectedEnvironmentRefreshCount, actualEnvironmentRefreshCount)

}
