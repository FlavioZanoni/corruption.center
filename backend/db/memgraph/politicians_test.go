package memgraph

import (
	"testing"

	"corruption-center/api/models"
)

func TestLabelToNodeTypeSanction(t *testing.T) {
	cases := []struct {
		name  string
		label string
		want  models.NodeType
	}{
		{"exact", "Sanction", models.NodeTypeSanction},
		{"lower", "sanction", models.NodeTypeSanction},
		{"politician still maps", "Politician", models.NodeTypePolitician},
		{"unknown lowercased", "Weird", models.NodeType("weird")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := labelToNodeType(tc.label); got != tc.want {
				t.Fatalf("labelToNodeType(%q) = %q, want %q", tc.label, got, tc.want)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
	}{
		{"int64", int64(7), 7},
		{"int", int(3), 3},
		{"float64", float64(5), 5},
		{"nil", nil, 0},
		{"string", "12", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := asInt(tc.in); got != tc.want {
				t.Fatalf("asInt(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
