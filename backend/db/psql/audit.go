package psql

import (
	"context"
	"fmt"
	"time"
)

type AuditEntry struct {
	ID         string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
	CreatedAt  time.Time
}

type AuditAction string

const (
	AuditActionCreate AuditAction = "create"
	AuditActionUpdate AuditAction = "update"
	AuditActionDelete AuditAction = "delete"
)

func (db *DB) LogAudit(ctx context.Context, actorID string, action AuditAction, targetType string, targetID string, metadata map[string]any) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO audit_log (actor_id, action, target_type, target_id, metadata, created_at)
    VALUES ($1, $2, $3, $4, $5, now())
  `, actorID, action, targetType, targetID, metadata)
	if err != nil {
		return fmt.Errorf("psql: log audit: %w", err)
	}
	return nil
}

// AuditFilter narrows a ListAuditEntries query. Empty fields are ignored so the
// backoffice audit view can filter by any combination of actor, action and
// target type.
type AuditFilter struct {
	ActorID    string
	Action     string
	TargetType string
}

// ListAuditEntries returns audit_log rows matching the given filter, newest
// first. It powers the filterable audit view in the backoffice (who/what/when).
func (db *DB) ListAuditEntries(ctx context.Context, f AuditFilter, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.conn.Query(ctx, `
    SELECT id, actor_id, action, target_type, target_id, metadata, created_at
    FROM audit_log
    WHERE ($1 = '' OR actor_id = $1)
      AND ($2 = '' OR action = $2)
      AND ($3 = '' OR target_type = $3)
    ORDER BY created_at DESC
    LIMIT $4
  `, f.ActorID, f.Action, f.TargetType, limit)
	if err != nil {
		return nil, fmt.Errorf("psql: list audit entries: %w", err)
	}
	defer rows.Close()

	entries := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.TargetType, &e.TargetID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("psql: scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate audit entries: %w", err)
	}
	return entries, nil
}

func (db *DB) GetAuditLog(ctx context.Context, targetType string, targetID string) ([]AuditEntry, error) {
	rows, err := db.conn.Query(ctx, `
    SELECT id, actor_id, action, target_type, target_id, metadata, created_at
    FROM audit_log
    WHERE target_type = $1 AND target_id = $2
    ORDER BY created_at DESC
  `, targetType, targetID)
	if err != nil {
		return nil, fmt.Errorf("psql: get audit log: %w", err)
	}
	defer rows.Close()

	entries := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID, &e.ActorID, &e.Action, &e.TargetType, &e.TargetID, &e.Metadata, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("psql: scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}

	return entries, nil
}
