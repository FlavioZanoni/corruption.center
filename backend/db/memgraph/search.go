package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (db *DB) QuerySearch(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	var result neo4j.ResultWithContext
	var err error

	switch nodeType {
	case "politician":
		result, err = searchPoliticians(ctx, session, q)
	case "scandal":
		result, err = searchScandals(ctx, session, q)
	case "organization":
		result, err = searchOrganizations(ctx, session, q)
	default:
		// no type filter — search all
		result, err = searchAll(ctx, session, q)
	}

	if err != nil {
		return nil, fmt.Errorf("memgraph: search: %w", err)
	}

	return collectGraph(ctx, result)
}

func searchPoliticians(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    CALL db.index.fulltext.queryNodes("politician_fulltext", $q)
    YIELD node, score
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    RETURN node, r, s
    ORDER BY score DESC
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchScandals(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    CALL db.index.fulltext.queryNodes("scandal_fulltext", $q)
    YIELD node, score
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(node)
    RETURN node, r, p
    ORDER BY score DESC
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchOrganizations(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    CALL db.index.fulltext.queryNodes("organization_fulltext", $q)
    YIELD node, score
    OPTIONAL MATCH (node)-[r:IMPLICATED_IN]->(s:Scandal)
    RETURN node, r, s
    ORDER BY score DESC
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchAll(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    CALL db.index.fulltext.queryNodes("global_fulltext", $q)
    YIELD node, score
    RETURN node
    ORDER BY score DESC
    LIMIT 20
  `, map[string]any{"q": q})
}
