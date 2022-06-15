package flagsmith

import (
	"context"
	"log"
	"time"

	"github.com/go-resty/resty/v2"
)

const AnalyticsTimerInMilli = 10 * 1000
const AnalyticsEndpoint = "analytics/flags/"

type AnalyticsProcessor struct {
	client   *resty.Client
	data     map[int]int
	endpoint string
}

func NewAnalyticsProcessor(ctx context.Context, client *resty.Client, baseURL string, timerInMilli *int) *AnalyticsProcessor {
	data := make(map[int]int)
	tickerInterval := AnalyticsTimerInMilli
	if timerInMilli != nil {
		tickerInterval = *timerInMilli
	}
	processor := AnalyticsProcessor{
		client:   client,
		data:     data,
		endpoint: baseURL + AnalyticsEndpoint,
	}

	ticker := time.NewTicker(time.Duration(tickerInterval) * time.Millisecond)
	go func() {
		select {
		case <-ticker.C:
			processor.Flush()
		case <-ctx.Done():
			return

		}
	}()
	return &processor
}

func (a *AnalyticsProcessor) Flush() {
	if len(a.data) == 0 {
		return
	}
	resp, err := a.client.R().SetBody(a.data).Post(a.endpoint)
	if err != nil {
		log.Printf("WARNING: failed to send analytics data: %v", err)
	}
	if !resp.IsSuccess() {
		log.Printf("WARNING: Received non 200 from server when sending analytics data")
	}

	// clear the map
	a.data = make(map[int]int)
}

func (a *AnalyticsProcessor) TrackFeature(featureID int) {
	currentCount := a.data[featureID]
	a.data[featureID] = currentCount + 1
}
