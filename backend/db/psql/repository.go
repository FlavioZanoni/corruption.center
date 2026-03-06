package psql

import (
	"context"

	"corruption-center/api/models"
)

type Repository interface {
	// sources
	UpsertSource(ctx context.Context, s models.Source, rawContent string, checksum string) error
	GetSource(ctx context.Context, id string) (*models.Source, error)
	GetSourceByURL(ctx context.Context, url string) (*models.Source, error)
	GetSourceChecksum(ctx context.Context, url string) (string, error)
	DeactivateSource(ctx context.Context, id string) error

	// scraper jobs
	CreateJob(ctx context.Context, worker string) (string, error)
	UpdateJob(ctx context.Context, id string, status JobStatus, recordsUpserted int, errMsg *string) error
	GetLastJob(ctx context.Context, worker string) (*ScraperJob, error)

	// audit
	LogAudit(ctx context.Context, actorID string, action AuditAction, targetType string, targetID string, metadata map[string]any) error
	GetAuditLog(ctx context.Context, targetType string, targetID string) ([]AuditEntry, error)

	// migration tracking for memgraph
	IsMemgraphMigrationApplied(ctx context.Context, filename string) (bool, error)
	RecordMemgraphMigration(ctx context.Context, filename string) error
}
