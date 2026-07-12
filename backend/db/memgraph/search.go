package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// $q is ALWAYS the folded query (see foldQuery), and every stored value it is
// compared against is wrapped in foldExpr. Comparing a raw $q against a folded
// value — or the reverse — matches nothing, and reads as "no results" rather than
// as a bug, so the folding is applied here once, at the entry point, and the
// query builders below take it as given.
func (db *DB) QuerySearch(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	params := map[string]any{"q": foldQuery(q)}

	var result neo4j.ResultWithContext
	var err error

	switch nodeType {
	case "politician":
		result, err = session.Run(ctx, searchPoliticiansQuery(), params)
	case "person":
		result, err = session.Run(ctx, searchPersonsQuery(), params)
	case "scandal":
		result, err = session.Run(ctx, searchScandalsQuery(), params)
	case "organization":
		result, err = session.Run(ctx, searchOrganizationsQuery(), params)
	case "sanction":
		result, err = session.Run(ctx, searchSanctionsQuery(), params)
	default:
		// no type filter: search all
		result, err = session.Run(ctx, searchAllQuery(), params)
	}

	if err != nil {
		return nil, fmt.Errorf("memgraph: search: %w", err)
	}

	return collectGraph(ctx, result)
}

func searchPersonsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Person)
    WHERE %s CONTAINS $q
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (lp)-[inv:INVESTIGATES]->(ls:Scandal)
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, d, lp, inv, ls, sanc, san
    LIMIT 20
  `, foldExpr("node.name"))
}

// searchPoliticiansQuery walks DEFENDANT_IN as well as INVOLVED_IN. A politician
// reaches a scandal through the cases they are a defendant in, so omitting it
// meant a searched politician could never show a proceeding.
func searchPoliticiansQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Politician)
    WHERE %s CONTAINS $q
       OR ANY(alias IN coalesce(node.name_aliases, []) WHERE %s CONTAINS $q)
    OPTIONAL MATCH (node)-[r:INVOLVED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[d:DEFENDANT_IN]->(lp:LegalProceeding)
    OPTIONAL MATCH (lp)-[inv:INVESTIGATES]->(ls:Scandal)
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, d, lp, inv, ls, sanc, san
    LIMIT 20
  `, foldExpr("node.name"), foldExpr("alias"))
}

// searchScandalsQuery returns a matching scandal WITH the subgraph that connects
// it. It used to walk only INVOLVED_IN, which no worker writes — zero exist — so
// a search answered with scandals and no edges, and the canvas drew them as
// floating bubbles. The real path from a scandal to a person runs through its
// cases:
//
//	(Politician|Person|Organization)-[:DEFENDANT_IN]->(LegalProceeding)-[:INVESTIGATES]->(Scandal)
//
// INVOLVED_IN is still matched because a human can create one in the backoffice.
func searchScandalsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Scandal)
    WHERE %s CONTAINS $q
       OR ANY(alias IN coalesce(node.aliases, []) WHERE %s CONTAINS $q)
       OR %s CONTAINS $q
    OPTIONAL MATCH (p:Politician)-[r:INVOLVED_IN]->(node)
    OPTIONAL MATCH (o:Organization)-[ri:IMPLICATED_IN]->(node)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(node)
    OPTIONAL MATCH (d)-[def:DEFENDANT_IN]->(lp)
    RETURN node, r, p, o, ri, lp, inv, d, def
    LIMIT 20
  `, foldExpr("node.name"), foldExpr("alias"), foldExpr(`coalesce(node.description, "")`))
}

func searchOrganizationsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Organization)
    WHERE %s CONTAINS $q
    OPTIONAL MATCH (node)-[r:IMPLICATED_IN]->(s:Scandal)
    OPTIONAL MATCH (node)-[sanc:SANCTIONED_IN]->(san:Sanction)
    RETURN node, r, s, sanc, san
    LIMIT 20
  `, foldExpr("node.name"))
}

func searchSanctionsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Sanction)
    WHERE %s CONTAINS $q
       OR %s CONTAINS $q
       OR %s CONTAINS $q
       OR %s CONTAINS $q
    OPTIONAL MATCH (subj)-[r:SANCTIONED_IN]->(node)
    RETURN node, r, subj
    LIMIT 20
  `,
		foldExpr(`coalesce(node.registry, "")`),
		foldExpr(`coalesce(node.sanction_type, "")`),
		foldExpr(`coalesce(node.organ, "")`),
		foldExpr(`coalesce(node.process_ref, "")`))
}

// searchAllQuery is the default search — no type filter — and so the one most
// users actually hit. It returned `RETURN node` and nothing else: no
// relationships at all, so every result was a set of unconnected nodes and the
// canvas could only ever draw floating bubbles from it.
//
// It now returns the edges around each hit: what the node points at, and — since
// a Scandal is reached through its cases rather than directly — the cases that
// investigate it and their defendants.
//
// LIMIT applies to the nodes, before the relationships fan the rows out; putting
// it at the end would cap rows instead and silently truncate a hit's edges.
func searchAllQuery() string {
	return fmt.Sprintf(`
    MATCH (node)
    WHERE (
      (node:Politician AND (%s CONTAINS $q OR ANY(alias IN coalesce(node.name_aliases, []) WHERE %s CONTAINS $q)))
      OR (node:Person AND %s CONTAINS $q)
      OR (node:Scandal AND (%s CONTAINS $q OR ANY(alias IN coalesce(node.aliases, []) WHERE %s CONTAINS $q)))
      OR (node:Organization AND %s CONTAINS $q)
      OR (node:Sanction AND (%s CONTAINS $q OR %s CONTAINS $q OR %s CONTAINS $q))
    )
    WITH node LIMIT 20
    OPTIONAL MATCH (node)-[out:INVOLVED_IN|DEFENDANT_IN|IMPLICATED_IN|SANCTIONED_IN]->(target)
    OPTIONAL MATCH (target)-[tinv:INVESTIGATES]->(tscandal:Scandal)
    OPTIONAL MATCH (lp:LegalProceeding)-[inv:INVESTIGATES]->(node)
    OPTIONAL MATCH (party)-[pdef:DEFENDANT_IN]->(lp)
    OPTIONAL MATCH (subj)-[sanc:SANCTIONED_IN]->(node)
    RETURN node, out, target, tinv, tscandal, lp, inv, party, pdef, subj, sanc
  `,
		foldExpr("node.name"), foldExpr("alias"),
		foldExpr("node.name"),
		foldExpr("node.name"), foldExpr("alias"),
		foldExpr("node.name"),
		foldExpr(`coalesce(node.registry, "")`),
		foldExpr(`coalesce(node.organ, "")`),
		foldExpr(`coalesce(node.sanction_type, "")`))
}
