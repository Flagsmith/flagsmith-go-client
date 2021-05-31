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

	// Set a Context if needed.
	// in a net/http based server request handler you can use
	// ctx := request.Context()
	ctx := context.TODO()
	b.SetContext(ctx)

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
}
