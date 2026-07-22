package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	apiHandlers "github.com/baubekTns/distributed-search-engine/backend/internal/api"
	"github.com/baubekTns/distributed-search-engine/backend/internal/config"
	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
	"github.com/baubekTns/distributed-search-engine/backend/internal/indexer"
)

const (
	startupTimeout  = 15 * time.Second
	shutdownTimeout = 10 * time.Second
	requestTimeout  = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

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
		startupTimeout,
	)
	defer startupCancel()

	if err := frontierService.Ping(startupContext); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	openSearchClient := indexer.NewClient(
		cfg.OpenSearchURL,
		indexer.NewHTTPClient(requestTimeout),
	)

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

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(
			map[string]string{
				"status":  "ok",
				"service": "search-api",
			},
		); err != nil {
			log.Printf(
				"failed to encode health response: %v",
				err,
			)
		}
	})

	frontierHandler := apiHandlers.NewFrontierHandler(
		frontierService,
	)
	frontierHandler.RegisterRoutes(mux)

	searchHandler := apiHandlers.NewSearchHandler(
		openSearchClient,
	)
	searchHandler.RegisterRoutes(mux)

	workersHandler := apiHandlers.NewWorkersHandler(
		frontierService,
	)
	workersHandler.RegisterRoutes(mux)

	handler := apiHandlers.CORS(
		"http://localhost:5173",
		mux,
	)

	server := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf(
			"search API listening on port %s",
			cfg.APIPort,
		)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("API shutdown requested")

	shutdownContext, shutdownCancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("API shutdown failed: %v", err)
	}

	log.Println("API stopped")
}
