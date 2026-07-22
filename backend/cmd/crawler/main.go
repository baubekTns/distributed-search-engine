package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/baubekTns/distributed-search-engine/backend/internal/config"
	"github.com/baubekTns/distributed-search-engine/backend/internal/crawler"
	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
	"github.com/baubekTns/distributed-search-engine/backend/internal/indexer"
	"github.com/baubekTns/distributed-search-engine/backend/internal/parser"
	"github.com/baubekTns/distributed-search-engine/backend/internal/repository"
	"github.com/baubekTns/distributed-search-engine/backend/internal/security"
)

const (
	redisStartupTimeout      = 5 * time.Second
	databaseStartupTimeout   = 10 * time.Second
	openSearchStartupTimeout = 15 * time.Second
	openSearchRequestTimeout = 10 * time.Second
	dequeueTimeout           = 5 * time.Second
	jobTimeout               = 20 * time.Second
	retryErrorDelay          = time.Second
	migrationPath            = "/app/migrations/001_create_pages.sql"
)

type workerDependencies struct {
	frontierService  *frontier.Frontier
	crawlerClient    *crawler.Client
	pageParser       *parser.HTMLParser
	pageRepository   *repository.PageRepository
	openSearchClient *indexer.Client
	maxDepth         int
	maxRetries       int
}

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

	frontierService := createFrontier(
		ctx,
		redisClient,
		cfg.CrawlerMaxPagesPerDomain,
	)
	defer func() {
		if err := frontierService.Close(); err != nil {
			log.Printf("failed to close Redis client: %v", err)
		}
	}()

	databasePool := createDatabase(ctx, cfg.PostgresDSN)
	defer databasePool.Close()

	pageRepository := repository.NewPageRepository(
		databasePool,
	)

	openSearchClient := createOpenSearchClient(
		ctx,
		cfg.OpenSearchURL,
	)

	validator := security.NewDestinationValidator(
		net.DefaultResolver,
	)

	transport := crawler.NewSafeTransport(validator)
	defer transport.CloseIdleConnections()

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.CrawlerRequestTimeout,
		CheckRedirect: createRedirectValidator(
			validator,
			cfg.CrawlerMaxRedirects,
		),
	}

	domainLimiter := crawler.NewRedisDomainLimiter(
		redisClient,
		cfg.CrawlerRequestDelay,
	)

	crawlerClient := crawler.NewClient(
		httpClient,
		validator,
		domainLimiter,
		cfg.CrawlerUserAgent,
		cfg.CrawlerMaxResponseBytes,
	)

	pageParser := parser.NewHTMLParser(
		cfg.CrawlerMaxLinksPerPage,
	)

	workerCount := cfg.CrawlerWorkerCount
	if workerCount < 1 {
		workerCount = 1
	}

	instanceID, hostname := resolveInstanceIdentity()

	startedAt := time.Now().UTC()

	heartbeatDone := make(chan struct{})

	go runHeartbeatLoop(
		ctx,
		frontierService,
		frontier.WorkerStatus{
			InstanceID:  instanceID,
			Hostname:    hostname,
			WorkerCount: workerCount,
			StartedAt:   startedAt,
		},
		cfg.CrawlerHeartbeatInterval,
		heartbeatDone,
	)

	dependencies := workerDependencies{
		frontierService:  frontierService,
		crawlerClient:    crawlerClient,
		pageParser:       pageParser,
		pageRepository:   pageRepository,
		openSearchClient: openSearchClient,
		maxDepth:         cfg.CrawlerMaxDepth,
		maxRetries:       cfg.CrawlerMaxRetries,
	}

	log.Printf(
		"crawler service started instance_id=%s workers=%d domain_delay=%s",
		instanceID,
		workerCount,
		cfg.CrawlerRequestDelay,
	)

	runWorkerPool(
		ctx,
		workerCount,
		dependencies,
	)

	<-heartbeatDone

	cleanupContext, cleanupCancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cleanupCancel()

	if err := frontierService.RemoveWorkerHeartbeat(
		cleanupContext,
		instanceID,
	); err != nil {
		log.Printf(
			"failed to remove worker heartbeat: %v",
			err,
		)
	}

	log.Println("crawler service stopped")
}

func resolveInstanceIdentity() (string, string) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}

	return hostname, hostname
}

func runHeartbeatLoop(
	ctx context.Context,
	frontierService *frontier.Frontier,
	status frontier.WorkerStatus,
	interval time.Duration,
	done chan<- struct{},
) {
	defer close(done)

	if interval <= 0 {
		interval = 10 * time.Second
	}

	ttl := interval * 3

	record := func() {
		status.LastSeen = time.Now().UTC()

		heartbeatContext, cancel := context.WithTimeout(
			ctx,
			3*time.Second,
		)
		defer cancel()

		if err := frontierService.RecordWorkerHeartbeat(
			heartbeatContext,
			status,
			ttl,
		); err != nil &&
			ctx.Err() == nil {
			log.Printf(
				"failed to record crawler heartbeat: %v",
				err,
			)
		}
	}

	record()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			record()
		}
	}
}

func createFrontier(
	ctx context.Context,
	redisClient *redis.Client,
	maxPagesPerDomain int,
) *frontier.Frontier {
	frontierService := frontier.New(
		redisClient,
		maxPagesPerDomain,
	)

	startupContext, cancel := context.WithTimeout(
		ctx,
		redisStartupTimeout,
	)
	defer cancel()

	if err := frontierService.Ping(startupContext); err != nil {
		if closeErr := frontierService.Close(); closeErr != nil {
			log.Printf(
				"failed to close Redis client after startup failure: %v",
				closeErr,
			)
		}

		log.Fatalf("Redis connection failed: %v", err)
	}

	return frontierService
}

func createDatabase(
	ctx context.Context,
	postgresDSN string,
) *pgxpool.Pool {
	startupContext, cancel := context.WithTimeout(
		ctx,
		databaseStartupTimeout,
	)
	defer cancel()

	databasePool, err := repository.OpenDatabase(
		startupContext,
		postgresDSN,
	)
	if err != nil {
		log.Fatalf("PostgreSQL connection failed: %v", err)
	}

	if err := repository.RunMigration(
		startupContext,
		databasePool,
		migrationPath,
	); err != nil {
		databasePool.Close()
		log.Fatalf("database migration failed: %v", err)
	}

	return databasePool
}

func createOpenSearchClient(
	ctx context.Context,
	openSearchURL string,
) *indexer.Client {
	httpClient := indexer.NewHTTPClient(
		openSearchRequestTimeout,
	)

	openSearchClient := indexer.NewClient(
		openSearchURL,
		httpClient,
	)

	startupContext, cancel := context.WithTimeout(
		ctx,
		openSearchStartupTimeout,
	)
	defer cancel()

	if err := openSearchClient.Ping(startupContext); err != nil {
		log.Fatalf("OpenSearch connection failed: %v", err)
	}

	if err := openSearchClient.EnsurePagesIndex(
		startupContext,
	); err != nil {
		log.Fatalf(
			"failed to initialize OpenSearch pages index: %v",
			err,
		)
	}

	return openSearchClient
}

func createRedirectValidator(
	validator *security.DestinationValidator,
	maxRedirects int,
) func(*http.Request, []*http.Request) error {
	return func(
		request *http.Request,
		previous []*http.Request,
	) error {
		if len(previous) >= maxRedirects {
			return crawler.ErrTooManyRedirects
		}

		_, err := validator.Validate(
			request.Context(),
			request.URL,
		)

		return err
	}
}

func runWorkerPool(
	ctx context.Context,
	workerCount int,
	dependencies workerDependencies,
) {
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)

	for workerID := 1; workerID <= workerCount; workerID++ {
		go func(id int) {
			defer waitGroup.Done()

			runWorker(
				ctx,
				id,
				dependencies,
			)
		}(workerID)
	}

	<-ctx.Done()

	log.Println("crawler shutdown requested")
	waitGroup.Wait()
}

func runWorker(
	ctx context.Context,
	workerID int,
	dependencies workerDependencies,
) {
	log.Printf("crawler worker started worker_id=%d", workerID)
	defer log.Printf(
		"crawler worker stopped worker_id=%d",
		workerID,
	)

	for {
		if ctx.Err() != nil {
			return
		}

		job, err := dependencies.frontierService.Dequeue(
			ctx,
			dequeueTimeout,
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			log.Printf(
				"failed to retrieve crawl job worker_id=%d: %v",
				workerID,
				err,
			)

			if !sleepWithContext(ctx, retryErrorDelay) {
				return
			}

			continue
		}

		if job.URL == "" {
			continue
		}

		processJob(
			ctx,
			workerID,
			job,
			dependencies,
		)
	}
}

func processJob(
	ctx context.Context,
	workerID int,
	job frontier.Job,
	dependencies workerDependencies,
) {
	jobContext, cancel := context.WithTimeout(
		ctx,
		jobTimeout,
	)
	defer cancel()

	result, err := dependencies.crawlerClient.Fetch(
		jobContext,
		job.URL,
	)
	if err != nil {
		handleFetchFailure(
			ctx,
			workerID,
			job,
			err,
			dependencies.frontierService,
			dependencies.maxRetries,
		)
		return
	}

	if !parser.SupportsContentType(result.ContentType) {
		markJobCompleted(
			ctx,
			dependencies.frontierService,
			job,
		)
		return
	}

	parsedPage, err := dependencies.pageParser.Parse(
		result.FinalURL,
		result.Body,
	)
	if err != nil {
		markJobFailed(
			ctx,
			dependencies.frontierService,
			job,
			err,
		)
		return
	}

	storedPage, err := storePage(
		jobContext,
		job,
		result,
		parsedPage,
		dependencies.pageRepository,
	)
	if err != nil {
		markJobFailed(
			ctx,
			dependencies.frontierService,
			job,
			err,
		)
		return
	}

	if err := indexPage(
		jobContext,
		storedPage,
		dependencies.openSearchClient,
	); err != nil {
		markJobFailed(
			ctx,
			dependencies.frontierService,
			job,
			err,
		)
		return
	}

	if job.Depth < dependencies.maxDepth {
		enqueueDiscoveredLinks(
			ctx,
			job,
			parsedPage,
			dependencies.frontierService,
		)
	}

	markJobCompleted(
		ctx,
		dependencies.frontierService,
		job,
	)

	log.Printf(
		"job completed worker_id=%d URL=%s depth=%d",
		workerID,
		job.URL,
		job.Depth,
	)
}

func storePage(
	ctx context.Context,
	job frontier.Job,
	result crawler.FetchResult,
	parsedPage parser.Page,
	pageRepository *repository.PageRepository,
) (repository.Page, error) {
	contentHash := parser.ContentHash(parsedPage.Text)

	_, _, err := pageRepository.FindByContentHash(
		ctx,
		contentHash,
	)
	if err != nil {
		return repository.Page{}, err
	}

	pageID, err := repository.NewUUID()
	if err != nil {
		return repository.Page{}, err
	}

	storedPage := repository.NewPage(
		pageID,
		job.URL,
		result.FinalURL,
		parsedPage.Title,
		parsedPage.Text,
		result.ContentType,
		result.StatusCode,
		contentHash,
		job.Depth,
		job.SourceURL,
	)

	if err := pageRepository.Save(ctx, storedPage); err != nil {
		return repository.Page{}, err
	}

	return storedPage, nil
}

func indexPage(
	ctx context.Context,
	page repository.Page,
	openSearchClient *indexer.Client,
) error {
	return openSearchClient.IndexDocument(
		ctx,
		indexer.Document{
			ID:          page.ID,
			URL:         page.URL,
			FinalURL:    page.FinalURL,
			Title:       page.Title,
			Content:     page.Content,
			ContentType: page.ContentType,
			StatusCode:  page.StatusCode,
			ContentHash: page.ContentHash,
			CrawlDepth:  page.CrawlDepth,
			SourceURL:   page.SourceURL,
			CrawledAt:   page.CrawledAt,
		},
	)
}

func enqueueDiscoveredLinks(
	ctx context.Context,
	parentJob frontier.Job,
	parsedPage parser.Page,
	frontierService *frontier.Frontier,
) {
	for _, discoveredURL := range parsedPage.Links {
		_, _, _ = frontierService.EnqueueJob(
			ctx,
			frontier.Job{
				URL:       discoveredURL,
				Depth:     parentJob.Depth + 1,
				SourceURL: parentJob.URL,
			},
		)
	}
}

func handleFetchFailure(
	ctx context.Context,
	workerID int,
	job frontier.Job,
	err error,
	frontierService *frontier.Frontier,
	maxRetries int,
) {
	log.Printf(
		"crawl failed worker_id=%d URL=%s error=%v",
		workerID,
		job.URL,
		err,
	)

	if shouldRetry(err) && job.Retry < maxRetries {
		job.Retry++

		if requeueErr := frontierService.Requeue(
			ctx,
			job,
		); requeueErr == nil {
			return
		}
	}

	markJobFailed(
		ctx,
		frontierService,
		job,
		err,
	)
}

func markJobCompleted(
	ctx context.Context,
	frontierService *frontier.Frontier,
	job frontier.Job,
) {
	if err := frontierService.MarkCompleted(
		ctx,
		job,
	); err != nil {
		log.Printf(
			"failed to mark job completed URL=%s: %v",
			job.URL,
			err,
		)
	}
}

func markJobFailed(
	ctx context.Context,
	frontierService *frontier.Frontier,
	job frontier.Job,
	err error,
) {
	if markErr := frontierService.MarkFailed(
		ctx,
		job,
		err.Error(),
	); markErr != nil {
		log.Printf(
			"failed to mark job failed URL=%s: %v",
			job.URL,
			markErr,
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

func sleepWithContext(
	ctx context.Context,
	duration time.Duration,
) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
