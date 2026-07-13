package memgraph

import (
	"context"
	"fmt"

	"corruption-center/workers/camara"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (db *DB) UpsertPoliticiansFromCamara(ctx context.Context, records []camara.DeputyRecord, batchSize int) (int, error) {
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
  p.name_aliases = COALESCE(p.name_aliases, []),
  p.tse_profile_urls = COALESCE(p.tse_profile_urls, [])
SET
  p.name = row.name,
  p.search_name = row.search_name,
  p.party_current = row.party_current,
  p.role_current = row.role_current,
  // An empty photo from Camara means "not published", not "delete the one we
  // have". Assigning it blindly would wipe the TSE candidate photo on every sync.
  p.photo_url = CASE WHEN row.photo_url <> '' THEN row.photo_url ELSE p.photo_url END,
  p.photo_source = CASE WHEN row.photo_url <> '' THEN 'camara' ELSE p.photo_source END,
  p.state = row.state,
  p.active = true,
  p.camara_id = row.camara_id
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
				"id":            deterministicPoliticianID(r.CPF),
				"cpf":           r.CPF,
				"name":          r.Name,
				"search_name":   foldQuery(r.Name),
				"party_current": r.PartyCurrent,
				"role_current":  r.RoleCurrent,
				"photo_url":     r.PhotoURL,
				"state":         r.State,
				"camara_id":     r.CamaraID,
			})
		}

		res, err := session.Run(ctx, query, map[string]any{"rows": rows})
		if err != nil {
			return total, fmt.Errorf("memgraph: upsert camara politicians batch: %w", err)
		}
		if res.Next(ctx) {
			if touched, ok := res.Record().Get("touched"); ok {
				if n, ok := touched.(int64); ok {
					total += int(n)
				}
			}
		}
		if err := res.Err(); err != nil {
			return total, fmt.Errorf("memgraph: upsert camara politicians result: %w", err)
		}
	}

	return total, nil
}
