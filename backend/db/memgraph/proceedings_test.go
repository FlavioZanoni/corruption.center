package memgraph

import (
	"testing"

	"corruption-center/api/models"
)

func TestProceedingListItemFromProps(t *testing.T) {
	item := proceedingListItemFromProps(map[string]any{
		"id":             "lp_1",
		"case_number":    "0001234-56.2020.8.26.0100",
		"court":          "STF",
		"status":         "concluded",
		"phase":          "sentenca",
		"has_conviction": true,
		"type":           "criminal",
	})

	if item.ID != "lp_1" || item.CaseNumber != "0001234-56.2020.8.26.0100" {
		t.Fatalf("identity not mapped: %+v", item)
	}
	if item.Court != "STF" || item.Phase != "sentenca" {
		t.Fatalf("court/phase not mapped: %+v", item)
	}
	if item.Status != models.ProceedingStatusConcluded {
		t.Fatalf("status = %q", item.Status)
	}
	if item.Type != models.ProceedingTypeCriminal {
		t.Fatalf("type = %q", item.Type)
	}
	if !item.HasConviction {
		t.Fatal("has_conviction = false, want true")
	}
}

// A proceeding freshly merged by case number carries almost nothing yet. Absent
// has_conviction in particular must read as false, never as "unknown but truthy"
// - a wrongly-true conviction flag is a defamation-grade bug.
func TestProceedingListItemFromPropsSparse(t *testing.T) {
	item := proceedingListItemFromProps(map[string]any{
		"id":          "lp_2",
		"case_number": "0009999-11.2021.8.26.0100",
		"status":      "ongoing",
	})

	if item.HasConviction {
		t.Fatal("has_conviction = true for a proceeding with no such property")
	}
	if item.Phase != "" || item.Court != "" || item.Type != "" {
		t.Fatalf("expected empty phase/court/type, got %+v", item)
	}
	if item.Status != models.ProceedingStatusOngoing {
		t.Fatalf("status = %q", item.Status)
	}
}

func TestDefendantFromNode(t *testing.T) {
	def := defendantFromNode(
		[]string{"Politician"},
		map[string]any{"id": "pol_1", "name": "Fulano de Tal"},
		map[string]any{
			"outcome":            "confirmed",
			"source":             "djen",
			"confidence":         0.91,
			"confidence_signals": []any{"exact_name", "same_uf"},
			"confirmed_by":       "operator@example.org",
		},
	)

	if def.ID != "pol_1" || def.Name != "Fulano de Tal" {
		t.Fatalf("identity not mapped: %+v", def)
	}
	if def.Type != "Politician" {
		t.Fatalf("type = %q, want Politician", def.Type)
	}
	if def.Outcome != "confirmed" || def.Source != "djen" {
		t.Fatalf("provenance not mapped: %+v", def)
	}
	if def.Confidence == nil || *def.Confidence != 0.91 {
		t.Fatalf("confidence = %v, want 0.91", def.Confidence)
	}
	if len(def.ConfidenceSignals) != 2 || def.ConfidenceSignals[0] != "exact_name" {
		t.Fatalf("confidence_signals = %v", def.ConfidenceSignals)
	}
	// Properties carries the whole edge, so a caller can still see fields the
	// typed struct does not hoist.
	if def.Properties["confirmed_by"] != "operator@example.org" {
		t.Fatalf("properties dropped confirmed_by: %v", def.Properties)
	}
}

// A node labelled both Person and Politician is a politician: the public,
// nameable identity must win, since that is what decides whether the frontend
// may render the name at all.
func TestDefendantFromNodeLabelPrecedence(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   string
	}{
		{"person", []string{"Person"}, "Person"},
		{"organization", []string{"Organization"}, "Organization"},
		{"politician wins over person", []string{"Person", "Politician"}, "Politician"},
		{"no labels", nil, "Node"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := defendantFromNode(tc.labels, map[string]any{"id": "n_1"}, map[string]any{})
			if def.Type != tc.want {
				t.Fatalf("type = %q, want %q", def.Type, tc.want)
			}
		})
	}
}

// An edge with no provenance at all (a hand-seeded link) must map to empty
// values, not panic. Confidence must be nil rather than 0: an unscored edge is
// not an edge we are "0% sure" of, and it serializes as absent.
func TestDefendantFromNodeBareEdge(t *testing.T) {
	def := defendantFromNode([]string{"Person"}, map[string]any{"id": "per_1", "name": "Anon"}, map[string]any{})

	if def.Confidence != nil {
		t.Fatalf("confidence = %v, want nil for an unscored edge", *def.Confidence)
	}
	if def.Source != "" || def.Outcome != "" {
		t.Fatalf("expected zero provenance, got %+v", def)
	}
	if def.ConfidenceSignals != nil {
		t.Fatalf("confidence_signals = %v, want nil", def.ConfidenceSignals)
	}
}
