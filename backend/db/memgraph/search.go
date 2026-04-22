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
	case "person":
		result, err = searchPersons(ctx, session, q)
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

func searchPersons(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Person)
    WHERE toLower(node.name) CONTAINS toLower($q)
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
    RETURN node, r, s, d, lp
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchPoliticians(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Politician)
    WHERE toLower(node.name) CONTAINS toLower($q)
       OR ANY(alias IN coalesce(node.name_aliases, []) WHERE toLower(alias) CONTAINS toLower($q))
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    RETURN node, r, s
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchScandals(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Scandal)
    WHERE toLower(node.name) CONTAINS toLower($q)
       OR ANY(alias IN coalesce(node.aliases, []) WHERE toLower(alias) CONTAINS toLower($q))
       OR toLower(coalesce(node.description, "")) CONTAINS toLower($q)
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(node)
    RETURN node, r, p
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchOrganizations(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Organization)
    WHERE toLower(node.name) CONTAINS toLower($q)
    OPTIONAL MATCH (node)-[r:IMPLICATED_IN]->(s:Scandal)
    RETURN node, r, s
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchAll(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node)
    WHERE (
      (node:Politician AND (toLower(node.name) CONTAINS toLower($q) OR ANY(alias IN coalesce(node.name_aliases, []) WHERE toLower(alias) CONTAINS toLower($q))))
      OR (node:Person AND toLower(node.name) CONTAINS toLower($q))
      OR (node:Scandal AND (toLower(node.name) CONTAINS toLower($q) OR ANY(alias IN coalesce(node.aliases, []) WHERE toLower(alias) CONTAINS toLower($q))))
      OR (node:Organization AND toLower(node.name) CONTAINS toLower($q))
    )
    RETURN node
    LIMIT 20
  `, map[string]any{"q": q})
}
