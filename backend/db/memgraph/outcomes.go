package memgraph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Per-defendant judicial outcomes: who was actually convicted, and who was only
// ever a defendant.
//
// This is the one claim in the database that no worker may ever make. A case that
// ends in a conviction says nothing about WHICH of its defendants was convicted,
// and no official source we use closes that gap: DataJud's public API exposes no
// parties at all (Portaria CNJ 160/2020), and DJEN publishes names with no
// outcome. So LegalProceeding.has_conviction is case-level and stays that way, and
// the per-person fact is entered by a human who has read the decision.
//
// The tempting shortcut — convict the defendant when the case has a conviction and
// we know of exactly one — is refuted by our own data: in Operação Calicute our
// roster holds exactly one defendant, ADRIANA DE LOURDES ANCELMO, and does not
// contain Sérgio Cabral at all, because DJEN only handed us the publications that
// named her. "Exactly one defendant" is a fact about our roster, never about the
// case. See docs/CONVICTIONS_AND_THE_TWO_ISLANDS.md.

// DefendantOutcome is the set of per-person outcomes an operator may record.
// Anything outside this set is rejected rather than written.
var DefendantOutcome = map[string]string{
	"convicted": "Condenado",
	"acquitted": "Absolvido",
	"dismissed": "Processo extinto / arquivado",
	"indicted":  "Denunciado (sem julgamento)",
	"cited":     "Apenas citado",
}

// ValidOutcome reports whether an operator-supplied outcome is one we accept.
func ValidOutcome(outcome string) bool {
	_, ok := DefendantOutcome[outcome]
	return ok
}

// ProceedingSummary is one case in the outcome queue.
type ProceedingSummary struct {
	ProceedingID  string
	CaseNumber    string
	ClassName     string
	Court         string
	ScandalID     string
	HasConviction bool
	Defendants    int
	Recorded      int // defendants whose outcome a human has recorded
}

// Defendant is one party to a case, with whatever outcome a human has recorded.
type Defendant struct {
	PartyID     string
	Label       string // "Politician" | "Person" | "Organization"
	Name        string
	Outcome     string
	EvidenceURL string
	RecordedBy  string
	RecordedAt  string
}

// ListProceedingsForOutcome returns the cases that have defendants, so an operator
// can work through them. Cases with no roster are omitted: there is nobody to
// record an outcome for.
func (db *DB) ListProceedingsForOutcome(ctx context.Context) ([]ProceedingSummary, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (x)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
OPTIONAL MATCH (lp)-[:INVESTIGATES]->(s:Scandal)
WITH lp, s,
     count(d) AS defendants,
     size([r IN collect(d) WHERE r.outcome_source = 'human']) AS recorded
RETURN lp.id AS proceeding_id, lp.case_number AS case_number,
       coalesce(lp.class_name, '') AS class_name, coalesce(lp.court, '') AS court,
       coalesce(s.id, '') AS scandal_id,
       coalesce(lp.has_conviction, false) AS has_conviction,
       defendants, recorded
ORDER BY recorded ASC, defendants DESC
`, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list proceedings for outcome: %w", err)
	}

	out := make([]ProceedingSummary, 0)
	for res.Next(ctx) {
		rec := res.Record()
		out = append(out, ProceedingSummary{
			ProceedingID:  recString(rec, "proceeding_id"),
			CaseNumber:    recString(rec, "case_number"),
			ClassName:     recString(rec, "class_name"),
			Court:         recString(rec, "court"),
			ScandalID:     recString(rec, "scandal_id"),
			HasConviction: recBool(rec, "has_conviction"),
			Defendants:    int(recInt(rec, "defendants")),
			Recorded:      int(recInt(rec, "recorded")),
		})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: list proceedings for outcome rows: %w", err)
	}
	return out, nil
}

// ListDefendants returns every party to a case, with the outcome recorded so far.
func (db *DB) ListDefendants(ctx context.Context, proceedingID string) ([]Defendant, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (x)-[d:DEFENDANT_IN]->(lp:LegalProceeding {id: $id})
RETURN x.id AS party_id, labels(x)[0] AS label, coalesce(x.name, '') AS name,
       coalesce(d.outcome, '') AS outcome,
       coalesce(d.outcome_evidence_url, '') AS evidence_url,
       coalesce(d.outcome_recorded_by, '') AS recorded_by,
       coalesce(d.outcome_recorded_at, '') AS recorded_at
ORDER BY name
`, map[string]any{"id": proceedingID})
	if err != nil {
		return nil, fmt.Errorf("memgraph: list defendants: %w", err)
	}

	out := make([]Defendant, 0)
	for res.Next(ctx) {
		rec := res.Record()
		out = append(out, Defendant{
			PartyID:     recString(rec, "party_id"),
			Label:       recString(rec, "label"),
			Name:        recString(rec, "name"),
			Outcome:     recString(rec, "outcome"),
			EvidenceURL: recString(rec, "evidence_url"),
			RecordedBy:  recString(rec, "recorded_by"),
			RecordedAt:  recString(rec, "recorded_at"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: list defendants rows: %w", err)
	}
	return out, nil
}

// SetDefendantOutcome records what the court decided for ONE party to ONE case.
//
// outcome_source is stamped 'human' on every write, and there is deliberately no
// code path that writes any other value: it is what lets a reader (and a lawyer)
// tell an operator's reading of a decision apart from a machine's guess. A
// conviction additionally requires an evidence URL — the decision itself — because
// an accusation of a crime with no source attached is the one thing this database
// must never publish.
func (db *DB) SetDefendantOutcome(ctx context.Context, proceedingID, partyID, outcome, evidenceURL, actor string) error {
	if !ValidOutcome(outcome) {
		return fmt.Errorf("memgraph: %q is not a recordable outcome", outcome)
	}
	if outcome == "convicted" && evidenceURL == "" {
		return fmt.Errorf("memgraph: recording a conviction requires an evidence URL (the decision)")
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (x {id: $party_id})-[d:DEFENDANT_IN]->(lp:LegalProceeding {id: $proceeding_id})
SET d.outcome = $outcome,
    d.outcome_source = 'human',
    d.outcome_evidence_url = $evidence_url,
    d.outcome_recorded_by = $actor,
    d.outcome_recorded_at = $at
RETURN d.outcome AS outcome
`, map[string]any{
		"party_id":      partyID,
		"proceeding_id": proceedingID,
		"outcome":       outcome,
		"evidence_url":  evidenceURL,
		"actor":         actor,
		"at":            time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("memgraph: set defendant outcome: %w", err)
	}
	// A MATCH that finds nothing would otherwise skip the SET in silence, leaving
	// the operator believing they recorded a conviction that was never written.
	if !res.Next(ctx) {
		return fmt.Errorf("memgraph: no DEFENDANT_IN edge from %s to %s", partyID, proceedingID)
	}
	return res.Err()
}

func recBool(rec *neo4j.Record, key string) bool {
	v, ok := rec.Get(key)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func recInt(rec *neo4j.Record, key string) int64 {
	v, ok := rec.Get(key)
	if !ok {
		return 0
	}
	return asInt(v)
}
