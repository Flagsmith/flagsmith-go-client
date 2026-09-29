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

Events are sent in batches every 10 seconds, or as soon as 1000 are buffered. A batch that fails with a network error or a 5xx response is retried once, then dropped. These options tune the behaviour:

- `WithEventsFlushInterval` sets the interval between sends. 0 disables the timer.
- `WithEventsMaxBufferSize` sets the number of buffered events that triggers a send.
- `WithEventsRetryBackoff` sets the wait before the retry. It defaults to the request timeout, capped at one second.
- `WithEventsBaseURL` sets the events API URL, which defaults to `https://events.api.flagsmith.com/`.

Events cannot be used with `WithOfflineMode`.

### Shutdown

The client has no `Close` method. Before the process exits, call `FlushEvents` with a deadline if the last events matter, then cancel the context passed to `WithEvents`.

```go
flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := client.FlushEvents(flushCtx); err != nil {
	log.Printf("flushing Flagsmith events: %v", err)
}
stopEvents()
```

`FlushEvents` returns once every event tracked before the call has been sent or dropped. Cancelling the `WithEvents` context also triggers one final send, but that send is best-effort and bounded by the request timeout.

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/kyle-ssg/c36a03aebe492e45cbd3eefb21cb0486) for details on our code of conduct, and the process for submitting pull requests to us.

## Getting Help

If you encounter a bug or feature request we would like to hear about it. Before you submit an issue please search existing issues in order to prevent duplicates.

## Get in touch

If you have any questions about our projects you can email <a href="mailto:support@flagsmith.com">support@flagsmith.com</a>.
