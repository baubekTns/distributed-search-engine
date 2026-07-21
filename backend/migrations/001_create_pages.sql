CREATE TABLE IF NOT EXISTS pages (
    id UUID PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    final_url TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    crawl_depth INTEGER NOT NULL CHECK (crawl_depth >= 0),
    source_url TEXT,
    crawled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pages_content_hash
    ON pages (content_hash);

CREATE INDEX IF NOT EXISTS idx_pages_final_url
    ON pages (final_url);

CREATE INDEX IF NOT EXISTS idx_pages_crawled_at
    ON pages (crawled_at DESC);