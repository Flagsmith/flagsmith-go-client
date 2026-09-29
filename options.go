package flagsmith

import (
	"context"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/go-resty/resty/v2"
)

const (
	OptionWithHTTPClient  = "WithHTTPClient"
	OptionWithRestyClient = "WithRestyClient"
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
	WithPolling(),
	WithRealtime(),
	WithRealtimeBaseURL(""),
	WithLogger(nil),
	WithSlogLogger(nil),
	WithRestyClient(nil),
	WithHTTPClient(nil),
	WithEvents(context.TODO()),
	WithEventsBaseURL(""),
	WithEventsFlushInterval(0),
	WithEventsMaxBufferSize(0),
	WithEventsRetryBackoff(0),
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
		if c.config.userProvidedClient {
			panic("options modifying the client can not be used with a custom client")
		}
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
		if c.config.userProvidedClient {
			panic("options modifying the client can not be used with a custom client")
		}
		c.client.SetRetryCount(count)
		c.client.SetRetryWaitTime(waitTime)
	}
}

func WithCustomHeaders(headers map[string]string) Option {
	return func(c *Client) {
		if c.config.userProvidedClient {
			panic("options modifying the client can not be used with a custom client")
		}
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
		c.log = newLoggerToSlogAdapter(logger)
	}
}

// WithSlogLogger allows the client to use a slog.Logger for logging.
func WithSlogLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.log = logger
	}
}

// WithProxy returns an Option function that sets the proxy(to be used by internal resty client).
// The proxyURL argument is a string representing the URL of the proxy server to use, e.g. "http://proxy.example.com:8080".
func WithProxy(proxyURL string) Option {
	return func(c *Client) {
		if c.config.userProvidedClient {
			panic("options modifying the client can not be used with a custom client")
		}
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

// WithErrorHandler provides a way to handle errors that occur during update of an environment.
func WithErrorHandler(handler func(handler *FlagsmithAPIError)) Option {
	return func(c *Client) {
		c.errorHandler = handler
	}
}

// WithRealtime returns an Option function that enables real-time updates for the Client.
// NOTE: Before enabling real-time updates, ensure that local evaluation is enabled.
func WithRealtime() Option {
	return func(c *Client) {
		c.config.useRealtime = true
	}
}

// WithRealtimeBaseURL returns an Option function for configuring the real-time base URL of the Client.
func WithRealtimeBaseURL(url string) Option {
	return func(c *Client) {
		// Ensure the URL ends with a trailing slash
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		c.config.realtimeBaseUrl = url
	}
}

// WithPolling makes it so that the client will poll for updates even when WithRealtime is used.
func WithPolling() Option {
	return func(c *Client) {
		c.config.polling = true
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithRestyClient(restyClient *resty.Client) Option {
	return func(c *Client) {
		if restyClient != nil {
			c.client = restyClient
		}
	}
}

// WithEvents enables experimentation event tracking (exposures and custom events).
//
// The goroutine responsible for asynchronously flushing buffered events uses the
// context provided here. When it is done the processor performs one final flush,
// bounded by the request timeout, and exits, so cancel it on shutdown to avoid
// losing the last batch. Call FlushEvents first if the last events matter.
//
// Events cannot be used together with WithOfflineMode.
func WithEvents(ctx context.Context) Option {
	return func(c *Client) {
		c.config.enableEvents = true
		c.ctxEvents = ctx
	}
}

// WithEventsBaseURL sets the events API base URL. A trailing slash is added if missing.
// Defaults to DefaultEventsBaseURL.
func WithEventsBaseURL(url string) Option {
	return func(c *Client) {
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		c.config.eventsBaseURL = url
	}
}

// WithEventsFlushInterval sets how often buffered events are sent. 0 disables the timer,
// leaving the buffer-full trigger and FlushEvents. Defaults to DefaultEventsFlushInterval.
func WithEventsFlushInterval(interval time.Duration) Option {
	return func(c *Client) {
		c.config.eventsFlushInterval = interval
	}
}

// WithEventsMaxBufferSize sets the number of buffered events that triggers a flush.
// Defaults to DefaultEventsMaxBufferSize.
func WithEventsMaxBufferSize(size int) Option {
	return func(c *Client) {
		c.config.eventsMaxBufferSize = size
	}
}

// WithEventsRetryBackoff sets how long to wait before retrying a batch of events that
// failed with a network error or a 5xx response. A batch is retried once, then dropped.
// Defaults to the request timeout, capped at one second.
func WithEventsRetryBackoff(backoff time.Duration) Option {
	return func(c *Client) {
		c.config.eventsRetryBackoff = &backoff
	}
}
