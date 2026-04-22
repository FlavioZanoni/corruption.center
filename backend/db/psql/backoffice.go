package psql

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type PendingReviewItem struct {
	ID         string
	Type       string
	Payload    string
	Worker     string
	Status     string
	CreatedAt  time.Time
	ReviewedBy *string
	ReviewedAt *time.Time
}

type WorkerLogEntry struct {
	Source          string
	RunID           string
	Status          string
	StartedAt       time.Time
	FinishedAt      *time.Time
	RecordsUpserted int
	ErrorMessage    *string
	Details         string
}

func (db *DB) ListPendingReviews(ctx context.Context, status string, limit int) ([]PendingReviewItem, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)

	rows, err := db.conn.Query(ctx, `
    SELECT id::text, type, payload::text, worker, status, created_at, reviewed_by, reviewed_at
    FROM pending_review
    WHERE ($1 = '' OR status = $1)
    ORDER BY created_at DESC
    LIMIT $2
  `, status, limit)
	if err != nil {
		return nil, fmt.Errorf("psql: list pending reviews: %w", err)
	}
	defer rows.Close()

	out := make([]PendingReviewItem, 0, limit)
	for rows.Next() {
		var item PendingReviewItem
		if err := rows.Scan(&item.ID, &item.Type, &item.Payload, &item.Worker, &item.Status, &item.CreatedAt, &item.ReviewedBy, &item.ReviewedAt); err != nil {
			return nil, fmt.Errorf("psql: scan pending review: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate pending reviews: %w", err)
	}
	return out, nil
}

func (db *DB) UpdatePendingReviewStatus(ctx context.Context, id string, status string, reviewedBy string) error {
	status = strings.TrimSpace(status)
	switch status {
	case "pending", "approved", "rejected", "deferred":
	default:
		return fmt.Errorf("psql: invalid pending review status: %s", status)
	}
	if strings.TrimSpace(reviewedBy) == "" {
		reviewedBy = "backoffice"
	}

	_, err := db.conn.Exec(ctx, `
    UPDATE pending_review
    SET status = $2,
        reviewed_by = $3,
        reviewed_at = now()
    WHERE id = $1::uuid
  `, id, status, reviewedBy)
	if err != nil {
		return fmt.Errorf("psql: update pending review status: %w", err)
	}
	return nil
}

func (db *DB) ListWorkerLogs(ctx context.Context, limit int) ([]WorkerLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.conn.Query(ctx, `
    WITH all_logs AS (
      SELECT
        'scraper_jobs'::text AS source,
        id::text AS run_id,
        status,
        started_at,
        finished_at,
        records_upserted,
        error_message,
        worker AS details
      FROM scraper_jobs

      UNION ALL

      SELECT
        'tse_import'::text AS source,
        id::text AS run_id,
        status,
        started_at,
        finished_at,
        records_imported AS records_upserted,
        error_message,
        ('year=' || election_year::text) AS details
      FROM tse_import_log

      UNION ALL

      SELECT
        'camara_sync'::text AS source,
        id::text AS run_id,
        status,
        started_at,
        finished_at,
        records_upserted,
        error_message,
        ('listed=' || listed_deputies::text || ', active=' || active_confirmed::text) AS details
      FROM camara_sync_log

      UNION ALL

      SELECT
        'senado_sync'::text AS source,
        id::text AS run_id,
        status,
        started_at,
        finished_at,
        records_upserted,
        error_message,
        ('listed=' || listed_senators::text || ', active=' || active_confirmed::text) AS details
      FROM senado_sync_log

      UNION ALL

      SELECT
        'datajud_poll'::text AS source,
        id::text AS run_id,
        status,
        last_polled_at AS started_at,
        NULL::timestamptz AS finished_at,
        0 AS records_upserted,
        NULL::text AS error_message,
        ('case=' || case_number || ', tribunal=' || tribunal_endpoint) AS details
      FROM watcher_tracking
      WHERE last_polled_at IS NOT NULL
    )
    SELECT source, run_id, status, started_at, finished_at, records_upserted, error_message, details
    FROM all_logs
    ORDER BY started_at DESC
    LIMIT $1
  `, limit)
	if err != nil {
		return nil, fmt.Errorf("psql: list worker logs: %w", err)
	}
	defer rows.Close()

	out := make([]WorkerLogEntry, 0, limit)
	for rows.Next() {
		var item WorkerLogEntry
		if err := rows.Scan(&item.Source, &item.RunID, &item.Status, &item.StartedAt, &item.FinishedAt, &item.RecordsUpserted, &item.ErrorMessage, &item.Details); err != nil {
			return nil, fmt.Errorf("psql: scan worker log: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate worker logs: %w", err)
	}
	return out, nil
}
