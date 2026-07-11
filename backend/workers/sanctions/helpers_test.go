package sanctions

import (
	"strings"
	"testing"
)

func TestCguRegistryPath(t *testing.T) {
	cases := []struct {
		group    string
		wantPath string
		wantReg  string
		wantOK   bool
	}{
		{"ceis", "ceis", RegistryCEIS, true},
		{"cnep", "cnep", RegistryCNEP, true},
		{"ceaf", "ceaf", RegistryCEAF, true},
		{"leniencia", "acordos-leniencia", RegistryLeniencia, true},
		{"tcu", "", "", false},
		{"unknown", "", "", false},
	}
	for _, c := range cases {
		path, reg, ok := cguRegistryPath(c.group)
		if path != c.wantPath || reg != c.wantReg || ok != c.wantOK {
			t.Errorf("cguRegistryPath(%q) = (%q,%q,%v), want (%q,%q,%v)", c.group, path, reg, ok, c.wantPath, c.wantReg, c.wantOK)
		}
	}
}

func TestCguSourceURL(t *testing.T) {
	// Explicit publication link wins.
	if got := cguSourceURL("  https://x/y  ", RegistryCEIS, 1); got != "https://x/y" {
		t.Errorf("explicit link = %q", got)
	}
	cases := []struct {
		registry string
		want     string
	}{
		{RegistryCEIS, "https://portaldatransparencia.gov.br/sancoes/ceis?id=7"},
		{RegistryCNEP, "https://portaldatransparencia.gov.br/sancoes/cnep?id=7"},
		{RegistryCEAF, "https://portaldatransparencia.gov.br/sancoes/ceaf?id=7"},
		{RegistryLeniencia, "https://portaldatransparencia.gov.br/acordos-leniencia?id=7"},
		{"UNKNOWN", "https://portaldatransparencia.gov.br/sancoes"},
	}
	for _, c := range cases {
		if got := cguSourceURL("", c.registry, 7); got != c.want {
			t.Errorf("cguSourceURL(%s) = %q, want %q", c.registry, got, c.want)
		}
	}
}

func TestTcuEntryID(t *testing.T) {
	cases := []struct {
		processo, doc, name string
		want                string
	}{
		{"029.212/2019-7", "72632078000130", "X", "02921220197-72632078000130"},
		{"029.212/2019-7", "", "Joao da Silva", "02921220197-joao_da_silva"},
		{"029.212/2019-7", "", "", "02921220197"},
		{"", "72632078000130", "", "72632078000130"},
		{"", "", "Joao Silva", "joao_silva"},
	}
	for _, c := range cases {
		if got := tcuEntryID(c.processo, c.doc, c.name); got != c.want {
			t.Errorf("tcuEntryID(%q,%q,%q) = %q, want %q", c.processo, c.doc, c.name, got, c.want)
		}
	}
}

func TestTcuSourceURL(t *testing.T) {
	// Deliberação that is an http URL wins.
	if got := tcuSourceURL("https://contas.tcu.gov.br/x", "029.212/2019-7", "file"); got != "https://contas.tcu.gov.br/x" {
		t.Errorf("http deliberacao = %q", got)
	}
	// Non-URL deliberação → constructed from process number (leading zeros stripped).
	if got := tcuSourceURL("AC-000738/2022-PL", "029.212/2019-7", "file"); got != "https://contas.tcu.gov.br/pesquisaJurisprudencia/#/resultado/acordao-completo/2921220197.PROC" {
		t.Errorf("constructed = %q", got)
	}
	// No deliberação, no process → file URL fallback.
	if got := tcuSourceURL("", "", "https://sites.tcu.gov.br/file.csv"); got != "https://sites.tcu.gov.br/file.csv" {
		t.Errorf("file fallback = %q", got)
	}
}

func TestSlugName(t *testing.T) {
	cases := map[string]string{
		"Joao da Silva": "joao_da_silva",
		"  A B  ":       "a_b", // outer space trimmed, inner space → underscore
		"AB123":         "ab123",
		"José & Filhos": "jos__filhos", // accents/punct dropped, each space → underscore
		"":              "",
	}
	for in, want := range cases {
		if got := slugName(in); got != want {
			t.Errorf("slugName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripLeadingZeros(t *testing.T) {
	cases := map[string]string{
		"000123": "123",
		"123":    "123",
		"0":      "",
		"":       "",
		"0010":   "10",
	}
	for in, want := range cases {
		if got := stripLeadingZeros(in); got != want {
			t.Errorf("stripLeadingZeros(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFoldAccent(t *testing.T) {
	cases := []struct {
		in   rune
		want rune
		ok   bool
	}{
		{'ã', 'A', true},
		{'É', 'E', true},
		{'ç', 'C', true},
		{'ñ', 'N', true},
		{'ý', 'Y', true},
		{'z', 0, false},
		{'1', 0, false},
	}
	for _, c := range cases {
		got, ok := foldAccent(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("foldAccent(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Errorf("firstNonEmpty = %q, want x", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Errorf("firstNonEmpty all-blank = %q, want empty", got)
	}
	if got := firstNonEmpty("  trimmed  "); got != "trimmed" {
		t.Errorf("firstNonEmpty trim = %q", got)
	}
}

func TestNormalizeHeader(t *testing.T) {
	if got := normalizeHeader("\ufeffNOME"); got != "NOME" {
		t.Errorf("BOM strip = %q", got)
	}
	if got := normalizeHeader("  cpf_cnpj  "); got != "CPF_CNPJ" {
		t.Errorf("upper/trim = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate([]byte("hello world"), 5); got != "hello" {
		t.Errorf("truncate = %q, want hello", got)
	}
	if got := truncate([]byte("hi"), 5); got != "hi" {
		t.Errorf("truncate short = %q, want hi", got)
	}
}

func TestDigitsOnly(t *testing.T) {
	if got := digitsOnly("12.345.678/0001-95"); got != "12345678000195" {
		t.Errorf("digitsOnly = %q", got)
	}
	if got := digitsOnly("abc"); got != "" {
		t.Errorf("digitsOnly no digits = %q", got)
	}
}

func TestMapCGUPage_UnknownRegistry(t *testing.T) {
	if _, err := mapCGUPage("BOGUS", []byte(`[]`)); err == nil {
		t.Fatalf("expected unknown registry error")
	}
}

func TestMapCGUPage_InvalidJSON(t *testing.T) {
	for _, reg := range []string{RegistryCEIS, RegistryCNEP, RegistryCEAF, RegistryLeniencia} {
		if _, err := mapCGUPage(reg, []byte(`{not-json`)); err == nil {
			t.Fatalf("expected decode error for %s", reg)
		}
	}
}

func TestMapCGUPage_CNEP(t *testing.T) {
	body := []byte(`[{"id": 9,"tipoSancao":{"descricaoResumida":"Impedida"},"pessoa":{"cnpjFormatado":"12.345.678/0001-95","nome":"EMPRESA"}}]`)
	recs, err := mapCGUPage(RegistryCNEP, body)
	if err != nil {
		t.Fatalf("mapCGUPage cnep: %v", err)
	}
	if len(recs) != 1 || recs[0].Registry != RegistryCNEP || recs[0].CNPJ != "12345678000195" {
		t.Fatalf("unexpected cnep record: %+v", recs)
	}
	if !strings.Contains(recs[0].SourceURL, "cnep") {
		t.Fatalf("cnep source url = %q", recs[0].SourceURL)
	}
}
