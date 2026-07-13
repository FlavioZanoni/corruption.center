package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const defaultScandalPageSize = 24

// clampPaging normalizes a 1-based page / page size pair into a (page, pageSize,
// skip) triple, defaulting an absent size and capping it so a caller cannot ask
// the graph for an unbounded slab.
func clampPaging(page, pageSize, defaultSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultSize
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

// scandalOrderBy maps a caller-supplied sort mode onto an ORDER BY clause.
// ORDER BY cannot be parameterized, so the clause is inlined; which makes an
// allowlist the only thing standing between a query string and Cypher
// injection. Anything unrecognized falls back to the default.
func scandalOrderBy(sort string) string {
	if sort == "name" {
		return "s.name"
	}
	return "s.date_start DESC, s.name"
}

// scandalListItemFromProps maps a Scandal node's properties plus its two
// aggregate counts onto a browse row.
func scandalListItemFromProps(p map[string]any, politicianCount, proceedingCount int) models.ScandalListItem {
	return models.ScandalListItem{
		ID:              strProp(p, "id"),
		Name:            strProp(p, "name"),
		Description:     strProp(p, "description"),
		DateStart:       timeProp(p, "date_start"),
		DateEnd:         timePtrProp(p, "date_end"),
		Status:          models.StatusType(strProp(p, "status")),
		PoliticianCount: politicianCount,
		ProceedingCount: proceedingCount,
	}
}

// QueryScandals returns a paginated list of Scandal nodes. It backs both the
// public browse grid and the SEO sitemap, so every scandal must be reachable by
// walking the pages; hence the stable total and the deterministic ORDER BY.
func (db *DB) QueryScandals(ctx context.Context, sort string, page, pageSize int) (*models.ScandalListResponse, error) {
	page, pageSize, skip := clampPaging(page, pageSize, defaultScandalPageSize)

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	params := map[string]any{
		"skip":  skip,
		"limit": pageSize,
	}

	countRes, err := session.Run(ctx, `
    MATCH (s:Scandal)
    RETURN count(s) AS total
  `, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: count scandals: %w", err)
	}
	total := 0
	if countRes.Next(ctx) {
		if v, ok := countRes.Record().Get("total"); ok {
			total = int(asInt(v))
		}
	}
	if err := countRes.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: count scandals rows: %w", err)
	}

	// politician_count counted only INVOLVED_IN, which no worker writes, so it read
	// 0 on every scandal while proceeding_count beside it was correct — a silent
	// zero, not an obvious break. A politician reaches a scandal either directly
	// (a human links them in the backoffice) or by being a defendant in one of its
	// cases, so count both. count(DISTINCT p) over one pattern is what de-duplicates
	// someone who is both; collecting the two paths separately and adding them would
	// count that person twice.
	// Drive the politician count FROM the scandal, not from a bare
	// `OPTIONAL MATCH (p:Politician)` — that scanned all politicians once per
	// scandal (a ScanAll the relationship makes unnecessary). Traversing inward
	// from s only touches politicians actually connected to it. The two paths are
	// unioned by node identity so someone who is both directly involved and a
	// defendant is counted once, matching the old count(DISTINCT p).
	res, err := session.Run(ctx, fmt.Sprintf(`
    MATCH (s:Scandal)
    OPTIONAL MATCH (lp:LegalProceeding)-[:INVESTIGATES]->(s)
    WITH s, count(DISTINCT lp) AS proceeding_count
    OPTIONAL MATCH (s)<-[:INVOLVED_IN]-(p:Politician)
    WITH s, proceeding_count, collect(DISTINCT p) AS direct
    OPTIONAL MATCH (s)<-[:INVESTIGATES]-(:LegalProceeding)<-[:DEFENDANT_IN]-(p2:Politician)
    WITH s, proceeding_count, direct, collect(DISTINCT p2) AS via_cases
    WITH s, proceeding_count,
         size(direct + [x IN via_cases WHERE NOT x IN direct]) AS politician_count
    RETURN s, politician_count, proceeding_count
    ORDER BY %s
    SKIP $skip LIMIT $limit
  `, scandalOrderBy(sort)), params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query scandals: %w", err)
	}

	items := make([]models.ScandalListItem, 0, pageSize)
	for res.Next(ctx) {
		rec := res.Record()
		nodeVal, _ := rec.Get("s")
		node, ok := nodeVal.(neo4j.Node)
		if !ok {
			continue
		}
		polVal, _ := rec.Get("politician_count")
		procVal, _ := rec.Get("proceeding_count")
		items = append(items, scandalListItemFromProps(node.Props, int(asInt(polVal)), int(asInt(procVal))))
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query scandals rows: %w", err)
	}

	return &models.ScandalListResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}
