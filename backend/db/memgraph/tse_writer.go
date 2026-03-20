package memgraph

import (
	"context"
	"fmt"
	"strings"

	"corruption-center/workers/tse"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (db *DB) UpsertPoliticiansFromTSE(ctx context.Context, records []tse.PoliticianRecord, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	if len(records) == 0 {
		return 0, nil
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
UNWIND $rows AS row
MERGE (p:Politician {cpf: row.cpf})
ON CREATE SET
  p.id = row.id,
  p.active = false
SET
  p.name = row.name,
  p.party_current = row.party_current,
  p.state = row.state
WITH p, row,
  COALESCE(p.name_aliases, []) AS current_aliases,
  COALESCE(p.tse_profile_urls, []) AS current_urls
SET p.name_aliases = reduce(acc = current_aliases, x IN row.name_aliases |
  CASE WHEN x IN acc THEN acc ELSE acc + x END
)
SET p.tse_profile_urls = reduce(acc = current_urls, x IN row.tse_profile_urls |
  CASE WHEN x IN acc THEN acc ELSE acc + x END
)
RETURN count(p) AS touched
`

	total := 0
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		rows := make([]map[string]any, 0, end-i)
		for _, r := range records[i:end] {
			rows = append(rows, map[string]any{
				"id":               deterministicPoliticianID(r.CPF),
				"cpf":              r.CPF,
				"name":             r.Name,
				"party_current":    r.PartyCurrent,
				"state":            r.State,
				"name_aliases":     r.NameAliases,
				"tse_profile_urls": r.TSEProfileURLs,
			})
		}

		res, err := session.Run(ctx, query, map[string]any{"rows": rows})
		if err != nil {
			return total, fmt.Errorf("memgraph: upsert politicians batch: %w", err)
		}
		if res.Next(ctx) {
			if touched, ok := res.Record().Get("touched"); ok {
				if n, ok := touched.(int64); ok {
					total += int(n)
				}
			}
		}
		if err := res.Err(); err != nil {
			return total, fmt.Errorf("memgraph: upsert politicians batch result: %w", err)
		}
	}

	return total, nil
}

func deterministicPoliticianID(cpf string) string {
	clean := strings.TrimSpace(cpf)
	return "pol_" + clean
}
