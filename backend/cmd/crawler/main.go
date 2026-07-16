package main

import (
	"log"
	"time"
)

func main() {
	log.Println("crawler worker started")

	for {
		log.Println("waiting for crawl jobs")
		time.Sleep(10 * time.Second)
	}
}