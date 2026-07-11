package memgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// SanctionUpsert carries the deterministic properties of a Sanction node.
// The node id is derived as registry + ":" + entry id.
type SanctionUpsert struct {
	Registry     string
	EntryID      string
	SanctionType string
	Organ        string
	DateStart    string // yyyy-mm-dd, or "" when unknown
	DateEnd      string // yyyy-mm-dd, or "" when unknown
	ProcessRef   string
	SourceURL    string
}

// PoliticianMatch is a Politician node candidate returned by masked-CPF or
// name matching. Never linked automatically — always routed through review.
type PoliticianMatch struct {
	ID   string
	Name string
}

// SanctionNodeID returns the deterministic Sanction node id.
func SanctionNodeID(registry, entryID string) string {
	return strings.ToUpper(strings.TrimSpace(registry)) + ":" + strings.TrimSpace(entryID)
}

// UpsertSanction merges a Sanction node by its deterministic id. source_url is a
// hard requirement (legal compliance) — every node must deep-link the record.
func (db *DB) UpsertSanction(ctx context.Context, s SanctionUpsert) (string, error) {
	if strings.TrimSpace(s.SourceURL) == "" {
		return "", fmt.Errorf("memgraph: sanction %s:%s missing source_url", s.Registry, s.EntryID)
	}
	id := SanctionNodeID(s.Registry, s.EntryID)

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MERGE (s:Sanction {id: $id})
SET
  s.registry = $registry,
  s.sanction_type = $sanction_type,
  s.organ = $organ,
  s.date_start = $date_start,
  s.date_end = $date_end,
  s.process_ref = $process_ref,
  s.source_url = $source_url
RETURN s.id AS id
`, map[string]any{
		"id":            id,
		"registry":      strings.ToUpper(strings.TrimSpace(s.Registry)),
		"sanction_type": s.SanctionType,
		"organ":         s.Organ,
		"date_start":    nilIfEmpty(s.DateStart),
		"date_end":      nilIfEmpty(s.DateEnd),
		"process_ref":   s.ProcessRef,
		"source_url":    s.SourceURL,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: upsert sanction: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", fmt.Errorf("memgraph: upsert sanction result: %w", err)
		}
		return "", fmt.Errorf("memgraph: upsert sanction returned no rows")
	}
	v, _ := res.Record().Get("id")
	out, _ := v.(string)
	return out, nil
}

// EnsureSanctionedInEdge creates the SANCTIONED_IN edge from a subject node
// (Politician/Person/Organization) to a Sanction node.
func (db *DB) EnsureSanctionedInEdge(ctx context.Context, nodeType, nodeID, sanctionID string) error {
	switch nodeType {
	case "Politician", "Person", "Organization":
	default:
		return fmt.Errorf("memgraph: invalid sanctioned-in node type %q", nodeType)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	query := fmt.Sprintf(`
MATCH (n:%s {id: $node_id})
MATCH (s:Sanction {id: $sanction_id})
MERGE (n)-[:SANCTIONED_IN]->(s)
`, nodeType)
	_, err := session.Run(ctx, query, map[string]any{"node_id": nodeID, "sanction_id": sanctionID})
	if err != nil {
		return fmt.Errorf("memgraph: ensure sanctioned-in edge: %w", err)
	}
	return nil
}

// EnsureOrganizationByCNPJ merges a bare Organization by CNPJ so the CNPJ
// Enricher can later fill it in. Returns the node id and whether it was created.
func (db *DB) EnsureOrganizationByCNPJ(ctx context.Context, cnpj string) (string, bool, error) {
	digits := digitsOnly(cnpj)
	if len(digits) != 14 {
		return "", false, fmt.Errorf("memgraph: invalid cnpj %q", cnpj)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
MERGE (o:Organization {cnpj: $cnpj})
ON CREATE SET o.id = $id, o._sanctions_created = true
WITH o, coalesce(o._sanctions_created, false) AS created
REMOVE o._sanctions_created
RETURN o.id AS id, created AS created
`, map[string]any{"id": "org_" + digits, "cnpj": digits})
	if err != nil {
		return "", false, fmt.Errorf("memgraph: ensure organization by cnpj: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", false, err
		}
		return "", false, fmt.Errorf("memgraph: ensure organization returned no rows")
	}
	rec := res.Record()
	idVal, _ := rec.Get("id")
	createdVal, _ := rec.Get("created")
	id, _ := idVal.(string)
	created, _ := createdVal.(bool)
	return id, created, nil
}

// FindSubjectByCPF resolves a full CPF to an existing Politician or Person node.
// Returns empty strings when no node matches.
func (db *DB) FindSubjectByCPF(ctx context.Context, cpf string) (nodeType, nodeID string, err error) {
	digits := digitsOnly(cpf)
	if len(digits) != 11 {
		return "", "", nil
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `MATCH (p:Politician {cpf: $cpf}) RETURN p.id AS id LIMIT 1`, map[string]any{"cpf": digits})
	if err != nil {
		return "", "", err
	}
	if res.Next(ctx) {
		id, _ := res.Record().Get("id")
		if s, ok := id.(string); ok {
			return "Politician", s, nil
		}
	}

	res, err = session.Run(ctx, `MATCH (p:Person {cpf: $cpf}) RETURN p.id AS id LIMIT 1`, map[string]any{"cpf": digits})
	if err != nil {
		return "", "", err
	}
	if res.Next(ctx) {
		id, _ := res.Record().Get("id")
		if s, ok := id.(string); ok {
			return "Person", s, nil
		}
	}
	return "", "", nil
}

// UpsertPersonByCPF merges a Person node keyed by a full CPF. Used when a
// deterministic full-CPF sanction has no existing subject node yet.
func (db *DB) UpsertPersonByCPF(ctx context.Context, name, cpf string) (string, bool, error) {
	digits := digitsOnly(cpf)
	if len(digits) != 11 {
		return "", false, fmt.Errorf("memgraph: invalid cpf %q", cpf)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
MERGE (p:Person {cpf: $cpf})
ON CREATE SET p.id = $id, p.name = $name, p._sanctions_created = true
WITH p, coalesce(p._sanctions_created, false) AS created
REMOVE p._sanctions_created
RETURN p.id AS id, created AS created
`, map[string]any{"id": "person_" + digits, "cpf": digits, "name": strings.TrimSpace(name)})
	if err != nil {
		return "", false, fmt.Errorf("memgraph: upsert person by cpf: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", false, err
		}
		return "", false, fmt.Errorf("memgraph: upsert person by cpf returned no rows")
	}
	rec := res.Record()
	idVal, _ := rec.Get("id")
	createdVal, _ := rec.Get("created")
	id, _ := idVal.(string)
	created, _ := createdVal.(bool)
	return id, created, nil
}

// MatchPoliticiansByMaskedCPF finds Politician nodes whose stored 11-digit CPF
// matches the 6 visible middle digits of a masked CPF (***.XXX.XXX-**).
func (db *DB) MatchPoliticiansByMaskedCPF(ctx context.Context, middleSix string) ([]PoliticianMatch, error) {
	middle := digitsOnly(middleSix)
	if len(middle) != 6 {
		return nil, nil
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
MATCH (p:Politician)
WHERE p.cpf IS NOT NULL AND size(p.cpf) = 11 AND substring(p.cpf, 3, 6) = $middle
RETURN p.id AS id, p.name AS name
`, map[string]any{"middle": middle})
	if err != nil {
		return nil, fmt.Errorf("memgraph: match politicians by masked cpf: %w", err)
	}
	var out []PoliticianMatch
	for res.Next(ctx) {
		rec := res.Record()
		idVal, _ := rec.Get("id")
		nameVal, _ := rec.Get("name")
		id, _ := idVal.(string)
		name, _ := nameVal.(string)
		if id != "" {
			out = append(out, PoliticianMatch{ID: id, Name: name})
		}
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MatchPoliticianByName returns the id of a Politician whose name matches
// case-insensitively, or "" when none matches. Self-contained so the Sanctions
// worker does not depend on other workers' evolving helpers.
func (db *DB) MatchPoliticianByName(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
MATCH (p:Politician)
WHERE toLower(p.name) = toLower($name)
RETURN p.id AS id LIMIT 1
`, map[string]any{"name": name})
	if err != nil {
		return "", fmt.Errorf("memgraph: match politician by name: %w", err)
	}
	if res.Next(ctx) {
		id, _ := res.Record().Get("id")
		if s, ok := id.(string); ok {
			return s, nil
		}
	}
	return "", nil
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
