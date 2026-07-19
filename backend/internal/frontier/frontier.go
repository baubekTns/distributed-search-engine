package frontier

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey  = "frontier:queue"
	queuedKey = "frontier:queued"
)

var enqueueScript = redis.NewScript(`
	local added = redis.call("SADD", KEYS[1], ARGV[1])

	if added == 1 then
		redis.call("LPUSH", KEYS[2], ARGV[1])
	end

	return added
`)

type Frontier struct {
	client *redis.Client
}

type Stats struct {
	QueuedCount int64 `json:"queued_count"`
	KnownCount  int64 `json:"known_count"`
}

func New(client *redis.Client) *Frontier {
	return &Frontier{
		client: client,
	}
}

func (f *Frontier) Ping(ctx context.Context) error {
	if err := f.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}

	return nil
}

func (f *Frontier) Enqueue(ctx context.Context, rawURL string) (string, bool, error) {
	normalizedURL, err := NormalizeURL(rawURL)
	if err != nil {
		return "", false, err
	}

	result, err := enqueueScript.Run(
		ctx,
		f.client,
		[]string{queuedKey, queueKey},
		normalizedURL,
	).Int()

	if err != nil {
		return "", false, fmt.Errorf("enqueue URL: %w", err)
	}

	return normalizedURL, result == 1, nil
}

func (f *Frontier) Dequeue(
	ctx context.Context,
	timeout time.Duration,
) (string, error) {
	result, err := f.client.BRPop(ctx, timeout, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}

		return "", fmt.Errorf("dequeue URL: %w", err)
	}

	if len(result) != 2 {
		return "", errors.New("unexpected Redis queue response")
	}

	return result[1], nil
}

func (f *Frontier) Stats(ctx context.Context) (Stats, error) {
	pipeline := f.client.Pipeline()

	queueLength := pipeline.LLen(ctx, queueKey)
	knownCount := pipeline.SCard(ctx, queuedKey)

	if _, err := pipeline.Exec(ctx); err != nil {
		return Stats{}, fmt.Errorf("read frontier statistics: %w", err)
	}

	return Stats{
		QueuedCount: queueLength.Val(),
		KnownCount:  knownCount.Val(),
	}, nil
}

func (f *Frontier) Close() error {
	return f.client.Close()
}
