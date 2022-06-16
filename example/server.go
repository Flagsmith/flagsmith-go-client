package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	flagsmith "github.com/Flagsmith/flagsmith-go-client"
)

func main() {
	http.HandleFunc("/", RootHandler)

	fmt.Printf("Starting server at port 5000\n")
	if err := http.ListenAndServe(":5000", nil); err != nil {
		log.Fatal(err)
	}
}

type TemplateData struct {
	Identifier   string
	ShowButton   bool
	ButtonColour string
}

func RootHandler(w http.ResponseWriter, r *http.Request) {
	// Intialise the flagsmith client
	client := flagsmith.NewClient(os.Getenv("FLAGSMITH_API_KEY"))
	q := r.URL.Query()

	if q.Get("identifier") != "" {
		identifier := q.Get("identifier")
		var traits []*flagsmith.Trait
		traits = nil

		if q.Get("trait-key") != "" {
			trait := flagsmith.Trait{TraitKey: q.Get("trait-key"), TraitValue: q.Get("trait-value")}
			traits = []*flagsmith.Trait{&trait}
		}

		flags, _ := client.GetIdentityFlags(identifier, traits)

		showButton, _ := flags.IsFeatureEnabled("secret_button")
		buttonData, _ := flags.GetFeatureValue("secret_button")

		// convert button data to map
		buttonData = buttonData.(string)
		var buttonDataMap map[string]string
		_ = json.Unmarshal([]byte(buttonData.(string)), &buttonDataMap)

		templateData := TemplateData{
			Identifier:   identifier,
			ShowButton:   showButton,
			ButtonColour: buttonDataMap["colour"],
		}
		t, _ := template.ParseFiles("home.html")
		_ = t.Execute(w, templateData)
		return
	}
	flags, _ := client.GetEnvironmentFlags()

	showButton, _ := flags.IsFeatureEnabled("secret_button")

	buttonData, _ := flags.GetFeatureValue("secret_button")

	// convert button data to map
	buttonData = buttonData.(string)
	var buttonDataMap map[string]string
	_ = json.Unmarshal([]byte(buttonData.(string)), &buttonDataMap)

	templateData := TemplateData{
		ShowButton:   showButton,
		ButtonColour: buttonDataMap["colour"],
	}

	t, _ := template.ParseFiles("home.html")
	_ = t.Execute(w, templateData)
}
