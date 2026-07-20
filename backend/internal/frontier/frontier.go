package frontier

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey      = "frontier:queue"
	knownKey      = "frontier:known"
	processingKey = "frontier:processing"
	completedKey  = "frontier:completed"
	failedKey     = "frontier:failed"
)

var enqueueScript = redis.NewScript(`
	local added = redis.call("SADD", KEYS[1], ARGV[1])

	if added == 0 then
		return 0
	end

	local domain_count = redis.call("HINCRBY", KEYS[2], ARGV[2], 1)
	local max_pages = tonumber(ARGV[4])

	if max_pages > 0 and domain_count > max_pages then
		redis.call("SREM", KEYS[1], ARGV[1])
		redis.call("HINCRBY", KEYS[2], ARGV[2], -1)
		return -1
	end

	redis.call("LPUSH", KEYS[3], ARGV[3])
	return 1
`)

type Frontier struct {
	client            *redis.Client
	maxPagesPerDomain int
}

type Stats struct {
	QueuedCount     int64 `json:"queued_count"`
	KnownCount      int64 `json:"known_count"`
	ProcessingCount int64 `json:"processing_count"`
	CompletedCount  int64 `json:"completed_count"`
	FailedCount     int64 `json:"failed_count"`
}

func New(
	client *redis.Client,
	maxPagesPerDomain ...int,
) *Frontier {
	limit := 0

	if len(maxPagesPerDomain) > 0 {
		limit = maxPagesPerDomain[0]
	}

	return &Frontier{
		client:            client,
		maxPagesPerDomain: limit,
	}
}

func (f *Frontier) Ping(ctx context.Context) error {
	if err := f.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}

	return nil
}

func (f *Frontier) Enqueue(
	ctx context.Context,
	rawURL string,
) (string, bool, error) {
	job := Job{
		URL:   rawURL,
		Depth: 0,
		Retry: 0,
	}

	normalizedJob, added, err := f.EnqueueJob(ctx, job)
	return normalizedJob.URL, added, err
}

func (f *Frontier) EnqueueJob(
	ctx context.Context,
	job Job,
) (Job, bool, error) {
	normalizedURL, err := NormalizeURL(job.URL)
	if err != nil {
		return Job{}, false, err
	}

	job.URL = normalizedURL

	if job.SourceURL != "" {
		normalizedSource, normalizeErr := NormalizeURL(job.SourceURL)
		if normalizeErr == nil {
			job.SourceURL = normalizedSource
		}
	}

	parsedURL, err := url.Parse(job.URL)
	if err != nil {
		return Job{}, false, fmt.Errorf("parse normalized URL: %w", err)
	}

	payload, err := job.Marshal()
	if err != nil {
		return Job{}, false, fmt.Errorf("encode crawl job: %w", err)
	}

	domainCountKey := "frontier:domain-count"

	result, err := enqueueScript.Run(
		ctx,
		f.client,
		[]string{
			knownKey,
			domainCountKey,
			queueKey,
		},
		job.URL,
		parsedURL.Hostname(),
		payload,
		f.maxPagesPerDomain,
	).Int()

	if err != nil {
		return Job{}, false, fmt.Errorf("enqueue crawl job: %w", err)
	}

	switch result {
	case 1:
		return job, true, nil

	case 0:
		return job, false, nil

	case -1:
		return job, false, errors.New(
			"maximum pages per domain reached",
		)

	default:
		return Job{}, false, errors.New(
			"unexpected Redis enqueue result",
		)
	}
}

func (f *Frontier) Dequeue(
	ctx context.Context,
	timeout time.Duration,
) (Job, error) {
	result, err := f.client.BRPop(
		ctx,
		timeout,
		queueKey,
	).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Job{}, nil
		}

		return Job{}, fmt.Errorf("dequeue crawl job: %w", err)
	}

	if len(result) != 2 {
		return Job{}, errors.New(
			"unexpected Redis queue response",
		)
	}

	job, err := UnmarshalJob(result[1])
	if err != nil {
		return Job{}, fmt.Errorf("decode crawl job: %w", err)
	}

	if err := f.client.HSet(
		ctx,
		processingKey,
		job.URL,
		result[1],
	).Err(); err != nil {
		return Job{}, fmt.Errorf(
			"mark crawl job processing: %w",
			err,
		)
	}

	return job, nil
}

func (f *Frontier) MarkCompleted(
	ctx context.Context,
	job Job,
) error {
	payload, err := job.Marshal()
	if err != nil {
		return err
	}

	pipeline := f.client.TxPipeline()

	pipeline.HDel(ctx, processingKey, job.URL)
	pipeline.HSet(ctx, completedKey, job.URL, payload)

	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("mark crawl job completed: %w", err)
	}

	return nil
}

func (f *Frontier) MarkFailed(
	ctx context.Context,
	job Job,
	failureMessage string,
) error {
	payload, err := jsonFailure(job, failureMessage)
	if err != nil {
		return err
	}

	pipeline := f.client.TxPipeline()

	pipeline.HDel(ctx, processingKey, job.URL)
	pipeline.HSet(ctx, failedKey, job.URL, payload)

	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("mark crawl job failed: %w", err)
	}

	return nil
}

func (f *Frontier) Requeue(
	ctx context.Context,
	job Job,
) error {
	payload, err := job.Marshal()
	if err != nil {
		return err
	}

	pipeline := f.client.TxPipeline()

	pipeline.HDel(ctx, processingKey, job.URL)
	pipeline.LPush(ctx, queueKey, payload)

	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("requeue crawl job: %w", err)
	}

	return nil
}

func (f *Frontier) Stats(ctx context.Context) (Stats, error) {
	pipeline := f.client.Pipeline()

	queued := pipeline.LLen(ctx, queueKey)
	known := pipeline.SCard(ctx, knownKey)
	processing := pipeline.HLen(ctx, processingKey)
	completed := pipeline.HLen(ctx, completedKey)
	failed := pipeline.HLen(ctx, failedKey)

	if _, err := pipeline.Exec(ctx); err != nil {
		return Stats{}, fmt.Errorf(
			"read frontier statistics: %w",
			err,
		)
	}

	return Stats{
		QueuedCount:     queued.Val(),
		KnownCount:      known.Val(),
		ProcessingCount: processing.Val(),
		CompletedCount:  completed.Val(),
		FailedCount:     failed.Val(),
	}, nil
}

func (f *Frontier) Close() error {
	return f.client.Close()
}
