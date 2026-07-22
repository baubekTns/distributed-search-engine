export interface CrawlerInstance {
  instance_id: string;
  hostname: string;
  worker_count: number;
  started_at: string;
  last_seen: string;
}

export interface CrawlerStatusResponse {
  active_instances: number;
  total_workers: number;
  generated_at: string;
  instances: CrawlerInstance[];
}
