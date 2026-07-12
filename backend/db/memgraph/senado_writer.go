package memgraph

import (
	"context"
	"fmt"

	"corruption-center/workers/senado"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (db *DB) UpsertPoliticiansFromSenado(ctx context.Context, records []senado.SenatorRecord, batchSize int) (int, error) {
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
MATCH (p:Politician)
WHERE p.state = row.state
  AND toLower(trim(p.name)) = toLower(trim(row.name))
SET
  p.party_current = row.party_current,
  p.role_current = row.role_current,
  // An empty photo from Senado means "not published", not "delete the one we
  // have". Assigning it blindly would wipe the TSE candidate photo on every sync.
  p.photo_url = CASE WHEN row.photo_url <> '' THEN row.photo_url ELSE p.photo_url END,
  p.photo_source = CASE WHEN row.photo_url <> '' THEN 'senado' ELSE p.photo_source END,
  p.active = true,
  p.senado_id = row.senado_id
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
				"senado_id":     r.SenadoID,
				"name":          r.Name,
				"state":         r.State,
				"party_current": r.PartyCurrent,
				"role_current":  r.RoleCurrent,
				"photo_url":     r.PhotoURL,
			})
		}
		res, err := session.Run(ctx, query, map[string]any{"rows": rows})
		if err != nil {
			return total, fmt.Errorf("memgraph: upsert senado politicians batch: %w", err)
		}
		if res.Next(ctx) {
			if touched, ok := res.Record().Get("touched"); ok {
				if n, ok := touched.(int64); ok {
					total += int(n)
				}
			}
		}
		if err := res.Err(); err != nil {
			return total, fmt.Errorf("memgraph: upsert senado politicians result: %w", err)
		}
	}

	return total, nil
}
