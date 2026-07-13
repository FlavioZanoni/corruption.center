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
	ListAuditEntries(ctx context.Context, f AuditFilter, limit int) ([]AuditEntry, error)

	// migration tracking for memgraph
	IsMemgraphMigrationApplied(ctx context.Context, filename string) (bool, error)
	RecordMemgraphMigration(ctx context.Context, filename string) error

	// backoffice
	UpsertWatcherCase(ctx context.Context, caseNumber, tribunalEndpoint, scandalID, proceedingID, addedBy string) error
	ListPendingReviews(ctx context.Context, status, typ string, limit int) ([]PendingReviewItem, error)
	CountPendingReviewsByType(ctx context.Context) ([]ReviewTypeCount, error)
	GetPendingReview(ctx context.Context, id string) (PendingReviewItem, error)
	UpdatePendingReviewStatus(ctx context.Context, id string, status string, reviewedBy string) error
	ListWorkerLogs(ctx context.Context, limit int) ([]WorkerLogEntry, error)

	// removal requests (LGPD)
	CreateRemovalRequest(ctx context.Context, requester, targetType, targetID, reason string) (string, error)
	ListRemovalRequests(ctx context.Context, status string, limit int) ([]RemovalRequest, error)
	GetRemovalRequest(ctx context.Context, id string) (RemovalRequest, error)
	ResolveRemovalRequest(ctx context.Context, id, status, resolution, resolvedBy string) error

	// purge tombstones (LGPD anti-resurrection); see tombstone.go
	CreatePurgeTombstones(ctx context.Context, keys []string, nodeID string, removalID string) error
	DeletePurgedSubjectName(ctx context.Context, name string) (int, error)
	IsSubjectPurged(ctx context.Context, keys ...string) (bool, error)
}
