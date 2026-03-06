package psql

import (
	"context"
	"fmt"
	"time"

	"corruption-center/api/models"
	"github.com/jackc/pgx/v5"
)

func (db *DB) UpsertSource(ctx context.Context, s models.Source, rawContent string, checksum string) error {
	_, err := db.conn.Exec(ctx, `
    INSERT INTO sources (
      id, url, title, publisher, type, reliability,
      date_published, date_scraped, raw_content, checksum, active
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8, $9, true)
    ON CONFLICT (url) DO UPDATE SET
      title          = EXCLUDED.title,
      reliability    = EXCLUDED.reliability,
      date_scraped   = now(),
      raw_content    = EXCLUDED.raw_content,
      checksum       = EXCLUDED.checksum,
      active         = true
  `,
		s.ID, s.URL, s.Title, s.Publisher, s.Type, s.Reliability,
		s.DatePublished, rawContent, checksum,
	)
	if err != nil {
		return fmt.Errorf("psql: upsert source: %w", err)
	}
	return nil
}

func (db *DB) GetSource(ctx context.Context, id string) (*models.Source, error) {
	row := db.conn.QueryRow(ctx, `
    SELECT id, url, title, publisher, type, reliability,
           date_published, date_scraped, active
    FROM sources
    WHERE id = $1
  `, id)

	return scanSource(row)
}

func (db *DB) GetSourceByURL(ctx context.Context, url string) (*models.Source, error) {
	row := db.conn.QueryRow(ctx, `
    SELECT id, url, title, publisher, type, reliability,
           date_published, date_scraped, active
    FROM sources
    WHERE url = $1
  `, url)

	return scanSource(row)
}

// GetSourceChecksum returns the stored checksum for a URL so the
// worker can skip re-processing if the content has not changed.
func (db *DB) GetSourceChecksum(ctx context.Context, url string) (string, error) {
	var checksum string
	err := db.conn.QueryRow(ctx,
		`SELECT COALESCE(checksum, '') FROM sources WHERE url = $1`, url,
	).Scan(&checksum)
	if err != nil {
		return "", fmt.Errorf("psql: get source checksum: %w", err)
	}
	return checksum, nil
}

func (db *DB) DeactivateSource(ctx context.Context, id string) error {
	_, err := db.conn.Exec(ctx,
		`UPDATE sources SET active = false WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("psql: deactivate source: %w", err)
	}
	return nil
}

func scanSource(row pgx.Row) (*models.Source, error) {
	var s models.Source
	var datePublished *time.Time

	err := row.Scan(
		&s.ID, &s.URL, &s.Title, &s.Publisher, &s.Type, &s.Reliability,
		&datePublished, &s.DateScraped, &s.Active,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("psql: scan source: %w", err)
	}

	s.DatePublished = datePublished
	return &s, nil
}
