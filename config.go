package flagsmith

import (
	"time"
)

const (
	// Number of seconds to wait for a request to
	// complete before terminating the request.
	DefaultTimeout = 10 * time.Second

	// Default base URL for the API.
	DefaultBaseURL = "https://edge.api.flagsmith.com/api/v1/"

	BulkIdentifyMaxCount   = 100
	DefaultRealtimeBaseUrl = "https://realtime.flagsmith.com/"
)

// config contains all configurable Client settings.
type config struct {
	baseURL            string
	timeout            time.Duration
	localEvaluation    bool
	envRefreshInterval time.Duration
	enableAnalytics    bool
	offlineMode        bool
	realtimeBaseUrl    string
	useRealtime        bool
	polling            bool
}

// defaultConfig returns default configuration.
func defaultConfig() config {
	return config{
		baseURL:            DefaultBaseURL,
		timeout:            DefaultTimeout,
		envRefreshInterval: time.Second * 60,
		realtimeBaseUrl:    DefaultRealtimeBaseUrl,
	}
}
