package flagsmith

import "time"

const (
	// DefaultTimeout is a default timeout for HTTP client
	DefaultTimeout = 10 * time.Second
	// DefaultBaseURI is a default URI
	DefaultBaseURI = "https://api.bullet-train.io/api/v1/"
)

// config contains all configurable Client settings
type config struct {
	baseURI string
	timeout time.Duration

	localEvaluation    bool
	envRefreshInterval time.Duration

	enableAnalytics bool
}

// defaultConfig returns default configuration
func defaultConfig() config {
	return config{
		baseURI: DefaultBaseURI,
		timeout: DefaultTimeout,
	}
}
