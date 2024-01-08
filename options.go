package flagsmith

import (
	"context"
	"time"
)

type Option func(c *Client)

// Make sure With* functions have correct type.
var _ = []Option{
	WithBaseURL(""),
	WithLocalEvaluation(context.TODO()),
	WithRemoteEvaluation(),
	WithRequestTimeout(0),
	WithEnvironmentRefreshInterval(0),
	WithAnalytics(context.TODO()),
	WithRetries(3, 1*time.Second),
	WithCustomHeaders(nil),
	WithDefaultHandler(nil),
	WithProxy(""),
}

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.config.baseURL = url
	}
}

// WithLocalEvaluation enables local evaluation of the Feature flags.
//
// The goroutine responsible for asynchronously updating the environment makes
// use of the context provided here, which means that if it expires the
// background process will exit.
func WithLocalEvaluation(ctx context.Context) Option {
	return func(c *Client) {
		c.config.localEvaluation = true
		c.ctxLocalEval = ctx
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

// WithAnalytics enables tracking of the usage of the Feature flags.
//
// The goroutine responsible for asynchronously uploading the locally stored
// cache uses the context provided here, which means that if it expires the
// background process will exit.
func WithAnalytics(ctx context.Context) Option {
	return func(c *Client) {
		c.config.enableAnalytics = true
		c.ctxAnalytics = ctx
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

// WithOfflineHandler returns an Option function that sets the offline handler.
func WithOfflineHandler(handler OfflineHandler) Option {
	return func(c *Client) {
		c.offlineHandler = handler
	}
}

// WithOfflineMode returns an Option function that enables the offline mode.
// NOTE: before using this option, you should set the offline handler.
func WithOfflineMode() Option {
	return func(c *Client) {
		c.config.offlineMode = true
	}
}
