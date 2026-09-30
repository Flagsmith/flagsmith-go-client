[![Go](https://github.com/flagsmith/flagsmith-go-client/workflows/Go/badge.svg?branch=main)](https://github.com/flagsmith/flagsmith-go-client/actions)
[![GoReportCard](https://goreportcard.com/badge/github.com/flagsmith/flagsmith-go-client)](https://goreportcard.com/report/github.com/flagsmith/flagsmith-go-client)
[![GoDoc](https://godoc.org/github.com/flagsmith/flagsmith-go-client/v5?status.svg)](https://pkg.go.dev/github.com/Flagsmith/flagsmith-go-client/v5#section-documentation)

# Flagsmith Go SDK

Flagsmith allows you to manage feature flags and remote config across multiple projects, environments and organisations.

This is the SDK for go for [https://flagsmith.com/](https://flagsmith.com/).

## Adding to your project

For full documentation visit [https://docs.flagsmith.com/clients/server-side?language=go](https://docs.flagsmith.com/clients/server-side?language=go).

## Experimentation

Enable events with `WithEvents`. Its context runs the background sender, and cancelling it is the shutdown flush.

```go
eventsCtx, stopEvents := context.WithCancel(context.Background())
client := flagsmith.NewClient(os.Getenv("FLAGSMITH_SERVER_KEY"), flagsmith.WithEvents(eventsCtx))

ec := flagsmith.NewEvaluationContext("user-123", map[string]interface{}{"plan": "premium"})
flag, err := client.GetExperimentFlag(ctx, "checkout_cta", ec)
```

`GetExperimentFlag` returns the flag and records a `$flag_exposure` when the identity is enrolled in a running experiment. Experiment metadata needs remote evaluation, so local evaluation records no exposure. `TrackExposureEvent` records one for a flag you evaluated some other way.

Record conversions with `TrackEvent`. Names starting with `$` are reserved.

```go
err = client.TrackEvent("purchase", &flagsmith.EventOptions{Identifier: "user-123", Value: 49.99})
```

### Delivery and failure handling

Events are sent every 10 seconds (`WithEventsFlushInterval`) or when 1000 are buffered (`WithEventsMaxBufferSize`). Failures never reach the code that tracks events.

- Network errors, timeouts and 408, 429, 502, 503 and 504 are retried: three attempts, backoff from 1 second (`WithEventsRetryBackoff`) doubling to 10 seconds, with full jitter. A batch that still fails goes back to the front of the buffer until the next scheduled send or `FlushEvents`.
- Any other error status, including 500, drops the batch.
- A 401 or 403 stops event tracking, drops the buffer and logs one warning, until the client is re-created. Flags keep working.
- One scheduled send is in flight at a time. When the buffer is full meanwhile, the oldest events are dropped.
- Events a 202 lists as rejected are logged by index and not resent.
- Equal exposures are sent once until the batch carrying them succeeds.
- Traits and metadata are captured when tracked. Values that cannot be encoded as JSON drop the event.
- `GetExperimentFlag` sends the identity's traits, transient ones included. A blank identifier is sent as none.
- The events request carries the SDK's own environment key and user agent. `WithCustomHeaders` is not applied to it.
- Logs never include identifiers, trait values or response content.

`DroppedEvents` returns a running count of events lost in any of these ways, including at shutdown.

Events cannot be used with `WithOfflineMode`.

### Shutdown

Cancelling the `WithEvents` context sends what is buffered, retrying only within one request timeout. The same deadline cuts sends already in progress, including `FlushEvents`. Anything left is dropped and counted.

For a guarantee, for example in a CLI tool, job or serverless function, call `FlushEvents` with a deadline before exit. It returns once every event tracked before the call has been sent, kept for retry or dropped.

```go
flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := client.FlushEvents(flushCtx); err != nil {
	log.Printf("flushing Flagsmith events: %v", err)
}
stopEvents()
```

## Contributing

Please read [CONTRIBUTING.md](https://gist.github.com/kyle-ssg/c36a03aebe492e45cbd3eefb21cb0486) for details on our code of conduct, and the process for submitting pull requests to us.

## Getting Help

If you encounter a bug or feature request we would like to hear about it. Before you submit an issue please search existing issues in order to prevent duplicates.

## Get in touch

If you have any questions about our projects you can email <a href="mailto:support@flagsmith.com">support@flagsmith.com</a>.
