package memgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// PoliticianNames is the minimal projection the DJEN worker needs to build its
// in-memory name→politician index for exact (normalized) roster matching.
type PoliticianNames struct {
	ID      string
	Name    string
	Aliases []string
}

// ListPoliticianNames returns every Politician's id, primary name and aliases.
// The DJEN worker normalizes these in Go (uppercase + accent-strip) so matching
// is done in memory rather than trying to accent-fold inside Cypher.
func (db *DB) ListPoliticianNames(ctx context.Context) ([]PoliticianNames, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (p:Politician)
RETURN p.id AS id, p.name AS name, p.name_aliases AS aliases
`, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list politician names: %w", err)
	}

	out := make([]PoliticianNames, 0)
	for res.Next(ctx) {
		rec := res.Record()
		idVal, _ := rec.Get("id")
		id, _ := idVal.(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		nameVal, _ := rec.Get("name")
		name, _ := nameVal.(string)

		aliasesVal, _ := rec.Get("aliases")
		aliases := make([]string, 0)
		if arr, ok := aliasesVal.([]any); ok {
			for _, a := range arr {
				if s, ok := a.(string); ok && strings.TrimSpace(s) != "" {
					aliases = append(aliases, s)
				}
			}
		}
		out = append(out, PoliticianNames{ID: id, Name: name, Aliases: aliases})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: iterate politician names: %w", err)
	}
	return out, nil
}

// DjenPersonUpsert carries the provenance the DJEN worker attaches to a Person
// node it creates from a case party (names only; no CPF/CNPJ available).
type DjenPersonUpsert struct {
	Name          string
	ComunicacaoID string // DJEN comunicação id the name was observed in
	Link          string // DJEN communication link (official source)
	Tribunal      string // siglaTribunal, e.g. "TRF1"
}

// UpsertDjenPerson creates/updates a name-only Person node whose provenance
// points back at the DJEN communication it was discovered in. It never touches
// Politician nodes: a name match to a Politician goes through pending_review.
func (db *DB) UpsertDjenPerson(ctx context.Context, p DjenPersonUpsert) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	id := djenPersonID(p.Name)
	res, err := session.Run(ctx, `
MERGE (p:Person {id: $id})
SET
  p.name = $name,
  p.provenance_source = 'djen',
  p.provenance_comunicacao_id = $comunicacao_id,
  p.provenance_link = $link,
  p.provenance_tribunal = $tribunal
RETURN p.id AS id
`, map[string]any{
		"id":             id,
		"name":           strings.TrimSpace(p.Name),
		"comunicacao_id": p.ComunicacaoID,
		"link":           p.Link,
		"tribunal":       p.Tribunal,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: upsert djen person: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", fmt.Errorf("memgraph: upsert djen person result: %w", err)
		}
		return "", fmt.Errorf("memgraph: upsert djen person returned no rows")
	}
	v, _ := res.Record().Get("id")
	got, _ := v.(string)
	return got, nil
}

// DjenOrganizationUpsert carries the provenance the DJEN worker attaches to an
// Organization node it creates from a corporate case party. Like Person parties,
// DJEN destinatarios carry names only (no CNPJ), so the node is name-only and a
// human attaches the CNPJ later via the "unknown_cnpj" review.
type DjenOrganizationUpsert struct {
	Name          string
	ComunicacaoID string // DJEN comunicação id the name was observed in
	Link          string // DJEN communication link (official source)
	Tribunal      string // siglaTribunal, e.g. "TRF1"
}

// UpsertDjenOrganization creates/updates a name-only Organization node whose
// provenance points back at the DJEN communication it was discovered in. It is
// the corporate counterpart of UpsertDjenPerson: company-like passive parties
// become Organization nodes rather than Person nodes.
func (db *DB) UpsertDjenOrganization(ctx context.Context, o DjenOrganizationUpsert) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	id := djenOrganizationID(o.Name)
	res, err := session.Run(ctx, `
MERGE (o:Organization {id: $id})
SET
  o.name = $name,
  o.provenance_source = 'djen',
  o.provenance_comunicacao_id = $comunicacao_id,
  o.provenance_link = $link,
  o.provenance_tribunal = $tribunal
RETURN o.id AS id
`, map[string]any{
		"id":             id,
		"name":           strings.TrimSpace(o.Name),
		"comunicacao_id": o.ComunicacaoID,
		"link":           o.Link,
		"tribunal":       o.Tribunal,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: upsert djen organization: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", fmt.Errorf("memgraph: upsert djen organization result: %w", err)
		}
		return "", fmt.Errorf("memgraph: upsert djen organization returned no rows")
	}
	v, _ := res.Record().Get("id")
	got, _ := v.(string)
	return got, nil
}

// EnsureDjenDefendantEdge merges a DEFENDANT_IN edge from a node to the tracked
// LegalProceeding with the given outcome (DJEN uses "cited"). Defined in this
// DJEN-owned file so the worker does not depend on churn in datajud_writer.go.
func (db *DB) EnsureDjenDefendantEdge(ctx context.Context, nodeType, nodeID, proceedingID, outcome string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	query := fmt.Sprintf(`
MATCH (n:%s {id: $node_id})
MATCH (lp:LegalProceeding {id: $proceeding_id})
MERGE (n)-[r:DEFENDANT_IN]->(lp)
SET r.outcome = $outcome, r.source = 'djen'
`, nodeType)
	_, err := session.Run(ctx, query, map[string]any{
		"node_id":       nodeID,
		"proceeding_id": proceedingID,
		"outcome":       outcome,
	})
	if err != nil {
		return fmt.Errorf("memgraph: ensure djen defendant edge: %w", err)
	}
	return nil
}

func djenPersonID(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.Join(strings.Fields(clean), "_")
	return "person_djen_" + clean
}

func djenOrganizationID(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.Join(strings.Fields(clean), "_")
	return "org_djen_" + clean
}

// CitedPerson is an anonymous Person already linked as a defendant. Rematch mode
// re-tests these names against the politician index.
type CitedPerson struct {
	PersonID     string
	Name         string
	ProceedingID string
	CaseNumber   string
	ScandalID    string
}

// ListCitedPersons returns every Person with a DEFENDANT_IN edge. A party is
// matched against the politician index only once, at discovery, and is then
// snapshotted so it never reappears in a roster delta: so when the politician
// base grows (a TSE import), previously unmatched defendants stay anonymous
// forever. This lets a rematch pass re-test them.
func (db *DB) ListCitedPersons(ctx context.Context) ([]CitedPerson, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (p:Person)-[:DEFENDANT_IN]->(lp:LegalProceeding)
OPTIONAL MATCH (lp)-[:INVESTIGATES]->(s:Scandal)
RETURN p.id AS person_id, p.name AS name, lp.id AS proceeding_id,
       lp.case_number AS case_number, s.id AS scandal_id
`, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list cited persons: %w", err)
	}

	out := make([]CitedPerson, 0)
	for res.Next(ctx) {
		rec := res.Record()
		cp := CitedPerson{
			PersonID:     recString(rec, "person_id"),
			Name:         recString(rec, "name"),
			ProceedingID: recString(rec, "proceeding_id"),
			CaseNumber:   recString(rec, "case_number"),
			ScandalID:    recString(rec, "scandal_id"),
		}
		if cp.PersonID == "" || cp.Name == "" || cp.ProceedingID == "" {
			continue
		}
		out = append(out, cp)
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: list cited persons rows: %w", err)
	}
	return out, nil
}

func recString(rec *neo4j.Record, key string) string {
	v, _ := rec.Get(key)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
