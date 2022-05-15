package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Flagsmith/flagsmith-go-client"
)

func main() {
	f := flagsmith.NewClient("MgfUaRCvvZMznuQyqjnQKt",
		flagsmith.WithBaseURI("https://api.bullet-train.io/api/v1/"), // what a coincidence ;)
		flagsmith.WithRequestTimeout(3*time.Second),
	)

	awesome, err := f.FeatureEnabled("awesome_feature")
	if err != nil {
		log.Fatal(err)
	}
	if awesome {
		// do something awesome!
	}

	traits, err := f.GetTraits(flagsmith.User{Identifier: "test_user"})
	if err != nil {
		log.Fatal(err)
	}
	for _, t := range traits {
		fmt.Println(t.Key, "->", t.Value)
	}

	// use a Context, perhaps from an incomming Request
	ctx := context.Background()
	awesome, err = f.FeatureEnabledWithContext(ctx, "awesome_feature")
	if err != nil {
		log.Fatal(err)
	}
	if awesome {
		// do something awesome!
	}

}
