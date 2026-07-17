package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	log.Println("crawler worker started")

	runWorker(ctx)

	log.Println("crawler worker stopped")
}

func runWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("crawler shutdown requested")
			return

		case <-ticker.C:
			log.Println("waiting for crawl jobs")
		}
	}
}
