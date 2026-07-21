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
	frontierService *frontier.Frontier
	crawlerClient   *crawler.Client
	pageParser      *parser.HTMLParser
	pageRepository  *repository.PageRepository
	openSearchClient *indexer.Client
	maxDepth        int
	maxRetries      int
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

	pageRepository := repository.NewPageRepository(databasePool)

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

	dependencies := workerDependencies{
		frontierService:  frontierService,
		crawlerClient:    crawlerClient,
		pageParser:       pageParser,
		pageRepository:   pageRepository,
		openSearchClient: openSearchClient,
		maxDepth:         cfg.CrawlerMaxDepth,
		maxRetries:       cfg.CrawlerMaxRetries,
	}

	workerCount := cfg.CrawlerWorkerCount
	if workerCount < 1 {
		workerCount = 1
	}

	log.Printf(
		"crawler service started workers=%d domain_delay=%s",
		workerCount,
		cfg.CrawlerRequestDelay,
	)

	runWorkerPool(
		ctx,
		workerCount,
		dependencies,
	)

	log.Println("crawler service stopped")
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
	defer log.Printf("crawler worker stopped worker_id=%d", workerID)

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

		log.Printf(
			"worker accepted job worker_id=%d URL=%s depth=%d retry=%d",
			workerID,
			job.URL,
			job.Depth,
			job.Retry,
		)

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

	log.Printf(
		"crawled worker_id=%d URL=%s depth=%d retry=%d status=%d type=%s bytes=%d",
		workerID,
		result.FinalURL,
		job.Depth,
		job.Retry,
		result.StatusCode,
		result.ContentType,
		len(result.Body),
	)

	if !parser.SupportsContentType(result.ContentType) {
		log.Printf(
			"content type does not require HTML parsing worker_id=%d URL=%s type=%s",
			workerID,
			result.FinalURL,
			result.ContentType,
		)

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
		log.Printf(
			"failed to parse page worker_id=%d URL=%s: %v",
			workerID,
			result.FinalURL,
			err,
		)

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
		log.Printf(
			"failed to store page worker_id=%d URL=%s: %v",
			workerID,
			job.URL,
			err,
		)

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
		log.Printf(
			"failed to index page worker_id=%d URL=%s: %v",
			workerID,
			job.URL,
			err,
		)

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
	} else {
		log.Printf(
			"maximum crawl depth reached worker_id=%d URL=%s depth=%d",
			workerID,
			job.URL,
			job.Depth,
		)
	}

	markJobCompleted(
		ctx,
		dependencies.frontierService,
		job,
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

	existingPage, duplicateContent, err :=
		pageRepository.FindByContentHash(
			ctx,
			contentHash,
		)
	if err != nil {
		return repository.Page{}, err
	}

	if duplicateContent && existingPage.URL != job.URL {
		log.Printf(
			"duplicate page content URL=%s existing_url=%s hash=%s",
			job.URL,
			existingPage.URL,
			contentHash,
		)
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

	log.Printf(
		"stored page URL=%s page_id=%s hash=%s duplicate_content=%t",
		job.URL,
		storedPage.ID,
		contentHash,
		duplicateContent,
	)

	return storedPage, nil
}

func indexPage(
	ctx context.Context,
	page repository.Page,
	openSearchClient *indexer.Client,
) error {
	document := indexer.Document{
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
	}

	if err := openSearchClient.IndexDocument(ctx, document); err != nil {
		return err
	}

	log.Printf(
		"indexed page URL=%s document_id=%s",
		page.URL,
		page.ID,
	)

	return nil
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

func handleFetchFailure(
	ctx context.Context,
	workerID int,
	job frontier.Job,
	err error,
	frontierService *frontier.Frontier,
	maxRetries int,
) {
	logFetchError(workerID, job.URL, err)

	if shouldRetry(err) && job.Retry < maxRetries {
		job.Retry++

		if requeueErr := frontierService.Requeue(ctx, job); requeueErr != nil {
			markJobFailed(
				ctx,
				frontierService,
				job,
				requeueErr,
			)
			return
		}

		log.Printf(
			"requeued worker_id=%d URL=%s retry=%d max_retries=%d",
			workerID,
			job.URL,
			job.Retry,
			maxRetries,
		)

		return
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
	if err := frontierService.MarkCompleted(ctx, job); err != nil {
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

func logFetchError(
	workerID int,
	targetURL string,
	err error,
) {
	log.Printf(
		"crawl failed worker_id=%d URL=%s error=%v",
		workerID,
		targetURL,
		err,
	)
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