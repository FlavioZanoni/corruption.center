package psql

import (
	"context"
	"fmt"
	"time"
)

// ListDjenCasesForPoll returns the cases DJEN case mode should poll, using a
// DJEN-specific poll cursor (watcher_tracking.djen_last_polled_at) rather than
// the DataJud watcher's last_polled_at. This is deliberate: the DataJud watcher
// cron runs before DJEN and refreshes last_polled_at, which previously starved
// concluded cases out of DJEN's window entirely. Cadence here:
//   - active cases: never polled by DJEN, or polled > 1 day ago;
//   - concluded cases: never polled by DJEN, or polled > 7 days ago;
//   - paused cases: skipped.
func (db *DB) ListDjenCasesForPoll(ctx context.Context) ([]WatcherCase, error) {
	rows, err := db.conn.Query(ctx, `
    SELECT case_number, tribunal_endpoint, scandal_id, proceeding_id, last_movement_id, status
    FROM watcher_tracking
    WHERE (status = 'active'
             AND (djen_last_polled_at IS NULL OR djen_last_polled_at < now() - interval '1 day'))
       OR (status = 'concluded'
             AND (djen_last_polled_at IS NULL OR djen_last_polled_at < now() - interval '7 days'))
    ORDER BY added_at ASC
  `)
	if err != nil {
		return nil, fmt.Errorf("psql: list djen cases for poll: %w", err)
	}
	defer rows.Close()

	out := make([]WatcherCase, 0)
	for rows.Next() {
		var c WatcherCase
		if err := rows.Scan(&c.CaseNumber, &c.TribunalEndpoint, &c.ScandalID, &c.ProceedingID, &c.LastMovementID, &c.Status); err != nil {
			return nil, fmt.Errorf("psql: scan djen poll case: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate djen poll cases: %w", err)
	}
	return out, nil
}

// UpdateDjenPolledAt advances the DJEN poll cursor for a case. Called after each
// case is polled in case mode so the next run applies the active/concluded
// cadence in ListDjenCasesForPoll.
func (db *DB) UpdateDjenPolledAt(ctx context.Context, caseNumber string, polledAt time.Time) error {
	_, err := db.conn.Exec(ctx, `
    UPDATE watcher_tracking SET djen_last_polled_at = $2 WHERE case_number = $1
  `, caseNumber, polledAt)
	if err != nil {
		return fmt.Errorf("psql: update djen polled at: %w", err)
	}
	return nil
}

// ListDjenSnapshotKeys returns the set of party_key values already recorded for
// a case, so the DJEN worker can process only the roster delta on each poll.
func (db *DB) ListDjenSnapshotKeys(ctx context.Context, caseNumber string) (map[string]bool, error) {
	rows, err := db.conn.Query(ctx, `
    SELECT party_key FROM djen_party_snapshot WHERE case_number = $1
  `, caseNumber)
	if err != nil {
		return nil, fmt.Errorf("psql: list djen snapshot keys: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("psql: scan djen snapshot key: %w", err)
		}
		out[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate djen snapshot keys: %w", err)
	}
	return out, nil
}

// InsertDjenSnapshotParty records a party as seen for a case. It is idempotent:
// re-inserting the same (case_number, party_key) is a no-op.
func (db *DB) InsertDjenSnapshotParty(ctx context.Context, caseNumber, partyKey, nome, polo string) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO djen_party_snapshot (case_number, party_key, nome, polo)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (case_number, party_key) DO NOTHING
  `, caseNumber, partyKey, nome, polo)
	if err != nil {
		return fmt.Errorf("psql: insert djen snapshot party: %w", err)
	}
	return nil
}

// Case-number normalization contract (Fix: case-number format seam)
// ─────────────────────────────────────────────────────────────────────────────
// The DJEN API returns bare 20-digit numero_processo values, but
// watcher_tracking.case_number (and payload->>'case_number' on candidate
// reviews) may hold the FORMATTED CNJ form ("5046512-94.2016.4.04.7000") when a
// row was seeded via the backoffice. Comparing the API's bare digits against a
// formatted stored value silently misses, so IsCaseTracked / HasDjenCaseCandidate
// normalize BOTH sides to digits-only: the stored column/JSON value is stripped
// with regexp_replace(..., '\D', '', 'g') and the caller passes a digits-only
// param (workers/djen normalizes with normalizeCaseNumber before calling).

// IsCaseTracked reports whether a case number is already present in
// watcher_tracking. DJEN name mode uses this to skip cases already under watch.
// Defined in this DJEN-owned file so the worker does not depend on churn in the
// shared watcher.go helpers. The stored case_number may be formatted, so it is
// normalized to digits-only for comparison; pass a digits-only caseNumber.
func (db *DB) IsCaseTracked(ctx context.Context, caseNumber string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (
      SELECT 1 FROM watcher_tracking
      WHERE regexp_replace(case_number, '\D', '', 'g') = $1
    )
  `, caseNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: is case tracked: %w", err)
	}
	return exists, nil
}

// HasDjenCaseCandidate reports whether a djen_case_candidate review already
// exists for this case number in any state. This is the name-mode dedup gate:
// it subsumes "already rejected" (a rejected row still exists) and also prevents
// re-flagging a case that is still pending or was approved/deferred. The stored
// payload case_number is normalized to digits-only for comparison; pass a
// digits-only caseNumber.
func (db *DB) HasDjenCaseCandidate(ctx context.Context, caseNumber string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (
      SELECT 1 FROM pending_review
      WHERE type = 'djen_case_candidate'
        AND regexp_replace(payload->>'case_number', '\D', '', 'g') = $1
    )
  `, caseNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: has djen case candidate: %w", err)
	}
	return exists, nil
}
