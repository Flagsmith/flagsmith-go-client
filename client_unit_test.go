package flagsmith

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Flagsmith/flagsmith-go-client/v2/fixtures"
	"github.com/stretchr/testify/assert"
)

func TestBulkIdentify(t *testing.T) {
	// Given
	traitKey := "foo"
	traitValue := "bar"
	identifierOne := "test_identity_1"
	identifierTwo := "test_identity_2"

	trait := Trait{TraitKey: traitKey, TraitValue: traitValue}
	data := []*IdentityTraits{
		{Traits: []*Trait{&trait}, Identifier: identifierOne},
		{Traits: []*Trait{&trait}, Identifier: identifierTwo},
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

	// Update the EdgeUrlPrefix to point to the test server
	edgeUrlPrefix = server.URL
	client := NewClient(fixtures.EnvironmentAPIKey, WithBaseURL(server.URL+"/api/v1/"))

	// When
	err := client.BulkIdentify(data)

	// Then
	assert.NoError(t, err)

}
