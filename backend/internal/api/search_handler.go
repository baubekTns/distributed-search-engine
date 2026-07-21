package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/baubekTns/distributed-search-engine/backend/internal/indexer"
)

type SearchHandler struct {
	searchClient *indexer.Client
}

func NewSearchHandler(
	searchClient *indexer.Client,
) *SearchHandler {
	return &SearchHandler{
		searchClient: searchClient,
	}
}

func (h *SearchHandler) RegisterRoutes(
	mux *http.ServeMux,
) {
	mux.HandleFunc(
		"GET /api/v1/search",
		h.search,
	)
}

func (h *SearchHandler) search(
	w http.ResponseWriter,
	r *http.Request,
) {
	query := strings.TrimSpace(
		r.URL.Query().Get("q"),
	)

	if query == "" {
		writeError(
			w,
			http.StatusBadRequest,
			"query parameter q is required",
		)
		return
	}

	limit, err := parseIntegerParameter(
		r,
		"limit",
		10,
	)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"limit must be a valid integer",
		)
		return
	}

	offset, err := parseIntegerParameter(
		r,
		"offset",
		0,
	)
	if err != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"offset must be a valid integer",
		)
		return
	}

	response, err := h.searchClient.Search(
		r.Context(),
		query,
		limit,
		offset,
	)
	if err != nil {
		log.Printf("search failed: %v", err)

		writeError(
			w,
			http.StatusInternalServerError,
			"search failed",
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

func parseIntegerParameter(
	r *http.Request,
	name string,
	fallback int,
) (int, error) {
	value := strings.TrimSpace(
		r.URL.Query().Get(name),
	)

	if value == "" {
		return fallback, nil
	}

	return strconv.Atoi(value)
}
