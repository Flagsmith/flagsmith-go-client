package flagsmith_test

import (
	"testing"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v4"
	"github.com/stretchr/testify/assert"
)

func TestNewLocalFileHandler(t *testing.T) {
	// Given
	envJsonPath := "./fixtures/environment.json"

	// When
	offlineHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)

	// Then
	assert.NoError(t, err)
	assert.NotNil(t, offlineHandler)
}

func TestLocalFileHandlerGetEnvironment(t *testing.T) {
	// Given
	envJsonPath := "./fixtures/environment.json"
	localHandler, err := flagsmith.NewLocalFileHandler(envJsonPath)

	assert.NoError(t, err)

	// When
	environment := localHandler.GetEnvironment()

	// Then
	assert.NotNil(t, environment.APIKey)
}
