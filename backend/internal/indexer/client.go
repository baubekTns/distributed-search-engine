package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const pagesIndex = "pages"

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(
	baseURL string,
	httpClient *http.Client,
) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create OpenSearch ping request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("ping OpenSearch: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("ping OpenSearch", response)
	}

	return nil
}

func (c *Client) EnsurePagesIndex(ctx context.Context) error {
	indexURL := c.baseURL + "/" + pagesIndex

	existsRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodHead,
		indexURL,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create index existence request: %w", err)
	}

	existsResponse, err := c.httpClient.Do(existsRequest)
	if err != nil {
		return fmt.Errorf("check pages index: %w", err)
	}
	defer existsResponse.Body.Close()

	switch existsResponse.StatusCode {
	case http.StatusOK:
		return nil

	case http.StatusNotFound:
		// Continue and create it.

	default:
		return responseError(
			"check pages index",
			existsResponse,
		)
	}

	indexDefinition := map[string]any{
		"settings": map[string]any{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"url": map[string]any{
					"type": "keyword",
				},
				"final_url": map[string]any{
					"type": "keyword",
				},
				"title": map[string]any{
					"type": "text",
					"fields": map[string]any{
						"keyword": map[string]any{
							"type":         "keyword",
							"ignore_above": 512,
						},
					},
				},
				"content": map[string]any{
					"type": "text",
				},
				"content_type": map[string]any{
					"type": "keyword",
				},
				"status_code": map[string]any{
					"type": "integer",
				},
				"content_hash": map[string]any{
					"type": "keyword",
				},
				"crawl_depth": map[string]any{
					"type": "integer",
				},
				"source_url": map[string]any{
					"type": "keyword",
				},
				"crawled_at": map[string]any{
					"type": "date",
				},
			},
		},
	}

	return c.doJSON(
		ctx,
		http.MethodPut,
		indexURL,
		indexDefinition,
		nil,
	)
}

func (c *Client) IndexDocument(
	ctx context.Context,
	document Document,
) error {
	if document.ID == "" {
		return errors.New("OpenSearch document ID cannot be empty")
	}

	documentURL := fmt.Sprintf(
		"%s/%s/_doc/%s",
		c.baseURL,
		pagesIndex,
		url.PathEscape(document.ID),
	)

	return c.doJSON(
		ctx,
		http.MethodPut,
		documentURL,
		document,
		nil,
	)
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	targetURL string,
	requestValue any,
	responseValue any,
) error {
	var body io.Reader

	if requestValue != nil {
		data, err := json.Marshal(requestValue)
		if err != nil {
			return fmt.Errorf("encode OpenSearch request: %w", err)
		}

		body = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		method,
		targetURL,
		body,
	)
	if err != nil {
		return fmt.Errorf("create OpenSearch request: %w", err)
	}

	if requestValue != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("perform OpenSearch request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("OpenSearch request failed", response)
	}

	if responseValue == nil {
		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(responseValue); err != nil {
		return fmt.Errorf("decode OpenSearch response: %w", err)
	}

	return nil
}

func responseError(
	operation string,
	response *http.Response,
) error {
	body, readErr := io.ReadAll(
		io.LimitReader(response.Body, 64*1024),
	)
	if readErr != nil {
		return fmt.Errorf(
			"%s: HTTP %d; failed to read response: %w",
			operation,
			response.StatusCode,
			readErr,
		)
	}

	return fmt.Errorf(
		"%s: HTTP %d: %s",
		operation,
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}
