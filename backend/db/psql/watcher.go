package psql

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type WatcherCase struct {
	CaseNumber       string
	TribunalEndpoint string
	ScandalID        string
	ProceedingID     string
	LastMovementID   *string
	Status           string
}

func (db *DB) ListWatcherCasesForPoll(ctx context.Context) ([]WatcherCase, error) {
	rows, err := db.conn.Query(ctx, `
    SELECT case_number, tribunal_endpoint, scandal_id, proceeding_id, last_movement_id, status
    FROM watcher_tracking
    WHERE status = 'active'
       OR (status = 'concluded' AND (last_polled_at IS NULL OR last_polled_at < now() - interval '7 days'))
    ORDER BY added_at ASC
  `)
	if err != nil {
		return nil, fmt.Errorf("psql: list watcher cases: %w", err)
	}
	defer rows.Close()

	out := make([]WatcherCase, 0)
	for rows.Next() {
		var c WatcherCase
		if err := rows.Scan(&c.CaseNumber, &c.TribunalEndpoint, &c.ScandalID, &c.ProceedingID, &c.LastMovementID, &c.Status); err != nil {
			return nil, fmt.Errorf("psql: scan watcher case: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate watcher cases: %w", err)
	}
	return out, nil
}

func (db *DB) UpdateWatcherTrackingPoll(ctx context.Context, caseNumber string, lastMovementID *string, status string, polledAt time.Time) error {
	_, err := db.conn.Exec(ctx, `
    UPDATE watcher_tracking
    SET
      last_movement_id = $2,
      status = $3,
      last_polled_at = $4
    WHERE case_number = $1
  `, caseNumber, lastMovementID, status, polledAt)
	if err != nil {
		return fmt.Errorf("psql: update watcher tracking poll: %w", err)
	}
	return nil
}

func (db *DB) IsWatcherCaseTracked(ctx context.Context, caseNumber string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (SELECT 1 FROM watcher_tracking WHERE case_number = $1)
  `, caseNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: is watcher case tracked: %w", err)
	}
	return exists, nil
}

func (db *DB) UpsertWatcherCase(ctx context.Context, caseNumber, tribunalEndpoint, scandalID, proceedingID, addedBy string) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO watcher_tracking (
      case_number, tribunal_endpoint, scandal_id, proceeding_id,
      status, added_by, added_at
    )
    VALUES ($1, $2, $3, $4, 'active', $5, now())
    ON CONFLICT (case_number) DO NOTHING
  `, caseNumber, tribunalEndpoint, scandalID, proceedingID, addedBy)
	if err != nil {
		return fmt.Errorf("psql: upsert watcher case: %w", err)
	}
	return nil
}

func (db *DB) CreatePendingReview(ctx context.Context, reviewType string, payload []byte, worker string) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO pending_review (type, payload, worker, status, created_at)
    VALUES ($1, $2::jsonb, $3, 'pending', now())
  `, reviewType, string(payload), worker)
	if err != nil {
		return fmt.Errorf("psql: create pending review: %w", err)
	}
	return nil
}

func (db *DB) GetProceedingScandalID(ctx context.Context, caseNumber string) (string, error) {
	var scandalID string
	err := db.conn.QueryRow(ctx, `
    SELECT scandal_id FROM watcher_tracking
    WHERE case_number = $1
    LIMIT 1
  `, caseNumber).Scan(&scandalID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("psql: get proceeding scandal id: %w", err)
	}
	return scandalID, nil
}
