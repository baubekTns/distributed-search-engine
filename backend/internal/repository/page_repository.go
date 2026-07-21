package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PageRepository struct {
	pool *pgxpool.Pool
}

func NewPageRepository(pool *pgxpool.Pool) *PageRepository {
	return &PageRepository{
		pool: pool,
	}
}

func (r *PageRepository) Save(
	ctx context.Context,
	page Page,
) error {
	const query = `
		INSERT INTO pages (
			id,
			url,
			final_url,
			title,
			content,
			content_type,
			status_code,
			content_hash,
			crawl_depth,
			source_url,
			crawled_at,
			updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, NULLIF($10, ''),
			$11, NOW()
		)
		ON CONFLICT (url)
		DO UPDATE SET
			final_url = EXCLUDED.final_url,
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			content_type = EXCLUDED.content_type,
			status_code = EXCLUDED.status_code,
			content_hash = EXCLUDED.content_hash,
			crawl_depth = EXCLUDED.crawl_depth,
			source_url = EXCLUDED.source_url,
			crawled_at = EXCLUDED.crawled_at,
			updated_at = NOW()
	`

	_, err := r.pool.Exec(
		ctx,
		query,
		page.ID,
		page.URL,
		page.FinalURL,
		page.Title,
		page.Content,
		page.ContentType,
		page.StatusCode,
		page.ContentHash,
		page.CrawlDepth,
		page.SourceURL,
		page.CrawledAt,
	)
	if err != nil {
		return fmt.Errorf("save page: %w", err)
	}

	return nil
}

func (r *PageRepository) FindByContentHash(
	ctx context.Context,
	contentHash string,
) (Page, bool, error) {
	const query = `
		SELECT
			id::text,
			url,
			final_url,
			title,
			content,
			content_type,
			status_code,
			content_hash,
			crawl_depth,
			COALESCE(source_url, ''),
			crawled_at
		FROM pages
		WHERE content_hash = $1
		ORDER BY crawled_at ASC
		LIMIT 1
	`

	var page Page

	err := r.pool.QueryRow(
		ctx,
		query,
		contentHash,
	).Scan(
		&page.ID,
		&page.URL,
		&page.FinalURL,
		&page.Title,
		&page.Content,
		&page.ContentType,
		&page.StatusCode,
		&page.ContentHash,
		&page.CrawlDepth,
		&page.SourceURL,
		&page.CrawledAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, false, nil
	}

	if err != nil {
		return Page{}, false, fmt.Errorf(
			"find page by content hash: %w",
			err,
		)
	}

	return page, true, nil
}

func NewPage(
	id string,
	url string,
	finalURL string,
	title string,
	content string,
	contentType string,
	statusCode int,
	contentHash string,
	crawlDepth int,
	sourceURL string,
) Page {
	return Page{
		ID:          id,
		URL:         url,
		FinalURL:    finalURL,
		Title:       title,
		Content:     content,
		ContentType: contentType,
		StatusCode:  statusCode,
		ContentHash: contentHash,
		CrawlDepth:  crawlDepth,
		SourceURL:   sourceURL,
		CrawledAt:   time.Now().UTC(),
	}
}
