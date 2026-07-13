package memgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// OrgToEnrich is the minimal projection the CNPJ Enricher needs: the node id and
// the 14-digit CNPJ to look up on the provider.
type OrgToEnrich struct {
	ID   string
	CNPJ string
}

// OrganizationEnrichment carries the fields the enricher writes onto an
// Organization node from the provider (minha receita / Receita Federal data).
type OrganizationEnrichment struct {
	CNPJ            string
	Name            string  // razão social
	Active          bool    // situação cadastral == "ATIVA"
	Type            string  // natureza jurídica (free string)
	UF              string  // address UF
	ShareCapitalBRL float64 // capital social
	MainActivity    string  // primary CNAE description
	SourceURL       string  // provider deep-link for provenance
}

// ListOrganizationsNeedingEnrichment returns Organization nodes that carry a
// 14-digit CNPJ and match any of: never enriched (enriched flag missing/false),
// missing enriched_at (backward compat for pre-timestamp orgs), or stale
// (enriched_at before the cutoff). cutoff should be an RFC3339 UTC timestamp
// string. Never-enriched/never-stamped orgs are returned first, then stale ones.
// Bounded by limit (limit <= 0 returns all).
func (db *DB) ListOrganizationsNeedingEnrichment(ctx context.Context, limit int, cutoff string) ([]OrgToEnrich, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
MATCH (o:Organization)
WHERE o.cnpj IS NOT NULL AND size(o.cnpj) = 14
  AND (
    o.enriched IS NULL OR o.enriched = false
    OR o.enriched_at IS NULL
    OR o.enriched_at < $cutoff
  )
ORDER BY
  CASE WHEN o.enriched IS NULL OR o.enriched = false THEN 0
       WHEN o.enriched_at IS NULL THEN 1
       ELSE 2
  END,
  o.enriched_at ASC
RETURN o.id AS id, o.cnpj AS cnpj
`
	params := map[string]any{"cutoff": cutoff}
	if limit > 0 {
		query += "LIMIT $limit"
		params["limit"] = limit
	}

	res, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list organizations needing enrichment: %w", err)
	}
	out := make([]OrgToEnrich, 0)
	for res.Next(ctx) {
		rec := res.Record()
		idVal, _ := rec.Get("id")
		cnpjVal, _ := rec.Get("cnpj")
		id, _ := idVal.(string)
		cnpj, _ := cnpjVal.(string)
		if id != "" && cnpj != "" {
			out = append(out, OrgToEnrich{ID: id, CNPJ: cnpj})
		}
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: iterate organizations needing enrichment: %w", err)
	}
	return out, nil
}

// UpdateOrganizationEnrichment merges an Organization by CNPJ and writes the
// enriched fields, setting enriched=true and enriched_at to the provided timestamp
// so the node is not re-processed. Merging by cnpj (not id) reconciles nodes seeded
// by other workers with the same CNPJ. enrichedAt should be an RFC3339 UTC timestamp.
func (db *DB) UpdateOrganizationEnrichment(ctx context.Context, e OrganizationEnrichment, enrichedAt string) (string, error) {
	digits := digitsOnly(e.CNPJ)
	if len(digits) != 14 {
		return "", fmt.Errorf("memgraph: invalid cnpj %q", e.CNPJ)
	}
	if strings.TrimSpace(e.SourceURL) == "" {
		return "", fmt.Errorf("memgraph: organization %s missing source_url", digits)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MERGE (o:Organization {cnpj: $cnpj})
ON CREATE SET o.id = $id
SET
  o.name = $name,
  o.search_name = $search_name,
  o.active = $active,
  o.type = $type,
  o.uf = $uf,
  o.share_capital_brl = $share_capital_brl,
  o.main_activity = $main_activity,
  o.source_url = $source_url,
  o.enriched = true,
  o.enriched_at = $enriched_at
RETURN o.id AS id
`, map[string]any{
		"id":                "org_" + digits,
		"cnpj":              digits,
		"name":              strings.TrimSpace(e.Name),
		"search_name":       foldQuery(e.Name),
		"active":            e.Active,
		"type":              e.Type,
		"uf":                strings.ToUpper(strings.TrimSpace(e.UF)),
		"share_capital_brl": e.ShareCapitalBRL,
		"main_activity":     e.MainActivity,
		"source_url":        e.SourceURL,
		"enriched_at":       enrichedAt,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: update organization enrichment: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("memgraph: update organization enrichment returned no rows")
	}
	v, _ := res.Record().Get("id")
	id, _ := v.(string)
	return id, nil
}

// QSAPersonUpsert carries the provenance the enricher attaches to a Person node
// created from a QSA board member. QSA individuals have only a MASKED CPF
// (***XXXXXX**), never a full one, so the node is keyed by name, not CPF.
type QSAPersonUpsert struct {
	Name          string
	MaskedCPF     string // as published, e.g. "***641988**"
	Qualification string // qualificacao_socio, e.g. "Diretor"
	SourceCNPJ    string // the org this person is a partner of (provenance)
	SourceURL     string
}

// UpsertQSAPerson merges a name-keyed Person node for a QSA board member and
// creates the CONTROLS edge (person → organization). It never touches Politician
// nodes: a masked-CPF hit on a Politician goes through pending_review instead.
func (db *DB) UpsertQSAPerson(ctx context.Context, orgID string, p QSAPersonUpsert) (string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("memgraph: qsa person missing name")
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	id := qsaPersonID(p.Name)
	res, err := session.Run(ctx, `
MATCH (o:Organization {id: $org_id})
MERGE (p:Person {id: $id})
SET
  p.name = $name,
  p.search_name = $search_name,
  p.provenance_source = 'cnpj',
  p.provenance_masked_cpf = $masked_cpf,
  p.provenance_source_cnpj = $source_cnpj
MERGE (p)-[r:CONTROLS]->(o)
SET r.source = 'cnpj', r.qualification = $qualification, r.source_url = $source_url
RETURN p.id AS id
`, map[string]any{
		"id":            id,
		"org_id":        orgID,
		"name":          strings.TrimSpace(p.Name),
		"search_name":   foldQuery(p.Name),
		"masked_cpf":    strings.TrimSpace(p.MaskedCPF),
		"qualification": p.Qualification,
		"source_cnpj":   digitsOnly(p.SourceCNPJ),
		"source_url":    p.SourceURL,
	})
	if err != nil {
		return "", fmt.Errorf("memgraph: upsert qsa person: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("memgraph: upsert qsa person returned no rows (org %s not found?)", orgID)
	}
	v, _ := res.Record().Get("id")
	got, _ := v.(string)
	return got, nil
}

// UpsertQSAOrganization merges the corporate partner Organization by its CNPJ and
// creates the OWNED_BY edge (enriched org → partner org): the org being enriched
// is owned by its corporate shareholder, a shell ownership chain. The partner is
// left un-enriched (no enriched flag) so it is naturally picked up on a later
// pass; created reports whether it is brand new (so the caller can bound-depth
// enqueue it within the same run). Returns the partner node id.
func (db *DB) UpsertQSAOrganization(ctx context.Context, ownedOrgID, partnerCNPJ, qualification, sourceURL string) (partnerID string, created bool, err error) {
	digits := digitsOnly(partnerCNPJ)
	if len(digits) != 14 {
		return "", false, fmt.Errorf("memgraph: invalid partner cnpj %q", partnerCNPJ)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (owned:Organization {id: $owned_id})
MERGE (partner:Organization {cnpj: $cnpj})
ON CREATE SET partner.id = $partner_id, partner._cnpj_created = true
WITH owned, partner, coalesce(partner._cnpj_created, false) AS created
REMOVE partner._cnpj_created
MERGE (owned)-[r:OWNED_BY]->(partner)
SET r.source = 'cnpj', r.qualification = $qualification, r.source_url = $source_url
RETURN partner.id AS id, created AS created
`, map[string]any{
		"owned_id":      ownedOrgID,
		"partner_id":    "org_" + digits,
		"cnpj":          digits,
		"qualification": qualification,
		"source_url":    sourceURL,
	})
	if err != nil {
		return "", false, fmt.Errorf("memgraph: upsert qsa organization: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return "", false, err
		}
		return "", false, fmt.Errorf("memgraph: upsert qsa organization returned no rows (org %s not found?)", ownedOrgID)
	}
	rec := res.Record()
	idVal, _ := rec.Get("id")
	createdVal, _ := rec.Get("created")
	partnerID, _ = idVal.(string)
	created, _ = createdVal.(bool)
	return partnerID, created, nil
}

// qsaPersonID derives a stable name-based Person id for a QSA board member.
// QSA individuals never expose a full CPF, so the name is the only stable key.
func qsaPersonID(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.Join(strings.Fields(clean), "_")
	return "person_qsa_" + clean
}
