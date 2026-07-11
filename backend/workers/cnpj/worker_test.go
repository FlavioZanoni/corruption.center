package cnpj

import (
	"encoding/json"
	"os"
	"testing"
)

func TestClassifyDoc(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want docKind
	}{
		{"masked cpf", "***641988**", docIndividual},
		{"masked cpf spaced", " ***035378** ", docIndividual},
		{"full cpf", "12345678901", docIndividual},
		{"full cnpj digits", "33683111000280", docCompany},
		{"formatted cnpj", "33.683.111/0002-80", docCompany},
		{"empty", "", docUnknown},
		{"garbage", "N/A", docUnknown},
		{"masked but wrong length", "***12**", docUnknown},
		{"cpf_representante placeholder", "***000000**", docIndividual}, // 6 masked digits, still individual
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDoc(tc.doc); got != tc.want {
				t.Fatalf("classifyDoc(%q) = %v, want %v", tc.doc, got, tc.want)
			}
		})
	}
}

func TestMaskedCPFMiddleSix(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		want   string
		wantOK bool
	}{
		{"masked", "***641988**", "641988", true},
		{"masked zeros", "***000000**", "000000", true},
		{"full cpf uses positions 4-9", "12345678901", "456789", true},
		{"cnpj not a cpf", "33683111000280", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := maskedCPFMiddleSix(tc.doc)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("maskedCPFMiddleSix(%q) = (%q,%v), want (%q,%v)", tc.doc, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// The masked-CPF matching contract: the 6 middle digits fed to
// MatchPoliticiansByMaskedCPF must equal digits 4-9 of the full CPF, which is
// exactly what the writer compares (substring(cpf, 3, 6)).
func TestMaskedCPFMatchingContract(t *testing.T) {
	const fullCPF = "12864198855" // -> masked "***641988**"
	masked := "***" + fullCPF[3:9] + "**"
	middle, ok := maskedCPFMiddleSix(masked)
	if !ok {
		t.Fatal("expected masked CPF to yield a middle six")
	}
	if middle != fullCPF[3:9] {
		t.Fatalf("middle = %q, want %q (digits 4-9 of the full CPF)", middle, fullCPF[3:9])
	}
}

func TestIsActive(t *testing.T) {
	cases := map[string]bool{
		"ATIVA":    true,
		"Ativa":    true,
		"ativa":    true,
		"BAIXADA":  false,
		"SUSPENSA": false,
		"NULA":     false,
		"INAPTA":   false,
		"":         false,
	}
	for in, want := range cases {
		if got := isActive(in); got != want {
			t.Fatalf("isActive(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestMapEnrichmentFromFixture maps the real captured minha receita response
// (testdata/serpro.json, curled 2026-07 from https://minhareceita.org/33683111000280).
func TestMapEnrichmentFromFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/serpro.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var resp CNPJResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	e := mapEnrichment(&resp, resp.CNPJ, "https://minhareceita.org/"+resp.CNPJ)

	if e.Name != "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)" {
		t.Errorf("Name = %q", e.Name)
	}
	if !e.Active {
		t.Errorf("Active = false, want true (situação ATIVA)")
	}
	if e.Type != "Empresa Pública" {
		t.Errorf("Type = %q", e.Type)
	}
	if e.UF != "DF" {
		t.Errorf("UF = %q", e.UF)
	}
	if e.ShareCapitalBRL != 1786196100 {
		t.Errorf("ShareCapitalBRL = %v", e.ShareCapitalBRL)
	}
	if e.MainActivity != "Consultoria em tecnologia da informação" {
		t.Errorf("MainActivity = %q", e.MainActivity)
	}

	// Every QSA member in this fixture is a masked-CPF individual.
	if len(resp.QSA) == 0 {
		t.Fatal("fixture QSA is empty")
	}
	for _, q := range resp.QSA {
		if got := classifyDoc(q.CNPJCPFDoSocio); got != docIndividual {
			t.Errorf("QSA %q doc %q classified %v, want docIndividual", q.NomeSocio, q.CNPJCPFDoSocio, got)
		}
	}
}
