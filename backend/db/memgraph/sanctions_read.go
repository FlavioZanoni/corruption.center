package memgraph

import (
	"context"
	"fmt"
	"strings"

	"corruption-center/api/models"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const defaultSanctionPageSize = 24

// sanctionOrderBy maps a caller-supplied sort mode onto an ORDER BY clause.
// ORDER BY cannot be parameterized, so the clause is inlined; allowlist
// is the only thing standing between a query string and Cypher injection.
func sanctionOrderBy(sort string) string {
	if sort == "registry" {
		return "sa.registry"
	}
	// Default: newest first — but a sanction with NO date must not sort to the top.
	// A plain "date_start DESC" put the undated rows first, so the opening page of
	// the browse (the first thing anyone sees) was nothing but records with no date
	// and, as it happens, no resolved party either: the least informative rows we
	// have, presented as the headline.
	return "CASE WHEN sa.date_start IS NULL OR sa.date_start = '' THEN 1 ELSE 0 END, sa.date_start DESC, sa.registry"
}

// sanctionListItemFromProps maps a Sanction node and its sanctioned party onto a browse row.
// A sanctioned COMPANY reaches us as a bare CNPJ: CGU and TCU publish the
// document, not the razão social, and the CNPJ enricher has not filled these in.
// So the party's name is very often empty, and a browse row rendered from it alone
// would be blank -- a sanction against nobody. Carry the document too, so the UI can
// always say WHO was sanctioned, even when all we can honestly say is the CNPJ.
func sanctionListItemFromProps(sanctionProps map[string]any, partyID, partyName, partyDocument, partyType string) models.SanctionListItem {
	return models.SanctionListItem{
		ID:                 strProp(sanctionProps, "id"),
		Registry:           strProp(sanctionProps, "registry"),
		Type:               strProp(sanctionProps, "sanction_type"),
		Organ:              strProp(sanctionProps, "organ"),
		DateStart:          timePtrProp(sanctionProps, "date_start"),
		DateEnd:            timePtrProp(sanctionProps, "date_end"),
		ProcessRef:         strProp(sanctionProps, "process_ref"),
		SourceURL:          strProp(sanctionProps, "source_url"),
		SanctionedID:       partyID,
		SanctionedName:     partyName,
		SanctionedDocument: partyDocument,
		SanctionedType:     partyType,
	}
}

// QuerySanctions returns a paginated list of Sanction nodes with their sanctioned parties.
// Filters:
// - registry: exact match on registry field
// - organ: exact match on organ field
// - q: folded case-insensitive substring search on name/registry/type
// Sorts by date_start DESC (default) or registry.
func (db *DB) QuerySanctions(ctx context.Context, registry, organ, q, sort string, page, pageSize int) (*models.SanctionListResponse, error) {
	page, pageSize, skip := clampPaging(page, pageSize, defaultSanctionPageSize)

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	params := map[string]any{"skip": skip, "limit": pageSize}

	// The party is OPTIONAL-matched and folded into the row BEFORE any filtering,
	// because the one search everybody actually types is the name of the sanctioned
	// company or person -- and that name lives on the PARTY node, not on the
	// Sanction. Filtering on sa.name matched nothing at all: a Sanction has no name
	// property, so a search for "odebrecht" in a corruption database returned zero.
	//
	// head(collect(...)) collapses the parties to one row per sanction before SKIP /
	// LIMIT. Paginating the raw match would page over sanction x party ROWS, so a
	// sanction with two parties would quietly eat two slots of a 24-row page and the
	// caller would get 23 items back.
	where := ""
	filters := []string{}
	if registry != "" {
		filters = append(filters, "sa.registry = $registry")
		params["registry"] = registry
	}
	if organ != "" {
		filters = append(filters, "sa.organ = $organ")
		params["organ"] = organ
	}
	if q != "" {
		filters = append(filters, fmt.Sprintf("(%s CONTAINS $q OR %s CONTAINS $q OR %s CONTAINS $q OR %s CONTAINS $q OR %s CONTAINS $q)",
			foldExpr("coalesce(party_name, '')"),
			foldExpr("coalesce(sa.organ, '')"),
			foldExpr("coalesce(sa.registry, '')"),
			foldExpr("coalesce(sa.sanction_type, '')"),
			foldExpr("coalesce(sa.process_ref, '')"),
		))
		params["q"] = foldQuery(q)
	}
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}

	// One shape, used for both the count and the page, so the two can never disagree
	// about what a "row" is.
	base := fmt.Sprintf(`
MATCH (sa:Sanction)
OPTIONAL MATCH (party)-[:SANCTIONED_IN]->(sa)
WITH sa,
     head(collect(party.id)) AS party_id,
     head(collect(party.name)) AS party_name,
     head(collect(coalesce(party.cnpj, party.cpf))) AS party_document,
     head(collect(labels(party)[0])) AS party_type
%s
`, where)

	countRes, err := session.Run(ctx, base+"\nRETURN count(sa) AS total", params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: count sanctions: %w", err)
	}
	total := 0
	if countRes.Next(ctx) {
		if v, ok := countRes.Record().Get("total"); ok {
			total = int(asInt(v))
		}
	}
	if err := countRes.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: count sanctions rows: %w", err)
	}

	res, err := session.Run(ctx, fmt.Sprintf(`%s
RETURN sa, party_id, party_name, party_document, party_type
ORDER BY %s
SKIP $skip LIMIT $limit
`, base, sanctionOrderBy(sort)), params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query sanctions: %w", err)
	}

	items := make([]models.SanctionListItem, 0, pageSize)
	for res.Next(ctx) {
		rec := res.Record()
		saVal, _ := rec.Get("sa")
		sanction, ok := saVal.(neo4j.Node)
		if !ok {
			continue
		}
		items = append(items, sanctionListItemFromProps(
			sanction.Props,
			recString(rec, "party_id"),
			recString(rec, "party_name"),
			recString(rec, "party_document"),
			recString(rec, "party_type"),
		))
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query sanctions rows: %w", err)
	}

	return &models.SanctionListResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// QuerySanction returns a single Sanction by ID with all its sanctioned parties.
func (db *DB) QuerySanction(ctx context.Context, id string) (*models.SanctionDetailResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
    MATCH (sa:Sanction {id: $id})
    OPTIONAL MATCH (party)-[r:SANCTIONED_IN]->(sa)
    RETURN sa, party, labels(party) as party_labels
  `, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("memgraph: query sanction: %w", err)
	}

	var sanction *models.Sanction
	parties := make([]models.SanctionedParty, 0)
	seen := make(map[string]bool)

	for res.Next(ctx) {
		rec := res.Record()

		// Initialize sanction from first record
		if sanction == nil {
			saVal, _ := rec.Get("sa")
			saNode, ok := saVal.(neo4j.Node)
			if !ok {
				continue
			}
			props := saNode.Props
			sanction = &models.Sanction{
				ID:         strProp(props, "id"),
				Registry:   strProp(props, "registry"),
				Type:       strProp(props, "sanction_type"),
				Organ:      strProp(props, "organ"),
				DateStart:  timePtrProp(props, "date_start"),
				DateEnd:    timePtrProp(props, "date_end"),
				ProcessRef: strProp(props, "process_ref"),
				SourceURL:  strProp(props, "source_url"),
			}
		}

		// Process party if present
		partyVal, _ := rec.Get("party")
		if partyVal == nil {
			continue
		}

		partyNode, ok := partyVal.(neo4j.Node)
		if !ok {
			continue
		}

		partyID := strProp(partyNode.Props, "id")
		if partyID == "" || seen[partyID] {
			continue
		}

		seen[partyID] = true
		partyLabels, _ := rec.Get("party_labels")
		var labels []string
		if labelList, ok := partyLabels.([]string); ok {
			labels = labelList
		} else if labelList, ok := partyLabels.([]any); ok {
			for _, l := range labelList {
				if s, ok := l.(string); ok {
					labels = append(labels, s)
				}
			}
		}

		label, _ := primaryLabel(labels)
		parties = append(parties, models.SanctionedParty{
			ID:   partyID,
			Name: strProp(partyNode.Props, "name"),
			Type: label,
		})
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query sanction rows: %w", err)
	}

	if sanction == nil {
		return nil, nil
	}

	return &models.SanctionDetailResponse{
		Sanction: sanction,
		Parties:  parties,
	}, nil
}

// QuerySanctionRegistries returns all distinct registry values with their counts.
func (db *DB) QuerySanctionRegistries(ctx context.Context) (*models.SanctionRegistriesResponse, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
    MATCH (sa:Sanction)
    RETURN sa.registry as registry, count(sa) as count
    ORDER BY count DESC, registry
  `, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query sanction registries: %w", err)
	}

	registries := make([]models.SanctionRegistryItem, 0)
	for res.Next(ctx) {
		rec := res.Record()
		reg, _ := rec.Get("registry")
		cnt, _ := rec.Get("count")
		registries = append(registries, models.SanctionRegistryItem{
			Registry: reg.(string),
			Count:    int(asInt(cnt)),
		})
	}

	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query sanction registries rows: %w", err)
	}

	return &models.SanctionRegistriesResponse{
		Registries: registries,
	}, nil
}
