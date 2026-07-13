package memgraph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Attaching a CNPJ to a name-only Organization is the company-side bridge between
// the two islands: DJEN gives us a company's NAME in a criminal case and never a
// document, while CGU/TCU give us a company's CNPJ and never a case. A human
// supplying the missing document is the only thing that can join them — and when
// it does, the sanctioned node and the cited node are the SAME company, so this
// must merge them, not leave two Organizations wearing the same CNPJ.

// AttachOrganizationCNPJ records an operator-supplied CNPJ on a name-only
// Organization. If another Organization already carries that CNPJ (the sanctions
// worker creates one per sanctioned company), the two are the same legal entity:
// srcID's edges are moved onto it and srcID is deleted. Returns the id of the node
// that survives.
func (db *DB) AttachOrganizationCNPJ(ctx context.Context, srcID, cnpj, actor string) (string, error) {
	digits := digitsOnly(cnpj)
	if len(digits) != 14 {
		return "", fmt.Errorf("memgraph: %q is not a 14-digit CNPJ", cnpj)
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	params := map[string]any{
		"src":   srcID,
		"cnpj":  digits,
		"actor": actor,
		"at":    time.Now().UTC().Format(time.RFC3339),
	}

	dstID, err := scalarString(ctx, session, `
MATCH (o:Organization {cnpj: $cnpj})
WHERE o.id <> $src
RETURN o.id AS id
LIMIT 1
`, params, "id")
	if err != nil {
		return "", err
	}

	// Nobody else holds this CNPJ: the name-only node simply gains its document.
	if dstID == "" {
		id, err := scalarString(ctx, session, `
MATCH (o:Organization {id: $src})
SET o.cnpj = $cnpj,
    o.cnpj_source = 'human',
    o.cnpj_recorded_by = $actor,
    o.cnpj_recorded_at = $at
RETURN o.id AS id
`, params, "id")
		if err != nil {
			return "", err
		}
		if id == "" {
			return "", fmt.Errorf("memgraph: no Organization %s", srcID)
		}
		return id, nil
	}

	// Same company, two nodes. Move every edge the src can carry onto the node that
	// holds the document, then delete it. Each rel type needs its own statement:
	// Cypher cannot create a relationship whose type comes from a variable.
	params["dst"] = dstID
	moves := []string{
		`MATCH (src:Organization {id: $src})-[r:DEFENDANT_IN]->(lp:LegalProceeding)
		 MATCH (dst:Organization {id: $dst})
		 MERGE (dst)-[n:DEFENDANT_IN]->(lp)
		 SET n += properties(r)
		 DELETE r`,
		`MATCH (src:Organization {id: $src})-[r:IMPLICATED_IN]->(s:Scandal)
		 MATCH (dst:Organization {id: $dst})
		 MERGE (dst)-[n:IMPLICATED_IN]->(s)
		 SET n += properties(r)
		 DELETE r`,
		`MATCH (p)-[r:CONTROLS]->(src:Organization {id: $src})
		 MATCH (dst:Organization {id: $dst})
		 MERGE (p)-[n:CONTROLS]->(dst)
		 SET n += properties(r)
		 DELETE r`,
	}
	for _, q := range moves {
		if err := run(ctx, session, q, params); err != nil {
			return "", err
		}
	}

	// The name-only node knows the company's name; the sanctions node often knows
	// only its CNPJ. Keep whichever name exists.
	if err := run(ctx, session, `
MATCH (src:Organization {id: $src}), (dst:Organization {id: $dst})
WHERE dst.name IS NULL OR dst.name = ''
SET dst.name = src.name, dst.search_name = src.search_name
`, params); err != nil {
		return "", err
	}

	// A plain DELETE (not DETACH) is the point: if src still holds an edge type this
	// function does not know how to move, the delete fails and the operator sees it,
	// rather than the edge being silently destroyed.
	if err := run(ctx, session, `
MATCH (src:Organization {id: $src})
SET src.merged_into = $dst
DELETE src
`, params); err != nil {
		return "", fmt.Errorf("memgraph: %s still holds edges this merge cannot move; not deleting it: %w", srcID, err)
	}

	if err := run(ctx, session, `
MATCH (dst:Organization {id: $dst})
SET dst.cnpj_source = 'human',
    dst.cnpj_recorded_by = $actor,
    dst.cnpj_recorded_at = $at
`, params); err != nil {
		return "", err
	}
	return dstID, nil
}

func run(ctx context.Context, session neo4j.SessionWithContext, query string, params map[string]any) error {
	res, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("memgraph: attach cnpj: %w", err)
	}
	_, err = res.Consume(ctx)
	if err != nil {
		return fmt.Errorf("memgraph: attach cnpj: %w", err)
	}
	return nil
}

func scalarString(ctx context.Context, session neo4j.SessionWithContext, query string, params map[string]any, key string) (string, error) {
	res, err := session.Run(ctx, query, params)
	if err != nil {
		return "", fmt.Errorf("memgraph: attach cnpj: %w", err)
	}
	if !res.Next(ctx) {
		return "", res.Err()
	}
	v, _ := res.Record().Get(key)
	s, _ := v.(string)
	return s, res.Err()
}
