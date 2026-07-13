package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Search returns MATCHES, and nothing else.
//
// Its only consumer is the search bar's dropdown, which lists every node in the
// response as a result. So a node that is merely adjacent to a hit is not a
// neighbour here, it is a wrong answer: pulling in the subgraph around each match
// made a search for "lava" offer TCU_IRREGULAR, a bare case number and PAULO
// ROBERTO COSTA as things the user might have meant. Fifteen of twenty results
// contained no "lava" at all.
//
// This is why the queries below return `node` alone and no relationships. They
// are not a graph query and an empty edge list is the right answer, not a bug —
// selecting a result opens the detail panel, it does not draw anything. If a
// future caller does want the connected subgraph for a hit, that belongs in a
// separate query rather than widened into this one.
//
// $q is ALWAYS the folded query (see foldQuery), and every stored value it is
// compared against is either the precomputed folded `search_name` (for the
// name-bearing labels that grow without bound — Person, Politician,
// Organization, Scandal) or, for the low-cardinality secondary fields that do
// not, wrapped in foldExpr at query time. Folding one side and not the other
// matches nothing, and reads as "no results" rather than as a bug, so it is
// applied here once, at the entry point.

// foldedName is the precomputed, folded name written by every name-writer
// (see foldQuery and migration 005_search_name). Comparing against it replaces
// the per-node, query-time fold that made a name search cost O(nodes × ~48
// string replaces) — an 8s scan for a rare name over ~130k nodes, and the hard
// blocker on importing the ~500k-row improbidade registry. coalesce guards the
// transition window: a node written before its writer learned to set
// search_name simply does not match, rather than erroring the whole query.
const foldedName = `coalesce(node.search_name, "")`
func (db *DB) QuerySearch(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	folded := foldQuery(q)
	params := map[string]any{
		"q":        folded,
		"q_digits": digitsOnly(folded),
	}

	var query string
	switch nodeType {
	case "politician":
		query = searchPoliticiansQuery()
	case "person":
		query = searchPersonsQuery()
	case "scandal":
		query = searchScandalsQuery()
	case "organization":
		query = searchOrganizationsQuery()
	case "sanction":
		query = searchSanctionsQuery()
	case "legal_proceeding":
		query = searchProceedingsQuery()
	default:
		// no type filter: search all
		query = searchAllQuery()
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: search: %w", err)
	}

	return collectGraph(ctx, result)
}

func searchPersonsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Person)
    WHERE %s CONTAINS $q
    RETURN node
    LIMIT 20
  `, foldedName)
}

func searchPoliticiansQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Politician)
    WHERE %s CONTAINS $q
       OR ANY(alias IN coalesce(node.name_aliases, []) WHERE %s CONTAINS $q)
    RETURN node
    LIMIT 20
  `, foldedName, foldExpr("alias"))
}

func searchScandalsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Scandal)
    WHERE %s CONTAINS $q
       OR ANY(alias IN coalesce(node.aliases, []) WHERE %s CONTAINS $q)
       OR %s CONTAINS $q
    RETURN node
    LIMIT 20
  `, foldedName, foldExpr("alias"), foldExpr(`coalesce(node.description, "")`))
}

func searchOrganizationsQuery() string {
	return fmt.Sprintf(`
    MATCH (node:Organization)
    WHERE %s CONTAINS $q
    RETURN node
    LIMIT 20
  `, foldedName)
}

func searchSanctionsQuery() string {
	return `
    MATCH (node:Sanction)
    WHERE coalesce(node.search_text, "") CONTAINS $q
    RETURN node
    LIMIT 20
  `
}

func searchProceedingsQuery() string {
	// Case numbers are stored as bare 20 digits (e.g., "50833760520144047000")
	// but humans type the formatted CNJ version with dots and hyphens
	// (e.g., "5083376-05.2014.4.04.7000") or a partial number.
	// To find a match either way, if the query contains mostly digits,
	// strip separators from both and compare; otherwise use normal folding.
	caseNorm := "replace(replace(node.case_number, '-', ''), '.', '')"
	return fmt.Sprintf(`
    MATCH (node:LegalProceeding)
    WHERE (
      ($q_digits <> '' AND %s CONTAINS $q_digits)
      OR %s CONTAINS $q
      OR %s CONTAINS $q
      OR %s CONTAINS $q
    )
    RETURN node
    LIMIT 20
  `,
		caseNorm,
		foldExpr("node.case_number"),
		foldExpr("node.class_name"),
		foldExpr("node.court"))
}

func searchAllQuery() string {
	caseNorm := "replace(replace(node.case_number, '-', ''), '.', '')"
	return fmt.Sprintf(`
    MATCH (node)
    WHERE (
      (node:Politician AND (%s CONTAINS $q OR ANY(alias IN coalesce(node.name_aliases, []) WHERE %s CONTAINS $q)))
      OR (node:Person AND %s CONTAINS $q)
      OR (node:Scandal AND (%s CONTAINS $q OR ANY(alias IN coalesce(node.aliases, []) WHERE %s CONTAINS $q)))
      OR (node:Organization AND %s CONTAINS $q)
      OR (node:Sanction AND coalesce(node.search_text, "") CONTAINS $q)
      OR (node:LegalProceeding AND (
        ($q_digits <> '' AND %s CONTAINS $q_digits)
        OR %s CONTAINS $q
        OR %s CONTAINS $q
        OR %s CONTAINS $q
      ))
    )
    RETURN node
    LIMIT 20
  `,
		foldedName, foldExpr("alias"),
		foldedName,
		foldedName, foldExpr("alias"),
		foldedName,
		caseNorm,
		foldExpr("node.case_number"),
		foldExpr("node.class_name"),
		foldExpr("node.court"))
}
