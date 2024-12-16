package flagsmith

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
			resp, err := http.Get(stream_url)
			if err != nil {
				c.log.Errorf("Error connecting to realtime server: %v", err)
				continue
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					parsedTime, err := parseUpdatedAtFromSSE(line)
					if err != nil {
						c.log.Errorf("Error reading realtime stream: %v", err)
						continue
					}
					if parsedTime.After(envUpdatedAt) {
						err = c.UpdateEnvironment(ctx)
						if err != nil {
							c.log.Errorf("Failed to update the environment: %v", err)
							continue
						}
						env, _ := c.environment.Load().(*environments.EnvironmentModel)

						envUpdatedAt = env.UpdatedAt
					}
				}
			}
			if err := scanner.Err(); err != nil {
				c.log.Errorf("Error reading realtime stream: %v", err)
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
