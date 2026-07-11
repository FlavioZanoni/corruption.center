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
// approach as workers/sanctions foldAccent — psql cannot import worker packages).
var tombstoneAccents = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

// TombstoneKeyName normalizes to uppercase, accent-stripped, single spaces.
func TombstoneKeyName(name string) string {
	folded := tombstoneAccents.Replace(strings.ToUpper(name))
	return "name:" + strings.Join(strings.Fields(folded), " ")
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
