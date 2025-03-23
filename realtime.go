package flagsmith

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) startRealtimeUpdates(ctx context.Context) {
	err := c.UpdateEnvironment(ctx)
	if err != nil {
		panic("Failed to fetch the environment while configuring real-time updates")
	}

	env, _ := c.state.GetEnvironment()
	envUpdatedAt := env.UpdatedAt
	log := c.log.With("environment", env.APIKey, "current_updated_at", &envUpdatedAt)

	streamPath, err := url.JoinPath(c.realtimeBaseUrl, "sse/environments", env.APIKey, "stream")
	if err != nil {
		log.Error("failed to build stream URL", "error", err)
		panic(err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			resp, err := http.Get(streamPath)
			if err != nil {
				log.Error("failed to connect to realtime service", "error", err)
				continue
			}
			defer resp.Body.Close()

			log.Info("connected to realtime")

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				parsedTime, err := parseUpdatedAtFromSSE(line)
				if err != nil {
					log.Error("failed to parse realtime update event", "error", err, "raw_event", line)
					continue
				}
				if parsedTime.After(envUpdatedAt) {
					log.WithGroup("event").
						Info("received update event",
							slog.Duration("update_delay", parsedTime.Sub(envUpdatedAt)),
							slog.Time("updated_at", parsedTime),
							slog.String("environment", env.APIKey),
						)
					err = c.UpdateEnvironment(ctx)
					if err != nil {
						log.Error("realtime update failed", "error", err)
						continue
					}
					envUpdatedAt = parsedTime
				}
			}
			if err := scanner.Err(); err != nil {
				log.Error("failed to read from realtime stream", "error", err)
			}
		}
	}
}

func parseUpdatedAtFromSSE(line string) (time.Time, error) {
	if !strings.HasPrefix(line, "data: ") {
		return time.Time{}, nil
	}

	var eventData struct {
		UpdatedAt float64 `json:"updated_at"`
	}

	data := strings.TrimPrefix(line, "data: ")
	err := json.Unmarshal([]byte(data), &eventData)
	if err != nil {
		return time.Time{}, err
	}

	if eventData.UpdatedAt <= 0 {
		return time.Time{}, errors.New("updated_at is <= 0")
	}

	// Convert the float timestamp into seconds and nanoseconds
	seconds := int64(eventData.UpdatedAt)
	nanoseconds := int64((eventData.UpdatedAt - float64(seconds)) * 1e9)

	// Return the parsed time
	return time.Unix(seconds, nanoseconds), nil
}
