package flagsmith

import "time"

const (
	// DefaultTimeout is a default timeout for HTTP client
	DefaultTimeout = 10 * time.Second
	// DefaultBaseURL is a default URI
	DefaultBaseURL = "https://api.bullet-train.io/api/v1/"
)

// config contains all configurable Client settings
type config struct {
	baseURL string
	timeout time.Duration

	localEvaluation    bool
	envRefreshInterval time.Duration

	enableAnalytics bool
}

// defaultConfig returns default configuration
func defaultConfig() config {
	return config{
		baseURL: DefaultBaseURL,
		timeout: DefaultTimeout,
		envRefreshInterval: time.Second * 60,
	}
}
