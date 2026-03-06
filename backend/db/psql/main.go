package psql

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	conn *pgxpool.Pool
	log  *slog.Logger
}

func New(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	conn, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("psql: new pool: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("psql: ping: %w", err)
	}

	db := &DB{conn: conn, log: log}

	if err := db.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	if err := db.runMigrations(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() {
	db.conn.Close()
}

// ensureMigrationsTable creates the schema_migrations table if it does not
// exist. This runs unconditionally before the migration runner.
func (db *DB) ensureMigrationsTable(ctx context.Context) error {
	_, err := db.conn.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS schema_migrations (
      id         SERIAL PRIMARY KEY,
      target     TEXT NOT NULL CHECK (target IN ('psql', 'memgraph')),
      filename   TEXT NOT NULL,
      applied_at TIMESTAMPTZ DEFAULT now(),
      UNIQUE (target, filename)
    )
  `)
	if err != nil {
		return fmt.Errorf("psql: ensure migrations table: %w", err)
	}
	return nil
}

func (db *DB) runMigrations(ctx context.Context) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("psql: read migrations dir: %w", err)
	}

	files := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		applied, err := db.isMigrationApplied(ctx, filename)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		content, err := migrations.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("psql: read migration %s: %w", filename, err)
		}

		if err := db.applyMigration(ctx, filename, string(content)); err != nil {
			return err
		}

		db.log.Info("psql: applied migration", "file", filename)
	}

	return nil
}

func (db *DB) isMigrationApplied(ctx context.Context, filename string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow(ctx,
		`SELECT EXISTS (
      SELECT 1 FROM schema_migrations WHERE target = 'psql' AND filename = $1
    )`, filename,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("psql: check migration %s: %w", filename, err)
	}
	return exists, nil
}

func (db *DB) applyMigration(ctx context.Context, filename, content string) error {
	tx, err := db.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("psql: begin tx for %s: %w", filename, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, content); err != nil {
		return fmt.Errorf("psql: exec migration %s: %w", filename, err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (target, filename) VALUES ('psql', $1)`,
		filename,
	); err != nil {
		return fmt.Errorf("psql: record migration %s: %w", filename, err)
	}

	return tx.Commit(ctx)
}
