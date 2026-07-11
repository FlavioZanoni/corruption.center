package djen

import (
	"context"
	"testing"
	"time"

	"corruption-center/db/psql"
)

// fakeStore implements djenStore for skip-path tests. mg stays nil: a correct
// tombstone skip must return before any memgraph call, so a nil *memgraph.DB
// would panic if the guard failed to short-circuit.
type fakeStore struct {
	purged        bool
	tracked       bool
	hasCandidate  bool
	hasPartyMatch bool
	reviews       int
	trackedArgs   []string
	candidateArgs []string
}

func (f *fakeStore) ListDjenCasesForPoll(context.Context) ([]psql.WatcherCase, error) {
	return nil, nil
}
func (f *fakeStore) ListDjenSnapshotKeys(context.Context, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (f *fakeStore) InsertDjenSnapshotParty(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeStore) UpdateDjenPolledAt(context.Context, string, time.Time) error { return nil }
func (f *fakeStore) CreatePendingReview(context.Context, string, []byte, string) error {
	f.reviews++
	return nil
}
func (f *fakeStore) IsCaseTracked(_ context.Context, caseNumber string) (bool, error) {
	f.trackedArgs = append(f.trackedArgs, caseNumber)
	return f.tracked, nil
}
func (f *fakeStore) HasDjenCaseCandidate(_ context.Context, caseNumber string) (bool, error) {
	f.candidateArgs = append(f.candidateArgs, caseNumber)
	return f.hasCandidate, nil
}
func (f *fakeStore) HasPartyMatchReview(_ context.Context, _, _ string) (bool, error) {
	return f.hasPartyMatch, nil
}
func (f *fakeStore) IsSubjectPurged(context.Context, ...string) (bool, error) {
	return f.purged, nil
}

// TestHandlePersonPartySkipsTombstoned covers Fix 1: a purged name must not
// resurrect a Person node.
func TestHandlePersonPartySkipsTombstoned(t *testing.T) {
	w := &Worker{pg: &fakeStore{purged: true}}
	stats := &RunStats{}
	c := psql.WatcherCase{ProceedingID: "proc_1"}
	party := Party{Nome: "JOAO DA SILVA", Polo: "P"}
	if err := w.handlePersonParty(context.Background(), Options{}, c, "JOAO DA SILVA", party, stats); err != nil {
		t.Fatalf("handlePersonParty: %v", err)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.PersonsLinked != 0 {
		t.Fatalf("purged name must not link a Person, got %d", stats.PersonsLinked)
	}
}

// TestHandleCompanyPartySkipsTombstoned covers Fix 1 for the organization path.
func TestHandleCompanyPartySkipsTombstoned(t *testing.T) {
	w := &Worker{pg: &fakeStore{purged: true}}
	stats := &RunStats{}
	c := psql.WatcherCase{ProceedingID: "proc_1"}
	party := Party{Nome: "CONSTRUTORA X LTDA", Polo: "P"}
	if err := w.handleCompanyParty(context.Background(), Options{}, c, "20000000020235010432", "CONSTRUTORA X LTDA", party, stats); err != nil {
		t.Fatalf("handleCompanyParty: %v", err)
	}
	if stats.SkippedTombstoned != 1 {
		t.Fatalf("SkippedTombstoned = %d, want 1", stats.SkippedTombstoned)
	}
	if stats.OrgsLinked != 0 || stats.PendingReviews != 0 {
		t.Fatalf("purged name must not link an Org or file a review: %+v", stats)
	}
}

// TestHandlePersonPartyDryRunLinks confirms the dry-run branch still counts the
// person without consulting the tombstone store or memgraph.
func TestHandlePersonPartyDryRunLinks(t *testing.T) {
	w := &Worker{pg: &fakeStore{purged: true}}
	stats := &RunStats{}
	c := psql.WatcherCase{ProceedingID: "proc_1"}
	party := Party{Nome: "JOAO DA SILVA", Polo: "P"}
	if err := w.handlePersonParty(context.Background(), Options{DryRun: true}, c, "JOAO DA SILVA", party, stats); err != nil {
		t.Fatalf("handlePersonParty dry-run: %v", err)
	}
	if stats.PersonsLinked != 1 || stats.SkippedTombstoned != 0 {
		t.Fatalf("dry-run should count a person without a tombstone check: %+v", stats)
	}
}
