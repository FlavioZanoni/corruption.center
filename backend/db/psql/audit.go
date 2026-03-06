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
