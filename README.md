[![Go](https://github.com/flagsmith/flagsmith-go-client/workflows/Go/badge.svg?branch=main)](https://github.com/flagsmith/flagsmith-go-client/actions)
[![GoReportCard](https://goreportcard.com/badge/github.com/flagsmith/flagsmith-go-client)](https://goreportcard.com/report/github.com/flagsmith/flagsmith-go-client)
[![GoDoc](https://godoc.org/github.com/flagsmith/flagsmith-go-client/v5?status.svg)](https://pkg.go.dev/github.com/Flagsmith/flagsmith-go-client/v5#section-documentation)

# Flagsmith Go SDK

Flagsmith allows you to manage feature flags and remote config across multiple projects, environments and organisations.

This is the SDK for go for [https://flagsmith.com/](https://flagsmith.com/).

## Adding to your project

For full documentation visit [https://docs.flagsmith.com/clients/server-side?language=go](https://docs.flagsmith.com/clients/server-side?language=go).

## Experimentation

Enable event tracking with `WithEvents`. The context you pass controls the background goroutine that sends buffered events.

```go
eventsCtx, stopEvents := context.WithCancel(context.Background())
client := flagsmith.NewClient(os.Getenv("FLAGSMITH_SERVER_KEY"),
	flagsmith.WithEvents(eventsCtx),
)
```

`GetExperimentFlag` evaluates one flag for an identity. When that identity is enrolled in a running experiment on the feature, it also records a `$flag_exposure` event.

```go
ec := flagsmith.NewEvaluationContext("user-123", map[string]interface{}{"plan": "premium"})
flag, err := client.GetExperimentFlag(ctx, "checkout_cta", ec)
if err != nil {
	return err
}
fmt.Println(flag.Value, flag.Variant, flag.Experiment)
```

Experiment metadata is only returned by remote evaluation. With local evaluation the flag is returned and no exposure is recorded.

Record conversions with `TrackEvent`. Event names starting with `$` are reserved.

```go
err = client.TrackEvent("purchase", &flagsmith.EventOptions{
	Identifier: "user-123",
	Value:      49.99,
	Metadata:   map[string]interface{}{"currency": "EUR"},
})
```

Use `TrackExposureEvent` to record an exposure for a flag you evaluated some other way. Exposures without an identifier are not sent.

```go
err = client.TrackExposureEvent("checkout_cta", "user-123", flag.Variant, nil)
```

Events are sent in batches every 10 seconds, or as soon as 1000 are buffered. Failures never reach the code that tracks events. They are handled like this:

- A network error, a timeout, or a 408, 429, 502, 503 or 504 response is retried, up to three attempts in total. The backoff starts at 1 second and doubles, up to 10 seconds. Each wait is a random duration between zero and the backoff. If every attempt fails, the batch goes back to the front of the buffer and waits for the next scheduled send or `FlushEvents` call. Until then, a full buffer drops its oldest events rather than sending early.
- Any other error status, including 500, drops the batch without a retry.
- A 401 or 403 means the environment key was rejected. Event tracking stops, the buffer is dropped, and one warning is logged. Later tracking calls do nothing, and are counted as dropped, until you create a new client. Flags are still evaluated as normal.
- Only one scheduled or buffer-full send is in flight at a time. The buffer never holds more than the maximum buffer size. While a send is pending, or the events API is unreachable, the oldest events are dropped first.
- When the events API accepts a batch but rejects some of its events, each rejection is logged as a warning. Rejected events are not sent again.
- Events whose traits or metadata cannot be encoded as JSON, such as channels or infinite numbers, are dropped. The rest of the batch is still sent.
- Logs never include identifiers, trait values or response content. When a batch is rejected, only its status and the size of the response body are logged.
- Equal exposures are sent once. They can be sent again after the events API returns a success response, including a partial one.

`DroppedEvents` returns how many events have been lost this way, so you can monitor it. The count only ever goes up.

```go
dropped := client.DroppedEvents()
```

These options tune the behaviour:

- `WithEventsFlushInterval` sets the interval between sends. 0 disables the timer.
- `WithEventsMaxBufferSize` sets the number of buffered events that triggers a send. It is also the most events the buffer holds.
- `WithEventsRetryBackoff` sets the backoff before the first retry, which then doubles up to 10 seconds. It defaults to 1 second.
- `WithEventsBaseURL` sets the events API URL, which defaults to `https://events.api.flagsmith.com/`.

Events cannot be used with `WithOfflineMode`.

### Shutdown

The client has no `Close` method. Cancelling the context passed to `WithEvents` is the shutdown flush. It sends whatever is still buffered, then stops the background goroutine. That final send retries with backoff, but only while the retries fit within the request timeout. The same deadline also cuts sends that were already in progress. Anything that still fails, and anything left in the buffer, is dropped and counted by `DroppedEvents`. Events tracked after shutdown are dropped and counted too.

When you need a guarantee, call `FlushEvents` with a deadline before the process exits. This matters most for short-lived processes such as CLI tools, jobs and serverless functions, which can exit before the next scheduled send.

```go
flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := client.FlushEvents(flushCtx); err != nil {
	log.Printf("flushing Flagsmith events: %v", err)
}
stopEvents()
```

`FlushEvents` returns once every event tracked before the call has been sent, put back in the buffer after a failure, or dropped. It uses the same retries, and skips any retry whose wait would pass the deadline. It returns the error when its batch could not be sent.

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/kyle-ssg/c36a03aebe492e45cbd3eefb21cb0486) for details on our code of conduct, and the process for submitting pull requests to us.

## Getting Help

If you encounter a bug or feature request we would like to hear about it. Before you submit an issue please search existing issues in order to prevent duplicates.

## Get in touch

If you have any questions about our projects you can email <a href="mailto:support@flagsmith.com">support@flagsmith.com</a>.
