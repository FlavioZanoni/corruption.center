package psql

import (
	"context"
	"fmt"
	"strings"
)

// Purge tombstones (LGPD): see migrations/008_purge_tombstones.sql.
// Key builders are exported so the backoffice (writer) and the workers
// (readers) agree on the exact format.

func TombstoneKeyCPF(cpfDigits string) string  { return "cpf:" + cpfDigits }
func TombstoneKeyCNPJ(cnpjDigits string) string { return "cnpj:" + cnpjDigits }

// tombstoneAccents folds the Portuguese accent set to ASCII (stdlib-only, same
// approach as workers/sanctions foldAccent: psql cannot import worker packages).
var tombstoneAccents = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

// tombstonePunct turns the punctuation that varies between spellings of the same
// name into spaces, so "JR." folds to the same token as "JR" and "MARIA-JOSE"
// to "MARIA JOSE". Fields() then collapses the resulting runs of whitespace.
var tombstonePunct = strings.NewReplacer(
	".", " ", ",", " ", "-", " ", "'", " ", "`", " ", "\"", " ", "/", " ",
)

// tombstoneSuffix canonicalizes the interchangeable Brazilian name suffixes so a
// person purged as "…JUNIOR" cannot slip back in as "…JR." on the next sync.
// A deliberately aggressive fold: the operator chose auto-block over
// review-on-near-match, accepting that two distinct people whose names collapse
// here will share a tombstone.
var tombstoneSuffix = map[string]string{
	"JUNIOR": "JR", "JR": "JR", "JUN": "JR",
	"FILHO": "FILHO", "FO": "FILHO", "FILHA": "FILHO",
	"NETO": "NETO", "NETA": "NETO",
	"SOBRINHO": "SOBRINHO", "SOBRINHA": "SOBRINHO",
	"SEGUNDO": "II", "II": "II",
	"TERCEIRO": "III", "III": "III",
}

// TombstoneKeyName normalizes a name to a resurrection-resistant key: uppercase,
// accent-stripped, punctuation-folded, single-spaced, with interchangeable name
// suffixes canonicalized. The SAME function keys both the tombstone written on
// purge and the check every ingest worker runs, so the two can never disagree
// about what counts as "the same name".
func TombstoneKeyName(name string) string {
	folded := tombstonePunct.Replace(tombstoneAccents.Replace(strings.ToUpper(name)))
	fields := strings.Fields(folded)
	for i, f := range fields {
		if canon, ok := tombstoneSuffix[f]; ok {
			fields[i] = canon
		}
	}
	return "name:" + strings.Join(fields, " ")
}

// CreatePurgeTombstones records tombstones for a purged node. removalID is the
// removal_request UUID; pass "" when not applicable.
func (db *DB) CreatePurgeTombstones(ctx context.Context, keys []string, nodeID string, removalID string) error {
	for _, k := range keys {
		if strings.TrimSpace(k) == "" || strings.HasSuffix(k, ":") {
			continue
		}
		var rid *string
		if strings.TrimSpace(removalID) != "" {
			rid = &removalID
		}
		_, err := db.conn.Exec(ctx, `
    INSERT INTO purge_tombstone (subject_key, node_id, removal_id)
    VALUES ($1, $2, $3)
    ON CONFLICT (subject_key) DO NOTHING`, k, nodeID, rid)
		if err != nil {
			return fmt.Errorf("psql: create purge tombstone %q: %w", k, err)
		}
	}
	return nil
}

// DeletePurgedSubjectName scrubs a purged subject's NAME from the review/audit
// substrate that PurgePersonNode does not touch: pending_review payloads, DJEN
// party snapshots, and pending aliases. Without this, a purge (LGPD art. 18)
// scrubbed the graph but left the person's name legible in three Postgres
// tables — an incomplete erasure. The purged node was created from these same
// source rows, so the raw name matches directly (case/space-insensitive).
//
// Returns the number of rows removed across the three tables. Best-effort per
// table: an error on any statement is returned, but earlier deletes stand
// (erasure errs toward removing more, never toward leaving the name behind).
func (db *DB) DeletePurgedSubjectName(ctx context.Context, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil
	}
	total := 0

	// DJEN snapshots and pending aliases store the raw name in one column.
	for _, stmt := range []string{
		`DELETE FROM djen_party_snapshot WHERE upper(btrim(nome)) = upper(btrim($1))`,
		`DELETE FROM pending_aliases WHERE upper(btrim(alias)) = upper(btrim($1))`,
	} {
		tag, err := db.conn.Exec(ctx, stmt, name)
		if err != nil {
			return total, fmt.Errorf("psql: delete purged subject rows: %w", err)
		}
		total += int(tag.RowsAffected())
	}

	// pending_review keeps everything in a JSONB payload whose name key varies by
	// review type, so match the name anywhere in the serialized payload. A purge
	// is an explicit erasure of one person, and a full personal name colliding
	// with an unrelated review is rare — erring toward removal is correct here.
	tag, err := db.conn.Exec(ctx,
		`DELETE FROM pending_review WHERE payload::text ILIKE '%' || $1 || '%'`, name)
	if err != nil {
		return total, fmt.Errorf("psql: delete purged subject reviews: %w", err)
	}
	total += int(tag.RowsAffected())

	return total, nil
}

// IsSubjectPurged reports whether ANY of the given keys has a tombstone.
func (db *DB) IsSubjectPurged(ctx context.Context, keys ...string) (bool, error) {
	for _, k := range keys {
		if strings.TrimSpace(k) == "" || strings.HasSuffix(k, ":") {
			continue
		}
		var exists bool
		err := db.conn.QueryRow(ctx, `
    SELECT EXISTS (SELECT 1 FROM purge_tombstone WHERE subject_key = $1)`, k).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("psql: check purge tombstone %q: %w", k, err)
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}
