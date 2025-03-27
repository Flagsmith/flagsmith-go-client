package flagsmith

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const AnalyticsTimerInMilli = 10 * 1000
const AnalyticsEndpoint = "analytics/flags/"

type analyticDataStore struct {
	mu   sync.Mutex
	data map[string]int
}
type AnalyticsProcessor struct {
	client   *resty.Client
	store    *analyticDataStore
	endpoint string
	log      Logger
}

func NewAnalyticsProcessor(ctx context.Context, client *resty.Client, baseURL string, timerInMilli *int, log Logger) *AnalyticsProcessor {
	data := make(map[string]int)
	dataStore := analyticDataStore{data: data}
	tickerInterval := AnalyticsTimerInMilli
	if timerInMilli != nil {
		tickerInterval = *timerInMilli
	}
	processor := AnalyticsProcessor{
		client:   client,
		store:    &dataStore,
		endpoint: baseURL + AnalyticsEndpoint,
		log:      log,
	}
	log.Debugf("analytics processor starting")
	go processor.start(ctx, tickerInterval)
	return &processor
}

func (a *AnalyticsProcessor) start(ctx context.Context, tickerInterval int) {
	ticker := time.NewTicker(time.Duration(tickerInterval) * time.Millisecond)
	defer func() {
		a.log.Debugf("analytics processor stopped")
	}()
	for {
		select {
		case <-ticker.C:
			if err := a.Flush(ctx); err != nil {
				a.log.Warnf("Failed to send analytics data: %s", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *AnalyticsProcessor) Flush(ctx context.Context) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if len(a.store.data) == 0 {
		return nil
	}
	resp, err := a.client.R().SetContext(ctx).SetBody(a.store.data).Post(a.endpoint)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("received unexpected response from server: %s", resp.Status())
	}

	// Clear the cache in case of success.
	a.store.data = make(map[string]int)
	return nil
}

func (a *AnalyticsProcessor) TrackFeature(featureName string) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	currentCount := a.store.data[featureName]
	a.store.data[featureName] = currentCount + 1
}
