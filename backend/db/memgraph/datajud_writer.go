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
	Type       string // TPU class code, e.g. "283"
	ClassName  string // TPU class name, e.g. "Ação Penal - Procedimento Ordinário"
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
  lp.class_name = CASE WHEN $class_name = '' THEN lp.class_name ELSE $class_name END,
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
		"class_name":  p.ClassName,
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

// UpdateProceedingCaseState applies the case-level movement state machine onto
// a LegalProceeding. disposition is three-valued and written verbatim each poll:
// "conviction" → has_conviction=true, "acquittal" → false, "" → the property is
// REMOVED (SET to null), because "the movements do not let us say" must render
// as "não verificado", never as an acquittal. Recomputed-not-latched, so a
// conviction reversed on appeal clears rather than sticking (defamation-grade
// if it did not). All flags are case-level; per-defendant outcomes are set only
// via backoffice review.
func (db *DB) UpdateProceedingCaseState(ctx context.Context, proceedingID, phase, disposition string, concluded bool) error {
	var hasConviction any // nil removes the property
	switch disposition {
	case "conviction":
		hasConviction = true
	case "acquittal":
		hasConviction = false
	}
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
