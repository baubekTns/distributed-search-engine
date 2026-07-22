package api

import (
	"log"
	"net/http"
	"time"

	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
)

type WorkersHandler struct {
	frontier *frontier.Frontier
}

type WorkersResponse struct {
	ActiveInstances int                     `json:"active_instances"`
	TotalWorkers    int                     `json:"total_workers"`
	GeneratedAt     time.Time               `json:"generated_at"`
	Instances       []frontier.WorkerStatus `json:"instances"`
}

func NewWorkersHandler(
	frontierService *frontier.Frontier,
) *WorkersHandler {
	return &WorkersHandler{
		frontier: frontierService,
	}
}

func (h *WorkersHandler) RegisterRoutes(
	mux *http.ServeMux,
) {
	mux.HandleFunc(
		"GET /api/v1/crawlers",
		h.listWorkers,
	)
}

func (h *WorkersHandler) listWorkers(
	w http.ResponseWriter,
	r *http.Request,
) {
	workers, err := h.frontier.ListWorkers(
		r.Context(),
	)
	if err != nil {
		log.Printf("failed to list crawler workers: %v", err)

		writeError(
			w,
			http.StatusInternalServerError,
			"failed to read crawler status",
		)
		return
	}

	totalWorkers := 0

	for _, worker := range workers {
		totalWorkers += worker.WorkerCount
	}

	writeJSON(
		w,
		http.StatusOK,
		WorkersResponse{
			ActiveInstances: len(workers),
			TotalWorkers:    totalWorkers,
			GeneratedAt:     time.Now().UTC(),
			Instances:       workers,
		},
	)
}
