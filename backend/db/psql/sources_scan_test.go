package psql

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"corruption-center/api/models"
	"github.com/jackc/pgx/v5"
)

// fakeRow implements pgx.Row for exercising scanSource without a live DB. It
// either assigns the prepared values into the scan destinations (in order) or
// returns a fixed error.
type fakeRow struct {
	values []any
	err    error
}

func (f fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(f.values[i]))
	}
	return nil
}

func TestScanSource_Success(t *testing.T) {
	published := time.Date(2022, 1, 2, 0, 0, 0, 0, time.UTC)
	scraped := time.Date(2022, 3, 4, 0, 0, 0, 0, time.UTC)
	row := fakeRow{values: []any{
		"src-1",                        // ID
		"https://example.com",          // URL
		"A Title",                      // Title
		"A Publisher",                  // Publisher
		models.SourceTypeNewsOutlet,    // Type
		models.ReliabilityHigh,         // Reliability
		&published,                     // datePublished (*time.Time)
		scraped,                        // DateScraped
		true,                           // Active
	}}

	got, err := scanSource(row)
	if err != nil {
		t.Fatalf("scanSource: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a source")
	}
	if got.ID != "src-1" || got.URL != "https://example.com" || got.Title != "A Title" {
		t.Fatalf("unexpected scalar fields: %+v", got)
	}
	if got.Type != models.SourceTypeNewsOutlet || got.Reliability != models.ReliabilityHigh {
		t.Fatalf("unexpected type/reliability: %+v", got)
	}
	if got.DatePublished == nil || !got.DatePublished.Equal(published) {
		t.Fatalf("DatePublished = %v, want %v", got.DatePublished, published)
	}
	if !got.Active {
		t.Fatalf("expected Active true")
	}
}

func TestScanSource_NoRows(t *testing.T) {
	got, err := scanSource(fakeRow{err: pgx.ErrNoRows})
	if err != nil {
		t.Fatalf("ErrNoRows should map to (nil, nil), got err %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil source for no rows, got %+v", got)
	}
}

func TestScanSource_ScanError(t *testing.T) {
	sentinel := errors.New("boom")
	got, err := scanSource(fakeRow{err: sentinel})
	if err == nil {
		t.Fatalf("expected wrapped scan error")
	}
	if got != nil {
		t.Fatalf("expected nil source on error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel, got %v", err)
	}
}

// TestScanSource_NilDatePublished ensures a nil publication date is preserved.
func TestScanSource_NilDatePublished(t *testing.T) {
	var nilTime *time.Time
	row := fakeRow{values: []any{
		"src-2", "u", "t", "p",
		models.SourceTypeAcademic, models.ReliabilityLow,
		nilTime, time.Now(), false,
	}}
	got, err := scanSource(row)
	if err != nil {
		t.Fatalf("scanSource: %v", err)
	}
	if got.DatePublished != nil {
		t.Fatalf("expected nil DatePublished, got %v", got.DatePublished)
	}
}
