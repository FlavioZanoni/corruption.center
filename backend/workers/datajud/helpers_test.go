package datajud

import (
	"testing"
	"time"
)

func TestMovementCode(t *testing.T) {
	if got := movementCode(map[string]any{"codigo": "51"}); got != "51" {
		t.Errorf("string codigo = %q, want 51", got)
	}
	if got := movementCode(map[string]any{"codigo": float64(848)}); got != "848" {
		t.Errorf("float codigo = %q, want 848", got)
	}
	if got := movementCode(map[string]any{"codigo": 60}); got != "60" {
		t.Errorf("int codigo = %q, want 60", got)
	}
	if got := movementCode(map[string]any{}); got != "" {
		t.Errorf("missing codigo = %q, want empty", got)
	}
	if got := movementCode(map[string]any{"codigo": "  51  "}); got != "51" {
		t.Errorf("trimmed codigo = %q, want 51", got)
	}
}

func TestMaxMovementID_EdgeCases(t *testing.T) {
	if got := maxMovementID(nil); got != "" {
		t.Errorf("nil movements = %q, want empty", got)
	}
	// Non-numeric and nil ids are skipped.
	movs := []map[string]any{{"id": "abc"}, {"id": nil}, {"id": "5"}, {"foo": "bar"}}
	if got := maxMovementID(movs); got != "5" {
		t.Errorf("max = %q, want 5", got)
	}
	// All zero/absent → empty.
	if got := maxMovementID([]map[string]any{{"id": "0"}}); got != "" {
		t.Errorf("zero-only = %q, want empty", got)
	}
	// Numeric id serialized as float.
	if got := maxMovementID([]map[string]any{{"id": float64(12)}, {"id": float64(3)}}); got != "12" {
		t.Errorf("float ids = %q, want 12", got)
	}
}

func TestProceedingTypeFromClasse(t *testing.T) {
	if got := proceedingTypeFromClasse(nil); got != "criminal" {
		t.Errorf("nil classe = %q, want criminal", got)
	}
	if got := proceedingTypeFromClasse(map[string]any{}); got != "criminal" {
		t.Errorf("empty classe = %q, want criminal", got)
	}
	if got := proceedingTypeFromClasse(map[string]any{"codigo": "283"}); got != "283" {
		t.Errorf("classe code = %q, want 283", got)
	}
	if got := proceedingTypeFromClasse(map[string]any{"codigo": float64(283)}); got != "283" {
		t.Errorf("numeric classe code = %q, want 283", got)
	}
}

func TestAssuntosCodes(t *testing.T) {
	assuntos := []map[string]any{
		{"codigo": "100"},
		{"codigo": float64(200)},
		{"codigo": ""}, // empty skipped
		{"nome": "x"},  // no codigo → "<nil>" skipped
	}
	got := assuntosCodes(assuntos)
	if len(got) != 2 || got[0] != "100" || got[1] != "200" {
		t.Fatalf("assuntosCodes = %v, want [100 200]", got)
	}
	if got := assuntosCodes(nil); len(got) != 0 {
		t.Fatalf("nil assuntos = %v, want empty", got)
	}
}

func TestParseDate(t *testing.T) {
	cases := map[string]bool{ // input → expect non-nil
		"":                     false,
		"garbage":              false,
		"2022-09-29":           true,
		"2022-09-29T10:00:00Z": true,
		"2022-09-29T10:00:00":  true,
	}
	for in, wantOK := range cases {
		got := parseDate(in)
		if wantOK && got == nil {
			t.Errorf("parseDate(%q) = nil, want a time", in)
		}
		if !wantOK && got != nil {
			t.Errorf("parseDate(%q) = %v, want nil", in, got)
		}
	}
	if got := parseDate("2022-09-29"); got == nil || !got.Equal(time.Date(2022, 9, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate value = %v", got)
	}
}

func TestCourtName(t *testing.T) {
	if got := courtName(nil); got != "" {
		t.Errorf("nil orgao = %q, want empty", got)
	}
	if got := courtName(map[string]any{"nome": "  Vara Federal  "}); got != "Vara Federal" {
		t.Errorf("courtName = %q, want trimmed", got)
	}
	if got := courtName(map[string]any{}); got != "" {
		t.Errorf("missing nome = %q, want empty", got)
	}
}

func TestFoldText(t *testing.T) {
	if got := foldText("SENTENÇA Condenatória"); got != "sentenca condenatoria" {
		t.Fatalf("foldText = %q", got)
	}
	if got := foldText("Absolvição"); got != "absolvicao" {
		t.Fatalf("foldText = %q", got)
	}
}

func TestComplementList(t *testing.T) {
	// []map[string]any passes through.
	direct := []map[string]any{{"nome": "a"}}
	if got := complementList(direct); len(got) != 1 {
		t.Fatalf("direct slice len = %d, want 1", len(got))
	}
	// []any of maps is coerced; non-map elements are dropped.
	mixed := []any{map[string]any{"nome": "a"}, "not-a-map", 42}
	got := complementList(mixed)
	if len(got) != 1 || got[0]["nome"] != "a" {
		t.Fatalf("mixed coerce = %v, want single map", got)
	}
	// Unsupported type → nil.
	if got := complementList("nope"); got != nil {
		t.Fatalf("string input = %v, want nil", got)
	}
	if got := complementList(nil); got != nil {
		t.Fatalf("nil input = %v, want nil", got)
	}
}

func TestProbeCapabilities_RawFallback(t *testing.T) {
	// numeroProcesso only in Raw, movimentos field present in Raw.
	src := &CaseSource{Raw: map[string]any{"numeroProcesso": "123", "movimentos": []any{}}}
	if !probeCapabilities(src) {
		t.Fatalf("expected raw-fallback probe to pass")
	}
	// Raw has numero but no movimentos key.
	src2 := &CaseSource{Raw: map[string]any{"numeroProcesso": "123"}}
	if probeCapabilities(src2) {
		t.Fatalf("expected false when movimentos key absent")
	}
}
