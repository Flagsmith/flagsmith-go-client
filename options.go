package flagsmith

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type Option func(c *Client)

// WithBaseURL sets the base URL of the Flagsmith API. Required if using a Flagsmith instance other than
// https://app.flagsmith.com.
//
// Defaults to https://edge.api.flagsmith.com/api/v1/.
//
// To set the URL of the real-time flags service, use [WithRealtimeBaseURL].
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithLocalEvaluation makes feature flags be evaluated locally instead of remotely by the Flagsmith API. It requires a
// server-side SDK key.
//
// Flags are evaluated locally by fetching the environment state from the Flagsmith API, and running the Flagsmith flag
// engine locally. When a [Client] is instantiated, a goroutine will be created using the provided context to poll
// the environment state for updates at regular intervals. The polling rate and retry behaviour can be  configured
// using [WithEnvironmentRefreshInterval] and [WithRetries].
func WithLocalEvaluation(ctx context.Context) Option {
	return func(c *Client) {
		c.localEvaluation = true
		c.ctxLocalEval = ctx
	}
}

// WithRequestTimeout sets the request timeout for all HTTP requests.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// WithEnvironmentRefreshInterval sets the delay between polls to fetch the current environment state when using
// [WithLocalEvaluation] to be at most once per interval.
func WithEnvironmentRefreshInterval(interval time.Duration) Option {
	return func(c *Client) {
		c.envRefreshInterval = interval
	}
}

// WithAnalytics makes the [Client] keep track of calls to [Flags.GetFlag], [Flags.IsFeatureEnabled] or
// [Flags.GetFeatureValue]. It will create a goroutine that periodically flushes this data to the Flagsmith API using
// the provided context.
func WithAnalytics(ctx context.Context) Option {
	return func(c *Client) {
		c.ctxAnalytics = ctx
	}
}

// WithRetries makes the [Client] retry all failed HTTP requests n times, waiting for waitTime between retries.
func WithRetries(n int, waitTime time.Duration) Option {
	return func(c *Client) {
		c.client.SetRetryCount(n)
		c.client.SetRetryWaitTime(waitTime)
	}
}

// WithCustomHeaders applies a set of HTTP headers on all requests made by the [Client].
func WithCustomHeaders(headers map[string]string) Option {
	return func(c *Client) {
		c.client.SetHeaders(headers)
	}
}

// WithDefaultHandler sets a handler function used to return fallback values when [Client.GetFlags] would have normally
// returned an error. For example, this handler makes all flags be disabled by default:
//
//	func handler(flagKey string) (Flag, error) {
//		return Flag{
//			FeatureName: flagKey,
//			Enabled: false,
//		}, nil
//	}
func WithDefaultHandler(handler func(string) (Flag, error)) Option {
	f := func(flagKey string) (flag Flag, err error) {
		flag, err = handler(flagKey)
		flag.IsDefault = true
		flag.FeatureName = flagKey
		return flag, err
	}
	return func(c *Client) {
		c.defaultFlagHandler = f
	}
}

// WithLogger sets a custom [slog.Logger] for the [Client].
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.log = logger
	}
}

// WithProxy sets a proxy server to use for all HTTP requests.
func WithProxy(url string) Option {
	return func(c *Client) {
		c.proxy = url
	}
}

// WithOfflineEnvironment sets the current environment and prevents Client from making network requests.
func WithOfflineEnvironment(env Environment) Option {
	return func(c *Client) {
		c.environment.SetOfflineEnvironment(env.Environment())
	}
}

// WithErrorHandler sets an error handler that is called if [Client.UpdateEnvironment] returns an error.
func WithErrorHandler(handler func(handler *APIError)) Option {
	return func(c *Client) {
		c.errorHandler = handler
	}
}

// WithRealtime enables real-time flag updates. It requires [WithLocalEvaluation].
//
// When [Client] is constructed, a server-sent events (SSE) connection will be kept open in a goroutine using the
// same context used by [WithLocalEvaluation].
//
// If you are using a Flagsmith instance other than https://app.flagsmith.com, use [WithRealtimeBaseURL] to set the URL
// of your real-time updates service.
func WithRealtime() Option {
	return func(c *Client) {
		c.useRealtime = true
	}
}

// WithRealtimeBaseURL sets a custom URL to use for subscribing to real-time flag updates. This is required if you are
// using a Flagsmith instance other than https://app.flagsmith.com.
//
// The default base URL is https://realtime.flagsmith.com/.
func WithRealtimeBaseURL(url string) Option {
	return func(c *Client) {
		// Ensure the URL ends with a trailing slash
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		c.realtimeBaseUrl = url
	}
}
