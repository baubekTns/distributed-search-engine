package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/baubekTns/distributed-search-engine/backend/internal/config"
	"github.com/baubekTns/distributed-search-engine/backend/internal/crawler"
	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
	"github.com/baubekTns/distributed-search-engine/backend/internal/parser"
	"github.com/baubekTns/distributed-search-engine/backend/internal/security"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	cfg := config.Load()

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})

	frontierService := frontier.New(
		redisClient,
		cfg.CrawlerMaxPagesPerDomain,
	)
	defer func() {
		if err := frontierService.Close(); err != nil {
			log.Printf("failed to close Redis client: %v", err)
		}
	}()

	startupContext, startupCancel := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer startupCancel()

	if err := frontierService.Ping(startupContext); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	validator := security.NewDestinationValidator(
		net.DefaultResolver,
	)

	transport := crawler.NewSafeTransport(validator)

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.CrawlerRequestTimeout,
		CheckRedirect: func(
			request *http.Request,
			previous []*http.Request,
		) error {
			if len(previous) >= cfg.CrawlerMaxRedirects {
				return crawler.ErrTooManyRedirects
			}

			_, err := validator.Validate(
				request.Context(),
				request.URL,
			)

			return err
		},
	}

	crawlerClient := crawler.NewClient(
		httpClient,
		validator,
		crawler.NewDomainLimiter(cfg.CrawlerRequestDelay),
		cfg.CrawlerUserAgent,
		cfg.CrawlerMaxResponseBytes,
	)

	pageParser := parser.NewHTMLParser(
		cfg.CrawlerMaxLinksPerPage,
	)

	log.Println("crawler worker started")

	runWorker(
		ctx,
		frontierService,
		crawlerClient,
		pageParser,
		cfg.CrawlerMaxDepth,
		cfg.CrawlerMaxRetries,
	)

	transport.CloseIdleConnections()

	log.Println("crawler worker stopped")
}

func runWorker(
	ctx context.Context,
	frontierService *frontier.Frontier,
	crawlerClient *crawler.Client,
	pageParser *parser.HTMLParser,
	maxDepth int,
	maxRetries int,
) {
	for {
		if ctx.Err() != nil {
			log.Println("crawler shutdown requested")
			return
		}

		job, err := frontierService.Dequeue(
			ctx,
			5*time.Second,
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			log.Printf("failed to retrieve crawl job: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if job.URL == "" {
			continue
		}

		processJob(
			ctx,
			job,
			frontierService,
			crawlerClient,
			pageParser,
			maxDepth,
			maxRetries,
		)
	}
}

func processJob(
	ctx context.Context,
	job frontier.Job,
	frontierService *frontier.Frontier,
	crawlerClient *crawler.Client,
	pageParser *parser.HTMLParser,
	maxDepth int,
	maxRetries int,
) {
	fetchContext, cancel := context.WithTimeout(
		ctx,
		20*time.Second,
	)
	defer cancel()

	result, err := crawlerClient.Fetch(
		fetchContext,
		job.URL,
	)
	if err != nil {
		logFetchError(job.URL, err)

		if shouldRetry(err) && job.Retry < maxRetries {
			job.Retry++

			if requeueErr := frontierService.Requeue(
				ctx,
				job,
			); requeueErr != nil {
				log.Printf(
					"failed to requeue %s: %v",
					job.URL,
					requeueErr,
				)
			}

			return
		}

		if markErr := frontierService.MarkFailed(
			ctx,
			job,
			err.Error(),
		); markErr != nil {
			log.Printf(
				"failed to mark job failed %s: %v",
				job.URL,
				markErr,
			)
		}

		return
	}

	log.Printf(
		"crawled URL=%s depth=%d retry=%d status=%d type=%s bytes=%d",
		result.FinalURL,
		job.Depth,
		job.Retry,
		result.StatusCode,
		result.ContentType,
		len(result.Body),
	)

	if parser.SupportsContentType(result.ContentType) {
		parsedPage, parseErr := pageParser.Parse(
			result.FinalURL,
			result.Body,
		)
		if parseErr != nil {
			log.Printf(
				"failed to parse page %s: %v",
				result.FinalURL,
				parseErr,
			)

			if markErr := frontierService.MarkFailed(
				ctx,
				job,
				parseErr.Error(),
			); markErr != nil {
				log.Printf(
					"failed to mark job failed: %v",
					markErr,
				)
			}

			return
		}

		log.Printf(
			"parsed URL=%s title=%q text_bytes=%d links=%d depth=%d",
			parsedPage.URL,
			parsedPage.Title,
			len(parsedPage.Text),
			len(parsedPage.Links),
			job.Depth,
		)

		if job.Depth < maxDepth {
			enqueueDiscoveredLinks(
				ctx,
				job,
				parsedPage,
				frontierService,
			)
		} else {
			log.Printf(
				"maximum crawl depth reached URL=%s depth=%d",
				job.URL,
				job.Depth,
			)
		}
	}

	if err := frontierService.MarkCompleted(
		ctx,
		job,
	); err != nil {
		log.Printf(
			"failed to mark job completed %s: %v",
			job.URL,
			err,
		)
	}
}

func enqueueDiscoveredLinks(
	ctx context.Context,
	parentJob frontier.Job,
	parsedPage parser.Page,
	frontierService *frontier.Frontier,
) {
	enqueuedCount := 0
	duplicateCount := 0
	rejectedCount := 0

	for _, discoveredURL := range parsedPage.Links {
		discoveredJob := frontier.Job{
			URL:       discoveredURL,
			Depth:     parentJob.Depth + 1,
			Retry:     0,
			SourceURL: parentJob.URL,
		}

		_, added, err := frontierService.EnqueueJob(
			ctx,
			discoveredJob,
		)

		if err != nil {
			rejectedCount++

			log.Printf(
				"failed to enqueue discovered URL %s: %v",
				discoveredURL,
				err,
			)

			continue
		}

		if added {
			enqueuedCount++
		} else {
			duplicateCount++
		}
	}

	log.Printf(
		"link discovery URL=%s depth=%d discovered=%d enqueued=%d duplicate=%d rejected=%d",
		parsedPage.URL,
		parentJob.Depth,
		len(parsedPage.Links),
		enqueuedCount,
		duplicateCount,
		rejectedCount,
	)
}

func logFetchError(targetURL string, err error) {
	switch {
	case errors.Is(err, crawler.ErrRobotsDenied):
		log.Printf(
			"crawl denied by robots.txt: %s",
			targetURL,
		)

	case errors.Is(err, crawler.ErrResponseTooLarge):
		log.Printf(
			"response too large: %s",
			targetURL,
		)

	case errors.Is(err, crawler.ErrUnsupportedType):
		log.Printf(
			"unsupported content type: %s",
			targetURL,
		)

	case errors.Is(err, crawler.ErrTooManyRedirects):
		log.Printf(
			"too many redirects: %s",
			targetURL,
		)

	case errors.Is(err, security.ErrUnsafeAddress):
		log.Printf(
			"unsafe destination rejected: %s",
			targetURL,
		)

	case errors.Is(err, context.DeadlineExceeded):
		log.Printf(
			"crawl request timed out: %s",
			targetURL,
		)

	case errors.Is(err, context.Canceled):
		log.Printf(
			"crawl request cancelled: %s",
			targetURL,
		)

	default:
		log.Printf(
			"failed to crawl %s: %v",
			targetURL,
			err,
		)
	}
}

func shouldRetry(err error) bool {
	switch {
	case errors.Is(err, crawler.ErrRobotsDenied):
		return false

	case errors.Is(err, crawler.ErrUnsupportedType):
		return false

	case errors.Is(err, crawler.ErrResponseTooLarge):
		return false

	case errors.Is(err, crawler.ErrTooManyRedirects):
		return false

	case errors.Is(err, security.ErrUnsafeAddress):
		return false

	case errors.Is(err, context.Canceled):
		return false

	default:
		return true
	}
}
