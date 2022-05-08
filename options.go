package flagsmith

import "time"

type Option func(c *Client)

var _ = []Option{
	WithBaseURI(""),
	WithLocalEvaluation(),
	WithRemoteEvaluation(),
	WithRequestTimeout(0),
	WithEnvironmentRefreshInterval(0),
	WithAnalytics(),
	WithRetries(3, 1*time.Second),
	WithCustomHeaders(nil),
}

func WithBaseURI(uri string) Option {
	return func(c *Client) {
		c.config.BaseURI = uri
	}
}

func WithLocalEvaluation() Option {
	return func(c *Client) {
		c.config.LocalEval = true
	}
}

func WithRemoteEvaluation() Option {
	return func(c *Client) {
		c.config.LocalEval = false
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.config.Timeout = timeout
	}
}

func WithEnvironmentRefreshInterval(interval time.Duration) Option {
	return func(c *Client) {
		c.config.EnvRefreshInterval = interval
	}
}

func WithAnalytics() Option {
	return func(c *Client) {
		c.config.EnableAnalytics = true
	}
}

func WithRetries(count int, waitTime time.Duration) Option {
	return func(c *Client) {
		c.client.SetRetryCount(count)
		c.client.SetRetryWaitTime(waitTime)
	}
}

func WithCustomHeaders(headers map[string]string) Option {
	return func(c *Client) {
		c.client.SetHeaders(headers)
	}
}
