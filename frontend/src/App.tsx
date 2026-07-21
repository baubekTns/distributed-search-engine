import { useEffect, useMemo, useRef, useState } from "react";
import type { SubmitEvent } from "react";
import "./App.css";
import { searchPages } from "./services/api";
import type { SearchResponse } from "./types/search";

const PAGE_SIZE = 10;

function stripMarkup(value: string): string {
  const parser = new DOMParser();
  return parser.parseFromString(value, "text/html").body.textContent ?? "";
}

function App() {
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");
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

    searchPages(query, PAGE_SIZE, page * PAGE_SIZE, controller.signal)
      .then(setData)
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

    return () => controller.abort();
  }, [query, page]);

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();

    const trimmedQuery = input.trim();
    if (!trimmedQuery) {
      setError("Enter a search query.");
      return;
    }

    setPage(0);
    setQuery(trimmedQuery);
  }

  return (
    <main className="page-shell">
      <section className="hero">
        <p className="eyebrow">Distributed Search Engine</p>
        <h1>Search the pages your crawler indexed</h1>
        <p className="subtitle">
          Results are ranked by OpenSearch using title and page-content
          relevance.
        </p>

        <form className="search-form" onSubmit={handleSubmit}>
          <label className="sr-only" htmlFor="search-query">
            Search indexed pages
          </label>
          <input
            id="search-query"
            type="search"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            placeholder="Try: distributed systems"
            autoComplete="off"
          />
          <button type="submit" disabled={loading}>
            {loading ? "Searching…" : "Search"}
          </button>
        </form>
      </section>

      <section className="results" aria-live="polite">
        {error && <div className="status error">{error}</div>}

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
              {totalPages > 0 && (
                <p>
                  Page {page + 1} of {totalPages}
                </p>
              )}
            </div>

            {data.results.length === 0 ? (
              <div className="status">No indexed pages matched this query.</div>
            ) : (
              <ol className="result-list">
                {data.results.map((result) => (
                  <li className="result-card" key={result.id}>
                    <a href={result.url} target="_blank" rel="noreferrer">
                      <h2>{stripMarkup(result.title) || result.url}</h2>
                    </a>
                    <p className="result-url">{result.url}</p>
                    <p
                      className="snippet"
                      dangerouslySetInnerHTML={{ __html: result.snippet }}
                    />
                    <p className="score">
                      Relevance score: {result.score.toFixed(3)}
                    </p>
                  </li>
                ))}
              </ol>
            )}

            {totalPages > 1 && (
              <nav className="pagination" aria-label="Search result pages">
                <button
                  type="button"
                  onClick={() => setPage((current) => Math.max(0, current - 1))}
                  disabled={page === 0 || loading}
                >
                  Previous
                </button>
                <button
                  type="button"
                  onClick={() => setPage((current) => current + 1)}
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
