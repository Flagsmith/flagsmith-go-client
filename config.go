package bullettrain

import "time"

const (
	// DefaultTimeout is a default timeout for HTTP client
	DefaultTimeout = 5 * time.Second
	// DefaultBaseURI is a default URI
	DefaultBaseURI = "https://api.bullet-train.io/api/v1/"
)

// Config contains all configurable Client settings
type Config struct {
	BaseURI string
	Timeout time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		BaseURI: DefaultBaseURI,
		Timeout: DefaultTimeout,
	}
}
