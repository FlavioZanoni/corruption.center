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
	case "sanction":
		result, err = searchSanctions(ctx, session, q)
	default:
		// no type filter: search all
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
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, d, lp, sanc, san
    LIMIT 20
  `, map[string]any{"q": q})
}

// searchPoliticians walks DEFENDANT_IN as well as INVOLVED_IN. A politician
// reaches a scandal through the cases they are a defendant in, so omitting it
// meant a searched politician could never show a proceeding — the same gap
// searchScandals had from the other end.
func searchPoliticians(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Politician)
    WHERE toLower(node.name) CONTAINS toLower($q)
       OR ANY(alias IN coalesce(node.name_aliases, []) WHERE toLower(alias) CONTAINS toLower($q))
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (lp)-[inv:INVESTIGATES]->(ls:Scandal)
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, d, lp, inv, ls, sanc, san
    LIMIT 20
  `, map[string]any{"q": q})
}

// searchScandals returns a matching scandal WITH the subgraph that connects it.
//
// It used to walk only INVOLVED_IN, which no worker writes — zero exist — so a
// search answered with scandals and no edges at all, and the canvas drew them as
// floating unconnected bubbles. That is the same defect QueryTimeline had; the
// real path from a scandal to a person runs through its cases:
//
//	(Politician|Person|Organization)-[:DEFENDANT_IN]->(LegalProceeding)-[:INVESTIGATES]->(Scandal)
//
// INVOLVED_IN is still matched because a human can create one in the backoffice.
func searchScandals(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Scandal)
    WHERE toLower(node.name) CONTAINS toLower($q)
       OR ANY(alias IN coalesce(node.aliases, []) WHERE toLower(alias) CONTAINS toLower($q))
       OR toLower(coalesce(node.description, "")) CONTAINS toLower($q)
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(node)
    OPTIONAL MATCH (o:Organization)-[ri:IMPLICATED_IN]->(node)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(node)
    OPTIONAL MATCH (d)-[def:DEFENDANT_IN]->(lp)
    RETURN node, r, p, o, ri, lp, inv, d, def
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchOrganizations(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Organization)
    WHERE toLower(node.name) CONTAINS toLower($q)
    OPTIONAL MATCH (node)-[r:IMPLICATED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, sanc, san
    LIMIT 20
  `, map[string]any{"q": q})
}

func searchSanctions(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node:Sanction)
    WHERE toLower(coalesce(node.registry, "")) CONTAINS toLower($q)
       OR toLower(coalesce(node.sanction_type, "")) CONTAINS toLower($q)
       OR toLower(coalesce(node.organ, "")) CONTAINS toLower($q)
       OR toLower(coalesce(node.process_ref, "")) CONTAINS toLower($q)
    OPTIONAL MATCH (subj)-[r:SANCTIONED_IN]->(node)
    RETURN node, r, subj
    LIMIT 20
  `, map[string]any{"q": q})
}

// searchAll is the default search — no type filter — and so the one most users
// actually hit. It returned `RETURN node` and nothing else: no relationships at
// all, so every result was a set of unconnected nodes and the canvas could only
// ever draw floating bubbles from it. Whatever the graph knew about how those
// nodes relate, a search never asked.
//
// It now returns the edges around each hit: what the node points at, and — since
// a Scandal is reached through its cases rather than directly — the cases that
// investigate it and their defendants.
//
// LIMIT applies to the nodes, before the relationships fan the rows out; putting
// it at the end would cap rows instead and silently truncate a hit's edges.
func searchAll(ctx context.Context, session neo4j.SessionWithContext, q string) (neo4j.ResultWithContext, error) {
	return session.Run(ctx, `
    MATCH (node)
    WHERE (
      (node:Politician AND (toLower(node.name) CONTAINS toLower($q) OR ANY(alias IN coalesce(node.name_aliases, []) WHERE toLower(alias) CONTAINS toLower($q))))
      OR (node:Person AND toLower(node.name) CONTAINS toLower($q))
      OR (node:Scandal AND (toLower(node.name) CONTAINS toLower($q) OR ANY(alias IN coalesce(node.aliases, []) WHERE toLower(alias) CONTAINS toLower($q))))
      OR (node:Organization AND toLower(node.name) CONTAINS toLower($q))
      OR (node:Sanction AND (toLower(coalesce(node.registry, "")) CONTAINS toLower($q) OR toLower(coalesce(node.organ, "")) CONTAINS toLower($q) OR toLower(coalesce(node.sanction_type, "")) CONTAINS toLower($q)))
    )
    WITH node LIMIT 20
    OPTIONAL MATCH (node)-[out:INVOLVED_IN|DEFENDANT_IN|IMPLICATED_IN|SANCTIONED_IN]->(target)
    OPTIONAL MATCH (target)-[tinv:INVESTIGATES]->(tscandal:Scandal)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(node)
    OPTIONAL MATCH (party)-[pdef:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (subj)-[sanc:SANCTIONED_IN]->(node)
    RETURN node, out, target, tinv, tscandal, lp, inv, party, pdef, subj, sanc
  `, map[string]any{"q": q})
}
