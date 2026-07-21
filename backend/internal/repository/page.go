package repository

import "time"

type Page struct {
	ID          string
	URL         string
	FinalURL    string
	Title       string
	Content     string
	ContentType string
	StatusCode  int
	ContentHash string
	CrawlDepth  int
	SourceURL   string
	CrawledAt   time.Time
}
