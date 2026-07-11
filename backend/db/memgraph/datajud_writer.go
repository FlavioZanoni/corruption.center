package memgraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type DataJudProceedingUpsert struct {
	CaseNumber string
	Court      string
	Type       string
	Status     string
	Assuntos   []string
	DateFiled  *time.Time
}

func (db *DB) UpsertLegalProceedingByCase(ctx context.Context, p DataJudProceedingUpsert) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Empty fields mean "unknown", not "clear it". Case registration (backoffice
	// seed form, baseline seed) only ensures the node exists and has no facts to
	// offer, so it must not reset court/status the watcher already derived; this
	// upsert runs on every API boot for the baseline cases.
	query := `
MERGE (lp:LegalProceeding {case_number: $case_number})
ON CREATE SET lp.id = $id, lp.status = 'ongoing'
SET
  lp.court = CASE WHEN $court = '' THEN lp.court ELSE $court END,
  lp.type = CASE WHEN $type = '' THEN lp.type ELSE $type END,
  lp.status = CASE WHEN $status = '' THEN lp.status ELSE $status END,
  lp.assuntos = CASE WHEN $assuntos IS NULL OR size($assuntos) = 0 THEN lp.assuntos ELSE $assuntos END,
  lp.date_filed = CASE WHEN $date_filed IS NULL THEN lp.date_filed ELSE $date_filed END
RETURN lp.id AS id
`

	dateFiled := any(nil)
	if p.DateFiled != nil {
		dateFiled = p.DateFiled.Format("2006-01-02")
	}

	res, err := session.Run(ctx, query, map[string]any{
		"id":          legalProceedingID(p.CaseNumber),
		"case_number": p.CaseNumber,
		"court":       p.Court,
		"type":        p.Type,
		"status":      p.Status,
		"assuntos":    p.Assuntos,
		"date_filed":  dateFiled,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: upsert legal proceeding: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", fmt.Errorf("memgraph: upsert legal proceeding result: %w", err)
		}
		return "", fmt.Errorf("memgraph: upsert legal proceeding returned no rows")
	}
	idVal, _ := res.Record().Get("id")
	id, _ := idVal.(string)
	return id, nil
}

func (db *DB) EnsureInvestigatesEdge(ctx context.Context, proceedingID, scandalID string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.Run(ctx, `
MATCH (lp:LegalProceeding {id: $proceeding_id})
MATCH (s:Scandal {id: $scandal_id})
MERGE (lp)-[:INVESTIGATES]->(s)
`, map[string]any{"proceeding_id": proceedingID, "scandal_id": scandalID})
	if err != nil {
		return fmt.Errorf("memgraph: ensure investigates edge: %w", err)
	}
	return nil
}

// EnsureDefendantInEdge links a Person/Organization node to a LegalProceeding
// with a DEFENDANT_IN edge carrying an outcome. The DataJud watcher no longer
// discovers parties (the public API exposes none); this is retained for the
// DJEN worker, which owns party discovery.
func (db *DB) EnsureDefendantInEdge(ctx context.Context, nodeType, nodeID, proceedingID, outcome string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	query := fmt.Sprintf(`
MATCH (n:%s {id: $node_id})
MATCH (lp:LegalProceeding {id: $proceeding_id})
MERGE (n)-[r:DEFENDANT_IN]->(lp)
SET r.outcome = $outcome
`, nodeType)
	_, err := session.Run(ctx, query, map[string]any{"node_id": nodeID, "proceeding_id": proceedingID, "outcome": outcome})
	if err != nil {
		return fmt.Errorf("memgraph: ensure defendant edge: %w", err)
	}
	return nil
}

// UpdateProceedingCaseState applies the case-level movement state machine onto
// a LegalProceeding. phase (when non-empty) sets lp.phase; hasConviction is the
// value recomputed from the full movement history each poll and is written
// verbatim (NOT OR-latched) so that a conviction later reversed on appeal, an
// explicit Absolvição after a Condenação, clears lp.has_conviction rather than
// leaving a stale (defamation-grade) true; concluded sets lp.status =
// "concluded". All flags are case-level; per-defendant outcomes are set only
// via backoffice review (see docs/workerDetails/DATAJUD.md).
func (db *DB) UpdateProceedingCaseState(ctx context.Context, proceedingID, phase string, hasConviction, concluded bool) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.Run(ctx, `
MATCH (lp:LegalProceeding {id: $id})
SET
  lp.phase = CASE WHEN $phase <> '' THEN $phase ELSE lp.phase END,
  lp.has_conviction = $has_conviction,
  lp.status = CASE WHEN $concluded THEN 'concluded' ELSE lp.status END
`, map[string]any{
		"id":             proceedingID,
		"phase":          phase,
		"has_conviction": hasConviction,
		"concluded":      concluded,
	})
	if err != nil {
		return fmt.Errorf("memgraph: update proceeding case state: %w", err)
	}
	return nil
}

func legalProceedingID(caseNumber string) string {
	clean := strings.TrimSpace(caseNumber)
	return "lp_" + strings.ReplaceAll(strings.ReplaceAll(clean, ".", ""), "-", "")
}

// digitsOnly strips everything but ASCII digits. Package-level helper shared by
// other memgraph writers (e.g. sanctions_writer.go).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
