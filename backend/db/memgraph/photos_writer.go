package memgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// PoliticianPhotoTarget is a Politician that currently has no photo. The photos
// worker never overwrites a non-empty photo_url (set by the camara/senado
// syncers for current politicians): the read query filters those out and the
// write query re-checks, so the rule holds even across concurrent writers.
type PoliticianPhotoTarget struct {
	ID       string
	CPF      string
	Name     string
	State    string
	Aliases  []string
	PhotoURL string // current value; always empty for returned targets
}

// ListPoliticiansNeedingPhoto returns Politicians whose photo_url is unset,
// optionally filtered by UF (state). limit <= 0 returns all.
func (db *DB) ListPoliticiansNeedingPhoto(ctx context.Context, uf string, limit int) ([]PoliticianPhotoTarget, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	uf = strings.ToUpper(strings.TrimSpace(uf))
	query := `
MATCH (p:Politician)
WHERE (p.photo_url IS NULL OR p.photo_url = '')
  AND ($uf = '' OR p.state = $uf)
RETURN p.id AS id, p.cpf AS cpf, p.name AS name, p.state AS state, p.name_aliases AS aliases
`
	if limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", limit)
	}

	res, err := session.Run(ctx, query, map[string]any{"uf": uf})
	if err != nil {
		return nil, fmt.Errorf("memgraph: list politicians needing photo: %w", err)
	}
	out := make([]PoliticianPhotoTarget, 0)
	for res.Next(ctx) {
		rec := res.Record()
		id, _ := valStr(rec, "id")
		cpf, _ := valStr(rec, "cpf")
		if strings.TrimSpace(cpf) == "" {
			continue
		}
		name, _ := valStr(rec, "name")
		state, _ := valStr(rec, "state")
		out = append(out, PoliticianPhotoTarget{
			ID:      id,
			CPF:     cpf,
			Name:    name,
			State:   state,
			Aliases: valStrSlice(rec, "aliases"),
		})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: iterate politicians needing photo: %w", err)
	}
	return out, nil
}

// SetPoliticianPhotoByCPF sets photo_url/photo_source/photo_attribution on a
// Politician, but ONLY when photo_url is still empty — so a photo set by the
// camara/senado syncers is never overwritten. Returns whether a row was updated.
func (db *DB) SetPoliticianPhotoByCPF(ctx context.Context, cpf, photoURL, source, attribution string) (bool, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (p:Politician {cpf: $cpf})
WHERE p.photo_url IS NULL OR p.photo_url = ''
SET p.photo_url = $photo_url,
    p.photo_source = $source,
    p.photo_attribution = $attribution
RETURN count(p) AS updated
`, map[string]any{
		"cpf":         cpf,
		"photo_url":   photoURL,
		"source":      source,
		"attribution": attribution,
	})
	if err != nil {
		return false, fmt.Errorf("memgraph: set politician photo: %w", err)
	}
	return countTrue(ctx, res)
}

// OrganizationPhotoTarget is an Organization keyed by CNPJ with no image yet.
type OrganizationPhotoTarget struct {
	ID       string
	CNPJ     string
	Name     string
	ImageURL string // current value; always empty for returned targets
}

// ListOrganizationsNeedingPhoto returns CNPJ-keyed Organizations with no image.
// limit <= 0 returns all.
func (db *DB) ListOrganizationsNeedingPhoto(ctx context.Context, limit int) ([]OrganizationPhotoTarget, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
MATCH (o:Organization)
WHERE o.cnpj IS NOT NULL AND size(o.cnpj) = 14
  AND (o.image_url IS NULL OR o.image_url = '')
RETURN o.id AS id, o.cnpj AS cnpj, o.name AS name
`
	if limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", limit)
	}

	res, err := session.Run(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("memgraph: list organizations needing photo: %w", err)
	}
	out := make([]OrganizationPhotoTarget, 0)
	for res.Next(ctx) {
		rec := res.Record()
		id, _ := valStr(rec, "id")
		cnpj, _ := valStr(rec, "cnpj")
		if strings.TrimSpace(cnpj) == "" {
			continue
		}
		name, _ := valStr(rec, "name")
		out = append(out, OrganizationPhotoTarget{ID: id, CNPJ: cnpj, Name: name})
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: iterate organizations needing photo: %w", err)
	}
	return out, nil
}

// SetOrganizationPhotoByCNPJ sets the Commons image on an Organization, only
// when it has no image yet. It writes image_url (the API Organization model
// field) and mirrors photo_url for the shared contract, plus photo_source and
// the legally-required photo_attribution. Returns whether a row was updated.
func (db *DB) SetOrganizationPhotoByCNPJ(ctx context.Context, cnpj, imageURL, source, attribution string) (bool, error) {
	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
MATCH (o:Organization {cnpj: $cnpj})
WHERE o.image_url IS NULL OR o.image_url = ''
SET o.image_url = $image_url,
    o.photo_url = $image_url,
    o.photo_source = $source,
    o.photo_attribution = $attribution
RETURN count(o) AS updated
`, map[string]any{
		"cnpj":        cnpj,
		"image_url":   imageURL,
		"source":      source,
		"attribution": attribution,
	})
	if err != nil {
		return false, fmt.Errorf("memgraph: set organization photo: %w", err)
	}
	return countTrue(ctx, res)
}

// ─── small helpers ────────────────────────────────────────────────────────────

func valStr(rec *neo4j.Record, key string) (string, bool) {
	v, ok := rec.Get(key)
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func valStrSlice(rec *neo4j.Record, key string) []string {
	v, ok := rec.Get(key)
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, a := range arr {
		if s, ok := a.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func countTrue(ctx context.Context, res neo4j.ResultWithContext) (bool, error) {
	if !res.Next(ctx) {
		if err := res.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	v, _ := res.Record().Get("updated")
	n, _ := v.(int64)
	return n > 0, nil
}
