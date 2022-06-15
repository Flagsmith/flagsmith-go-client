package flagsmith

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const AnalyticsTimerInMilli = 10 * 1000
const AnalyticsEndpoint = "analytics/flags/"

type analyticDataStore struct {
	mu   sync.Mutex
	data map[int]int
}
type AnalyticsProcessor struct {
	client   *resty.Client
	store    *analyticDataStore
	endpoint string
}

func NewAnalyticsProcessor(ctx context.Context, client *resty.Client, baseURL string, timerInMilli *int) *AnalyticsProcessor {
	data := make(map[int]int)
	dataStore := analyticDataStore{data: data}
	tickerInterval := AnalyticsTimerInMilli
	if timerInMilli != nil {
		tickerInterval = *timerInMilli
	}
	processor := AnalyticsProcessor{
		client:   client,
		store:    &dataStore,
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
	a.store.mu.Lock()
	if len(a.store.data) == 0 {
		return
	}
	resp, err := a.client.R().SetBody(a.store.data).Post(a.endpoint)
	if err != nil {
		log.Printf("WARNING: failed to send analytics data: %v", err)
	}
	if !resp.IsSuccess() {
		log.Printf("WARNING: Received non 200 from server when sending analytics data")
	}

	// clear the map
	a.store.data = make(map[int]int)
	a.store.mu.Unlock()
}

func (a *AnalyticsProcessor) TrackFeature(featureID int) {
	a.store.mu.Lock()
	currentCount := a.store.data[featureID]
	a.store.data[featureID] = currentCount + 1
	a.store.mu.Unlock()
}
