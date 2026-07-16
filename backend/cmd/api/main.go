package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "search-api",
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("search API listening on port 8080")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}