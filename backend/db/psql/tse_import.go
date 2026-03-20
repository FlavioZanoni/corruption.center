package psql

import (
	"context"
	"fmt"
)

func (db *DB) IsTSEYearSuccessful(ctx context.Context, year int) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (
      SELECT 1 FROM tse_import_log
      WHERE election_year = $1 AND status = 'success'
    )
  `, year).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: check tse year success: %w", err)
	}
	return exists, nil
}

func (db *DB) UpsertTSEImportLog(ctx context.Context, year int, status JobStatus, recordsImported int, errMsg *string) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO tse_import_log (election_year, status, records_imported, started_at, finished_at, error_message)
    VALUES ($1, $2, $3, now(), CASE WHEN $2 = 'running' THEN NULL ELSE now() END, $4)
    ON CONFLICT (election_year) DO UPDATE SET
      status = EXCLUDED.status,
      records_imported = EXCLUDED.records_imported,
      error_message = EXCLUDED.error_message,
      finished_at = CASE WHEN EXCLUDED.status = 'running' THEN NULL ELSE now() END
  `, year, status, recordsImported, errMsg)
	if err != nil {
		return fmt.Errorf("psql: upsert tse import log: %w", err)
	}
	return nil
}
