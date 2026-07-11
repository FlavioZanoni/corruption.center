package memgraph

import "testing"

func TestHasPurgeableLabel(t *testing.T) {
	cases := []struct {
		labels []string
		want   bool
	}{
		{[]string{"Person"}, true},
		{[]string{"Organization"}, true},
		{[]string{"Person", "Politician"}, true}, // label check alone; caller also gates on IsPolitician
		{[]string{"Scandal"}, false},
		{[]string{"LegalProceeding"}, false},
		{[]string{"Sanction"}, false},
		{[]string{"Source"}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := hasPurgeableLabel(tc.labels); got != tc.want {
			t.Fatalf("hasPurgeableLabel(%v) = %v, want %v", tc.labels, got, tc.want)
		}
	}
}

// TestPurgeableGate documents the exact predicate PurgePersonNode enforces:
// only a Person/Organization that is NOT a Politician may be purged. This is the
// unit-level guard behind "purging a Scandal id must be refused".
func TestPurgeableGate(t *testing.T) {
	cases := []struct {
		name         string
		labels       []string
		wantPurgeabl bool
	}{
		{"person", []string{"Person"}, true},
		{"organization", []string{"Organization"}, true},
		{"politician person", []string{"Person", "Politician"}, false}, // politician wins
		{"scandal refused", []string{"Scandal"}, false},
		{"legal proceeding refused", []string{"LegalProceeding"}, false},
		{"sanction refused", []string{"Sanction"}, false},
		{"source refused", []string{"Source"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, isPol := primaryLabel(tc.labels)
			purgeable := hasPurgeableLabel(tc.labels) && !isPol
			if purgeable != tc.wantPurgeabl {
				t.Fatalf("purgeable(%v) = %v, want %v", tc.labels, purgeable, tc.wantPurgeabl)
			}
		})
	}
}

func TestPrimaryLabel_DetectsPolitician(t *testing.T) {
	label, isPol := primaryLabel([]string{"Person", "Politician"})
	if !isPol {
		t.Fatalf("expected Politician to be detected")
	}
	if label != "Politician" {
		t.Fatalf("politician nodes must display as Politician, got %q", label)
	}

	label, isPol = primaryLabel([]string{"Person"})
	if isPol {
		t.Fatalf("Person must not be flagged as politician")
	}
	if label != "Person" {
		t.Fatalf("expected primary label Person, got %q", label)
	}

	if l, _ := primaryLabel(nil); l != "Node" {
		t.Fatalf("expected fallback label Node, got %q", l)
	}
}

func TestBuildCreationReason(t *testing.T) {
	got := buildCreationReason(&NodeProvenance{Source: "djen", Tribunal: "TRF4", ComunicacaoID: "123"})
	want := "discovered by djen; tribunal TRF4; comunicação 123"
	if got != want {
		t.Fatalf("creation reason = %q, want %q", got, want)
	}
	if r := buildCreationReason(&NodeProvenance{}); r != "no provenance recorded on this node" {
		t.Fatalf("empty provenance = %q", r)
	}
}
