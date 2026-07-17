package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := getEnvironmentVariable("API_PORT", "8080")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "search-api",
		}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("search API listening on port %s", port)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("API server failed: %v", err)
	}
}

func getEnvironmentVariable(key string, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
