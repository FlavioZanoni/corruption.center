package psql

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RemovalRequest is a single LGPD data-removal request tracked in the backoffice
// queue. It records who asked, what node/edge is targeted, and how it was
// resolved (including whether a purge was performed or refused).
type RemovalRequest struct {
	ID         string
	Requester  string
	TargetType string
	TargetID   string
	Reason     string
	Status     string
	Resolution string
	ReceivedAt time.Time
	ResolvedAt *time.Time
	ResolvedBy *string
}

// CreateRemovalRequest inserts a new pending removal request and returns its id.
func (db *DB) CreateRemovalRequest(ctx context.Context, requester, targetType, targetID, reason string) (string, error) {
	requester = strings.TrimSpace(requester)
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	if requester == "" || targetType == "" || targetID == "" {
		return "", fmt.Errorf("psql: create removal request: requester, target_type and target_id are required")
	}
	var id string
	err := db.conn.QueryRow(ctx, `
    INSERT INTO removal_request (requester, target_type, target_id, reason)
    VALUES ($1, $2, $3, $4)
    RETURNING id::text
  `, requester, targetType, targetID, strings.TrimSpace(reason)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("psql: create removal request: %w", err)
	}
	return id, nil
}

// ListRemovalRequests returns removal requests, optionally filtered by status,
// newest first.
func (db *DB) ListRemovalRequests(ctx context.Context, status string, limit int) ([]RemovalRequest, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)

	rows, err := db.conn.Query(ctx, `
    SELECT id::text, requester, target_type, target_id,
           coalesce(reason, ''), status, coalesce(resolution, ''),
           received_at, resolved_at, resolved_by
    FROM removal_request
    WHERE ($1 = '' OR status = $1)
    ORDER BY received_at DESC
    LIMIT $2
  `, status, limit)
	if err != nil {
		return nil, fmt.Errorf("psql: list removal requests: %w", err)
	}
	defer rows.Close()

	out := make([]RemovalRequest, 0, limit)
	for rows.Next() {
		var r RemovalRequest
		if err := rows.Scan(&r.ID, &r.Requester, &r.TargetType, &r.TargetID,
			&r.Reason, &r.Status, &r.Resolution, &r.ReceivedAt, &r.ResolvedAt, &r.ResolvedBy); err != nil {
			return nil, fmt.Errorf("psql: scan removal request: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("psql: iterate removal requests: %w", err)
	}
	return out, nil
}

// GetRemovalRequest fetches a single removal request by id.
func (db *DB) GetRemovalRequest(ctx context.Context, id string) (RemovalRequest, error) {
	var r RemovalRequest
	err := db.conn.QueryRow(ctx, `
    SELECT id::text, requester, target_type, target_id,
           coalesce(reason, ''), status, coalesce(resolution, ''),
           received_at, resolved_at, resolved_by
    FROM removal_request
    WHERE id = $1::uuid
  `, id).Scan(&r.ID, &r.Requester, &r.TargetType, &r.TargetID,
		&r.Reason, &r.Status, &r.Resolution, &r.ReceivedAt, &r.ResolvedAt, &r.ResolvedBy)
	if err != nil {
		return RemovalRequest{}, fmt.Errorf("psql: get removal request: %w", err)
	}
	return r, nil
}

// ResolveRemovalRequest closes a removal request with a terminal status
// ('resolved' or 'rejected'), a resolution note and the operator identity. It
// only transitions requests that are still 'pending'; resolving an already
// terminal request is an error so the audit trail stays truthful.
func (db *DB) ResolveRemovalRequest(ctx context.Context, id, status, resolution, resolvedBy string) error {
	status = strings.TrimSpace(status)
	switch status {
	case "resolved", "rejected":
	default:
		return fmt.Errorf("psql: invalid removal request resolution status: %s", status)
	}
	if strings.TrimSpace(resolvedBy) == "" {
		resolvedBy = "backoffice"
	}

	tag, err := db.conn.Exec(ctx, `
    UPDATE removal_request
    SET status = $2,
        resolution = $3,
        resolved_by = $4,
        resolved_at = now()
    WHERE id = $1::uuid AND status = 'pending'
  `, id, status, strings.TrimSpace(resolution), resolvedBy)
	if err != nil {
		return fmt.Errorf("psql: resolve removal request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("psql: removal request %s is not pending (already resolved or not found)", id)
	}
	return nil
}
