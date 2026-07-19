package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
)

const maxSeedURLs = 100

type FrontierHandler struct {
	frontier *frontier.Frontier
}

type SeedRequest struct {
	URLs []string `json:"urls"`
}

type SeedResult struct {
	OriginalURL   string `json:"original_url"`
	NormalizedURL string `json:"normalized_url,omitempty"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type SeedResponse struct {
	Submitted int          `json:"submitted"`
	Enqueued  int          `json:"enqueued"`
	Duplicate int          `json:"duplicate"`
	Rejected  int          `json:"rejected"`
	Results   []SeedResult `json:"results"`
}

func NewFrontierHandler(frontierService *frontier.Frontier) *FrontierHandler {
	return &FrontierHandler{
		frontier: frontierService,
	}
}

func (h *FrontierHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/seeds", h.submitSeeds)
	mux.HandleFunc("GET /api/v1/frontier/stats", h.getStats)
}

func (h *FrontierHandler) submitSeeds(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var request SeedRequest

	decoder := json.NewDecoder(
		http.MaxBytesReader(w, r.Body, 64*1024),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	if len(request.URLs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one URL is required")
		return
	}

	if len(request.URLs) > maxSeedURLs {
		writeError(w, http.StatusBadRequest, "a maximum of 100 URLs is allowed")
		return
	}

	response := SeedResponse{
		Submitted: len(request.URLs),
		Results:   make([]SeedResult, 0, len(request.URLs)),
	}

	for _, rawURL := range request.URLs {
		normalizedURL, added, err := h.frontier.Enqueue(
			r.Context(),
			rawURL,
		)

		result := SeedResult{
			OriginalURL: rawURL,
		}

		if err != nil {
			result.Status = "rejected"
			result.Error = err.Error()
			response.Rejected++
			response.Results = append(response.Results, result)
			continue
		}

		result.NormalizedURL = normalizedURL

		if added {
			result.Status = "enqueued"
			response.Enqueued++
		} else {
			result.Status = "duplicate"
			response.Duplicate++
		}

		response.Results = append(response.Results, result)
	}

	writeJSON(w, http.StatusAccepted, response)
}

func (h *FrontierHandler) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.frontier.Stats(r.Context())
	if err != nil {
		log.Printf("failed to read frontier stats: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read frontier statistics")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// This reference avoids accidental removal if error classification is
// expanded later.
var _ = errors.Is
