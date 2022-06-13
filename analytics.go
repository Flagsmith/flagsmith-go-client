package flagsmith

import (
	"time"
	"log"
	"github.com/go-resty/resty/v2"
)

const AnalyticsTimerInMilli = 10 *1000
const AnalyticsEndpoint = "analytics/flags/"

type AnalyticsProcessor struct {
	client *resty.Client
	data map[int]int
	endpoint string
}

func NewAnalyticsProcessor(client *resty.Client, baseURL string, timerInMilli *int) *AnalyticsProcessor {
	data := make(map[int]int)
	sleepTime := AnalyticsTimerInMilli
	if timerInMilli != nil {
		sleepTime = *timerInMilli
	}
	processor := AnalyticsProcessor{
		client: client,
		data: data,
		endpoint: baseURL + AnalyticsEndpoint,
	}
	go func() {
		for {
			processor.Flush()
			time.Sleep(time.Duration(sleepTime) * time.Millisecond)
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
