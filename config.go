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
