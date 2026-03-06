package psql

import (
	"context"
	"fmt"
	"time"
)

type ScraperJob struct {
	ID              string
	Worker          string
	Status          string
	StartedAt       time.Time
	FinishedAt      *time.Time
	RecordsUpserted int
	ErrorMessage    *string
}

type JobStatus string

const (
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusFailed  JobStatus = "failed"
	JobStatusSkipped JobStatus = "skipped"
)

func (db *DB) CreateJob(ctx context.Context, worker string) (string, error) {
	var id string
	err := db.conn.QueryRow(ctx, `
    INSERT INTO scraper_jobs (worker, status, started_at)
    VALUES ($1, 'running', now())
    RETURNING id
  `, worker).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("psql: create job: %w", err)
	}
	return id, nil
}

func (db *DB) UpdateJob(ctx context.Context, id string, status JobStatus, recordsUpserted int, errMsg *string) error {
	_, err := db.conn.Exec(ctx, `
    UPDATE scraper_jobs SET
      status           = $2,
      finished_at      = now(),
      records_upserted = $3,
      error_message    = $4
    WHERE id = $1
  `, id, status, recordsUpserted, errMsg)
	if err != nil {
		return fmt.Errorf("psql: update job: %w", err)
	}
	return nil
}

func (db *DB) GetLastJob(ctx context.Context, worker string) (*ScraperJob, error) {
	row := db.conn.QueryRow(ctx, `
    SELECT id, worker, status, started_at, finished_at, records_upserted, error_message
    FROM scraper_jobs
    WHERE worker = $1
    ORDER BY started_at DESC
    LIMIT 1
  `, worker)

	var j ScraperJob
	err := row.Scan(
		&j.ID, &j.Worker, &j.Status, &j.StartedAt,
		&j.FinishedAt, &j.RecordsUpserted, &j.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("psql: get last job: %w", err)
	}
	return &j, nil
}
