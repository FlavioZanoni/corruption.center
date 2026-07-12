package memgraph

import (
	"strings"
	"testing"
)

func TestFoldQuery(t *testing.T) {
	cases := map[string]string{
		"José":              "jose",
		"JOSÉ":              "jose",
		"josé":              "jose",
		"jose":              "jose",
		"Operação":          "operacao",
		"OPERAÇÃO":          "operacao",
		"operacao":          "operacao",
		"João":              "joao",
		"Antônio":           "antonio",
		"MAURÍCIO":          "mauricio",
		"  Conceição  ":     "conceicao",
		"Muñoz":             "munoz",
		"":                  "",
		"no accents at all": "no accents at all",
	}
	for in, want := range cases {
		if got := foldQuery(in); got != want {
			t.Errorf("foldQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// The two sides of every comparison must fold identically. Fold one and not the
// other and the search silently matches nothing, which reads as "no results"
// rather than as a bug — so this pins that every character foldQuery collapses is
// also collapsed by the Cypher expression.
func TestFoldExpr_CoversEveryCharacterFoldQueryDoes(t *testing.T) {
	expr := foldExpr("node.name")
	for _, f := range accentFold {
		if !strings.Contains(expr, "'"+f[0]+"', '"+f[1]+"'") {
			t.Errorf("foldExpr does not fold %q -> %q, but foldQuery does: a name containing %q becomes unsearchable",
				f[0], f[1], f[0])
		}
	}
	if !strings.HasPrefix(expr, "replace(") || !strings.Contains(expr, "toLower(node.name)") {
		t.Fatalf("foldExpr should lowercase then replace; got %q", expr)
	}
}

// Memgraph's toLower() is ASCII-only — toLower('JOSÉ') is 'josÉ' — so the
// accented CAPITALS never reach their lowercase form and have to be folded
// explicitly. Most stored names are uppercase (TSE and CGU publish them that
// way), so dropping the capitals from the table would break the common case while
// leaving the rare lowercase one working, which is the hardest kind of bug to
// notice.
func TestAccentFold_FoldsCapitalsNotJustLowercase(t *testing.T) {
	for _, upper := range []string{"Á", "É", "Í", "Ó", "Ú", "Ç", "Ã", "Õ", "Ê", "Ô", "Â", "Í", "Ñ", "Ü"} {
		found := false
		for _, f := range accentFold {
			if f[0] == upper {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("accentFold has no entry for %q; Memgraph's toLower will not produce its lowercase form, so names containing it stay unsearchable", upper)
		}
	}
}
