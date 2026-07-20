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

	frontierService := frontier.New(redisClient)
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
	)

	transport.CloseIdleConnections()

	log.Println("crawler worker stopped")
}

func runWorker(
	ctx context.Context,
	frontierService *frontier.Frontier,
	crawlerClient *crawler.Client,
	pageParser *parser.HTMLParser,
) {
	for {
		if ctx.Err() != nil {
			log.Println("crawler shutdown requested")
			return
		}

		targetURL, err := frontierService.Dequeue(
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

		if targetURL == "" {
			continue
		}

		processURL(
			ctx,
			targetURL,
			frontierService,
			crawlerClient,
			pageParser,
		)
	}
}

func processURL(
	ctx context.Context,
	targetURL string,
	frontierService *frontier.Frontier,
	crawlerClient *crawler.Client,
	pageParser *parser.HTMLParser,
) {
	fetchContext, cancel := context.WithTimeout(
		ctx,
		20*time.Second,
	)
	defer cancel()

	result, err := crawlerClient.Fetch(
		fetchContext,
		targetURL,
	)
	if err != nil {
		logFetchError(targetURL, err)
		return
	}

	log.Printf(
		"crawled URL=%s status=%d type=%s bytes=%d",
		result.FinalURL,
		result.StatusCode,
		result.ContentType,
		len(result.Body),
	)

	if !parser.SupportsContentType(result.ContentType) {
		log.Printf(
			"content type does not require HTML parsing: URL=%s type=%s",
			result.FinalURL,
			result.ContentType,
		)
		return
	}

	parsedPage, err := pageParser.Parse(
		result.FinalURL,
		result.Body,
	)
	if err != nil {
		log.Printf(
			"failed to parse page %s: %v",
			result.FinalURL,
			err,
		)
		return
	}

	log.Printf(
		"parsed URL=%s title=%q text_bytes=%d links=%d",
		parsedPage.URL,
		parsedPage.Title,
		len(parsedPage.Text),
		len(parsedPage.Links),
	)

	enqueueDiscoveredLinks(
		ctx,
		parsedPage,
		frontierService,
	)
}

func enqueueDiscoveredLinks(
	ctx context.Context,
	parsedPage parser.Page,
	frontierService *frontier.Frontier,
) {
	enqueuedCount := 0
	duplicateCount := 0
	rejectedCount := 0

	for _, discoveredURL := range parsedPage.Links {
		_, added, err := frontierService.Enqueue(
			ctx,
			discoveredURL,
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
			continue
		}

		duplicateCount++
	}

	log.Printf(
		"link discovery URL=%s discovered=%d enqueued=%d duplicate=%d rejected=%d",
		parsedPage.URL,
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
