// Package steps implements the Flagsmith SDK behavioural spec: https://github.com/Flagsmith/sdk-spec
//
// Each function implements the Gherkin step in the comment above it.
package steps

import (
	"context"

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
