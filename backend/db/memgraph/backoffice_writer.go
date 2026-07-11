package memgraph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ErrPoliticianNotPurgeable is returned by PurgePersonNode when the targeted
// node is a Politician. Politicians are public officials: under LGPD art. 23 the
// processing of their data for public-interest accountability is lawful, so they
// are never removable from the transparency graph. The full justification is
// carried in the error message so it can be surfaced verbatim in the backoffice.
var ErrPoliticianNotPurgeable = errors.New(
	"refusing to purge a Politician node: politicians are public officials and, " +
		"under LGPD art. 23, the processing of their data for public-interest " +
		"accountability is lawful — they cannot be removed from the transparency graph")

// ErrNodeNotPurgeable is returned by PurgePersonNode when the targeted node is
// neither a Person nor an Organization. Only those two labels carry personal /
// corporate data a subject may request removal of (docs/legal_compliance.md);
// Scandal, LegalProceeding, Sanction and Source nodes are transparency records
// and must never be deleted through the removal-request flow. The message is
// surfaced verbatim in the backoffice so the operator understands the refusal.
var ErrNodeNotPurgeable = errors.New(
	"refusing to purge this node: only Person and Organization nodes are " +
		"purgeable via the removal-request flow — Scandal, LegalProceeding, " +
		"Sanction and Source nodes are transparency records and cannot be removed")

// ScandalOption is a minimal Scandal projection (id + name) used to populate the
// scandal dropdown in the backoffice seed and DJEN-approval forms.
type ScandalOption struct {
	ID   string
	Name string
}

// UpsertScandal merges a Scandal node by id. Name/date_start are set only on
// create (an existing scandal is never renamed by a case registration); name
// falls back to the id so the timeline always has something to render.
func (db *DB) UpsertScandal(ctx context.Context, id, name, dateStart string) error {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if strings.TrimSpace(name) == "" {
		name = id
	}
	_, err := session.Run(ctx, `
MERGE (s:Scandal {id: $id})
ON CREATE SET s.name = $name, s.date_start = $date_start
`, map[string]any{"id": id, "name": name, "date_start": dateStart})
	if err != nil {
		return fmt.Errorf("memgraph: upsert scandal: %w", err)
	}
	return nil
}

// ListScandals returns every Scandal node's id and name, ordered by name, for
// the backoffice scandal selector.
func (db *DB) ListScandals(ctx context.Context) ([]ScandalOption, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (s:Scandal)
RETURN s.id AS id, s.name AS name
ORDER BY toLower(coalesce(s.name, s.id))
`, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list scandals: %w", err)
	}
	out := make([]ScandalOption, 0)
	for res.Next(ctx) {
		rec := res.Record()
		idVal, _ := rec.Get("id")
		id, _ := idVal.(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		nameVal, _ := rec.Get("name")
		name, _ := nameVal.(string)
		out = append(out, ScandalOption{ID: id, Name: name})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: iterate scandals: %w", err)
	}
	return out, nil
}

// NodeProvenance describes a graph node for the backoffice: its label, name,
// whether it is a Politician (and therefore not purgeable), the provenance
// properties the DJEN/enricher writers set, and a human-readable creation
// reason derived from them.
type NodeProvenance struct {
	ID             string
	Label          string
	Name           string
	IsPolitician   bool
	Purgeable      bool   // true only for Person/Organization non-Politician nodes
	CPF            string // n.cpf (11 digits) when present — for purge tombstones
	CNPJ           string // n.cnpj (14 digits) when present — for purge tombstones
	Source         string // provenance_source, e.g. "djen"
	ComunicacaoID  string // provenance_comunicacao_id
	Link           string // provenance_link (official source URL)
	Tribunal       string // provenance_tribunal
	CreationReason string // human-readable summary of the above
	EdgeCount      int
}

// GetNodeProvenance loads the provenance of a node by id so the backoffice can
// display its creation reason before a purge/resolution decision. Returns a nil
// pointer (no error) when the node does not exist.
func (db *DB) GetNodeProvenance(ctx context.Context, id string) (*NodeProvenance, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (n {id: $id})
OPTIONAL MATCH (n)-[r]-()
RETURN labels(n) AS labels,
       n.name AS name,
       n.cpf AS cpf,
       n.cnpj AS cnpj,
       n.provenance_source AS source,
       n.provenance_comunicacao_id AS comunicacao_id,
       n.provenance_link AS link,
       n.provenance_tribunal AS tribunal,
       count(r) AS deg
`, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: get node provenance: %w", err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return nil, fmt.Errorf("memgraph: get node provenance result: %w", err)
		}
		return nil, nil
	}
	rec := res.Record()

	np := &NodeProvenance{ID: id}
	labels := recordStrings(rec, "labels")
	np.Label, np.IsPolitician = primaryLabel(labels)
	np.Purgeable = hasPurgeableLabel(labels) && !np.IsPolitician
	np.Name = recordString(rec, "name")
	np.CPF = recordString(rec, "cpf")
	np.CNPJ = recordString(rec, "cnpj")
	np.Source = recordString(rec, "source")
	np.ComunicacaoID = recordString(rec, "comunicacao_id")
	np.Link = recordString(rec, "link")
	np.Tribunal = recordString(rec, "tribunal")
	if degVal, ok := rec.Get("deg"); ok {
		if deg, ok := degVal.(int64); ok {
			np.EdgeCount = int(deg)
		}
	}
	np.CreationReason = buildCreationReason(np)
	return np, nil
}

// PurgePersonNode deletes a Person/Organization node and all of its edges
// (DETACH DELETE), leaving no orphaned edges. It refuses Politician nodes,
// returning ErrPoliticianNotPurgeable. The caller is responsible for writing the
// audit_log deletion record with the returned metadata (label, name, edge
// count) — that record is what satisfies the LGPD "why was my data here" duty.
//
// It returns the node's provenance (as it was before deletion) so the audit
// record can capture the creation reason of what was removed.
func (db *DB) PurgePersonNode(ctx context.Context, id string) (*NodeProvenance, error) {
	np, err := db.GetNodeProvenance(ctx, id)
	if err != nil {
		return nil, err
	}
	if np == nil {
		return nil, fmt.Errorf("memgraph: purge node: node %q not found", id)
	}
	if np.IsPolitician {
		return np, ErrPoliticianNotPurgeable
	}
	// Only Person/Organization nodes carry personal/corporate data that a subject
	// may have removed. Refuse anything else (Scandal/LegalProceeding/Sanction/
	// Source) so the removal flow can never delete a transparency record.
	if !np.Purgeable {
		return np, ErrNodeNotPurgeable
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Guard the delete with a label predicate as well, so a concurrent re-label
	// cannot slip a Politician (or any non Person/Organization node) through the
	// read/delete gap: only Person or Organization nodes are ever detached.
	_, err = session.Run(ctx, `
MATCH (n {id: $id})
WHERE (n:Person OR n:Organization) AND NOT n:Politician
DETACH DELETE n
`, map[string]any{"id": id})
	if err != nil {
		return np, fmt.Errorf("memgraph: purge node: %w", err)
	}
	return np, nil
}

// primaryLabel picks the most meaningful label for display and reports whether
// the node carries the Politician label.
func primaryLabel(labels []string) (string, bool) {
	isPol := false
	primary := ""
	for _, l := range labels {
		if strings.EqualFold(l, "Politician") {
			isPol = true
		}
		if primary == "" {
			primary = l
		}
	}
	if isPol {
		return "Politician", true
	}
	if primary == "" {
		primary = "Node"
	}
	return primary, false
}

// hasPurgeableLabel reports whether the node carries a Person or Organization
// label — the only two labels the removal-request purge is allowed to delete.
func hasPurgeableLabel(labels []string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, "Person") || strings.EqualFold(l, "Organization") {
			return true
		}
	}
	return false
}

// ── Human-confirmed edge writers ────────────────────────────────────────────
// These are the writes the backoffice performs when an operator APPROVES a
// pending_review that proposed a link between a Politician and another node.
// Per the review-queue contract (docs/legal_compliance.md), such links are
// never auto-created by the workers — they only exist once a human confirms
// them here. Each MERGE is idempotent, and every writer RETURNs a row so a
// missing Politician / target id surfaces as a visible error rather than a
// silent no-op (MATCH failing would otherwise skip the MERGE quietly).

// EnsurePoliticianDefendantEdge confirms a DEFENDANT_IN edge from a Politician
// to a LegalProceeding (approval of a "djen_party_match" review).
func (db *DB) EnsurePoliticianDefendantEdge(ctx context.Context, politicianID, proceedingID string) error {
	return db.mergeConfirmedPoliticianEdge(ctx, confirmedEdge{
		targetLabel: "LegalProceeding",
		relType:     "DEFENDANT_IN",
		politician:  politicianID,
		targetID:    proceedingID,
		extraSet:    "r.outcome = 'confirmed', ",
		what:        "politician defendant",
	})
}

// EnsurePoliticianControlsOrganization confirms a CONTROLS edge from a
// Politician to an Organization (approval of a "possible_politician_in_qsa"
// review). Direction matches the CNPJ worker's Person→CONTROLS→Organization.
func (db *DB) EnsurePoliticianControlsOrganization(ctx context.Context, politicianID, organizationID string) error {
	return db.mergeConfirmedPoliticianEdge(ctx, confirmedEdge{
		targetLabel: "Organization",
		relType:     "CONTROLS",
		politician:  politicianID,
		targetID:    organizationID,
		what:        "politician controls organization",
	})
}

// EnsurePoliticianSanctionedInEdge confirms a SANCTIONED_IN edge from a
// Politician to a Sanction (approval of a "possible_politician_sanction"
// review).
func (db *DB) EnsurePoliticianSanctionedInEdge(ctx context.Context, politicianID, sanctionID string) error {
	return db.mergeConfirmedPoliticianEdge(ctx, confirmedEdge{
		targetLabel: "Sanction",
		relType:     "SANCTIONED_IN",
		politician:  politicianID,
		targetID:    sanctionID,
		what:        "politician sanctioned in",
	})
}

type confirmedEdge struct {
	targetLabel string
	relType     string
	politician  string
	targetID    string
	extraSet    string // extra "r.x = ..., " prefix for the SET clause
	what        string // human label for errors
}

// mergeConfirmedPoliticianEdge merges a human-confirmed edge from a Politician
// to a target node. It stamps r.source='backoffice_review' so the provenance of
// the confirmation is auditable in the graph, and errors if either endpoint id
// does not resolve to a node.
func (db *DB) mergeConfirmedPoliticianEdge(ctx context.Context, e confirmedEdge) error {
	if strings.TrimSpace(e.politician) == "" || strings.TrimSpace(e.targetID) == "" {
		return fmt.Errorf("memgraph: ensure %s edge: politician id and target id are required", e.what)
	}
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
MATCH (p:Politician {id: $pol_id})
MATCH (t:%s {id: $target_id})
MERGE (p)-[r:%s]->(t)
SET %sr.source = 'backoffice_review'
RETURN p.id AS id
`, e.targetLabel, e.relType, e.extraSet)

	res, err := session.Run(ctx, query, map[string]any{
		"pol_id":    strings.TrimSpace(e.politician),
		"target_id": strings.TrimSpace(e.targetID),
	})
	if err != nil {
		return fmt.Errorf("memgraph: ensure %s edge: %w", e.what, err)
	}
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return fmt.Errorf("memgraph: ensure %s edge result: %w", e.what, err)
		}
		return fmt.Errorf("memgraph: ensure %s edge: no Politician %q or %s %q in graph",
			e.what, e.politician, e.targetLabel, e.targetID)
	}
	return nil
}

func buildCreationReason(np *NodeProvenance) string {
	parts := []string{}
	if np.Source != "" {
		parts = append(parts, "discovered by "+np.Source)
	}
	if np.Tribunal != "" {
		parts = append(parts, "tribunal "+np.Tribunal)
	}
	if np.ComunicacaoID != "" {
		parts = append(parts, "comunicação "+np.ComunicacaoID)
	}
	if len(parts) == 0 {
		return "no provenance recorded on this node"
	}
	return strings.Join(parts, "; ")
}

func recordString(rec *neo4j.Record, key string) string {
	v, ok := rec.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func recordStrings(rec *neo4j.Record, key string) []string {
	v, ok := rec.Get(key)
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, a := range arr {
		if s, ok := a.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
