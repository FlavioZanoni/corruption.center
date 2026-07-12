package memgraph

import (
	"testing"
)

// TestSanitizeProperties_StripsPII asserts that outcome_recorded_by (operator
// username) never reaches a public response.
func TestSanitizeProperties_StripsPII(t *testing.T) {
	props := map[string]any{
		"outcome":           "convicted",
		"outcome_source":    "human",
		"outcome_recorded_by": "alice.admin@example.com", // PII: operator's email/username
		"outcome_evidence_url": "https://example.com/decision",
		"outcome_recorded_at":  "2026-07-12T10:00:00Z",
	}

	sanitized := SanitizeProperties(props)

	// outcome_recorded_by must not be in the sanitized result
	if _, exists := sanitized["outcome_recorded_by"]; exists {
		t.Error("outcome_recorded_by should be stripped (PII)")
	}

	// All other outcome properties must survive
	if v, ok := sanitized["outcome"]; !ok || v != "convicted" {
		t.Error("outcome should survive sanitization")
	}
	if v, ok := sanitized["outcome_source"]; !ok || v != "human" {
		t.Error("outcome_source should survive sanitization (distinguishes human from machine)")
	}
	if v, ok := sanitized["outcome_evidence_url"]; !ok || v != "https://example.com/decision" {
		t.Error("outcome_evidence_url should survive sanitization (links to decision)")
	}
	if v, ok := sanitized["outcome_recorded_at"]; !ok || v != "2026-07-12T10:00:00Z" {
		t.Error("outcome_recorded_at should survive sanitization (timestamp is not PII)")
	}
}

// TestSanitizeProperties_StripsMgraphInternals asserts that fields starting
// with underscore (Memgraph internal scratch fields) never reach a public response.
func TestSanitizeProperties_StripsMgraphInternals(t *testing.T) {
	props := map[string]any{
		"id":                 "sanc-123",
		"registry":           "CEIS",
		"_sanctions_created": "2026-01-15", // Internal scratch field used by sanctions_writer.go
		"_internal_flag":     true,           // Any Memgraph internal field
		"sanction_type":      "Permanent",
	}

	sanitized := SanitizeProperties(props)

	// Any field starting with _ must not be in the result
	for key := range sanitized {
		if len(key) > 0 && key[0] == '_' {
			t.Errorf("field %q starts with underscore and should have been stripped", key)
		}
	}

	// Legitimate fields must survive
	if v, ok := sanitized["id"]; !ok || v != "sanc-123" {
		t.Error("id should survive sanitization")
	}
	if v, ok := sanitized["registry"]; !ok || v != "CEIS" {
		t.Error("registry should survive sanitization")
	}
	if v, ok := sanitized["sanction_type"]; !ok || v != "Permanent" {
		t.Error("sanction_type should survive sanitization")
	}
}

// TestSanitizeProperties_PreservesLegitimateFields asserts that properties that
// are not on the denylist survive untouched, ensuring the sanitizer does not
// silently drop new legitimate properties added by workers.
func TestSanitizeProperties_PreservesLegitimateFields(t *testing.T) {
	props := map[string]any{
		"id":                   "def-456",
		"name":                 "Alice Worker",
		"cpf":                  "12345678901",
		"outcome":              "acquitted",
		"outcome_source":       "human",
		"outcome_evidence_url": "https://example.com/dec2",
		"confidence":           0.95,
		"source":               "djen",
		"party_type":           "Politician",
		"date_matched":         "2026-03-01",
	}

	sanitized := SanitizeProperties(props)

	// All fields without PII or internal markers must be present
	expectedKeys := []string{
		"id", "name", "cpf", "outcome", "outcome_source", "outcome_evidence_url",
		"confidence", "source", "party_type", "date_matched",
	}
	for _, key := range expectedKeys {
		if _, ok := sanitized[key]; !ok {
			t.Errorf("legitimate field %q should survive sanitization", key)
		}
	}

	if len(sanitized) != len(expectedKeys) {
		t.Errorf("expected %d fields after sanitization, got %d",
			len(expectedKeys), len(sanitized))
	}
}

// TestSanitizeProperties_HandlesNil ensures the sanitizer gracefully handles nil input.
func TestSanitizeProperties_HandlesNil(t *testing.T) {
	result := SanitizeProperties(nil)
	if result != nil {
		t.Error("SanitizeProperties(nil) should return nil")
	}
}

// TestSanitizeProperties_HandlesEmpty ensures the sanitizer gracefully handles empty map.
func TestSanitizeProperties_HandlesEmpty(t *testing.T) {
	props := map[string]any{}
	sanitized := SanitizeProperties(props)

	if sanitized == nil {
		t.Error("SanitizeProperties({}) should return empty map, not nil")
	}
	if len(sanitized) != 0 {
		t.Error("SanitizeProperties({}) should return empty map")
	}
}

// TestSanitizeProperties_FullDefendantInEdge tests a realistic DEFENDANT_IN edge
// with all properties that SetDefendantOutcome writes, verifying the complete behavior.
func TestSanitizeProperties_FullDefendantInEdge(t *testing.T) {
	// Simulate what SetDefendantOutcome writes to a DEFENDANT_IN edge
	edgeProps := map[string]any{
		"outcome":             "convicted",
		"outcome_source":      "human",
		"outcome_evidence_url": "https://stf.jus.br/decisao/123",
		"outcome_recorded_by":  "maria.operator@corruption-center.org", // PII
		"outcome_recorded_at":  "2026-06-15T14:30:00Z",
		"source":               "djen", // from initial matching
		"confidence":           0.85,   // from review queue scoring
	}

	sanitized := SanitizeProperties(edgeProps)

	// Verify PII is stripped
	if _, exists := sanitized["outcome_recorded_by"]; exists {
		t.Fatal("outcome_recorded_by must not appear in public responses (operator credential)")
	}

	// Verify critical fields survive to let readers verify conviction
	testCases := []struct {
		field       string
		description string
	}{
		{"outcome", "tells readers if defendant was convicted, acquitted, etc"},
		{"outcome_source", "tells readers outcome was human-verified, not guessed"},
		{"outcome_evidence_url", "links to the court decision — the source of authority"},
		{"outcome_recorded_at", "timestamp of when the record was entered (not PII)"},
	}

	for _, tc := range testCases {
		if v, ok := sanitized[tc.field]; !ok {
			t.Errorf("%s missing: %s", tc.field, tc.description)
		} else if tc.field == "outcome" && v != "convicted" {
			t.Errorf("%s value wrong: %v", tc.field, v)
		}
	}
}

// TestSanitizeProperties_OutcomeSourceMustSurvive verifies that outcome_source
// must NEVER be stripped, as it distinguishes human-verified outcomes from machine guesses.
// This is critical for legal liability: readers must know if an outcome was a human's
// reading of the decision or the backend's inference.
func TestSanitizeProperties_OutcomeSourceMustSurvive(t *testing.T) {
	humanVerified := map[string]any{
		"outcome":        "convicted",
		"outcome_source": "human", // Human read the decision
	}

	sanitized := SanitizeProperties(humanVerified)

	// outcome_source MUST be present, even alone
	source, ok := sanitized["outcome_source"]
	if !ok || source != "human" {
		t.Fatal("outcome_source must survive sanitization; readers need to know if a conviction is human-verified")
	}
}

// TestSanitizeProperties_EvidenceLinkMustSurvive verifies that outcome_evidence_url
// must NEVER be stripped, as it links to the court decision that justifies a conviction.
// Publishing a conviction without its source is indefensible.
func TestSanitizeProperties_EvidenceLinkMustSurvive(t *testing.T) {
	withEvidence := map[string]any{
		"outcome":             "convicted",
		"outcome_evidence_url": "https://stf.jus.br/portal/processo/verProcessoFormulario.asp?seqprocesso=123",
	}

	sanitized := SanitizeProperties(withEvidence)

	// outcome_evidence_url MUST be present
	url, ok := sanitized["outcome_evidence_url"]
	if !ok {
		t.Fatal("outcome_evidence_url must survive sanitization; readers need the link to the decision")
	}
	if urlStr, ok := url.(string); ok && urlStr != "https://stf.jus.br/portal/processo/verProcessoFormulario.asp?seqprocesso=123" {
		t.Errorf("outcome_evidence_url value corrupted: %v", urlStr)
	}
}

// TestSanitizeProperties_RegressionOutcomeRecordedBy is a regression test that MUST fail
// if outcome_recorded_by is ever reintroduced into sanitized results. This is the single
// most critical field to strip, as it contains the operator's credentials.
func TestSanitizeProperties_RegressionOutcomeRecordedBy(t *testing.T) {
	// Simulate an edge that was written by SetDefendantOutcome with a real operator username
	edgeWithSecret := map[string]any{
		"id":                   "edge-uuid-123",
		"outcome":              "convicted",
		"outcome_source":       "human",
		"outcome_recorded_by":  "operator.alice@corruption-center.org", // THE SECRET
		"outcome_recorded_at":  "2026-06-10T09:15:00Z",
		"outcome_evidence_url": "https://example.com/decision",
		"created_at":           "2026-01-01T00:00:00Z",
	}

	sanitized := SanitizeProperties(edgeWithSecret)

	// The secret MUST NOT exist in the result
	if recorded, exists := sanitized["outcome_recorded_by"]; exists {
		t.Fatalf("CRITICAL: outcome_recorded_by leaked to public API: %v", recorded)
	}

	// Count fields: 6 allowed, 1 disallowed + 1 internal _ field would be 8 total
	expectedCount := 6 // id, outcome, outcome_source, outcome_recorded_at, outcome_evidence_url, created_at
	if len(sanitized) != expectedCount {
		t.Errorf("expected %d fields after sanitization (only outcome_recorded_by stripped), got %d: %v",
			expectedCount, len(sanitized), sanitized)
	}
}

// TestSanitizeProperties_RegressionInternalFields is a regression test that MUST fail
// if internal Memgraph fields (starting with _) ever leak into public results.
func TestSanitizeProperties_RegressionInternalFields(t *testing.T) {
	// Simulate a sanction with internal working fields
	sanctionWithInternals := map[string]any{
		"id":                   "sanc-123",
		"registry":             "CEIS",
		"sanction_type":        "Permanent",
		"_sanctions_created":   "2026-01-01T12:00:00Z", // Internal working field
		"_processing_status":   "complete",               // Internal working field
		"_last_updated_by":     "batch-worker",           // Internal working field
		"_sync_timestamp":      1234567890,               // Internal working field
		"date_start":           "2026-01-15",
	}

	sanitized := SanitizeProperties(sanctionWithInternals)

	// NO field starting with underscore should be present
	for key := range sanitized {
		if len(key) > 0 && key[0] == '_' {
			t.Fatalf("CRITICAL: internal field %q leaked to public API", key)
		}
	}

	// Only 4 legitimate fields should survive
	expectedFields := map[string]bool{
		"id": true, "registry": true, "sanction_type": true, "date_start": true,
	}
	if len(sanitized) != 4 {
		t.Errorf("expected 4 fields after sanitization (4 internals stripped), got %d",
			len(sanitized))
	}
	for field := range sanitized {
		if !expectedFields[field] {
			t.Errorf("unexpected field in result: %q", field)
		}
	}
}
