package psql

import (
	"context"
	"fmt"
)

func (db *DB) IsMemgraphMigrationApplied(ctx context.Context, filename string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx,
		`SELECT EXISTS (
      SELECT 1 FROM schema_migrations WHERE target = 'memgraph' AND filename = $1
    )`, filename,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: check memgraph migration %s: %w", filename, err)
	}
	return exists, nil
}

func (db *DB) RecordMemgraphMigration(ctx context.Context, filename string) error {
	_, err := db.conn.Exec(ctx,
		`INSERT INTO schema_migrations (target, filename) VALUES ('memgraph', $1)`,
		filename,
	)
	if err != nil {
		return fmt.Errorf("psql: record memgraph migration %s: %w", filename, err)
	}
	return nil
}
