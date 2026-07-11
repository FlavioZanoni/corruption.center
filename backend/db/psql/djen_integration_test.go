//go:build integration

package psql

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
)

// TestDjenCaseNumberNormalization pins Fix 2: the DJEN dedup queries compare the
// API's bare 20-digit numero_processo against a case_number that may be stored in
// FORMATTED CNJ form. Both IsCaseTracked and HasDjenCaseCandidate must match after
// normalizing to digits-only.
func TestDjenCaseNumberNormalization(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := startPostgresContainer(t, ctx)
	defer cleanup()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	db, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("create psql db: %v", err)
	}
	defer db.Close()

	const formatted = "5046512-94.2016.4.04.7000"
	const digits = "50465129420164047000"

	// A backoffice-seeded watcher row stored in formatted CNJ form.
	if err := db.UpsertWatcherCase(ctx, formatted, "api_publica_trf4", "scandal_1", "proc_1", "tester"); err != nil {
		t.Fatalf("seed watcher case: %v", err)
	}

	tracked, err := db.IsCaseTracked(ctx, digits)
	if err != nil {
		t.Fatalf("IsCaseTracked: %v", err)
	}
	if !tracked {
		t.Fatalf("formatted seeded case not matched by bare 20-digit lookup")
	}

	// A never-seeded number must not match.
	other, err := db.IsCaseTracked(ctx, "99999999999999999999")
	if err != nil {
		t.Fatalf("IsCaseTracked (other): %v", err)
	}
	if other {
		t.Fatalf("unrelated case number should not be tracked")
	}

	// A prior candidate review whose payload holds the formatted case number.
	payload, _ := json.Marshal(map[string]any{"case_number": formatted})
	if err := db.CreatePendingReview(ctx, "djen_case_candidate", payload, "djen"); err != nil {
		t.Fatalf("seed candidate review: %v", err)
	}
	has, err := db.HasDjenCaseCandidate(ctx, digits)
	if err != nil {
		t.Fatalf("HasDjenCaseCandidate: %v", err)
	}
	if !has {
		t.Fatalf("formatted candidate payload not matched by bare 20-digit lookup")
	}
}

// TestHasSanctionReviewDedup pins Fix 4: HasSanctionReview matches on the
// (sanction_id, politician_id) pair regardless of review status.
func TestHasSanctionReviewDedup(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := startPostgresContainer(t, ctx)
	defer cleanup()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	db, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("create psql db: %v", err)
	}
	defer db.Close()

	payload, _ := json.Marshal(map[string]any{
		"sanction_id":   "CEIS:1001",
		"politician_id": "pol_1",
	})
	if err := db.CreatePendingReview(ctx, "possible_politician_sanction", payload, "sanctions_sync"); err != nil {
		t.Fatalf("seed sanction review: %v", err)
	}

	has, err := db.HasSanctionReview(ctx, "CEIS:1001", "pol_1")
	if err != nil {
		t.Fatalf("HasSanctionReview: %v", err)
	}
	if !has {
		t.Fatalf("existing (sanction_id, politician_id) review not detected")
	}

	// Different politician for the same sanction must NOT dedup.
	has, err = db.HasSanctionReview(ctx, "CEIS:1001", "pol_2")
	if err != nil {
		t.Fatalf("HasSanctionReview (other): %v", err)
	}
	if has {
		t.Fatalf("unrelated politician pair should not be deduped")
	}
}
