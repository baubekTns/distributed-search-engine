package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Wait(ctx context.Context, targetURL *url.URL) error
}

type RedisDomainLimiter struct {
	client   *redis.Client
	interval time.Duration
	prefix   string
}

var acquireDomainSlotScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	if redis.call("SET", key, "1", "NX", "PX", ttl) then
		return 1
	end

	return redis.call("PTTL", key)
`)

func NewRedisDomainLimiter(
	client *redis.Client,
	interval time.Duration,
) *RedisDomainLimiter {
	if interval <= 0 {
		interval = time.Second
	}

	return &RedisDomainLimiter{
		client:   client,
		interval: interval,
		prefix:   "crawler:domain-limit:",
	}
}

func (l *RedisDomainLimiter) Wait(
	ctx context.Context,
	targetURL *url.URL,
) error {
	if targetURL == nil {
		return errors.New("rate limiter target URL is nil")
	}

	hostname := strings.ToLower(
		strings.TrimSpace(targetURL.Hostname()),
	)
	if hostname == "" {
		return errors.New("rate limiter target URL has no hostname")
	}

	key := l.prefix + hostname
	ttlMilliseconds := l.interval.Milliseconds()

	if ttlMilliseconds < 1 {
		ttlMilliseconds = 1
	}

	for {
		result, err := acquireDomainSlotScript.Run(
			ctx,
			l.client,
			[]string{key},
			ttlMilliseconds,
		).Int64()
		if err != nil {
			return fmt.Errorf(
				"acquire Redis domain rate-limit slot: %w",
				err,
			)
		}

		if result == 1 {
			return nil
		}

		waitDuration := time.Duration(result) * time.Millisecond
		if waitDuration <= 0 || waitDuration > l.interval {
			waitDuration = 50 * time.Millisecond
		}

		timer := time.NewTimer(waitDuration)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}

			return ctx.Err()

		case <-timer.C:
		}
	}
}