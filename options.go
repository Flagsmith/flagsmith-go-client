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
// Events are buffered and sent in batches by a background goroutine that uses the
// context provided here. Cancelling it is the shutdown flush: the processor sends what is
// still buffered, retrying with backoff as long as the retries fit within the request
// timeout. The same deadline cuts batches that were already in flight. What still fails,
// and anything left in the buffer, is dropped and counted, nothing stays in flight, and
// the goroutine exits. Tracking after that is a no-op. Cancel it on shutdown. When the
// last events must be delivered, for example in a short-lived process, call FlushEvents
// with a deadline first.
//
// Sending failures never reach the code that tracks events. Network errors, timeouts and
// 408, 429, 502, 503 and 504 responses are retried, three attempts in total, and a batch
// that still fails waits in the buffer for the next flush. Other error statuses drop the
// batch. A 401 or 403 stops event tracking until the client is re-created. Lost events
// are counted by Client.DroppedEvents.
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

// WithEventsFlushInterval sets how often buffered events are sent. A batch that failed with
// a retryable error is sent again on the next tick. 0 disables the timer, leaving the
// buffer-full trigger and FlushEvents. Defaults to DefaultEventsFlushInterval.
func WithEventsFlushInterval(interval time.Duration) Option {
	return func(c *Client) {
		c.config.eventsFlushInterval = interval
	}
}

// WithEventsMaxBufferSize sets the number of buffered events that triggers a flush. It is
// also the most events the buffer holds. Only one timer or buffer-full send is in flight
// at a time, so while it is pending, or while failed batches are put back, the oldest
// events are dropped and counted by Client.DroppedEvents. Defaults to
// DefaultEventsMaxBufferSize.
func WithEventsMaxBufferSize(size int) Option {
	return func(c *Client) {
		c.config.eventsMaxBufferSize = size
	}
}

// WithEventsRetryBackoff sets the backoff before the first retry of a batch of events that
// failed with a retryable error: a network error, a timeout, or a 408, 429, 502, 503 or 504
// response. A batch is posted up to three times in total. The backoff doubles before each
// further retry, up to 10 seconds, and each wait is a random duration between zero and
// the backoff. A batch that still fails is kept for the next flush. Defaults to
// DefaultEventsRetryBackoff.
func WithEventsRetryBackoff(backoff time.Duration) Option {
	return func(c *Client) {
		c.config.eventsRetryBackoff = &backoff
	}
}
