package datajud

import "testing"

func TestMaxMovementID(t *testing.T) {
	movs := []map[string]any{{"id": "10"}, {"id": "7"}, {"id": "42"}}
	got := maxMovementID(movs)
	if got != "42" {
		t.Fatalf("expected 42, got %s", got)
	}
}

func TestHasRequiredFields(t *testing.T) {
	src := &CaseSource{
		NumeroProcesso:      "5046512-94.2016.4.04.7000",
		Movimentos:          []map[string]any{{"id": "1", "codigo": "51"}},
		ProcessoRelacionado: []map[string]any{{"numeroProcesso": "x"}},
		Partes:              []map[string]any{{"documento": "123"}},
	}
	if !hasRequiredFields(src) {
		t.Fatalf("expected true")
	}
}

func TestProbeCapabilitiesOptionalFields(t *testing.T) {
	src := &CaseSource{
		NumeroProcesso: "5046512-94.2016.4.04.7000",
		Movimentos:     []map[string]any{{"id": "1", "codigo": "51"}},
	}
	coreOK, hasPartes, hasRelated := probeCapabilities(src)
	if !coreOK {
		t.Fatalf("expected coreOK true")
	}
	if hasPartes {
		t.Fatalf("expected hasPartes false")
	}
	if hasRelated {
		t.Fatalf("expected hasRelated false")
	}
}

func TestDeriveOutcome(t *testing.T) {
	movs := []map[string]any{{"codigo": 51}, {"codigo": 848, "nome": "Sentenca condenatoria"}}
	if out := deriveOutcome(movs); out != "convicted" {
		t.Fatalf("expected convicted, got %s", out)
	}
}

func TestExtractCaseNumber(t *testing.T) {
	txt := "Desmembramento para o processo 5046512-94.2016.4.04.7000"
	got := extractCaseNumber(txt)
	if got != "5046512-94.2016.4.04.7000" {
		t.Fatalf("unexpected extracted case number: %s", got)
	}
}
