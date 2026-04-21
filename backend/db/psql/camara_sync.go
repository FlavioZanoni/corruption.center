package psql

import (
	"context"
	"fmt"

	"corruption-center/workers/camara"
)

type CamaraSyncRun struct {
	ID     string
	Status JobStatus
}

func (db *DB) CreateCamaraSyncRun(ctx context.Context) (*CamaraSyncRun, error) {
	var id string
	err := db.conn.QueryRow(ctx, `
    INSERT INTO camara_sync_log (status, started_at)
    VALUES ('running', now())
    RETURNING id
  `).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("psql: create camara sync run: %w", err)
	}
	return &CamaraSyncRun{ID: id, Status: JobStatusRunning}, nil
}

func (db *DB) FinalizeCamaraSyncRun(ctx context.Context, id string, status JobStatus, stats camara.SyncStats, recordsUpserted int, errMsg *string) error {
	_, err := db.conn.Exec(ctx, `
    UPDATE camara_sync_log SET
      status = $2,
      listed_deputies = $3,
      detail_fetched = $4,
      active_confirmed = $5,
      skipped_no_cpf = $6,
      skipped_not_active = $7,
      records_upserted = $8,
      finished_at = now(),
      error_message = $9
    WHERE id = $1
  `,
		id,
		status,
		stats.ListedDeputies,
		stats.DetailFetched,
		stats.ActiveConfirmed,
		stats.SkippedNoCPF,
		stats.SkippedNotActive,
		recordsUpserted,
		errMsg,
	)
	if err != nil {
		return fmt.Errorf("psql: finalize camara sync run: %w", err)
	}
	return nil
}
