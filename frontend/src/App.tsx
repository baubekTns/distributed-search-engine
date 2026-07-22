import { useEffect, useMemo, useRef, useState } from "react";
import type { SubmitEvent } from "react";

import "./App.css";
import { CrawlerStatus } from "./components/CrawlerStatus";
import { searchPages } from "./services/api";
import type { SearchResponse } from "./types/search";

const PAGE_SIZE = 10;

function stripMarkup(value: string): string {
  const document = new DOMParser().parseFromString(value, "text/html");

  return document.body.textContent ?? "";
}

function App() {
  const [input, setInput] = useState("");
  const [domain, setDomain] = useState("");
  const [crawledAfter, setCrawledAfter] = useState("");
  const [crawledBefore, setCrawledBefore] = useState("");

  const [query, setQuery] = useState("");
  const [activeDomain, setActiveDomain] = useState("");
  const [activeCrawledAfter, setActiveCrawledAfter] = useState("");
  const [activeCrawledBefore, setActiveCrawledBefore] = useState("");

  const [page, setPage] = useState(0);
  const [data, setData] = useState<SearchResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const requestController = useRef<AbortController | null>(null);

  const totalPages = useMemo(() => {
    if (!data || data.result_count === 0) {
      return 0;
    }

    return Math.ceil(data.result_count / PAGE_SIZE);
  }, [data]);

  useEffect(() => {
    if (!query) {
      return;
    }

    requestController.current?.abort();

    const controller = new AbortController();
    requestController.current = controller;

    setLoading(true);
    setError("");

    searchPages(
      {
        query,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        domain: activeDomain,
        crawledAfter: activeCrawledAfter,
        crawledBefore: activeCrawledBefore,
      },
      controller.signal,
    )
      .then((response) => {
        setData(response);
      })
      .catch((requestError: unknown) => {
        if (
          requestError instanceof DOMException &&
          requestError.name === "AbortError"
        ) {
          return;
        }

        setData(null);

        setError(
          requestError instanceof Error
            ? requestError.message
            : "An unexpected error occurred.",
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => {
      controller.abort();
    };
  }, [query, page, activeDomain, activeCrawledAfter, activeCrawledBefore]);

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();

    const trimmedQuery = input.trim();

    if (!trimmedQuery) {
      setError("Enter a search query.");
      return;
    }

    if (crawledAfter && crawledBefore && crawledAfter > crawledBefore) {
      setError(
        "The crawled-after date cannot be later than the crawled-before date.",
      );
      return;
    }

    setError("");
    setPage(0);
    setQuery(trimmedQuery);
    setActiveDomain(domain.trim());
    setActiveCrawledAfter(crawledAfter);
    setActiveCrawledBefore(crawledBefore);
  }

  function clearFilters() {
    setDomain("");
    setCrawledAfter("");
    setCrawledBefore("");
  }

  function goToPreviousPage() {
    setPage((currentPage) => Math.max(0, currentPage - 1));
  }

  function goToNextPage() {
    setPage((currentPage) => currentPage + 1);
  }

  return (
    <main className="page-shell">
      <CrawlerStatus />

      <section className="hero">
        <p className="eyebrow">Distributed Search Engine</p>

        <h1>Search the pages your crawler indexed</h1>

        <p className="subtitle">
          BM25-ranked results with optional domain and crawl-date filters.
        </p>

        <form className="search-form" onSubmit={handleSubmit}>
          <label className="sr-only" htmlFor="search-query">
            Search indexed pages
          </label>

          <input
            id="search-query"
            type="search"
            value={input}
            onChange={(event) => {
              setInput(event.target.value);
            }}
            placeholder="Try: distributed systems"
            autoComplete="off"
          />

          <button type="submit" disabled={loading}>
            {loading ? "Searching…" : "Search"}
          </button>

          <div className="filter-grid">
            <label htmlFor="domain-filter">
              Domain
              <input
                id="domain-filter"
                type="text"
                value={domain}
                onChange={(event) => {
                  setDomain(event.target.value);
                }}
                placeholder="go.dev"
                autoComplete="off"
              />
            </label>

            <label htmlFor="crawled-after-filter">
              Crawled after
              <input
                id="crawled-after-filter"
                type="date"
                value={crawledAfter}
                onChange={(event) => {
                  setCrawledAfter(event.target.value);
                }}
              />
            </label>

            <label htmlFor="crawled-before-filter">
              Crawled before
              <input
                id="crawled-before-filter"
                type="date"
                value={crawledBefore}
                onChange={(event) => {
                  setCrawledBefore(event.target.value);
                }}
              />
            </label>
          </div>

          <div className="filter-actions">
            <button
              type="button"
              onClick={clearFilters}
              disabled={loading || (!domain && !crawledAfter && !crawledBefore)}
            >
              Clear filters
            </button>
          </div>
        </form>
      </section>

      <section className="results" aria-live="polite" aria-busy={loading}>
        {error && (
          <div className="status error" role="alert">
            {error}
          </div>
        )}

        {!error && loading && (
          <div className="status">Searching indexed pages…</div>
        )}

        {!loading && data && (
          <>
            <div className="results-header">
              <p>
                <strong>{data.result_count}</strong> result
                {data.result_count === 1 ? "" : "s"} for “{data.query}”
              </p>

              <p>
                {data.query_duration_ms} ms
                {totalPages > 0 ? ` · Page ${page + 1} of ${totalPages}` : ""}
              </p>
            </div>

            {data.results.length === 0 ? (
              <div className="status">No indexed pages matched this query.</div>
            ) : (
              <ol className="result-list">
                {data.results.map((result) => {
                  const safeTitle = stripMarkup(result.title) || result.url;

                  return (
                    <li className="result-card" key={result.id}>
                      <a href={result.url} target="_blank" rel="noreferrer">
                        <h2>{safeTitle}</h2>
                      </a>

                      <p className="result-url">{result.url}</p>

                      <p
                        className="snippet"
                        dangerouslySetInnerHTML={{
                          __html: result.snippet,
                        }}
                      />

                      <p className="score">
                        Relevance score: {result.score.toFixed(3)}
                      </p>
                    </li>
                  );
                })}
              </ol>
            )}

            {totalPages > 1 && (
              <nav className="pagination" aria-label="Search result pages">
                <button
                  type="button"
                  onClick={goToPreviousPage}
                  disabled={page === 0 || loading}
                >
                  Previous
                </button>

                <button
                  type="button"
                  onClick={goToNextPage}
                  disabled={page + 1 >= totalPages || loading}
                >
                  Next
                </button>
              </nav>
            )}
          </>
        )}
      </section>
    </main>
  );
}

export default App;
