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

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
)

func (c *Client) startRealtimeUpdates(ctx context.Context) {
	err := c.UpdateEnvironment(ctx)
	if err != nil {
		panic("Failed to fetch the environment while configuring real-time updates")
	}
	env, _ := c.environment.Load().(*environments.EnvironmentModel)
	stream_url := c.config.realtimeBaseUrl + "sse/environments/" + env.APIKey + "/stream"
	envUpdatedAt := env.UpdatedAt
	log := c.log.With(
		slog.String("worker", "realtime"),
		slog.String("stream", stream_url),
	)
	defer func() {
		log.Info("realtime stopped")
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			resp, err := http.Get(stream_url)
			if err != nil {
				log.Error("failed to connect to realtime stream", "error", err)
				continue
			}
			defer resp.Body.Close()
			log.Info("connected")

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					parsedTime, err := parseUpdatedAtFromSSE(line)
					if err != nil {
						log.Error("failed to parse event message", "error", err, "message", line)
						continue
					}
					if parsedTime.After(envUpdatedAt) {
						err = c.UpdateEnvironment(ctx)
						if err != nil {
							log.Error("failed to update environment after receiving event", "error", err)
							continue
						}
						env, _ := c.environment.Load().(*environments.EnvironmentModel)
						envUpdatedAt = env.UpdatedAt
					}
				}
			}
			if err := scanner.Err(); err != nil {
				log.Error("error reading from realtime stream", "error", err)
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
