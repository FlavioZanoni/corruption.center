package memgraph

import "strings"

// SanitizeProperties removes PII and internal fields from a property map before
// it is published to the public API. This is a single choke point that all graph
// and proceeding endpoints route through to ensure no operator credentials or
// internal scratch fields ever leak.
//
// Strategy: explicit denylist, not allowlist. An allowlist would silently drop
// every new legitimate property a worker adds; a denylist drops only the fields
// we know are sensitive, letting the database evolve without breaking the API.
//
// The denylist:
//   - outcome_recorded_by: operator's BASIC-AUTH username. PII. Ever written by
//     SetDefendantOutcome, never published.
//   - _*: Memgraph internal scratch fields (e.g., _sanctions_created used by
//     sanctions_writer.go). Never meant for the public.
func SanitizeProperties(props map[string]any) map[string]any {
	if props == nil {
		return nil
	}

	sanitized := make(map[string]any)
	for key, value := range props {
		// Strip ANY *_recorded_by field, not a named list. The named list already
		// failed once: outcome_recorded_by was stripped, then AttachOrganizationCNPJ
		// introduced cnpj_recorded_by in the same session and it sailed straight to
		// the public API. Every "a human recorded this" field carries the operator's
		// basic-auth username, so the suffix IS the category.
		if strings.HasSuffix(key, "_recorded_by") {
			continue
		}
		// Strip Memgraph internal fields (start with underscore).
		if len(key) > 0 && key[0] == '_' {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}
