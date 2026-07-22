import { useEffect, useState } from "react";
import { getCrawlerStatus } from "../services/api";
import type { CrawlerStatusResponse } from "../types/crawler";

const REFRESH_INTERVAL_MS = 10_000;

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value));
}

export function CrawlerStatus() {
  const [status, setStatus] = useState<CrawlerStatusResponse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let activeController: AbortController | null = null;

    const loadStatus = () => {
      activeController?.abort();

      const controller = new AbortController();
      activeController = controller;

      getCrawlerStatus(controller.signal)
        .then((response) => {
          setStatus(response);
          setError("");
        })
        .catch((requestError: unknown) => {
          if (
            requestError instanceof DOMException &&
            requestError.name === "AbortError"
          ) {
            return;
          }

          setError(
            requestError instanceof Error
              ? requestError.message
              : "Failed to read crawler status.",
          );
        });
    };

    loadStatus();

    const timer = window.setInterval(loadStatus, REFRESH_INTERVAL_MS);

    return () => {
      window.clearInterval(timer);
      activeController?.abort();
    };
  }, []);

  return (
    <section className="crawler-status" aria-live="polite">
      <div>
        <p className="crawler-status-label">Crawler cluster</p>
        {status ? (
          <p className="crawler-status-summary">
            <strong>{status.total_workers}</strong> worker
            {status.total_workers === 1 ? "" : "s"} across{" "}
            <strong>{status.active_instances}</strong> active instance
            {status.active_instances === 1 ? "" : "s"}
          </p>
        ) : (
          <p className="crawler-status-summary">Reading crawler status…</p>
        )}
      </div>

      {status && status.instances.length > 0 && (
        <ul className="crawler-instance-list">
          {status.instances.map((instance) => (
            <li key={instance.instance_id}>
              <span>{instance.hostname}</span>
              <span>{instance.worker_count} workers</span>
              <span>Seen {formatTime(instance.last_seen)}</span>
            </li>
          ))}
        </ul>
      )}

      {status && status.instances.length === 0 && (
        <p className="crawler-status-empty">No active crawler instances.</p>
      )}

      {error && <p className="crawler-status-error">{error}</p>}
    </section>
  );
}
