package psql

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SanctionImportState is the per-registry checkpoint for the Sanctions Sync
// worker, so weekly runs are observable and resumable.
type SanctionImportState struct {
	Registry     string
	LastPage     int
	RecordsSeen  int
	LastSyncedAt *time.Time
}

// GetSanctionImportState returns the checkpoint for a registry, or nil when the
// registry has never been synced.
func (db *DB) GetSanctionImportState(ctx context.Context, registry string) (*SanctionImportState, error) {
	var s SanctionImportState
	err := db.conn.QueryRow(ctx, `
    SELECT registry, last_page, records_seen, last_synced_at
    FROM sanctions_import_state
    WHERE registry = $1
  `, registry).Scan(&s.Registry, &s.LastPage, &s.RecordsSeen, &s.LastSyncedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("psql: get sanction import state: %w", err)
	}
	return &s, nil
}

// HasSanctionReview reports whether a possible_politician_sanction review already
// exists for this (sanction_id, politician_id) pair in ANY status. This is the
// sanctions dedup gate mirroring HasDjenCaseCandidate: it subsumes "already
// rejected" (a rejected row still exists) and also prevents re-filing a review
// that is still pending or was approved/deferred, so a weekly re-run of the same
// masked-CPF/name match does not pile up duplicate reviews.
//
// payload is JSONB (001_init.sql), so the pair is extracted with ->> text
// accessors on the two stable payload keys queueReview writes.
func (db *DB) HasSanctionReview(ctx context.Context, sanctionID, politicianID string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (
      SELECT 1 FROM pending_review
      WHERE type = 'possible_politician_sanction'
        AND payload->>'sanction_id' = $1
        AND payload->>'politician_id' = $2
    )
  `, sanctionID, politicianID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: has sanction review: %w", err)
	}
	return exists, nil
}

// UpsertSanctionImportState records where a registry sync finished.
func (db *DB) UpsertSanctionImportState(ctx context.Context, registry string, lastPage, recordsSeen int, syncedAt time.Time) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO sanctions_import_state (registry, last_page, records_seen, last_synced_at, updated_at)
    VALUES ($1, $2, $3, $4, now())
    ON CONFLICT (registry) DO UPDATE SET
      last_page = EXCLUDED.last_page,
      records_seen = EXCLUDED.records_seen,
      last_synced_at = EXCLUDED.last_synced_at,
      updated_at = now()
  `, registry, lastPage, recordsSeen, syncedAt)
	if err != nil {
		return fmt.Errorf("psql: upsert sanction import state: %w", err)
	}
	return nil
}
