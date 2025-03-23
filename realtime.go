package flagsmith

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func (c *Client) startRealtimeUpdates(ctx context.Context) {
	err := c.UpdateEnvironment(ctx)
	if err != nil {
		panic("Failed to fetch the environment while configuring real-time updates")
	}
	env := c.environment.GetEnvironment()
	stream_url := c.config.realtimeBaseUrl + "sse/environments/" + env.APIKey + "/stream"
	envUpdatedAt := env.UpdatedAt
	for {
		select {
		case <-ctx.Done():
			return
		default:
			resp, err := http.Get(stream_url)
			if err != nil {
				c.log.Error("failed to connect to realtime service", "error", err)
				continue
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					parsedTime, err := parseUpdatedAtFromSSE(line)
					if err != nil {
						c.log.Error("failed to parse realtime update event", "error", err, "raw_event", line)
						continue
					}
					if parsedTime.After(envUpdatedAt) {
						c.log.Info("received update event",
							slog.Time("updated_at", parsedTime),
							slog.String("environment", env.APIKey),
						)
						err = c.UpdateEnvironment(ctx)
						if err != nil {
							continue
						}
						envUpdatedAt = parsedTime
					}
				}
			}
			if err := scanner.Err(); err != nil {
				c.log.Error("failed to read from realtime stream", "error", err, "stream_url", &resp.Request.URL)
			}
		}
	}
}
func parseUpdatedAtFromSSE(line string) (time.Time, error) {
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
