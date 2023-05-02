package flagsmith

import (
	"context"
	"time"
)

type Option func(c *Client)

// Make sure With* functions have correct type.
var _ = []Option{
	WithBaseURL(""),
	WithLocalEvaluation(),
	WithRemoteEvaluation(),
	WithRequestTimeout(0),
	WithEnvironmentRefreshInterval(0),
	WithAnalytics(),
	WithRetries(3, 1*time.Second),
	WithCustomHeaders(nil),
	WithDefaultHandler(nil),
	WithContext(context.TODO()),
	WithProxy(""),
}

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.config.baseURL = url
	}
}

func WithLocalEvaluation() Option {
	return func(c *Client) {
		c.config.localEvaluation = true
	}
}

func WithRemoteEvaluation() Option {
	return func(c *Client) {
		c.config.localEvaluation = false
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.client.SetTimeout(timeout)
	}
}

func WithEnvironmentRefreshInterval(interval time.Duration) Option {
	return func(c *Client) {
		c.config.envRefreshInterval = interval
	}
}

func WithAnalytics() Option {
	return func(c *Client) {
		c.config.enableAnalytics = true
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

func WithDefaultHandler(handler func(string) (Flag, error)) Option {
	return func(c *Client) {
		c.defaultFlagHandler = handler
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Client) {
		c.ctx = ctx
	}
}

// Allows the client to use any logger that implements the `Logger` interface.
func WithLogger(logger Logger) Option {
	return func(c *Client) {
		c.log = logger
	}
}

// WithProxy returns an Option function that sets the proxy(to be used by internal resty client).
// The proxyURL argument is a string representing the URL of the proxy server to use, e.g. "http://proxy.example.com:8080".
func WithProxy(proxyURL string) Option {
	return func(c *Client) {
		c.client.SetProxy(proxyURL)
	}
}
