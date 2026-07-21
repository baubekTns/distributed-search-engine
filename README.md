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
- [x] Track crawl depth and retry count
- [x] Track queued, processing, completed and failed jobs
- [x] Limit pages discovered per domain

### Phase 3 — Safe crawler

- [x] Download pages concurrently
- [x] Add per-domain rate limiting
- [x] Respect robots.txt
- [x] Block private and local IP addresses
- [x] Validate redirects
- [x] Restrict response types and sizes
- [x] Add timeout and retry handling

### Phase 4 — Parsing and storage

- [x] Extract page titles and visible text
- [x] Extract and normalise outgoing links
- [x] Generate content hashes
- [x] Detect duplicate page content
- [x] Store page metadata in PostgreSQL

### Phase 5 — Indexing and search

- [x] Create OpenSearch document index
- [x] Index cleaned page content
- [x] Add BM25 full-text search
- [x] Generate highlighted result snippets
- [x] Add pagination parameters
- [ ] Add domain and date filters

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
