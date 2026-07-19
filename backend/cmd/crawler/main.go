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

	log.Println("crawler worker started")

	runWorker(ctx, frontierService, crawlerClient)

	transport.CloseIdleConnections()

	log.Println("crawler worker stopped")
}

func runWorker(
	ctx context.Context,
	frontierService *frontier.Frontier,
	crawlerClient *crawler.Client,
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

		fetchContext, cancel := context.WithTimeout(
			ctx,
			20*time.Second,
		)

		result, err := crawlerClient.Fetch(
			fetchContext,
			targetURL,
		)

		cancel()

		if err != nil {
			switch {
			case errors.Is(err, crawler.ErrRobotsDenied):
				log.Printf("crawl denied by robots.txt: %s", targetURL)

			case errors.Is(err, security.ErrUnsafeAddress):
				log.Printf("unsafe destination rejected: %s", targetURL)

			default:
				log.Printf("failed to crawl %s: %v", targetURL, err)
			}

			continue
		}

		log.Printf(
			"crawled URL=%s status=%d type=%s bytes=%d",
			result.FinalURL,
			result.StatusCode,
			result.ContentType,
			len(result.Body),
		)

		// HTML parsing and link extraction come in the next phase.
	}
}
