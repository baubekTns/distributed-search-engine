package indexer

import "time"

type Document struct {
	ID          string    `json:"-"`
	URL         string    `json:"url"`
	FinalURL    string    `json:"final_url"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	StatusCode  int       `json:"status_code"`
	ContentHash string    `json:"content_hash"`
	CrawlDepth  int       `json:"crawl_depth"`
	SourceURL   string    `json:"source_url,omitempty"`
	CrawledAt   time.Time `json:"crawled_at"`
}
