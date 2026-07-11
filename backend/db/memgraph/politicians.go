package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const defaultPoliticianPageSize = 24

// QueryPoliticians returns a paginated, filterable list of Politician nodes.
// It exists so a fresh install (politicians synced, no scandals yet) still has
// browsable content. Sanction and proceeding counts are cheap aggregate hops.
func (db *DB) QueryPoliticians(ctx context.Context, filter, party, uf string, page, pageSize int) (*models.PoliticianListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPoliticianPageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}
	skip := (page - 1) * pageSize

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	params := map[string]any{
		"filter": filter,
		"party":  party,
		"uf":     uf,
		"skip":   skip,
		"limit":  pageSize,
	}

	countRes, err := session.Run(ctx, `
    MATCH (p:Politician)
    WHERE ($filter = '' OR toLower(p.name) CONTAINS toLower($filter))
      AND ($party = '' OR p.party_current = $party)
      AND ($uf = '' OR p.state = $uf)
    RETURN count(p) AS total
  `, params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: count politicians: %w", err)
	}
	total := 0
	if countRes.Next(ctx) {
		if v, ok := countRes.Record().Get("total"); ok {
			total = int(asInt(v))
		}
	}
	if err := countRes.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: count politicians rows: %w", err)
	}

	res, err := session.Run(ctx, `
    MATCH (p:Politician)
    WHERE ($filter = '' OR toLower(p.name) CONTAINS toLower($filter))
      AND ($party = '' OR p.party_current = $party)
      AND ($uf = '' OR p.state = $uf)
    OPTIONAL MATCH (p)-[:SANCTIONED_IN]->(san:Sanction)
    OPTIONAL MATCH (p)-[:DEFENDANT_IN]->(lp:LegalProceeding)
    WITH p, count(DISTINCT san) AS sanction_count, count(DISTINCT lp) AS proceeding_count
    RETURN p, sanction_count, proceeding_count
    ORDER BY p.name
    SKIP $skip LIMIT $limit
  `, params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query politicians: %w", err)
	}

	items := make([]models.PoliticianListItem, 0, pageSize)
	for res.Next(ctx) {
		rec := res.Record()
		nodeVal, _ := rec.Get("p")
		node, ok := nodeVal.(neo4j.Node)
		if !ok {
			continue
		}
		p := node.Props
		scVal, _ := rec.Get("sanction_count")
		pcVal, _ := rec.Get("proceeding_count")
		items = append(items, models.PoliticianListItem{
			ID:               strProp(p, "id"),
			Name:             strProp(p, "name"),
			PartyCurrent:     strProp(p, "party_current"),
			RoleCurrent:      strProp(p, "role_current"),
			State:            strProp(p, "state"),
			PhotoURL:         strProp(p, "photo_url"),
			PhotoAttribution: strProp(p, "photo_attribution"),
			SanctionCount:    int(asInt(scVal)),
			ProceedingCount:  int(asInt(pcVal)),
		})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query politicians rows: %w", err)
	}

	return &models.PoliticianListResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// asInt coerces the various numeric types the driver may return for count()
// aggregates (int64 in practice) into a plain int64.
func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
