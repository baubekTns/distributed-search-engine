package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/baubekTns/distributed-search-engine/backend/internal/config"
	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
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

	log.Println("crawler worker started")

	runWorker(ctx, frontierService)

	log.Println("crawler worker stopped")
}

func runWorker(ctx context.Context, frontierService *frontier.Frontier) {
	for {
		select {
		case <-ctx.Done():
			log.Println("crawler shutdown requested")
			return

		default:
			targetURL, err := frontierService.Dequeue(ctx, 5*time.Second)
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

			// Actual HTTP fetching will be implemented in the next phase.
			log.Printf("received crawl job: %s", targetURL)
		}
	}
}
