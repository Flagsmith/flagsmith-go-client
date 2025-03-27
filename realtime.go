package flagsmith

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
)

// realtime handles the SSE connection and reconnection logic.
type realtime struct {
	client       *Client
	ctx          context.Context
	log          *slog.Logger
	streamURL    string
	envUpdatedAt time.Time
	backoff      *backoff
}

// newRealtime creates a new realtime instance.
func newRealtime(client *Client, ctx context.Context, streamURL string, envUpdatedAt time.Time) *realtime {
	return &realtime{
		client: client,
		ctx:    ctx,
		log: client.log.With(
			slog.String("worker", "realtime"),
			slog.String("stream", streamURL),
		),
		streamURL:    streamURL,
		envUpdatedAt: envUpdatedAt,
		backoff:      newBackoff(),
	}
}

// start begins the realtime connection process.
func (r *realtime) start() {
	r.log.Debug("connecting to realtime")
	defer func() {
		r.log.Info("stopped")
	}()
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			if err := r.connect(); err != nil {
				r.log.Error("failed to connect", "error", err)
				r.backoff.wait(r.ctx)
			}
		}
	}
}

// connect establishes and maintains the SSE connection.
func (r *realtime) connect() error {
	resp, err := http.Get(r.streamURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error response connecting to stream: %d", resp.StatusCode)
	}

	r.log.Info("connected")
	r.backoff.reset()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		default:
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				if err := r.handleEvent(line); err != nil {
					r.log.Error("failed to handle event", "error", err, "message", line)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		r.log.Error("failed to read from stream", "error", err)
		return err
	}

	return nil
}

// handleEvent processes a single SSE event.
func (r *realtime) handleEvent(line string) error {
	parsedTime, err := parseUpdatedAtFromSSE(line)
	if err != nil {
		return err
	}

	if parsedTime.After(r.envUpdatedAt) {
		if err := r.client.UpdateEnvironment(r.ctx); err != nil {
			return err
		}
		if env, ok := r.client.environment.Load().(*environments.EnvironmentModel); ok {
			r.envUpdatedAt = env.UpdatedAt
		}
	}
	return nil
}

func parseUpdatedAtFromSSE(line string) (time.Time, error) {
	var eventData struct {
		UpdatedAt float64 `json:"updated_at"`
	}

	data := strings.TrimPrefix(line, "data: ")
	err := json.Unmarshal([]byte(data), &eventData)
	if err != nil {
		return time.Time{}, errors.New("failed to parse event data: " + err.Error())
	}

	if eventData.UpdatedAt <= 0 {
		return time.Time{}, errors.New("invalid 'updated_at' value in event data")
	}

	// Convert the float timestamp into seconds and nanoseconds
	seconds := int64(eventData.UpdatedAt)
	nanoseconds := int64((eventData.UpdatedAt - float64(seconds)) * 1e9)

	// Return the parsed time
	return time.Unix(seconds, nanoseconds), nil
}
