// Command spec-adapter lets the Flagsmith SDK behavioural spec drive this SDK.
//
// Protocol: https://github.com/Flagsmith/sdk-spec/blob/main/ADAPTER.md
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v5"
)

// version is the SDK version, set at build time with -ldflags "-X main.version=...".
var version = "unknown"

var (
	mu      sync.Mutex
	clients = map[string]*flagsmith.Client{}
	lastID  int
)

type flag struct {
	FeatureName string      `json:"featureName"`
	Enabled     bool        `json:"enabled"`
	Value       interface{} `json:"value"`
}

func serialiseFlags(flags flagsmith.Flags) map[string][]flag {
	result := []flag{}
	for _, f := range flags.AllFlags() {
		result = append(result, flag{FeatureName: f.FeatureName, Enabled: f.Enabled, Value: f.Value})
	}
	return map[string][]flag{"flags": result}
}

func send(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		if err := json.NewEncoder(w).Encode(body); err != nil {
			log.Printf("adapter: encoding response: %v", err)
		}
	}
}

func sendError(w http.ResponseWriter, err error) {
	log.Printf("adapter: %v", err)
	send(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// decode reads a JSON request body, keeping numbers as json.Number so they reach the SDK unchanged.
func decode(r *http.Request, v interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	return decoder.Decode(v)
}

func getClient(w http.ResponseWriter, r *http.Request) *flagsmith.Client {
	mu.Lock()
	defer mu.Unlock()
	client, ok := clients[r.PathValue("clientId")]
	if !ok {
		send(w, http.StatusNotFound, map[string]string{"error": "No client " + r.PathValue("clientId")})
	}
	return client
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		send(w, http.StatusOK, map[string]string{"sdk": "flagsmith-go-client", "version": version})
	})

	mux.HandleFunc("POST /clients", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			EnvironmentKey string `json:"environmentKey"`
			APIURL         string `json:"apiUrl"`
		}
		if err := decode(r, &body); err != nil {
			sendError(w, err)
			return
		}
		client := flagsmith.NewClient(body.EnvironmentKey, flagsmith.WithBaseURL(body.APIURL))

		mu.Lock()
		lastID++
		clientID := strconv.Itoa(lastID)
		clients[clientID] = client
		mu.Unlock()

		send(w, http.StatusCreated, map[string]string{"clientId": clientID})
	})

	mux.HandleFunc("DELETE /clients/{clientId}", func(w http.ResponseWriter, r *http.Request) {
		if getClient(w, r) == nil {
			return
		}
		// Remote evaluation without analytics runs no background work, so there is nothing to stop.
		mu.Lock()
		delete(clients, r.PathValue("clientId"))
		mu.Unlock()
		send(w, http.StatusNoContent, nil)
	})

	mux.HandleFunc("POST /clients/{clientId}/environment-flags", func(w http.ResponseWriter, r *http.Request) {
		client := getClient(w, r)
		if client == nil {
			return
		}
		flags, err := client.GetEnvironmentFlags(r.Context())
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, serialiseFlags(flags))
	})

	mux.HandleFunc("POST /clients/{clientId}/identity-flags", func(w http.ResponseWriter, r *http.Request) {
		client := getClient(w, r)
		if client == nil {
			return
		}
		var body struct {
			Identifier string                 `json:"identifier"`
			Traits     map[string]interface{} `json:"traits"`
		}
		if err := decode(r, &body); err != nil {
			sendError(w, err)
			return
		}
		traits := make([]*flagsmith.Trait, 0, len(body.Traits))
		for key, value := range body.Traits {
			traits = append(traits, &flagsmith.Trait{TraitKey: key, TraitValue: value})
		}
		flags, err := client.GetIdentityFlags(r.Context(), body.Identifier, traits)
		if err != nil {
			sendError(w, err)
			return
		}
		send(w, http.StatusOK, serialiseFlags(flags))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("adapter: listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
