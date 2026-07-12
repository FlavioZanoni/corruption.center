package memgraph

import (
	"context"
	"fmt"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const defaultProceedingPageSize = 24

// proceedingOrderBy maps a caller-supplied sort mode onto an ORDER BY clause.
// ORDER BY cannot be parameterized, so the clause is inlined; allowlist
// is the only thing standing between a query string and Cypher injection.
func proceedingOrderBy(sort string) string {
	if sort == "court" {
		return "lp.court, lp.case_number"
	}
	// Default: case_number (deterministic order for pagination)
	return "lp.case_number"
}

// proceedingListItemFromProps maps a LegalProceeding node's properties onto a
// browse row. Polled is true when has_conviction IS NOT NULL (we have polled DataJud),
// false when has_conviction is NULL (we have not polled it yet).
func proceedingListItemFromProps(p map[string]any) models.ProceedingListItem {
	// has_conviction field: can be NULL (never polled), true, or false
	// Polled should be true only when has_conviction is NOT NULL
	polled := false
	if v, ok := p["has_conviction"]; ok && v != nil {
		polled = true
	}

	return models.ProceedingListItem{
		ID:            strProp(p, "id"),
		CaseNumber:    strProp(p, "case_number"),
		Court:         strProp(p, "court"),
		Status:        models.ProceedingStatus(strProp(p, "status")),
		Phase:         strProp(p, "phase"),
		HasConviction: boolProp(p, "has_conviction"),
		Type:          models.ProceedingType(strProp(p, "type")),
		Polled:        polled,
	}
}

// defendantFromNode maps the far end of a DEFENDANT_IN edge onto a defendant
// row, carrying the edge's provenance with it. Type is the node's primary label
// so the frontend can tell a named Politician from an anonymous Person.
func defendantFromNode(labels []string, props, edgeProps map[string]any) models.ProceedingDefendant {
	label, _ := primaryLabel(labels)
	return models.ProceedingDefendant{
		ID:                strProp(props, "id"),
		Name:              strProp(props, "name"),
		Type:              label,
		Outcome:           strProp(edgeProps, "outcome"),
		Source:            strProp(edgeProps, "source"),
		Confidence:        float64PtrProp(edgeProps, "confidence"),
		ConfidenceSignals: strSliceProp(edgeProps, "confidence_signals"),
		Properties:        SanitizeProperties(edgeProps),
	}
}

// float64PtrProp returns nil when the property is absent, so an edge that was
// never scored is distinguishable from one scored at zero.
func float64PtrProp(p map[string]any, key string) *float64 {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return &n
	case int64:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	}
	return nil
}

// QueryProceedings returns a paginated list of LegalProceeding nodes with support
// for filters and sorting.
// Filters:
//   - court: exact match on court field
//   - has_conviction: three-state filter (true, false, or absent/null for all)
//     When true: matches only IS NOT NULL AND = true
//     When false: matches only IS NOT NULL AND = false (excludes NULL)
//     When absent: all rows including NULL
//   - q: folded case-insensitive substring search on case_number or class_name
//
// Sorts by case_number (default) or court. case_number order is deterministic for pagination.
func (db *DB) QueryProceedings(ctx context.Context, court, hasConviction, q, sort string, page, pageSize int) (*models.ProceedingListResponse, error) {
	page, pageSize, skip := clampPaging(page, pageSize, defaultProceedingPageSize)

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Build WHERE clause for filters
	whereClause := "WHERE 1=1"
	params := map[string]any{
		"skip":  skip,
		"limit": pageSize,
	}

	if court != "" {
		whereClause += " AND lp.court = $court"
		params["court"] = court
	}

	// Three-state conviction filter:
	//   "true"    → a court convicted (13 cases)
	//   "false"   → DataJud looked and there is no conviction (1 case)
	//   "unknown" → DataJud has NEVER looked (6,489 cases)
	//   absent    → all of the above
	//
	// "unknown" has to be its own filter value, not the absence of one. Without it
	// the UI's "Não verificado" option sent no filter at all and returned everything
	// — convicted cases included — under a heading promising the opposite. Offering
	// to filter for "we have not checked" and then showing convictions is worse than
	// not offering the filter.
	switch hasConviction {
	case "true":
		whereClause += " AND lp.has_conviction = true"
	case "false":
		// NULL is not false. A case nobody examined has not been cleared.
		whereClause += " AND lp.has_conviction = false AND lp.has_conviction IS NOT NULL"
	case "unknown":
		whereClause += " AND lp.has_conviction IS NULL"
	}

	if q != "" {
		foldedQuery := foldQuery(q)
		whereClause += fmt.Sprintf(" AND (%s CONTAINS $q OR %s CONTAINS $q)",
			foldExpr("lp.case_number"),
			foldExpr("lp.class_name"))
		params["q"] = foldedQuery
	}

	// Count total
	countRes, err := session.Run(ctx, fmt.Sprintf(`
    MATCH (lp:LegalProceeding)
    %s
    RETURN count(lp) AS total
  `, whereClause), params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: count proceedings: %w", err)
	}
	total := 0
	if countRes.Next(ctx) {
		if v, ok := countRes.Record().Get("total"); ok {
			total = int(asInt(v))
		}
	}
	if err := countRes.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: count proceedings rows: %w", err)
	}

	res, err := session.Run(ctx, fmt.Sprintf(`
    MATCH (lp:LegalProceeding)
    %s
    RETURN lp
    ORDER BY %s
    SKIP $skip LIMIT $limit
  `, whereClause, proceedingOrderBy(sort)), params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query proceedings: %w", err)
	}

	items := make([]models.ProceedingListItem, 0, pageSize)
	for res.Next(ctx) {
		nodeVal, _ := res.Record().Get("lp")
		node, ok := nodeVal.(neo4j.Node)
		if !ok {
			continue
		}
		items = append(items, proceedingListItemFromProps(node.Props))
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query proceedings rows: %w", err)
	}

	return &models.ProceedingListResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// QueryProceeding returns one LegalProceeding by node id, with the scandal it
// investigates (if any) and every defendant linked to it. Returns (nil, nil)
// when no such node exists so the handler can answer 404.
func (db *DB) QueryProceeding(ctx context.Context, id string) (*models.ProceedingDetailResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
    MATCH (lp:LegalProceeding {id: $id})
    OPTIONAL MATCH (lp)-[:INVESTIGATES]->(s:Scandal)
    OPTIONAL MATCH (d)-[r:DEFENDANT_IN]->(lp)
    RETURN lp, s, d, r
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query proceeding: %w", err)
	}

	var out *models.ProceedingDetailResponse
	seen := make(map[string]bool)
	for res.Next(ctx) {
		rec := res.Record()

		if out == nil {
			nodeVal, _ := rec.Get("lp")
			node, ok := nodeVal.(neo4j.Node)
			if !ok {
				continue
			}
			p := node.Props
			out = &models.ProceedingDetailResponse{
				ProceedingListItem: proceedingListItemFromProps(p),
				Assuntos:           strSliceProp(p, "assuntos"),
				URL:                strProp(p, "url"),
				Defendants:         make([]models.ProceedingDefendant, 0),
			}
			if sVal, _ := rec.Get("s"); sVal != nil {
				if sNode, ok := sVal.(neo4j.Node); ok {
					out.Scandal = &models.ProceedingScandalRef{
						ID:   strProp(sNode.Props, "id"),
						Name: strProp(sNode.Props, "name"),
					}
				}
			}
		}

		// The scandal OPTIONAL MATCH multiplies rows against the defendant one,
		// so the same defendant can come back more than once: dedupe by node id.
		dVal, _ := rec.Get("d")
		dNode, ok := dVal.(neo4j.Node)
		if !ok {
			continue
		}
		rVal, _ := rec.Get("r")
		rel, ok := rVal.(neo4j.Relationship)
		if !ok {
			continue
		}
		def := defendantFromNode(dNode.Labels, dNode.Props, rel.Props)
		if def.ID == "" || seen[def.ID] {
			continue
		}
		seen[def.ID] = true
		out.Defendants = append(out.Defendants, def)
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query proceeding rows: %w", err)
	}

	return out, nil
}
