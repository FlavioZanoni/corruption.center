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

type DataJudPartyMatch struct {
	NodeType string
	NodeID   string
}

func (db *DB) UpsertLegalProceedingByCase(ctx context.Context, p DataJudProceedingUpsert) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
MERGE (lp:LegalProceeding {case_number: $case_number})
ON CREATE SET lp.id = $id
SET
  lp.court = $court,
  lp.type = $type,
  lp.status = $status,
  lp.assuntos = $assuntos,
  lp.date_filed = $date_filed
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

func (db *DB) FindDefendantByDocument(ctx context.Context, doc string) (*DataJudPartyMatch, error) {
	doc = digitsOnly(doc)
	if doc == "" {
		return nil, nil
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if len(doc) == 11 {
		res, err := session.Run(ctx, `MATCH (p:Politician {cpf: $cpf}) RETURN p.id AS id LIMIT 1`, map[string]any{"cpf": doc})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			id, _ := res.Record().Get("id")
			if s, ok := id.(string); ok {
				return &DataJudPartyMatch{NodeType: "Politician", NodeID: s}, nil
			}
		}
	}

	if len(doc) == 14 {
		res, err := session.Run(ctx, `MATCH (o:Organization {cnpj: $cnpj}) RETURN o.id AS id LIMIT 1`, map[string]any{"cnpj": doc})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			id, _ := res.Record().Get("id")
			if s, ok := id.(string); ok {
				return &DataJudPartyMatch{NodeType: "Organization", NodeID: s}, nil
			}
		}
	}

	return nil, nil
}

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

func (db *DB) UpsertUnknownPerson(ctx context.Context, name, cpfMasked string) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	id := "person_unknown_" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))
	res, err := session.Run(ctx, `
MERGE (p:Person {id: $id})
SET p.name = $name, p.cpf = $cpf
RETURN p.id AS id
`, map[string]any{"id": id, "name": name, "cpf": cpfMasked})
	if err != nil {
		return "", err
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("memgraph: upsert unknown person no rows")
	}
	v, _ := res.Record().Get("id")
	id, _ = v.(string)
	return id, nil
}

func (db *DB) UpsertUnknownOrganization(ctx context.Context, cnpj string) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	id := "org_" + digitsOnly(cnpj)
	res, err := session.Run(ctx, `
MERGE (o:Organization {cnpj: $cnpj})
ON CREATE SET o.id = $id
RETURN o.id AS id
`, map[string]any{"id": id, "cnpj": digitsOnly(cnpj)})
	if err != nil {
		return "", err
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("memgraph: upsert unknown organization no rows")
	}
	v, _ := res.Record().Get("id")
	id, _ = v.(string)
	return id, nil
}

func (db *DB) SearchPoliticianByNameState(ctx context.Context, name string) (string, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
MATCH (p:Politician)
WHERE toLower(p.name) = toLower($name)
RETURN p.id AS id LIMIT 1
`, map[string]any{"name": strings.TrimSpace(name)})
	if err != nil {
		return "", err
	}
	if res.Next(ctx) {
		id, _ := res.Record().Get("id")
		if s, ok := id.(string); ok {
			return s, nil
		}
	}
	return "", nil
}

func (db *DB) UpdateProceedingStatusByID(ctx context.Context, proceedingID, status string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.Run(ctx, `
MATCH (lp:LegalProceeding {id: $id})
SET lp.status = $status
`, map[string]any{"id": proceedingID, "status": status})
	if err != nil {
		return err
	}
	return nil
}

func legalProceedingID(caseNumber string) string {
	clean := strings.TrimSpace(caseNumber)
	return "lp_" + strings.ReplaceAll(strings.ReplaceAll(clean, ".", ""), "-", "")
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
