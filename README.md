# Distributed Search Engine

A distributed web crawler and search engine built with Go, Redis, PostgreSQL, OpenSearch, React, TypeScript and Docker.

## Overview

The system crawls approved websites, extracts and cleans page content, stores crawl metadata, builds a full-text search index and returns ranked search results through a web interface.

The project is designed to demonstrate:

- Concurrent and distributed web crawling
- Redis-backed URL queue management
- URL normalisation and deduplication
- Safe URL validation and SSRF protection
- HTML parsing and content extraction
- PostgreSQL metadata storage
- OpenSearch indexing and BM25 ranking
- Containerised service deployment
- Monitoring and fault-tolerant worker processing

## Planned Architecture

```text
Seed URLs
    ↓
Redis URL Frontier
    ↓
Go Crawler Workers
    ↓
URL Validation and HTML Parsing
    ↓
PostgreSQL Metadata Storage
    ↓
OpenSearch Index
    ↓
Go Search API
    ↓
React Search Interface
```

## Technology Stack

### Backend

- Go
- Redis
- PostgreSQL
- OpenSearch

### Frontend

- React
- TypeScript
- Vite

### Infrastructure

- Docker
- Docker Compose

## Project Roadmap

### Phase 1 — Repository and service setup

- [x] Create repository structure
- [x] Initialise Go backend
- [x] Create API health endpoint
- [x] Create crawler worker entry point
- [x] Initialise React frontend
- [x] Add Docker Compose development environment

### Phase 2 — URL frontier

- [x] Add seed URL submission
- [x] Create Redis-backed URL queue
- [x] Normalise URLs
- [x] Prevent duplicate queue entries
- [ ] Track crawl depth and retry count

### Phase 3 — Safe crawler

- [ ] Download pages concurrently
- [ ] Add per-domain rate limiting
- [ ] Respect robots.txt
- [ ] Block private and local IP addresses
- [ ] Validate redirects
- [ ] Restrict response types and sizes
- [ ] Add timeout and retry handling

### Phase 4 — Parsing and storage

- [x] Extract page titles and visible text
- [x] Extract and normalise outgoing links
- [ ] Generate content hashes
- [ ] Detect duplicate pages
- [ ] Store page metadata in PostgreSQL

### Phase 5 — Indexing and search

- [ ] Create OpenSearch document index
- [ ] Index cleaned page content
- [ ] Add BM25 full-text search
- [ ] Generate highlighted result snippets
- [ ] Add pagination and filters

### Phase 6 — Frontend

- [ ] Create search form
- [ ] Display ranked search results
- [ ] Add pagination
- [ ] Display result count and query duration
- [ ] Add crawler status dashboard

### Phase 7 — Distributed deployment and monitoring

- [ ] Run multiple crawler workers
- [ ] Add graceful worker shutdown
- [ ] Add structured logging
- [ ] Add Prometheus metrics
- [ ] Add Grafana dashboards
- [ ] Add automated tests and CI

## Development Status

The project is currently in the initial repository and service setup phase.
