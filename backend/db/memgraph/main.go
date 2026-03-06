package memgraph

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

//go:embed migrations/*.cypher
var migrations embed.FS

type DB struct {
	driver neo4j.DriverWithContext
	psql   migrationTracker
	log    *slog.Logger
}

// migrationTracker is a minimal interface so memgraph can record
// its migrations into the shared schema_migrations table in Postgres
// without importing the full psql package.
type migrationTracker interface {
	IsMemgraphMigrationApplied(ctx context.Context, filename string) (bool, error)
	RecordMemgraphMigration(ctx context.Context, filename string) error
}

func New(ctx context.Context, uri, user, pass string, psql migrationTracker, log *slog.Logger) (*DB, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		return nil, fmt.Errorf("memgraph: new driver: %w", err)
	}

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, fmt.Errorf("memgraph: connectivity: %w", err)
	}

	db := &DB{driver: driver, psql: psql, log: log}

	if err := db.runMigrations(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) Close(ctx context.Context) error {
	return db.driver.Close(ctx)
}

func (db *DB) runMigrations(ctx context.Context) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("memgraph: read migrations dir: %w", err)
	}

	files := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cypher") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		applied, err := db.psql.IsMemgraphMigrationApplied(ctx, filename)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		content, err := migrations.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("memgraph: read migration %s: %w", filename, err)
		}

		if err := db.applyMigration(ctx, filename, string(content)); err != nil {
			return err
		}

		db.log.Info("memgraph: applied migration", "file", filename)
	}

	return nil
}

func (db *DB) applyMigration(ctx context.Context, filename, content string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	// each statement in the file runs separately
	statements := splitStatements(content)

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := session.Run(ctx, stmt, nil); err != nil {
			return fmt.Errorf("memgraph: exec migration %s: %w", filename, err)
		}
	}

	return db.psql.RecordMemgraphMigration(ctx, filename)
}

// splitStatements parses a .cypher file into individual executable statements.
// Rules:
//   - // line comments are stripped
//   - statements terminated by ; are flushed immediately
//   - closing ); or bare ) ends a CALL block
//   - blank and whitespace-only results are dropped
func splitStatements(content string) []string {
	var statements []string
	var current strings.Builder

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)

		// skip full-line comments and blank lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// strip inline trailing comment
		if idx := strings.Index(trimmed, "//"); idx != -1 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}

		if trimmed == "" {
			continue
		}

		current.WriteString(trimmed)
		current.WriteString(" ")

		// flush on semicolon terminator or closing paren of CALL block
		if strings.HasSuffix(trimmed, ";") || trimmed == ");" || trimmed == ")" {
			stmt := strings.TrimSuffix(strings.TrimSpace(current.String()), ";")
			stmt = strings.TrimSpace(stmt)
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	// flush anything remaining without a terminator
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

