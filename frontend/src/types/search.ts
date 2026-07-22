export interface SearchResult {
  id: string;
  url: string;
  title: string;
  snippet: string;
  score: number;
}

export interface SearchResponse {
  query: string;
  result_count: number;
  query_duration_ms: number;
  results: SearchResult[];
}

export interface SearchOptions {
  query: string;
  limit?: number;
  offset?: number;
  domain?: string;
  crawledAfter?: string;
  crawledBefore?: string;
}
