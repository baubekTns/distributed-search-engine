import type { SearchResponse } from "../types/search";

const API_BASE_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export async function searchPages(
  query: string,
  limit = 10,
  offset = 0,
  signal?: AbortSignal,
): Promise<SearchResponse> {
  const params = new URLSearchParams({
    q: query,
    limit: String(limit),
    offset: String(offset),
  });

  const response = await fetch(`${API_BASE_URL}/api/v1/search?${params}`, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

  if (!response.ok) {
    let message = `Search failed with HTTP ${response.status}`;

    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) {
        message = body.error;
      }
    } catch {
      // Keep the fallback message when the response is not JSON.
    }

    throw new Error(message);
  }

  return (await response.json()) as SearchResponse;
}
