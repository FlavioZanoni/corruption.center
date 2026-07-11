package sanctions

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// fakeStore implements sanctionsStore for skip-path tests. mg stays nil in these
// tests: a correct skip must return BEFORE any memgraph call, so a nil *memgraph.DB
// would panic if the guard failed to short-circuit.
type fakeStore struct {
	purged      bool
	hasReview   bool
	reviews     int // CreatePendingReview calls
	purgedCalls int // IsSubjectPurged calls
}

func (f *fakeStore) UpsertSanctionImportState(context.Context, string, int, int, time.Time) error {
	return nil
}
func (f *fakeStore) CreatePendingReview(context.Context, string, []byte, string) error {
	f.reviews++
	return nil
}
func (f *fakeStore) IsSubjectPurged(_ context.Context, _ ...string) (bool, error) {
	f.purgedCalls++
	return f.purged, nil
}
func (f *fakeStore) HasSanctionReview(context.Context, string, string) (bool, error) {
	return f.hasReview, nil
}

// TestPlanRegistries covers Fix 3: keyless CGU must not abort the run before the
// keyless TCU lists get a chance to run, while an explicit CGU-only selection with
// no key stays a hard error.
func TestPlanRegistries(t *testing.T) {
	tests := []struct {
		name        string
		selected    []string
		hasKey      bool
		wantRun     []string
		wantSkipped []string
		wantErr     bool
	}{
		{"default no key skips cgu runs tcu", []string{"ceis", "cnep", "ceaf", "leniencia", "tcu"}, false,
			[]string{"tcu"}, []string{"ceis", "cnep", "ceaf", "leniencia"}, false},
		{"default with key runs all", []string{"ceis", "cnep", "ceaf", "leniencia", "tcu"}, true,
			[]string{"ceis", "cnep", "ceaf", "leniencia", "tcu"}, nil, false},
		{"cgu only no key is hard error", []string{"ceis"}, false, nil, nil, true},
		{"cgu only with key runs", []string{"ceis"}, true, []string{"ceis"}, nil, false},
		{"tcu only no key runs", []string{"tcu"}, false, []string{"tcu"}, nil, false},
		{"cgu plus tcu no key skips cgu", []string{"ceis", "tcu"}, false,
			[]string{"tcu"}, []string{"ceis"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run, skipped, err := planRegistries(tc.selected, tc.hasKey)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(run, tc.wantRun) {
				t.Errorf("run = %v, want %v", run, tc.wantRun)
			}
			if !reflect.DeepEqual(skipped, tc.wantSkipped) {
				t.Errorf("skipped = %v, want %v", skipped, tc.wantSkipped)
			}
		})
	}
}

// TestLinkFullCPFSkipsTombstoned covers Fix 1: a purged CPF/name must not
// resurrect a Person node. mg is nil, so reaching UpsertPersonByCPF would panic.
func TestLinkFullCPFSkipsTombstoned(t *testing.T) {
	fs := &fakeStore{purged: true}
	w := &Worker{pg: fs}
	stats := &Stats{PerRegistry: map[string]int{}}
	rec := SanctionRecord{CPF: "21580545300", Name: "JOAO DA SILVA"}
	if err := w.linkFullCPF(context.Background(), rec, "sanc_1", stats); err != nil {
		t.Fatalf("linkFullCPF: %v", err)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.PersonsCreated != 0 || stats.EdgesCreated != 0 {
		t.Fatalf("purged subject must not create nodes/edges: %+v", stats)
	}
	if fs.purgedCalls != 1 {
		t.Fatalf("expected exactly one tombstone check, got %d", fs.purgedCalls)
	}
}

// TestLinkCNPJSkipsTombstoned covers Fix 1 for the organization path.
func TestLinkCNPJSkipsTombstoned(t *testing.T) {
	fs := &fakeStore{purged: true}
	w := &Worker{pg: fs}
	stats := &Stats{PerRegistry: map[string]int{}}
	rec := SanctionRecord{CNPJ: "12345678000195", Name: "EMPRESA FANTASMA LTDA"}
	if err := w.linkCNPJ(context.Background(), rec, "sanc_1", stats); err != nil {
		t.Fatalf("linkCNPJ: %v", err)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.OrgsCreated != 0 || stats.EdgesCreated != 0 {
		t.Fatalf("purged subject must not create nodes/edges: %+v", stats)
	}
}

// TestQueueReviewDedup covers Fix 4: an existing (sanction_id, politician_id)
// review in any status suppresses a duplicate; a fresh pair files one.
func TestQueueReviewDedup(t *testing.T) {
	rec := SanctionRecord{Registry: RegistryCEIS, Name: "X", MaskedCPF: "456789"}

	dup := &fakeStore{hasReview: true}
	w := &Worker{pg: dup}
	created, err := w.queueReview(context.Background(), rec, "sanc_1", "pol_1", "Politician")
	if err != nil {
		t.Fatalf("queueReview: %v", err)
	}
	if created {
		t.Fatalf("expected no new review when one already exists")
	}
	if dup.reviews != 0 {
		t.Fatalf("CreatePendingReview must not be called on dedup, got %d", dup.reviews)
	}

	fresh := &fakeStore{hasReview: false}
	w = &Worker{pg: fresh}
	created, err = w.queueReview(context.Background(), rec, "sanc_1", "pol_1", "Politician")
	if err != nil {
		t.Fatalf("queueReview: %v", err)
	}
	if !created || fresh.reviews != 1 {
		t.Fatalf("expected a new review filed, created=%v reviews=%d", created, fresh.reviews)
	}
}
