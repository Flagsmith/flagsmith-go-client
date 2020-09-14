package bullettrain

import "time"

const (
	DefaultTimeout = 5 * time.Second
	DefaultBaseURI = "https://api.bullet-train.io/api/v1/"
)

type Config struct {
	BaseURI string
	Timeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		BaseURI: DefaultBaseURI,
		Timeout: DefaultTimeout,
	}
}
