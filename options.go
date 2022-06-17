package flagsmith

import (
	"context"
	"time"
)

type Option func(c *Client)

var _ = []Option{
	WithBaseURL(""),
	WithLocalEvaluation(),
	WithRemoteEvaluation(),
	WithRequestTimeout(0),
	WithEnvironmentRefreshInterval(0),
	WithAnalytics(),
	WithRetries(3, 1*time.Second),
	WithCustomHeaders(nil),
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
		c.config.timeout = timeout
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

func WithDefaultHandler(handler func(string) Flag) Option {
	return func(c *Client) {
		c.defaultFlagHandler = handler
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Client) {
		c.ctx = ctx
	}
}
