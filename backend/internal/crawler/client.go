package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/baubekTns/distributed-search-engine/backend/internal/security"
)

var (
	ErrRobotsDenied      = errors.New("robots.txt denied access")
	ErrUnsupportedType   = errors.New("unsupported response content type")
	ErrResponseTooLarge  = errors.New("response exceeds maximum size")
	ErrTooManyRedirects  = errors.New("maximum redirect count exceeded")
	ErrUnsuccessfulReply = errors.New("server returned unsuccessful status")
)

type FetchResult struct {
	RequestedURL string
	FinalURL     string
	StatusCode   int
	ContentType  string
	Body         []byte
}

type Client struct {
	httpClient       *http.Client
	validator        *security.DestinationValidator
	robots           *RobotsManager
	limiter          *DomainLimiter
	userAgent        string
	maxResponseBytes int64
}

func NewClient(
	httpClient *http.Client,
	validator *security.DestinationValidator,
	limiter *DomainLimiter,
	userAgent string,
	maxResponseBytes int64,
) *Client {
	return &Client{
		httpClient:       httpClient,
		validator:        validator,
		robots:           NewRobotsManager(httpClient, userAgent),
		limiter:          limiter,
		userAgent:        userAgent,
		maxResponseBytes: maxResponseBytes,
	}
}

func (c *Client) Fetch(
	ctx context.Context,
	rawURL string,
) (FetchResult, error) {
	targetURL, err := url.Parse(rawURL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("parse target URL: %w", err)
	}

	if _, err := c.validator.Validate(ctx, targetURL); err != nil {
		return FetchResult{}, err
	}

	allowed, err := c.robots.Allowed(ctx, targetURL)
	if err != nil {
		return FetchResult{}, err
	}

	if !allowed {
		return FetchResult{}, ErrRobotsDenied
	}

	if err := c.limiter.Wait(ctx, targetURL); err != nil {
		return FetchResult{}, fmt.Errorf("wait for domain rate limit: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		targetURL.String(),
		nil,
	)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create request: %w", err)
	}

	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,text/plain;q=0.8",
	)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return FetchResult{}, fmt.Errorf("perform request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf(
			"%w: HTTP %d",
			ErrUnsuccessfulReply,
			response.StatusCode,
		)
	}

	contentType := normalizeContentType(
		response.Header.Get("Content-Type"),
	)

	if !isAllowedContentType(contentType) {
		return FetchResult{}, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedType,
			contentType,
		)
	}

	if response.ContentLength > c.maxResponseBytes {
		return FetchResult{}, ErrResponseTooLarge
	}

	limitedReader := io.LimitReader(
		response.Body,
		c.maxResponseBytes+1,
	)

	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return FetchResult{}, fmt.Errorf("read response body: %w", err)
	}

	if int64(len(body)) > c.maxResponseBytes {
		return FetchResult{}, ErrResponseTooLarge
	}

	return FetchResult{
		RequestedURL: rawURL,
		FinalURL:     response.Request.URL.String(),
		StatusCode:   response.StatusCode,
		ContentType:  contentType,
		Body:         body,
	}, nil
}

func normalizeContentType(rawContentType string) string {
	mediaType, _, err := mime.ParseMediaType(rawContentType)
	if err != nil {
		return strings.ToLower(
			strings.TrimSpace(
				strings.Split(rawContentType, ";")[0],
			),
		)
	}

	return strings.ToLower(mediaType)
}

func isAllowedContentType(contentType string) bool {
	switch contentType {
	case "text/html",
		"application/xhtml+xml",
		"text/plain":
		return true

	default:
		return false
	}
}
