package crawler

import (
	"context"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type DomainLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	interval time.Duration
}

func NewDomainLimiter(interval time.Duration) *DomainLimiter {
	if interval <= 0 {
		interval = time.Second
	}

	return &DomainLimiter{
		limiters: make(map[string]*rate.Limiter),
		interval: interval,
	}
}

func (l *DomainLimiter) Wait(
	ctx context.Context,
	targetURL *url.URL,
) error {
	hostname := targetURL.Hostname()

	l.mu.Lock()

	limiter, exists := l.limiters[hostname]
	if !exists {
		limiter = rate.NewLimiter(
			rate.Every(l.interval),
			1,
		)
		l.limiters[hostname] = limiter
	}

	l.mu.Unlock()

	return limiter.Wait(ctx)
}
