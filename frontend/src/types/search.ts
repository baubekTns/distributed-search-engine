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
  results: SearchResult[];
}
