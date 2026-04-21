package psql

import (
	"context"
	"fmt"

	"corruption-center/workers/senado"
)

type SenadoSyncRun struct {
	ID     string
	Status JobStatus
}

func (db *DB) CreateSenadoSyncRun(ctx context.Context) (*SenadoSyncRun, error) {
	var id string
	err := db.conn.QueryRow(ctx, `
    INSERT INTO senado_sync_log (status, started_at)
    VALUES ('running', now())
    RETURNING id
  `).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("psql: create senado sync run: %w", err)
	}
	return &SenadoSyncRun{ID: id, Status: JobStatusRunning}, nil
}

func (db *DB) FinalizeSenadoSyncRun(ctx context.Context, id string, status JobStatus, stats senado.SyncStats, recordsUpserted int, errMsg *string) error {
	_, err := db.conn.Exec(ctx, `
    UPDATE senado_sync_log SET
      status = $2,
      listed_senators = $3,
      active_confirmed = $4,
      skipped_not_active = $5,
      skipped_invalid = $6,
      records_upserted = $7,
      finished_at = now(),
      error_message = $8
    WHERE id = $1
  `,
		id,
		status,
		stats.ListedSenators,
		stats.ActiveConfirmed,
		stats.SkippedNotActive,
		stats.SkippedInvalid,
		recordsUpserted,
		errMsg,
	)
	if err != nil {
		return fmt.Errorf("psql: finalize senado sync run: %w", err)
	}
	return nil
}
