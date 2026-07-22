package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/baubekTns/distributed-search-engine/backend/internal/indexer"
)

type SearchHandler struct { searchClient *indexer.Client }

func NewSearchHandler(searchClient *indexer.Client) *SearchHandler { return &SearchHandler{searchClient: searchClient} }
func (h *SearchHandler) RegisterRoutes(mux *http.ServeMux) { mux.HandleFunc("GET /api/v1/search", h.search) }

func (h *SearchHandler) search(w http.ResponseWriter, r *http.Request) {
    query := strings.TrimSpace(r.URL.Query().Get("q"))
    if query == "" { writeError(w, http.StatusBadRequest, "query parameter q is required"); return }

    limit, err := parseIntegerParameter(r, "limit", 10)
    if err != nil { writeError(w, http.StatusBadRequest, "limit must be a valid integer"); return }
    offset, err := parseIntegerParameter(r, "offset", 0)
    if err != nil { writeError(w, http.StatusBadRequest, "offset must be a valid integer"); return }
    after, err := parseDateParameter(r, "crawled_after")
    if err != nil { writeError(w, http.StatusBadRequest, "crawled_after must use YYYY-MM-DD"); return }
    before, err := parseDateParameter(r, "crawled_before")
    if err != nil { writeError(w, http.StatusBadRequest, "crawled_before must use YYYY-MM-DD"); return }

    response, err := h.searchClient.Search(r.Context(), indexer.SearchOptions{
        Query: query,
        Limit: limit,
        Offset: offset,
        Domain: r.URL.Query().Get("domain"),
        CrawledAfter: after,
        CrawledBefore: before,
    })
    if err != nil { log.Printf("search failed: %v", err); writeError(w, http.StatusInternalServerError, "search failed"); return }
    writeJSON(w, http.StatusOK, response)
}

func parseIntegerParameter(r *http.Request, name string, fallback int) (int, error) {
    value := strings.TrimSpace(r.URL.Query().Get(name))
    if value == "" { return fallback, nil }
    return strconv.Atoi(value)
}

func parseDateParameter(r *http.Request, name string) (*time.Time, error) {
    value := strings.TrimSpace(r.URL.Query().Get(name))
    if value == "" { return nil, nil }
    parsed, err := time.Parse("2006-01-02", value)
    if err != nil { return nil, err }
    return &parsed, nil
}