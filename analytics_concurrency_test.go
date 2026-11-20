package flagsmith

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type analyticsReply struct {
	status int
	err    error
}

type analyticsRequest struct {
	counts map[string]int
	reply  chan analyticsReply
}

type analyticsTransport chan analyticsRequest

func (requests analyticsTransport) RoundTrip(r *http.Request) (
	*http.Response, error,
) {
	counts := make(map[string]int)
	if err := json.NewDecoder(r.Body).Decode(&counts); err != nil {
		return nil, err
	}
	request := analyticsRequest{counts, make(chan analyticsReply, 1)}
	select {
	case requests <- request:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	select {
	case reply := <-request.reply:
		if reply.err != nil {
			return nil, reply.err
		}
		return &http.Response{
			StatusCode: reply.status,
			Status:     http.StatusText(reply.status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("{}")),
			Request:    r,
		}, nil
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
}

func analyticsReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for analytics operation")
		var zero T
		return zero
	}
}

func newAnalyticsFixture(t *testing.T) (
	*AnalyticsProcessor, analyticsTransport, context.Context,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	requests := make(analyticsTransport, 16)
	client := resty.NewWithClient(&http.Client{Transport: requests})
	timer := 60 * 60 * 1000
	processor := NewAnalyticsProcessor(ctx, client,
		"https://analytics.invalid/", &timer,
		newSlogToLoggerAdapter(createLogger()))
	return processor, requests, ctx
}

func flushAnalytics(ctx context.Context, p *AnalyticsProcessor) <-chan error {
	done := make(chan error, 1)
	go func() { done <- p.Flush(ctx) }()
	return done
}

func TestAnalyticsFlushTracksDuringUpload(t *testing.T) {
	for _, outcome := range []string{
		"success", "http error", "transport error", "cancel", "deadline",
	} {
		t.Run(outcome, func(t *testing.T) {
			p, requests, ctx := newAnalyticsFixture(t)
			p.TrackFeature("same")
			uploadCtx, cancel := context.WithCancel(ctx)
			if outcome == "deadline" {
				uploadCtx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
			}
			defer cancel()
			flushed := flushAnalytics(uploadCtx, p)
			first := analyticsReceive(t, requests)
			require.Equal(t, map[string]int{"same": 1}, first.counts)

			tracked := make(chan struct{})
			go func() {
				p.TrackFeature("same")
				p.TrackFeature("same")
				p.TrackFeature("new")
				close(tracked)
			}()
			select {
			case <-tracked:
			case <-time.After(100 * time.Millisecond):
				t.Error("tracking waited for the pending analytics response")
			}

			switch outcome {
			case "success":
				first.reply <- analyticsReply{status: http.StatusOK}
			case "http error":
				first.reply <- analyticsReply{status: http.StatusBadGateway}
			case "transport error":
				first.reply <- analyticsReply{err: errors.New("offline fixture")}
			case "cancel":
				cancel()
			}
			err := analyticsReceive(t, flushed)
			if outcome == "success" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if outcome == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				if outcome == "deadline" {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			}
			analyticsReceive(t, tracked)

			flushed = flushAnalytics(ctx, p)
			second := analyticsReceive(t, requests)
			expected := map[string]int{"same": 2, "new": 1}
			if outcome != "success" {
				expected["same"]++
			}
			assert.Equal(t, expected, second.counts)
			second.reply <- analyticsReply{status: http.StatusOK}
			require.NoError(t, analyticsReceive(t, flushed))
			require.NoError(t, p.Flush(ctx))
			assert.Empty(t, requests, "successful counts were sent again")
		})
	}
}

func TestAnalyticsFlushRetrySnapshot(t *testing.T) {
	p, requests, ctx := newAnalyticsFixture(t)
	p.client.SetRetryCount(1).SetRetryWaitTime(time.Millisecond)
	p.client.AddRetryCondition(func(r *resty.Response, err error) bool {
		return err != nil || r.StatusCode() == http.StatusBadGateway
	})
	p.TrackFeature("same")
	flushed := flushAnalytics(ctx, p)
	first := analyticsReceive(t, requests)
	p.TrackFeature("same")
	p.TrackFeature("new")
	first.reply <- analyticsReply{status: http.StatusBadGateway}
	retry := analyticsReceive(t, requests)
	assert.Equal(t, map[string]int{"same": 1}, retry.counts)
	retry.reply <- analyticsReply{status: http.StatusOK}
	require.NoError(t, analyticsReceive(t, flushed))
	flushed = flushAnalytics(ctx, p)
	remaining := analyticsReceive(t, requests)
	assert.Equal(t, map[string]int{"same": 1, "new": 1}, remaining.counts)
	remaining.reply <- analyticsReply{status: http.StatusOK}
	require.NoError(t, analyticsReceive(t, flushed))
}

func TestAnalyticsFlushSerializesUploads(t *testing.T) {
	p, requests, ctx := newAnalyticsFixture(t)
	p.TrackFeature("same")
	firstDone := flushAnalytics(ctx, p)
	first := analyticsReceive(t, requests)
	secondDone := flushAnalytics(ctx, p)
	tracked := make(chan struct{})
	go func() {
		p.TrackFeature("same")
		close(tracked)
	}()
	select {
	case <-tracked:
	case <-time.After(100 * time.Millisecond):
		t.Error("tracking waited for overlapping flushes")
	}
	select {
	case <-requests:
		t.Error("overlapping flush sent an unacknowledged count twice")
	case <-time.After(20 * time.Millisecond):
	}
	first.reply <- analyticsReply{status: http.StatusOK}
	require.NoError(t, analyticsReceive(t, firstDone))
	analyticsReceive(t, tracked)
	second := analyticsReceive(t, requests)
	assert.Equal(t, map[string]int{"same": 1}, second.counts)
	second.reply <- analyticsReply{status: http.StatusOK}
	require.NoError(t, analyticsReceive(t, secondDone))
}

func TestAnalyticsConcurrentCounts(t *testing.T) {
	p, requests, ctx := newAnalyticsFixture(t)
	const workers, perWorker = 8, 100
	var tracking sync.WaitGroup
	var flushing sync.WaitGroup
	for range workers {
		tracking.Add(1)
		go func() {
			defer tracking.Done()
			for range perWorker {
				p.TrackFeature("same")
				p.TrackFeature("other")
			}
		}()
		flushing.Add(1)
		go func() {
			defer flushing.Done()
			assert.NoError(t, p.Flush(ctx))
		}()
	}
	finished := make(chan struct{})
	go func() {
		tracking.Wait()
		flushing.Wait()
		assert.NoError(t, p.Flush(ctx))
		close(finished)
	}()
	totals := make(map[string]int)
	for {
		select {
		case request := <-requests:
			for name, count := range request.counts {
				totals[name] += count
			}
			request.reply <- analyticsReply{status: http.StatusOK}
		case <-finished:
			assert.Equal(t, map[string]int{
				"same": workers * perWorker, "other": workers * perWorker,
			}, totals)
			return
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent tracking or flush did not finish")
		}
	}
}
