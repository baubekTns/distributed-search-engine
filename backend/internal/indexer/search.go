package indexer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SearchOptions struct {
	Query         string
	Limit         int
	Offset        int
	Domain        string
	CrawledAfter  *time.Time
	CrawledBefore *time.Time
}

type SearchResult struct {
	ID      string  `json:"id"`
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type SearchResponse struct {
	Query           string         `json:"query"`
	ResultCount     int            `json:"result_count"`
	QueryDurationMS int64          `json:"query_duration_ms"`
	Results         []SearchResult `json:"results"`
}

type openSearchResponse struct {
	Took int64 `json:"took"`
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID     string  `json:"_id"`
			Score  float64 `json:"_score"`
			Source struct {
				URL     string `json:"url"`
				Title   string `json:"title"`
				Content string `json:"content"`
			} `json:"_source"`
			Highlight map[string][]string `json:"highlight"`
		} `json:"hits"`
	} `json:"hits"`
}

func (c *Client) Search(ctx context.Context, options SearchOptions) (SearchResponse, error) {
	options.Query = strings.TrimSpace(options.Query)
	if options.Query == "" {
		return SearchResponse{}, errors.New("search query cannot be empty")
	}
	if options.Limit <= 0 {
		options.Limit = 10
	}
	if options.Limit > 50 {
		options.Limit = 50
	}
	if options.Offset < 0 {
		options.Offset = 0
	}

	filters := make([]any, 0, 2)
	if domain := strings.TrimSpace(options.Domain); domain != "" {
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimSuffix(domain, "/")
		filters = append(filters, map[string]any{"wildcard": map[string]any{"url": map[string]any{"value": "*://" + domain + "/*"}}})
	}

	dateRange := map[string]any{}
	if options.CrawledAfter != nil {
		dateRange["gte"] = options.CrawledAfter.UTC().Format(time.RFC3339)
	}
	if options.CrawledBefore != nil {
		dateRange["lte"] = options.CrawledBefore.UTC().Format(time.RFC3339)
	}
	if len(dateRange) > 0 {
		filters = append(filters, map[string]any{"range": map[string]any{"crawled_at": dateRange}})
	}

	boolQuery := map[string]any{"must": []any{map[string]any{"multi_match": map[string]any{
		"query":    options.Query,
		"fields":   []string{"title^3", "content"},
		"type":     "best_fields",
		"operator": "or",
	}}}}
	if len(filters) > 0 {
		boolQuery["filter"] = filters
	}

	requestBody := map[string]any{
		"from":             options.Offset,
		"size":             options.Limit,
		"track_total_hits": true,
		"_source":          []string{"url", "title", "content"},
		"query":            map[string]any{"bool": boolQuery},
		"highlight": map[string]any{
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
			"fields": map[string]any{
				"title":   map[string]any{},
				"content": map[string]any{"fragment_size": 180, "number_of_fragments": 1},
			},
		},
	}

	var raw openSearchResponse
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/"+pagesIndex+"/_search", requestBody, &raw); err != nil {
		return SearchResponse{}, fmt.Errorf("search indexed pages: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Hits.Hits))
	for _, hit := range raw.Hits.Hits {
		title := hit.Source.Title
		if highlighted := firstHighlight(hit.Highlight, "title"); highlighted != "" {
			title = highlighted
		}
		snippet := firstHighlight(hit.Highlight, "content")
		if snippet == "" {
			snippet = truncateText(hit.Source.Content, 180)
		}
		results = append(results, SearchResult{ID: hit.ID, URL: hit.Source.URL, Title: title, Snippet: snippet, Score: hit.Score})
	}

	return SearchResponse{Query: options.Query, ResultCount: raw.Hits.Total.Value, QueryDurationMS: raw.Took, Results: results}, nil
}

func firstHighlight(highlights map[string][]string, field string) string {
	values := highlights[field]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func truncateText(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength] + "..."
}
