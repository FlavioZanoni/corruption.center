package memgraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"corruption-center/api/models"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

func (db *DB) QueryScandalGraph(ctx context.Context, id string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (s:Scandal {id: $id})
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(s)
    OPTIONAL MATCH (person:Person)-[pr:INVOLVED_IN]->(s)
    OPTIONAL MATCH (o:Organization)-[ri:IMPLICATED_IN]->(s)
    OPTIONAL MATCH (s)-[rel:RELATED_TO]-(s2:Scandal)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(s)
    OPTIONAL MATCH (p2:Politician)-[d1:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (person2:Person)-[d2:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (o2:Organization)-[d3:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (src:Source)-[sup:SUPPORTS]->(s)
    RETURN s, p, r, person, pr, o, ri, s2, rel, lp, inv, p2, d1, person2, d2, o2, d3, src, sup
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query scandal graph: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryPoliticianGraph(ctx context.Context, id string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (p:Politician {id: $id})
    OPTIONAL MATCH (p)-[r:INVOLVED_IN]->(s:Scandal)
    OPTIONAL MATCH (p)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (p)-[m:MEMBER_OF]->(o:Organization)
    OPTIONAL MATCH (p)-[c:CONTROLS]->(o2:Organization)
    OPTIONAL MATCH (p)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN p, r, s, d, lp, m, o, c, o2, sanc, san
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query politician graph: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryExpandNode(ctx context.Context, id string, hops int) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Memgraph rejects property-map matching and parameterized bounds inside
	// variable-length patterns, so filter via WHERE and inline the clamped
	// integer bound.
	if hops < 1 {
		hops = 1
	}
	if hops > 3 {
		hops = 3
	}
	query := fmt.Sprintf(`
    MATCH path = (n)-[*1..%d]-(m)
    WHERE n.id = $id
    UNWIND nodes(path) AS node
    UNWIND relationships(path) AS rel
    RETURN DISTINCT node, rel
  `, hops)
	result, err := session.Run(ctx, query, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query expand node: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryTimeline(ctx context.Context, from time.Time, to time.Time) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// The timeline feeds the graph canvas, which builds its whole adjacency map
	// from this one response: it draws scandals by default and reveals a node's
	// neighbours on click. So it has to return the edges, not just the scandals.
	//
	// It used to ask only for (:Politician)-[:INVOLVED_IN]->(:Scandal). Nothing
	// writes that edge except a human in the backoffice; the workers connect people
	// to scandals through the cases instead, via
	//
	//   (s:Scandal)<-[:INVESTIGATES]-(:LegalProceeding)<-[:DEFENDANT_IN]-(person)
	//
	// so the canvas received scandals with no edges at all and clicking one
	// revealed nothing, while the detail panel (which queries the scandal directly
	// and does walk the cases) happily listed the connections it could not draw.
	result, err := session.Run(ctx, `
    MATCH (s:Scandal)
    WHERE s.date_start <= $to AND (s.date_end IS NULL OR s.date_end >= $from)
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(s)
    WHERE r.date_from <= $to AND (r.date_to IS NULL OR r.date_to >= $from)
    OPTIONAL MATCH (person:Person)-[pr:INVOLVED_IN]->(s)
    OPTIONAL MATCH (o:Organization)-[ri:IMPLICATED_IN]->(s)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(s)
    OPTIONAL MATCH (p2:Politician)-[d1:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (person2:Person)-[d2:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (o2:Organization)-[d3:DEFENDANT_IN]->(lp)
    RETURN s, p, r, person, pr, o, ri, lp, inv, p2, d1, person2, d2, o2, d3
  `, map[string]any{
		"from": from.Format("2006-01-02"),
		"to":   to.Format("2006-01-02"),
	})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query timeline: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryPolitician(ctx context.Context, id string) (*models.Politician, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (p:Politician {id: $id}) RETURN p
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query politician: %w", err)
	}

	if result.Next(ctx) {
		node, _ := result.Record().Get("p")
		return nodeToPolit(node.(neo4j.Node)), nil
	}

	return nil, nil
}

func (db *DB) QueryScandal(ctx context.Context, id string) (*models.Scandal, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (s:Scandal {id: $id}) RETURN s
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query scandal: %w", err)
	}

	if result.Next(ctx) {
		node, _ := result.Record().Get("s")
		return nodeToScandal(node.(neo4j.Node)), nil
	}

	return nil, nil
}

// collectGraph walks a result set and builds a GraphResponse, deduplicating
// nodes and edges by ID.
func collectGraph(ctx context.Context, result neo4j.ResultWithContext) (*models.GraphResponse, error) {
	nodeMap := map[string]models.Node{}
	edgeMap := map[string]models.Edge{}
	elementToDomainID := map[string]string{}

	for result.Next(ctx) {
		record := result.Record()

		for _, val := range record.Values {
			if v, ok := val.(neo4j.Node); ok {
				n := neoNodeToModel(v)
				nodeMap[n.ID] = n
				elementToDomainID[v.ElementId] = n.ID
			}
		}

		for _, val := range record.Values {
			switch v := val.(type) {
			case neo4j.Relationship:
				e := neoRelToModel(v, elementToDomainID)
				edgeMap[e.ID] = e
			}
		}
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: collect graph: %w", err)
	}

	resp := &models.GraphResponse{
		Nodes: make([]models.Node, 0, len(nodeMap)),
		Edges: make([]models.Edge, 0, len(edgeMap)),
	}
	for _, n := range nodeMap {
		resp.Nodes = append(resp.Nodes, n)
	}
	for _, e := range edgeMap {
		resp.Edges = append(resp.Edges, e)
	}

	return resp, nil
}

func neoNodeToModel(n neo4j.Node) models.Node {
	label := ""
	if len(n.Labels) > 0 {
		label = n.Labels[0]
	}

	id, _ := n.Props["id"].(string)
	name := strProp(n.Props, "name")
	if name == "" {
		name = strProp(n.Props, "case_number")
	}
	if name == "" {
		name = strProp(n.Props, "registry")
	}
	if name == "" {
		name = id
	}

	return models.Node{
		ID:         id,
		Type:       labelToNodeType(label),
		Label:      name,
		Properties: n.Props,
	}
}

func neoRelToModel(r neo4j.Relationship, elementToDomainID map[string]string) models.Edge {
	id := fmt.Sprintf("%s", r.ElementId)
	from := fmt.Sprintf("%s", r.StartElementId)
	to := fmt.Sprintf("%s", r.EndElementId)
	if mapped, ok := elementToDomainID[r.StartElementId]; ok {
		from = mapped
	}
	if mapped, ok := elementToDomainID[r.EndElementId]; ok {
		to = mapped
	}

	return models.Edge{
		ID:         id,
		From:       from,
		To:         to,
		Type:       models.EdgeType(r.Type),
		Properties: r.Props,
	}
}

func nodeToPolit(n neo4j.Node) *models.Politician {
	p := n.Props
	return &models.Politician{
		ID:               strProp(p, "id"),
		Name:             strProp(p, "name"),
		CPF:              strProp(p, "cpf"),
		NameAliases:      strSliceProp(p, "name_aliases"),
		PartyCurrent:     strProp(p, "party_current"),
		RoleCurrent:      strProp(p, "role_current"),
		State:            strProp(p, "state"),
		TSEProfileURLs:   strSliceProp(p, "tse_profile_urls"),
		PhotoURL:         strProp(p, "photo_url"),
		PhotoSource:      strProp(p, "photo_source"),
		PhotoAttribution: strProp(p, "photo_attribution"),
		Active:           boolProp(p, "active"),
	}
}

func nodeToScandal(n neo4j.Node) *models.Scandal {
	p := n.Props
	return &models.Scandal{
		ID:             strProp(p, "id"),
		Name:           strProp(p, "name"),
		Aliases:        strSliceProp(p, "aliases"),
		Description:    strProp(p, "description"),
		DateStart:      timeProp(p, "date_start"),
		DateEnd:        timePtrProp(p, "date_end"),
		TotalAmountBRL: float64Prop(p, "total_amount_brl"),
		Status:         models.StatusType(strProp(p, "status")),
		WikipediaURL:   strProp(p, "wikipedia_url"),
	}
}

func labelToNodeType(label string) models.NodeType {
	switch strings.ToLower(label) {
	case "politician":
		return models.NodeTypePolitician
	case "person":
		return models.NodeTypePerson
	case "scandal":
		return models.NodeTypeScandal
	case "organization":
		return models.NodeTypeOrganization
	case "legalproceeding":
		return models.NodeTypeLegalProceeding
	case "source":
		return models.NodeTypeSource
	case "sanction":
		return models.NodeTypeSanction
	default:
		return models.NodeType(strings.ToLower(label))
	}
}

func strProp(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return v
}

func strSliceProp(p map[string]any, key string) []string {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	if vv, ok := v.([]string); ok {
		return vv
	}
	if vv, ok := v.([]any); ok {
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func boolProp(p map[string]any, key string) bool {
	v, _ := p[key].(bool)
	return v
}

func float64Prop(p map[string]any, key string) float64 {
	v, _ := p[key].(float64)
	return v
}

func timeProp(p map[string]any, key string) time.Time {
	v, ok := p[key]
	if !ok || v == nil {
		return time.Time{}
	}
	if t, ok := toTime(v); ok {
		return t
	}
	return time.Time{}
}

func timePtrProp(p map[string]any, key string) *time.Time {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	t, ok := toTime(v)
	if !ok {
		return nil
	}
	return &t
}

func toTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case dbtype.Date:
		return t.Time(), true
	case string:
		parsed, err := time.Parse("2006-01-02", t)
		if err == nil {
			return parsed, true
		}
		parsed, err = time.Parse(time.RFC3339, t)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
