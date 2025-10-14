package flagsmith

// This file exports internal functions for testing purposes only.
// It is compiled only when running tests (no build tags needed).

// GetUserAgentForTest exposes the getUserAgent function for external tests.
func GetUserAgentForTest() string {
	return getUserAgent()
}
