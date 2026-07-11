package cnpj

import (
	"context"
	"testing"
)

// fakeStore implements cnpjStore for skip-path tests. mg stays nil: a correct
// tombstone skip must return before any memgraph call, so a nil *memgraph.DB
// would panic if the guard failed to short-circuit.
type fakeStore struct {
	purged  bool
	reviews int
}

func (f *fakeStore) CreatePendingReview(context.Context, string, []byte, string) error {
	f.reviews++
	return nil
}
func (f *fakeStore) IsSubjectPurged(context.Context, ...string) (bool, error) {
	return f.purged, nil
}

// TestHandleIndividualSkipsTombstoned covers Fix 1: a purged QSA individual (no
// politician match) must not resurrect a Person node. An empty document skips the
// masked-CPF match block so mg is never consulted before the tombstone guard.
func TestHandleIndividualSkipsTombstoned(t *testing.T) {
	w := &Worker{pg: &fakeStore{purged: true}}
	stats := &RunStats{}
	entry := QSAEntry{NomeSocio: "JOAO DA SILVA", CNPJCPFDoSocio: ""}
	if err := w.handleIndividual(context.Background(), Options{}, "org_1", "12345678000195", "http://x", entry, stats); err != nil {
		t.Fatalf("handleIndividual: %v", err)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.PersonsLinked != 0 {
		t.Fatalf("purged individual must not link a Person, got %d", stats.PersonsLinked)
	}
}

// TestHandleCompanySkipsTombstoned covers Fix 1 for the QSA organization path.
func TestHandleCompanySkipsTombstoned(t *testing.T) {
	w := &Worker{pg: &fakeStore{purged: true}}
	stats := &RunStats{}
	parent := job{orgID: "org_1", cnpj: "12345678000195", depth: 0}
	entry := QSAEntry{NomeSocio: "PARTNER LTDA", CNPJCPFDoSocio: "33.683.111/0002-80"}
	child, err := w.handleCompany(context.Background(), Options{}, parent, "org_1", "http://x", entry, stats)
	if err != nil {
		t.Fatalf("handleCompany: %v", err)
	}
	if child != nil {
		t.Fatalf("purged partner must not be queued for enrichment, got %+v", child)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.OrgsChained != 0 {
		t.Fatalf("purged partner must not create an Org, got %d", stats.OrgsChained)
	}
}
