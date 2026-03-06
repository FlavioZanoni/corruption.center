package memgraph

import (
	"context"
	"fmt"
	"time"

	"corruption-center/api/models"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (db *DB) QueryScandalGraph(ctx context.Context, id string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (s:Scandal {id: $id})
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(s)
    OPTIONAL MATCH (o:Organization)-[ri:IMPLICATED_IN]->(s)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(s)
    RETURN s, p, r, o, ri, lp, inv
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
    RETURN p, r, s, d, lp, m, o
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query politician graph: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryExpandNode(ctx context.Context, id string, hops int) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH path = (n {id: $id})-[*1..$hops]-(m)
    UNWIND nodes(path) AS node
    UNWIND relationships(path) AS rel
    RETURN DISTINCT node, rel
  `, map[string]any{"id": id, "hops": hops})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query expand node: %w", err)
	}

	return collectGraph(ctx, result)
}

func (db *DB) QueryTimeline(ctx context.Context, from time.Time, to time.Time) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
    MATCH (s:Scandal)
    WHERE s.date_start <= $to AND (s.date_end IS NULL OR s.date_end >= $from)
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(s)
    WHERE r.date_from <= $to AND (r.date_to IS NULL OR r.date_to >= $from)
    RETURN s, p, r
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

	for result.Next(ctx) {
		record := result.Record()
		for _, val := range record.Values {
			switch v := val.(type) {
			case neo4j.Node:
				n := neoNodeToModel(v)
				nodeMap[n.ID] = n
			case neo4j.Relationship:
				e := neoRelToModel(v)
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
	name, _ := n.Props["name"].(string)

	return models.Node{
		ID:         id,
		Type:       models.NodeType(label),
		Label:      name,
		Properties: n.Props,
	}
}

func neoRelToModel(r neo4j.Relationship) models.Edge {
	id := fmt.Sprintf("%s", r.ElementId)
	return models.Edge{
		ID:         id,
		From:       fmt.Sprintf("%s", r.StartElementId),
		To:         fmt.Sprintf("%s", r.EndElementId),
		Type:       models.EdgeType(r.Type),
		Properties: r.Props,
	}
}

func nodeToPolit(n neo4j.Node) *models.Politician {
	p := n.Props
	aliases, _ := p["name_aliases"].([]string)
	return &models.Politician{
		ID:            strProp(p, "id"),
		Name:          strProp(p, "name"),
		NameAliases:   aliases,
		PartyCurrent:  strProp(p, "party_current"),
		RoleCurrent:   strProp(p, "role_current"),
		State:         strProp(p, "state"),
		TSEProfileURL: strProp(p, "tse_profile_url"),
		PhotoURL:      strProp(p, "photo_url"),
		Active:        boolProp(p, "active"),
	}
}

func nodeToScandal(n neo4j.Node) *models.Scandal {
	p := n.Props
	aliases, _ := p["aliases"].([]string)
	return &models.Scandal{
		ID:             strProp(p, "id"),
		Name:           strProp(p, "name"),
		Aliases:        aliases,
		Description:    strProp(p, "description"),
		TotalAmountBRL: float64Prop(p, "total_amount_brl"),
		Status:         models.StatusType(strProp(p, "status")),
		WikipediaURL:   strProp(p, "wikipedia_url"),
	}
}

func strProp(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return v
}

func boolProp(p map[string]any, key string) bool {
	v, _ := p[key].(bool)
	return v
}

func float64Prop(p map[string]any, key string) float64 {
	v, _ := p[key].(float64)
	return v
}
