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
	offlineHandler, err := flagsmith.ReadEnvironmentFromFile(envJsonPath)

	// Then
	assert.NoError(t, err)
	assert.NotNil(t, offlineHandler)
}

func TestLocalFileHandlerGetEnvironment(t *testing.T) {
	// Given
	envJsonPath := "./fixtures/environment.json"

	// When
	env, err := flagsmith.ReadEnvironmentFromFile(envJsonPath)

	// Then
	assert.NoError(t, err)
	assert.NotEmpty(t, env.GetEnvironment().APIKey)
}
