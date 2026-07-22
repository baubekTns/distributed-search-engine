package frontier

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const workerKeyPrefix = "crawler:workers:"

type WorkerStatus struct {
	InstanceID  string    `json:"instance_id"`
	Hostname    string    `json:"hostname"`
	WorkerCount int       `json:"worker_count"`
	StartedAt   time.Time `json:"started_at"`
	LastSeen    time.Time `json:"last_seen"`
}

func (f *Frontier) RecordWorkerHeartbeat(
	ctx context.Context,
	status WorkerStatus,
	ttl time.Duration,
) error {
	if strings.TrimSpace(status.InstanceID) == "" {
		return fmt.Errorf("worker instance ID cannot be empty")
	}

	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	payload, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode worker heartbeat: %w", err)
	}

	key := workerKeyPrefix + status.InstanceID

	if err := f.client.Set(
		ctx,
		key,
		payload,
		ttl,
	).Err(); err != nil {
		return fmt.Errorf("store worker heartbeat: %w", err)
	}

	return nil
}

func (f *Frontier) RemoveWorkerHeartbeat(
	ctx context.Context,
	instanceID string,
) error {
	if strings.TrimSpace(instanceID) == "" {
		return nil
	}

	if err := f.client.Del(
		ctx,
		workerKeyPrefix+instanceID,
	).Err(); err != nil {
		return fmt.Errorf("remove worker heartbeat: %w", err)
	}

	return nil
}

func (f *Frontier) ListWorkers(
	ctx context.Context,
) ([]WorkerStatus, error) {
	var (
		cursor  uint64
		workers []WorkerStatus
	)

	for {
		keys, nextCursor, err := f.client.Scan(
			ctx,
			cursor,
			workerKeyPrefix+"*",
			100,
		).Result()
		if err != nil {
			return nil, fmt.Errorf("scan worker heartbeats: %w", err)
		}

		if len(keys) > 0 {
			values, err := f.client.MGet(ctx, keys...).Result()
			if err != nil {
				return nil, fmt.Errorf("read worker heartbeats: %w", err)
			}

			for _, value := range values {
				if value == nil {
					continue
				}

				payload, ok := value.(string)
				if !ok {
					continue
				}

				var status WorkerStatus

				if err := json.Unmarshal(
					[]byte(payload),
					&status,
				); err != nil {
					continue
				}

				workers = append(workers, status)
			}
		}

		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	sort.Slice(
		workers,
		func(left int, right int) bool {
			return workers[left].InstanceID <
				workers[right].InstanceID
		},
	)

	return workers, nil
}