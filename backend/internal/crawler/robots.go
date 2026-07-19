package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

type cachedRobots struct {
	data      *robotstxt.RobotsData
	expiresAt time.Time
}

type RobotsManager struct {
	client    *http.Client
	userAgent string

	mu    sync.RWMutex
	cache map[string]cachedRobots
}

func NewRobotsManager(
	client *http.Client,
	userAgent string,
) *RobotsManager {
	return &RobotsManager{
		client:    client,
		userAgent: userAgent,
		cache:     make(map[string]cachedRobots),
	}
}

func (m *RobotsManager) Allowed(
	ctx context.Context,
	targetURL *url.URL,
) (bool, error) {
	cacheKey := targetURL.Scheme + "://" + targetURL.Host

	m.mu.RLock()
	cached, exists := m.cache[cacheKey]
	m.mu.RUnlock()

	if exists && time.Now().Before(cached.expiresAt) {
		return cached.data.TestAgent(
			targetURL.RequestURI(),
			m.userAgent,
		), nil
	}

	robotsURL := &url.URL{
		Scheme: targetURL.Scheme,
		Host:   targetURL.Host,
		Path:   "/robots.txt",
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		robotsURL.String(),
		nil,
	)
	if err != nil {
		return false, fmt.Errorf("create robots request: %w", err)
	}

	request.Header.Set("User-Agent", m.userAgent)

	response, err := m.client.Do(request)
	if err != nil {
		return false, fmt.Errorf("fetch robots.txt: %w", err)
	}
	defer response.Body.Close()

	var robotsData *robotstxt.RobotsData

	switch {
	case response.StatusCode == http.StatusNotFound:
		robotsData, err = robotstxt.FromString("")

	case response.StatusCode >= 200 && response.StatusCode < 300:
		robotsData, err = robotstxt.FromResponse(response)

	default:
		// Conservative policy: do not crawl when robots.txt
		// cannot be interpreted reliably.
		return false, fmt.Errorf(
			"robots.txt returned status %d",
			response.StatusCode,
		)
	}

	if err != nil {
		return false, fmt.Errorf("parse robots.txt: %w", err)
	}

	m.mu.Lock()
	m.cache[cacheKey] = cachedRobots{
		data:      robotsData,
		expiresAt: time.Now().Add(time.Hour),
	}
	m.mu.Unlock()

	return robotsData.TestAgent(
		targetURL.RequestURI(),
		m.userAgent,
	), nil
}
