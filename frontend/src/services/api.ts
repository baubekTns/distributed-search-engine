import type { CrawlerStatusResponse } from "../types/crawler";
import type { SearchOptions, SearchResponse } from "../types/search";

const API_BASE_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

async function readError(
  response: Response,
  fallback: string,
): Promise<string> {
  try {
    const body = (await response.json()) as { error?: string };
    return body.error || fallback;
  } catch {
    return fallback;
  }
}

export async function searchPages(
  options: SearchOptions,
  signal?: AbortSignal,
): Promise<SearchResponse> {
  const params = new URLSearchParams({
    q: options.query,
    limit: String(options.limit ?? 10),
    offset: String(options.offset ?? 0),
  });
  if (options.domain?.trim()) params.set("domain", options.domain.trim());
  if (options.crawledAfter) params.set("crawled_after", options.crawledAfter);
  if (options.crawledBefore)
    params.set("crawled_before", options.crawledBefore);

  const response = await fetch(`${API_BASE_URL}/api/v1/search?${params}`, {
    method: "GET",
    headers: { Accept: "application/json" },
    signal,
  });
  if (!response.ok)
    throw new Error(
      await readError(response, `Search failed with HTTP ${response.status}`),
    );
  return (await response.json()) as SearchResponse;
}

export async function getCrawlerStatus(
  signal?: AbortSignal,
): Promise<CrawlerStatusResponse> {
  const response = await fetch(`${API_BASE_URL}/api/v1/crawlers`, {
    method: "GET",
    headers: { Accept: "application/json" },
    signal,
  });
  if (!response.ok)
    throw new Error(
      await readError(
        response,
        `Crawler status failed with HTTP ${response.status}`,
      ),
    );
  return (await response.json()) as CrawlerStatusResponse;
}
