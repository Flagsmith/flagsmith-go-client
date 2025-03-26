package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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
	log      *slog.Logger
}

func NewAnalyticsProcessor(ctx context.Context, client *resty.Client, baseURL string, timerInMilli *int, log *slog.Logger) *AnalyticsProcessor {
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
	go processor.start(ctx, tickerInterval)
	return &processor
}

func (a *AnalyticsProcessor) start(ctx context.Context, tickerInterval int) {
	ticker := time.NewTicker(time.Duration(tickerInterval) * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			if resp, err := a.Flush(ctx); err != nil {
				a.log.Warn("failed to send analytics data",
					"error", err,
					slog.Int("status", resp.StatusCode),
					slog.String("url", resp.Request.URL.String()),
				)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *AnalyticsProcessor) Flush(ctx context.Context) (*http.Response, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if len(a.store.data) == 0 {
		return nil, nil
	}
	resp, err := a.client.R().SetContext(ctx).SetBody(a.store.data).Post(a.endpoint)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return resp.RawResponse, fmt.Errorf("AnalyticsProcessor.Flush received error response %d %s", resp.StatusCode(), resp.Status())
	}
	a.store.data = make(map[string]int)
	return resp.RawResponse, nil
}

func (a *AnalyticsProcessor) TrackFeature(featureName string) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	currentCount := a.store.data[featureName]
	a.store.data[featureName] = currentCount + 1
}
