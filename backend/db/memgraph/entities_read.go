package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// QueryPerson returns a Person node by ID with all its connections
// (proceedings and sanctions).
func (db *DB) QueryPerson(ctx context.Context, id string) (*models.PersonProfileResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Fetch the person node
	res, err := session.Run(ctx, `
    MATCH (p:Person {id: $id})
    RETURN p
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query person: %w", err)
	}

	var person *models.Person
	if res.Next(ctx) {
		pVal, _ := res.Record().Get("p")
		pNode, ok := pVal.(neo4j.Node)
		if !ok {
			return nil, nil
		}
		props := pNode.Props
		person = &models.Person{
			ID:                    strProp(props, "id"),
			Name:                  strProp(props, "name"),
			CPF:                   maskCPF(strProp(props, "cpf")),
			ProvenanceSource:      strProp(props, "provenance_source"),
			ProvenanceLink:        strProp(props, "provenance_link"),
			ProvenanceTribunal:    strProp(props, "provenance_tribunal"),
			ProvenanceComunicaoID: strProp(props, "provenance_comunicacao_id"),
			// No full CPF ⇒ the node is keyed by name, so its records may belong to
			// more than one real person. Politician/sanction nodes carry a full CPF
			// and are not flagged.
			Ambiguous: strProp(props, "cpf") == "",
		}
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query person rows: %w", err)
	}

	if person == nil {
		return nil, nil
	}

	// Fetch connections
	connections, err := db.QueryPersonGraph(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.PersonProfileResponse{
		Person:      person,
		Connections: connections,
	}, nil
}

// QueryOrganization returns an Organization node by ID with all its connections
// (controlled_by, controls, proceedings, sanctions).
func (db *DB) QueryOrganization(ctx context.Context, id string) (*models.OrganizationProfileResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Fetch the organization node
	res, err := session.Run(ctx, `
    MATCH (o:Organization {id: $id})
    RETURN o
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query organization: %w", err)
	}

	var org *models.Organization
	if res.Next(ctx) {
		oVal, _ := res.Record().Get("o")
		oNode, ok := oVal.(neo4j.Node)
		if !ok {
			return nil, nil
		}
		props := oNode.Props
		org = &models.Organization{
			ID:               strProp(props, "id"),
			CNPJ:             strProp(props, "cnpj"),
			Name:             strProp(props, "name"),
			Active:           boolProp(props, "active"),
			Type:             strProp(props, "type"),
			UF:               strProp(props, "uf"),
			ShareCapitalBRL:  float64Prop(props, "share_capital_brl"),
			MainActivity:     strProp(props, "main_activity"),
			SourceURL:        strProp(props, "source_url"),
			Enriched:         boolProp(props, "enriched"),
			PhotoURL:         strProp(props, "photo_url"),
			PhotoAttribution: strProp(props, "photo_attribution"),
		}
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query organization rows: %w", err)
	}

	if org == nil {
		return nil, nil
	}

	// Fetch connections
	connections, err := db.QueryOrganizationGraph(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.OrganizationProfileResponse{
		Organization: org,
		Connections:  connections,
	}, nil
}

// QueryPersonGraph returns the graph connections for a Person node.
// This mirrors the pattern in graph.go for other entity types.
func (db *DB) QueryPersonGraph(ctx context.Context, personID string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Get all connected nodes: proceedings (DEFENDANT_IN) and sanctions (SANCTIONED_IN)
	res, err := session.Run(ctx, `
    MATCH (p:Person {id: $id})
    OPTIONAL MATCH (p)-[r:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (p)-[s:SANCTIONED_IN]->(sa:Sanction)
    RETURN p, r, lp, s, sa
  `, map[string]any{"id": personID})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query person graph: %w", err)
	}

	// Build graph response using collectGraph pattern from graph.go
	nodeMap := map[string]models.Node{}
	edgeMap := map[string]models.Edge{}
	elementToDomainID := map[string]string{}

	for res.Next(ctx) {
		rec := res.Record()

		// Collect all nodes
		for _, val := range rec.Values {
			if v, ok := val.(neo4j.Node); ok {
				n := neoNodeToModel(v)
				nodeMap[n.ID] = n
				elementToDomainID[v.ElementId] = n.ID
			}
		}

		// Collect all edges
		for _, val := range rec.Values {
			if v, ok := val.(neo4j.Relationship); ok {
				e := neoRelToModel(v, elementToDomainID)
				edgeMap[e.ID] = e
			}
		}
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query person graph rows: %w", err)
	}

	// Build response
	graphRes := &models.GraphResponse{
		Nodes: make([]models.Node, 0, len(nodeMap)),
		Edges: make([]models.Edge, 0, len(edgeMap)),
	}
	for _, n := range nodeMap {
		graphRes.Nodes = append(graphRes.Nodes, n)
	}
	for _, e := range edgeMap {
		graphRes.Edges = append(graphRes.Edges, e)
	}

	return graphRes, nil
}

// QueryOrganizationGraph returns the graph connections for an Organization node.
func (db *DB) QueryOrganizationGraph(ctx context.Context, orgID string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Get all connected nodes: CONTROLS/CONTROLLED_BY, proceedings, sanctions
	res, err := session.Run(ctx, `
    MATCH (o:Organization {id: $id})
    OPTIONAL MATCH (o)-[ctrl:CONTROLS]->(entity)
    OPTIONAL MATCH (controller)-[cby:CONTROLS]->(o)
    OPTIONAL MATCH (o)-[r:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (o)-[s:SANCTIONED_IN]->(sa:Sanction)
    RETURN o, ctrl, entity, cby, controller, r, lp, s, sa
  `, map[string]any{"id": orgID})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query organization graph: %w", err)
	}

	// Build graph response using collectGraph pattern from graph.go
	nodeMap := map[string]models.Node{}
	edgeMap := map[string]models.Edge{}
	elementToDomainID := map[string]string{}

	for res.Next(ctx) {
		rec := res.Record()

		// Collect all nodes
		for _, val := range rec.Values {
			if v, ok := val.(neo4j.Node); ok {
				n := neoNodeToModel(v)
				nodeMap[n.ID] = n
				elementToDomainID[v.ElementId] = n.ID
			}
		}

		// Collect all edges
		for _, val := range rec.Values {
			if v, ok := val.(neo4j.Relationship); ok {
				e := neoRelToModel(v, elementToDomainID)
				edgeMap[e.ID] = e
			}
		}
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query organization graph rows: %w", err)
	}

	// Build response
	graphRes := &models.GraphResponse{
		Nodes: make([]models.Node, 0, len(nodeMap)),
		Edges: make([]models.Edge, 0, len(edgeMap)),
	}
	for _, n := range nodeMap {
		graphRes.Nodes = append(graphRes.Nodes, n)
	}
	for _, e := range edgeMap {
		graphRes.Edges = append(graphRes.Edges, e)
	}

	return graphRes, nil
}
