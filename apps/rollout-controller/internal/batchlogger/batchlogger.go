package batchlogger

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventRow struct {
	ID             string
	RequestID      string
	UserID         string
	RolloutID      *string
	RolloutPhaseID *string
	ModelVersionID string
	Assignment     string
	Success        bool
	ErrorType      *string
	LatencyMs      int
	OccurredAt     time.Time
}

// BatchLogger accumulates inference events and flushes them to Postgres
// in bulk every flushInterval to avoid per-row insert overhead.
type BatchLogger struct {
	pool          *pgxpool.Pool
	events        chan EventRow
	flushInterval time.Duration
}

func New(pool *pgxpool.Pool, flushInterval time.Duration) *BatchLogger {
	return &BatchLogger{
		pool:          pool,
		events:        make(chan EventRow, 2000),
		flushInterval: flushInterval,
	}
}

// Enqueue adds an event to the buffer. Non-blocking: drops the event and
// logs a warning if the buffer is full rather than stalling the ingestion path.
func (b *BatchLogger) Enqueue(row EventRow) {
	select {
	case b.events <- row:
	default:
		log.Printf("batchlogger: buffer full, dropping event %s", row.ID)
	}
}

func (b *BatchLogger) Run(ctx context.Context) {
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	var buf []EventRow

	for {
		select {
		case row := <-b.events:
			buf = append(buf, row)

		case <-ticker.C:
			if len(buf) == 0 {
				continue
			}
			if err := b.flush(ctx, buf); err != nil {
				log.Printf("batchlogger: flush failed: %v", err)
			} else {
				log.Printf("batchlogger: flushed %d events", len(buf))
			}
			buf = buf[:0]

		case <-ctx.Done():
			if len(buf) > 0 {
				flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := b.flush(flushCtx, buf); err != nil {
					log.Printf("batchlogger: final flush failed: %v", err)
				}
			}
			return
		}
	}
}

func (b *BatchLogger) flush(ctx context.Context, rows []EventRow) error {
	_, err := b.pool.CopyFrom(ctx,
		pgx.Identifier{"inference_events"},
		[]string{
			"id", "request_id", "user_id", "rollout_id", "rollout_phase_id",
			"model_version_id", "assignment", "success", "error_type",
			"latency_ms", "occurred_at",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{
				r.ID, r.RequestID, r.UserID, r.RolloutID, r.RolloutPhaseID,
				r.ModelVersionID, r.Assignment, r.Success, r.ErrorType,
				r.LatencyMs, r.OccurredAt,
			}, nil
		}),
	)
	return err
}
