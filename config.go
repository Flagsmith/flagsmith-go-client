package flagsmith

import "time"

const (
	DefaultTimeout = 10 * time.Second
	DefaultBaseURL = "https://edge.api.flagsmith.com/api/v1/"
)

// config contains all configurable Client settings
type config struct {
	baseURL            string
	timeout            time.Duration
	localEvaluation    bool
	envRefreshInterval time.Duration
	enableAnalytics    bool
}

// defaultConfig returns default configuration
func defaultConfig() config {
	return config{
		baseURL:            DefaultBaseURL,
		timeout:            DefaultTimeout,
		envRefreshInterval: time.Second * 60,
	}
}
