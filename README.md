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

- A network error or a 503 is retried, up to three attempts in total, with exponential backoff and jitter. If every attempt fails, the batch goes back to the front of the buffer for the next send.
- Any other error status, such as 400 or 500, drops the batch without a retry.
- A 401 or 403 means the environment key was rejected. Event tracking stops, and the error is logged once. Flags are still evaluated as normal.
- The buffer never holds more than the maximum buffer size. While the events API is unreachable, the oldest events are dropped first.
- When the events API accepts a batch but rejects some of its events, each rejection is logged as a warning. Rejected events are not sent again.

`DroppedEvents` returns how many events have been lost this way, so you can monitor it.

```go
dropped := client.DroppedEvents()
```

These options tune the behaviour:

- `WithEventsFlushInterval` sets the interval between sends. 0 disables the timer.
- `WithEventsMaxBufferSize` sets the number of buffered events that triggers a send. It is also the most events the buffer holds.
- `WithEventsRetryBackoff` sets the wait before the first retry, which doubles before the second. It defaults to the request timeout, capped at one second.
- `WithEventsBaseURL` sets the events API URL, which defaults to `https://events.api.flagsmith.com/`.

Events cannot be used with `WithOfflineMode`.

### Shutdown

The client has no `Close` method. Cancelling the context passed to `WithEvents` is the shutdown flush. It sends whatever is still buffered once, then stops the background goroutine. That final send is best-effort. It is bounded by the request timeout, and it does not wait out long retries.

When you need a guarantee, call `FlushEvents` with a deadline before the process exits. This matters most for short-lived processes such as CLI tools, jobs and serverless functions, which can exit before the next scheduled send.

```go
flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := client.FlushEvents(flushCtx); err != nil {
	log.Printf("flushing Flagsmith events: %v", err)
}
stopEvents()
```

`FlushEvents` returns once every event tracked before the call has been sent, put back in the buffer after a failure, or dropped. It returns the error when its batch could not be sent.

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/kyle-ssg/c36a03aebe492e45cbd3eefb21cb0486) for details on our code of conduct, and the process for submitting pull requests to us.

## Getting Help

If you encounter a bug or feature request we would like to hear about it. Before you submit an issue please search existing issues in order to prevent duplicates.

## Get in touch

If you have any questions about our projects you can email <a href="mailto:support@flagsmith.com">support@flagsmith.com</a>.
