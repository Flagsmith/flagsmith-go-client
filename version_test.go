package flagsmith

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUserAgent(t *testing.T) {
	// Given/When
	userAgent := getUserAgent()

	// Then - should return a non-empty string
	assert.NotEmpty(t, userAgent, "User-Agent should not be empty")
}

func TestGetUserAgentFormat(t *testing.T) {
	// Given/When
	userAgent := getUserAgent()

	// Then - should start with "flagsmith-go-sdk/"
	assert.True(t, strings.HasPrefix(userAgent, "flagsmith-go-sdk/"),
		"User-Agent should start with 'flagsmith-go-sdk/', got: %s", userAgent)
}

func TestGetUserAgentValidFormats(t *testing.T) {
	// Given/When
	userAgent := getUserAgent()

	// Then - should be either a valid version or "unknown"
	parts := strings.Split(userAgent, "/")
	assert.Equal(t, 2, len(parts), "User-Agent should have exactly two parts separated by '/'")
	assert.Equal(t, "flagsmith-go-sdk", parts[0], "First part should be 'flagsmith-go-sdk'")

	// Version part should be either a version string (starting with 'v') or "unknown"
	versionPart := parts[1]
	isValid := versionPart == "unknown" || strings.HasPrefix(versionPart, "v")
	assert.True(t, isValid,
		"Version should be 'unknown' or start with 'v', got: %s", versionPart)
}
