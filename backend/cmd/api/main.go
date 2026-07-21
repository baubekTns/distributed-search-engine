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

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})

	openSearchClient := indexer.NewClient(
		cfg.OpenSearchURL,
		indexer.NewHTTPClient(10*time.Second),
	)

	openSearchContext, openSearchCancel := context.WithTimeout(
		ctx,
		15*time.Second,
	)
	defer openSearchCancel()

	if err := openSearchClient.Ping(openSearchContext); err != nil {
		log.Fatalf("OpenSearch connection failed: %v", err)
	}

	if err := openSearchClient.EnsurePagesIndex(
		openSearchContext,
	); err != nil {
		log.Fatalf("failed to initialize OpenSearch index: %v", err)
	}

	frontierService := frontier.New(
		redisClient,
		cfg.CrawlerMaxPagesPerDomain,
	)
	defer func() {
		if err := frontierService.Close(); err != nil {
			log.Printf("failed to close Redis client: %v", err)
		}
	}()

	startupContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := frontierService.Ping(startupContext); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "search-api",
		}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
	})

	frontierHandler := apiHandlers.NewFrontierHandler(frontierService)
	frontierHandler.RegisterRoutes(mux)

	searchHandler := apiHandlers.NewSearchHandler(
		openSearchClient,
	)
	searchHandler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("search API listening on port %s", cfg.APIPort)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	<-ctx.Done()

	log.Println("API shutdown requested")

	shutdownContext, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("API shutdown failed: %v", err)
	}

	log.Println("API stopped")
}
