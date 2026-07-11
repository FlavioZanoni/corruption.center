package memgraph

import (
	"testing"
	"time"

	"corruption-center/api/models"
)

func TestClampPaging(t *testing.T) {
	cases := []struct {
		name                         string
		page, pageSize, def          int
		wantPage, wantSize, wantSkip int
	}{
		{"defaults", 0, 0, 24, 1, 24, 0},
		{"negative page floors to 1", -3, 10, 24, 1, 10, 0},
		{"second page skips a full page", 2, 10, 24, 2, 10, 10},
		{"zero size takes the default", 3, 0, 24, 3, 24, 48},
		{"oversized page caps at 100", 1, 5000, 24, 1, 100, 0},
		{"cap applies to the skip too", 2, 5000, 24, 2, 100, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, size, skip := clampPaging(tc.page, tc.pageSize, tc.def)
			if page != tc.wantPage || size != tc.wantSize || skip != tc.wantSkip {
				t.Fatalf("clampPaging(%d, %d, %d) = (%d, %d, %d), want (%d, %d, %d)",
					tc.page, tc.pageSize, tc.def, page, size, skip, tc.wantPage, tc.wantSize, tc.wantSkip)
			}
		})
	}
}

// The ORDER BY clause is string-interpolated into the query, so anything outside
// the allowlist must collapse to the default rather than reach Cypher.
func TestScandalOrderBy(t *testing.T) {
	cases := []struct {
		name string
		sort string
		want string
	}{
		{"name", "name", "s.name"},
		{"date", "date", "s.date_start DESC, s.name"},
		{"empty falls back", "", "s.date_start DESC, s.name"},
		{"unknown falls back", "bogus", "s.date_start DESC, s.name"},
		{"injection falls back", "s.name; MATCH (n) DETACH DELETE n", "s.date_start DESC, s.name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scandalOrderBy(tc.sort); got != tc.want {
				t.Fatalf("scandalOrderBy(%q) = %q, want %q", tc.sort, got, tc.want)
			}
		})
	}
}

func TestScandalListItemFromProps(t *testing.T) {
	item := scandalListItemFromProps(map[string]any{
		"id":          "sc_1",
		"name":        "Mensalão",
		"description": "Vote-buying scheme",
		"date_start":  "2005-06-06",
		"date_end":    "2012-12-17",
		"status":      "concluded",
	}, 12, 4)

	if item.ID != "sc_1" || item.Name != "Mensalão" {
		t.Fatalf("identity not mapped: %+v", item)
	}
	if item.Description != "Vote-buying scheme" {
		t.Fatalf("description = %q", item.Description)
	}
	if want := time.Date(2005, 6, 6, 0, 0, 0, 0, time.UTC); !item.DateStart.Equal(want) {
		t.Fatalf("date_start = %v, want %v", item.DateStart, want)
	}
	if item.DateEnd == nil {
		t.Fatal("date_end = nil, want 2012-12-17")
	}
	if want := time.Date(2012, 12, 17, 0, 0, 0, 0, time.UTC); !item.DateEnd.Equal(want) {
		t.Fatalf("date_end = %v, want %v", *item.DateEnd, want)
	}
	if item.Status != models.StatusTypeConcluded {
		t.Fatalf("status = %q", item.Status)
	}
	if item.PoliticianCount != 12 || item.ProceedingCount != 4 {
		t.Fatalf("counts = (%d, %d), want (12, 4)", item.PoliticianCount, item.ProceedingCount)
	}
}

// An ongoing scandal has no end date, and a bare seed may have no dates at all:
// neither may blow up or invent a value.
func TestScandalListItemFromPropsSparse(t *testing.T) {
	item := scandalListItemFromProps(map[string]any{"id": "sc_2", "name": "Ongoing"}, 0, 0)

	if item.DateEnd != nil {
		t.Fatalf("date_end = %v, want nil", *item.DateEnd)
	}
	if !item.DateStart.IsZero() {
		t.Fatalf("date_start = %v, want zero", item.DateStart)
	}
	if item.Status != "" || item.Description != "" {
		t.Fatalf("expected empty status/description, got %+v", item)
	}
}
