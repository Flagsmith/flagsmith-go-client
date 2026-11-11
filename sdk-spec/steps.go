// Package steps implements the Flagsmith SDK behavioural spec: https://github.com/Flagsmith/sdk-spec
//
// Each function implements the Gherkin step in the comment above it.
package steps

import (
	"context"
	"fmt"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v5"
)

type Flag struct {
	Feature string `json:"feature"`
	Enabled bool   `json:"enabled"`
	Value   any    `json:"value"`
}

func asSpecFlags(flags flagsmith.Flags) []Flag {
	result := []Flag{}
	for _, flag := range flags.AllFlags() {
		result = append(result, Flag{Feature: flag.FeatureName, Enabled: flag.Enabled, Value: flag.Value})
	}
	return result
}

// Create the SDK client
func CreateClient(environmentKey, apiURL string) *flagsmith.Client {
	return flagsmith.NewClient(environmentKey, flagsmith.WithBaseURL(apiURL))
}

// Create the SDK client with events enabled
func CreateClientWithEvents(environmentKey, apiURL, eventsURL string) *flagsmith.Client {
	return flagsmith.NewClient(environmentKey,
		flagsmith.WithBaseURL(apiURL),
		flagsmith.WithEvents(context.Background()),
		flagsmith.WithEventsBaseURL(eventsURL),
	)
}

// Create the SDK client with local evaluation
func CreateClientWithLocalEvaluation(environmentKey, apiURL string) (*flagsmith.Client, error) {
	ctx := context.Background()
	client := flagsmith.NewClient(environmentKey,
		flagsmith.WithBaseURL(apiURL),
		flagsmith.WithLocalEvaluation(ctx),
	)
	return client, client.UpdateEnvironment(ctx)
}

// Create the SDK client with local evaluation and events enabled
func CreateClientWithLocalEvaluationAndEvents(environmentKey, apiURL, eventsURL string) (*flagsmith.Client, error) {
	ctx := context.Background()
	client := flagsmith.NewClient(environmentKey,
		flagsmith.WithBaseURL(apiURL),
		flagsmith.WithLocalEvaluation(ctx),
		flagsmith.WithEvents(ctx),
		flagsmith.WithEventsBaseURL(eventsURL),
	)
	return client, client.UpdateEnvironment(ctx)
}

// When the application gets the environment flags
func GetEnvironmentFlags(client *flagsmith.Client) ([]Flag, error) {
	flags, err := client.GetEnvironmentFlags(context.Background())
	return asSpecFlags(flags), err
}

// When the application gets the flags for identity "{identifier}" with traits:
func GetIdentityFlags(client *flagsmith.Client, identifier string, traits map[string]any) ([]Flag, error) {
	var sdkTraits []*flagsmith.Trait
	for key, value := range traits {
		sdkTraits = append(sdkTraits, &flagsmith.Trait{TraitKey: key, TraitValue: value})
	}
	flags, err := client.GetIdentityFlags(context.Background(), identifier, sdkTraits)
	return asSpecFlags(flags), err
}

// When the application gets the experiment flag "{feature}" for identity "{identifier}" with traits:
func GetExperimentFlag(client *flagsmith.Client, feature, identifier string, traits map[string]any) ([]Flag, error) {
	flag, err := client.GetExperimentFlag(context.Background(), feature, flagsmith.NewEvaluationContext(identifier, traits))
	return []Flag{{Feature: flag.FeatureName, Enabled: flag.Enabled, Value: flag.Value}}, err
}

// When the application tracks event "{event}" for identity "{identifier}" with value {value} and metadata:
func TrackEvent(client *flagsmith.Client, event, identifier string, value any, metadata map[string]any) error {
	return client.TrackEvent(event, &flagsmith.EventOptions{Identifier: identifier, Value: value, Metadata: metadata})
}

// When the application tracks an exposure of feature "{feature}" for identity "{identifier}" with value {value} and metadata:
func TrackExposureEvent(client *flagsmith.Client, feature, identifier string, value any, metadata map[string]any) error {
	return client.TrackExposureEvent(feature, identifier, value, &flagsmith.EventOptions{Metadata: metadata})
}

// When the application tracks events "event_{first}" to "event_{last}" for identity "{identifier}"
func TrackEvents(client *flagsmith.Client, first, last int, identifier string) error {
	for number := first; number <= last; number++ {
		if err := client.TrackEvent(fmt.Sprintf("event_%d", number), &flagsmith.EventOptions{Identifier: identifier}); err != nil {
			return err
		}
	}
	return nil
}

// When the application flushes events
func FlushEvents(client *flagsmith.Client) error {
	return client.FlushEvents(context.Background())
}

// When the application closes the client with a timeout of {timeout} seconds
func CloseClient(client *flagsmith.Client, timeout float64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout*float64(time.Second)))
	defer cancel()
	client.Close(ctx)
}
