package memgraph

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Retraction: unpublishing a record that vanished from its official source.
//
// Every write path in this codebase was MERGE-only. Nothing could ever
// un-publish. That is fine while a source only ever ADDS — and no source only
// ever adds. CGU delists a sanction when a court annuls it, when a debarment
// expires, or when it was entered in error. We kept publishing it anyway, and
// would have gone on doing so forever: 64,779 sanctions, append-only, about
// named people and companies.
//
// A sanction the state has withdrawn, still published by us, is exactly the
// defamation this project exists to avoid. The absence of a record at the source
// IS a fact; we were the only ones not listening for it.
//
// HOW IT WORKS. Every upsert stamps the id of the sync run that saw the record.
// After a run finishes cleanly, anything of that source the run did NOT see is
// retracted.
//
// Retraction REMOVES THE :Sanction LABEL and adds :RetractedSanction. It does
// not delete, and it does not set a flag.
//
//   - Not a delete, because the node and its edges are the audit trail, and a
//     record can come back (a suspension reinstated on appeal).
//   - Not a flag, because a flag must be checked, and there are eight separate
//     queries that surface a Sanction to the public — with a ninth arriving the
//     day someone adds an endpoint. A `WHERE retracted_at IS NULL` that someone
//     forgets is a republished falsehood. Dropping the label means every query
//     that matches (:Sanction) excludes retracted records for free, including
//     the ones not written yet. The safe thing happens by default; you would
//     have to go out of your way to publish a retracted record.
//
// THE SWEEP IS THE MOST DESTRUCTIVE OPERATION IN THIS SYSTEM. It is the only
// code that unpublishes in bulk, and it fires on the ABSENCE of data — which is
// also what a truncated response, a silently changed API, an auth failure
// returning an empty 200, and a half-finished run all look like. The guards
// below are not paranoia; they are the whole safety design.

// RetractionGuard bounds what a single sweep may unpublish.
type RetractionGuard struct {
	// MaxFraction: refuse if the sweep would retract more than this share of the
	// source. Real retraction is a trickle — courts annul a handful of sanctions;
	// they do not annul 30% of CEIS overnight. A large proposed retraction is
	// therefore evidence about OUR fetch, not about the world, and the right
	// response is to refuse and shout.
	MaxFraction float64

	// MinRecords: never sweep a source we hold fewer than this many records for.
	// With a tiny denominator the fraction guard is meaningless (1 missing of 3
	// is 33%).
	MinRecords int
}

// DefaultRetractionGuard is deliberately tight. Loosen it with evidence, never
// with a hunch.
var DefaultRetractionGuard = RetractionGuard{MaxFraction: 0.05, MinRecords: 100}

// SweepResult reports what a sweep did, or refused to do.
type SweepResult struct {
	Source    string
	Held      int // records held for this source (published + already retracted)
	Seen      int // records this run saw
	Missing   int // published but unseen — the retraction candidates
	Retracted int // actually retracted (0 when refused)
	Refused   bool
	Reason    string
}

// SweepRetractedSanctions retracts the sanctions of one registry that the given
// run did not see, subject to the guard.
//
// Call it ONLY after that registry finished with no fetch errors. A run that
// gave up early saw fewer records for reasons that have nothing to do with the
// world, and its silence must never be read as a retraction.
func (db *DB) SweepRetractedSanctions(ctx context.Context, registry, runID string, guard RetractionGuard) (SweepResult, error) {
	res := SweepResult{Source: registry}
	if registry == "" || runID == "" {
		return res, fmt.Errorf("memgraph: sweep needs a registry and a run id")
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	params := map[string]any{"registry": registry, "run": runID}

	// Count first, decide second, write third. Never "delete and see".
	// Held spans both labels: a record retracted last week is still one we hold,
	// and excluding it would inflate the missing-fraction on every later sweep.
	counts, err := session.Run(ctx, `
MATCH (s:SanctionRecord {registry: $registry})
RETURN count(s) AS held,
       sum(CASE WHEN s.last_seen_run = $run THEN 1 ELSE 0 END) AS seen,
       sum(CASE WHEN s:Sanction AND s.last_seen_run <> $run THEN 1 ELSE 0 END) AS missing
`, params)
	if err != nil {
		return res, fmt.Errorf("memgraph: sweep count: %w", err)
	}
	if counts.Next(ctx) {
		rec := counts.Record()
		res.Held = int(recInt(rec, "held"))
		res.Seen = int(recInt(rec, "seen"))
		res.Missing = int(recInt(rec, "missing"))
	}
	if err := counts.Err(); err != nil {
		return res, fmt.Errorf("memgraph: sweep count rows: %w", err)
	}

	if res.Held < guard.MinRecords {
		res.Refused = true
		res.Reason = fmt.Sprintf("hold only %d records for %s (min %d): too few for an absence to mean anything",
			res.Held, registry, guard.MinRecords)
		slog.Warn("memgraph: retraction sweep refused", "registry", registry, "reason", res.Reason)
		return res, nil
	}

	// The guard that matters: a truncated fetch and a mass annulment are the same
	// shape, and only one of them is real.
	if frac := float64(res.Missing) / float64(res.Held); frac > guard.MaxFraction {
		res.Refused = true
		res.Reason = fmt.Sprintf("would retract %d of %d records (%.1f%%) for %s, over the %.1f%% ceiling — "+
			"that is evidence about our fetch, not about the world; refusing, everything stays published",
			res.Missing, res.Held, frac*100, registry, guard.MaxFraction*100)
		slog.Error("memgraph: retraction sweep REFUSED", "registry", registry, "reason", res.Reason)
		return res, nil
	}

	// No restore pass here: an upsert already republished (SET s:Sanction) anything
	// the source started reporting again, before this sweep ever ran.
	params["at"] = time.Now().UTC().Format(time.RFC3339)

	// Retract: drop the label, and with it every public read path at once.
	out, err := session.Run(ctx, `
MATCH (s:Sanction {registry: $registry})
WHERE s.last_seen_run <> $run
REMOVE s:Sanction
SET s:RetractedSanction,
    s.retracted_at = $at,
    s.retraction_reason = 'absent_from_source'
RETURN count(s) AS n
`, params)
	if err != nil {
		return res, fmt.Errorf("memgraph: sweep retract: %w", err)
	}
	if out.Next(ctx) {
		res.Retracted = int(recInt(out.Record(), "n"))
	}
	if err := out.Err(); err != nil {
		return res, fmt.Errorf("memgraph: sweep retract rows: %w", err)
	}

	slog.Info("memgraph: retraction sweep",
		"registry", registry, "held", res.Held, "seen", res.Seen, "retracted", res.Retracted)
	return res, nil
}
