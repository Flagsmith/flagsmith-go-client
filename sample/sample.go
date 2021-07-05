package main

import (
	"context"
	"fmt"
	"log"
	"time"

	bullettrain "github.com/BulletTrainHQ/bullet-train-go-client"
)

func main() {
	b := bullettrain.NewClient("MgfUaRCvvZMznuQyqjnQKt", bullettrain.Config{
		Timeout: 3 * time.Second,
		BaseURI: "https://api.bullet-train.io/api/v1/", // what a coincidence ;)
	})

	awesome, err := b.FeatureEnabled("awesome_feature")
	if err != nil {
		log.Fatal(err)
	}
	if awesome {
		// do something awesome!
	}

	traits, err := b.GetTraits(bullettrain.User{"test_user"})
	if err != nil {
		log.Fatal(err)
	}
	for _, t := range traits {
		fmt.Println(t.Key, "->", t.Value)
	}

	// use a Context, perhaps from an incomming Request
	ctx := context.Background()
	awesome, err = b.FeatureEnabledWithContext(ctx, "awesome_feature")
	if err != nil {
		log.Fatal(err)
	}
	if awesome {
		// do something awesome!
	}

}
