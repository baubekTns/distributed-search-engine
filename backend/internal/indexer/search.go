package indexer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type SearchResult struct {
	ID      string  `json:"id"`
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type SearchResponse struct {
	Query       string         `json:"query"`
	ResultCount int            `json:"result_count"`
	Results     []SearchResult `json:"results"`
}

type openSearchResponse struct {
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

func (c *Client) Search(
	ctx context.Context,
	query string,
	limit int,
	offset int,
) (SearchResponse, error) {
	query = strings.TrimSpace(query)

	if query == "" {
		return SearchResponse{}, errors.New(
			"search query cannot be empty",
		)
	}

	if limit <= 0 {
		limit = 10
	}

	if limit > 50 {
		limit = 50
	}

	if offset < 0 {
		offset = 0
	}

	requestBody := map[string]any{
		"from":             offset,
		"size":             limit,
		"track_total_hits": true,
		"_source": []string{
			"url",
			"title",
			"content",
		},
		"query": map[string]any{
			"multi_match": map[string]any{
				"query": query,
				"fields": []string{
					"title^3",
					"content",
				},
				"type":     "best_fields",
				"operator": "or",
			},
		},
		"highlight": map[string]any{
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
			"fields": map[string]any{
				"title": map[string]any{},
				"content": map[string]any{
					"fragment_size":       180,
					"number_of_fragments": 1,
				},
			},
		},
	}

	var rawResponse openSearchResponse

	err := c.doJSON(
		ctx,
		http.MethodPost,
		c.baseURL+"/"+pagesIndex+"/_search",
		requestBody,
		&rawResponse,
	)
	if err != nil {
		return SearchResponse{}, fmt.Errorf(
			"search indexed pages: %w",
			err,
		)
	}

	results := make(
		[]SearchResult,
		0,
		len(rawResponse.Hits.Hits),
	)

	for _, hit := range rawResponse.Hits.Hits {
		title := hit.Source.Title

		if highlightedTitle := firstHighlight(
			hit.Highlight,
			"title",
		); highlightedTitle != "" {
			title = highlightedTitle
		}

		snippet := firstHighlight(
			hit.Highlight,
			"content",
		)

		if snippet == "" {
			snippet = truncateText(
				hit.Source.Content,
				180,
			)
		}

		results = append(results, SearchResult{
			ID:      hit.ID,
			URL:     hit.Source.URL,
			Title:   title,
			Snippet: snippet,
			Score:   hit.Score,
		})
	}

	return SearchResponse{
		Query:       query,
		ResultCount: rawResponse.Hits.Total.Value,
		Results:     results,
	}, nil
}

func firstHighlight(
	highlights map[string][]string,
	field string,
) string {
	values := highlights[field]

	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func truncateText(
	value string,
	maxLength int,
) string {
	value = strings.TrimSpace(value)

	if len(value) <= maxLength {
		return value
	}

	return value[:maxLength] + "..."
}
