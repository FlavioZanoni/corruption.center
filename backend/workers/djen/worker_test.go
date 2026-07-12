package djen

import (
	"encoding/json"
	"testing"

	"corruption-center/db/memgraph"
)

// fixtureJSON mirrors the real DJEN response shape (verified against the live
// API 2026-07). Two communications for one criminal case plus one improbidade
// case, with a homonym on the passivo side.
const fixtureJSON = `{
  "status": "success",
  "message": "Sucesso",
  "count": 3,
  "items": [
    {
      "id": 100,
      "siglaTribunal": "TRF1",
      "numero_processo": "10000000020234013700",
      "nomeClasse": "AÇÃO PENAL - PROCEDIMENTO ORDINÁRIO",
      "codigoClasse": "283",
      "link": "https://example.jus.br/c/100",
      "texto": "<p>Intimação de <b>JOÃO DA SILVA</b> na ação penal.</p>",
      "destinatarios": [
        {"nome": "MINISTÉRIO PÚBLICO FEDERAL", "polo": "A", "comunicacao_id": 100},
        {"nome": "João da Silva", "polo": "P", "comunicacao_id": 100},
        {"nome": "Fulano de Tal", "polo": "P", "comunicacao_id": 100}
      ]
    },
    {
      "id": 101,
      "siglaTribunal": "TRF1",
      "numero_processo": "10000000020234013700",
      "nomeClasse": "AÇÃO PENAL - PROCEDIMENTO ORDINÁRIO",
      "codigoClasse": "283",
      "link": "https://example.jus.br/c/101",
      "texto": "Nova intimação",
      "destinatarios": [
        {"nome": "JOAO DA SILVA", "polo": "P", "comunicacao_id": 101}
      ]
    },
    {
      "id": 200,
      "siglaTribunal": "TRT1",
      "numero_processo": "20000000020235010432",
      "nomeClasse": "CARTA PRECATÓRIA CÍVEL",
      "codigoClasse": "261",
      "link": "https://example.jus.br/c/200",
      "texto": "Distribuição",
      "destinatarios": [
        {"nome": "João da Silva", "polo": "P", "comunicacao_id": 200}
      ]
    }
  ]
}`

func parseFixture(t *testing.T) []Item {
	t.Helper()
	var resp apiResponse
	if err := json.Unmarshal([]byte(fixtureJSON), &resp); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return resp.Items
}

func TestRosterFromItemsDedup(t *testing.T) {
	items := parseFixture(t)
	roster := rosterFromItems(items)

	// The three spellings "João da Silva"/"JOAO DA SILVA"/"JOÃO DA SILVA" fold to
	// one normalized passivo key. Distinct entries across all items: MPF (A),
	// João da Silva (P), Fulano de Tal (P) → 3.
	counts := map[string]int{}
	for _, p := range roster {
		counts[partyKey(p.Nome, p.Polo)]++
	}
	for k, c := range counts {
		if c != 1 {
			t.Fatalf("expected roster deduped, key %q seen %d times", k, c)
		}
	}
	if _, ok := counts["JOAO DA SILVA|P"]; !ok {
		t.Fatalf("expected normalized JOAO DA SILVA|P in roster")
	}
	if got := len(roster); got != 3 {
		t.Fatalf("expected 3 deduped parties, got %d: %+v", got, roster)
	}

	// Evidence from first observation is retained.
	for _, p := range roster {
		if partyKey(p.Nome, p.Polo) == "JOAO DA SILVA|P" {
			if p.Link != "https://example.jus.br/c/100" {
				t.Fatalf("expected first-seen link, got %q", p.Link)
			}
		}
	}
}

func TestRosterDelta(t *testing.T) {
	roster := []Party{
		{Nome: "João da Silva", Polo: "P"},
		{Nome: "Fulano de Tal", Polo: "P"},
	}
	existing := map[string]bool{"JOAO DA SILVA|P": true}
	delta := rosterDelta(roster, existing)
	if len(delta) != 1 {
		t.Fatalf("expected 1 new party, got %d", len(delta))
	}
	if delta[0].Nome != "Fulano de Tal" {
		t.Fatalf("expected Fulano de Tal, got %q", delta[0].Nome)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"João da Silva":  "JOAO DA SILVA",
		"  JOSÉ  AÇÃO  ": "JOSE ACAO",
		"Antônio Núñez":  "ANTONIO NUNEZ",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Fatalf("normalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchPolitician(t *testing.T) {
	index := buildPoliticianIndex([]memgraph.PoliticianNames{
		{ID: "pol_1", Name: "José Serra", Aliases: []string{"Zé Serra"}},
		{ID: "pol_2", Name: "João da Silva"},
	})

	if id, ok := matchPolitician("JOAO DA SILVA", index); !ok || id != "pol_2" {
		t.Fatalf("expected pol_2 exact match, got %q ok=%v", id, ok)
	}
	if id, ok := matchPolitician("zé serra", index); !ok || id != "pol_1" {
		t.Fatalf("expected alias match pol_1, got %q ok=%v", id, ok)
	}
	// Substring must NOT match (homonym safety).
	if _, ok := matchPolitician("João da Silva Júnior", index); ok {
		t.Fatalf("substring/partial name must not match")
	}
	if _, ok := matchPolitician("Silva", index); ok {
		t.Fatalf("single token must not match full name")
	}
}

func TestClassFilter(t *testing.T) {
	tests := []struct {
		nome   string
		codigo string
		want   bool
	}{
		{"AÇÃO PENAL - PROCEDIMENTO ORDINÁRIO", "283", true},       // code allow
		{"AÇÃO CIVIL DE IMPROBIDADE ADMINISTRATIVA", "64", true},   // code allow
		{"Algo qualquer sobre crime", "9999", true},                // keyword fallback
		{"AÇÃO CIVIL DE IMPROBIDADE ADMINISTRATIVA", "9999", true}, // keyword fallback
		{"CARTA PRECATÓRIA CÍVEL", "261", false},                   // labor/civil, excluded
		{"EXECUÇÃO FISCAL", "1116", false},
	}
	for _, tc := range tests {
		if got := isCriminalOrImprobidadeClass(tc.nome, tc.codigo); got != tc.want {
			t.Fatalf("isCriminalOrImprobidadeClass(%q,%q)=%v want %v", tc.nome, tc.codigo, got, tc.want)
		}
	}
}

func TestGroupByProcessoAndAllowedClass(t *testing.T) {
	items := parseFixture(t)
	groups := groupByProcesso(items)
	if len(groups) != 2 {
		t.Fatalf("expected 2 case groups, got %d", len(groups))
	}
	crimCase := groups["10000000020234013700"]
	if !groupHasAllowedClass(crimCase) {
		t.Fatalf("criminal case should pass class filter")
	}
	civilCase := groups["20000000020235010432"]
	if groupHasAllowedClass(civilCase) {
		t.Fatalf("carta precatória cível case should be filtered out")
	}
}

func TestNormalizeCaseNumber(t *testing.T) {
	cases := map[string]string{
		"5046512-94.2016.4.04.7000": "50465129420164047000", // formatted CNJ → 20 digits
		"10000000020234013700":      "10000000020234013700", // already bare
		"  1234 / 5678 ":            "12345678",             // spaces and slash stripped
		"":                          "",
	}
	for in, want := range cases {
		if got := normalizeCaseNumber(in); got != want {
			t.Fatalf("normalizeCaseNumber(%q) = %q, want %q", in, got, want)
		}
	}
	// The formatted CNJ example must be exactly 20 digits (the DJEN requirement).
	if got := normalizeCaseNumber("5046512-94.2016.4.04.7000"); len(got) != 20 {
		t.Fatalf("expected 20 digits, got %d (%q)", len(got), got)
	}
}

func TestStripEOutros(t *testing.T) {
	cases := map[string]string{
		"FULANO DE TAL E OUTROS (3)": "FULANO DE TAL",
		"FULANO DE TAL E OUTROS":     "FULANO DE TAL",
		"FULANO DE TAL E OUTRO":      "FULANO DE TAL",
		"João da Silva e outros (5)": "João da Silva",
		"FULANO DE TAL":              "FULANO DE TAL",        // no marker → unchanged
		"OUTROS COMERCIO LTDA":       "OUTROS COMERCIO LTDA", // "OUTROS" not a trailing marker
	}
	for in, want := range cases {
		if got := stripEOutros(in); got != want {
			t.Fatalf("stripEOutros(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsCompanyName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Corporate suffixes / punctuated forms.
		{"CONSTRUÇÕES ABC LTDA", true},
		{"EMPRESA XYZ S/A", true},
		{"EMPRESA XYZ S.A.", true},
		{"BANCO DO BRASIL SA", true},
		{"PADARIA DO ZE EIRELI", true},
		{"MERCADINHO CENTRAL ME", true},
		{"OFICINA POPULAR MEI", true},
		{"COMERCIO DE ALIMENTOS EPP", true},
		{"TRANSPORTES XYZ CIA", true},
		// Token markers anywhere.
		{"CONSTRUTORA NORTE SUL", true},
		{"CONSORCIO OBRAS PUBLICAS", true},
		{"BANCO SANTANDER", true},
		{"COMPANHIA ENERGETICA", true},
		{"INCORPORADORA ALVORADA", true},
		{"ASSOCIAÇÃO DOS MORADORES", true},
		{"FUNDAÇÃO GETULIO VARGAS", true},
		{"INSTITUTO NACIONAL", true},
		// Public bodies.
		{"MUNICIPIO DE SAO PAULO", true},
		{"PREFEITURA DE CAMPINAS", true},
		{"ESTADO DE MINAS GERAIS", true},
		{"UNIAO FEDERAL", true},
		{"MINISTÉRIO PÚBLICO FEDERAL", true},
		// Natural persons must NOT be classified as companies.
		{"João da Silva", false},
		{"Maria Aparecida de Sousa", false},
		{"Sergio Cabral Coelho", false},
		{"Ana Sá", false}, // "SA" appears only inside a token, not standalone
		{"Rosana Meireles", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isCompanyName(tc.name); got != tc.want {
			t.Fatalf("isCompanyName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClassFilterJuriBoundary(t *testing.T) {
	// Regression: "juri" used to match JURISDICAO/JURIDICA via substring.
	tests := []struct {
		nome string
		want bool
	}{
		{"CONFLITO DE JURISDICAO", false},
		{"JURISDICAO VOLUNTARIA", false},
		{"ACAO PENAL DE COMPETENCIA DO JURI", true},   // whole-word "juri" (also "penal")
		{"TRIBUNAL DO JÚRI", true},                    // phrase marker
		{"HABILITACAO DE JURISDICAO JURIDICA", false}, // neither token is whole-word "juri"
	}
	for _, tc := range tests {
		// codigoClasse 9999 forces the keyword fallback path.
		if got := isCriminalOrImprobidadeClass(tc.nome, "9999"); got != tc.want {
			t.Fatalf("isCriminalOrImprobidadeClass(%q, keyword) = %v, want %v", tc.nome, got, tc.want)
		}
	}
}

func TestEndpointForGroup(t *testing.T) {
	items := parseFixture(t)
	group := groupByProcesso(items)["10000000020234013700"]
	if got := endpointForGroup(group); got != "api_publica_trf1" {
		t.Fatalf("endpointForGroup = %q, want api_publica_trf1", got)
	}
	if got := endpointForGroup([]Item{{}}); got != "" {
		t.Fatalf("a publication with no tribunal must not resolve an endpoint, got %q", got)
	}
}

func TestSnippetStripsTagsAndTruncates(t *testing.T) {
	got := snippet("<p>Intimação de <b>JOÃO</b>.</p>", 500)
	if got != "Intimação de JOÃO." {
		t.Fatalf("unexpected snippet: %q", got)
	}
	long := snippet("<b>aaaaaaaaaa", 5)
	if len([]rune(long)) != 5 {
		t.Fatalf("expected truncation to 5 runes, got %d", len([]rune(long)))
	}
}

// DJEN substring-matches nomeParte, so a search for a politician answers with the
// cases of everyone whose name merely contains it. These are the real names that
// a "SERGIO CABRAL" search pulled into the database before this guard existed.
func TestGroupNamesParty_RejectsSubstringStrangers(t *testing.T) {
	strangers := []string{
		"ALEXANDRE SERGIO CABRAL DE BRITO",
		"EDUARDO SERGIO CABRAL DE LIMA",
		"PAULO SERGIO CABRAL DUARTE",
		"SERGIO CABRAL DA SILVA",
		"JULIANA CABRAL DE LIMA OLIVEIRA",
	}
	for _, s := range strangers {
		group := []Item{{Destinatarios: []Destinatario{{Nome: s, Polo: "P"}}}}
		if groupNamesParty(group, "SERGIO CABRAL") {
			t.Errorf("%q is not SERGIO CABRAL; registering their case tracks a stranger", s)
		}
	}

	// The politician himself must still match, accents and co-party marker included.
	for _, real := range []string{"SERGIO CABRAL", "sergio cabral", "SÉRGIO CABRAL E OUTROS (3)"} {
		group := []Item{{Destinatarios: []Destinatario{{Nome: real, Polo: "P"}}}}
		if !groupNamesParty(group, "SERGIO CABRAL") {
			t.Errorf("%q is SERGIO CABRAL and must match", real)
		}
	}
}

// Name mode searches legal names only. Aliases are ballot nicknames; courts write
// legal names, so an alias search returns substring noise and nothing else.
func TestSearchNamesFor_SearchesLegalNameOnlyNotAliases(t *testing.T) {
	pol := memgraph.PoliticianNames{
		ID:      "pol_1",
		Name:    "LUIZ INACIO LULA DA SILVA",
		Aliases: []string{"LULA", "LULA DA SILVA"},
	}
	got := searchNamesFor(pol)
	if len(got) != 1 || got[0] != "LUIZ INACIO LULA DA SILVA" {
		t.Fatalf("searchNamesFor = %v, want only the legal name", got)
	}

	// The aliases stay in the matching index — recognising a nickname a court did
	// print is free; only searching for one is not.
	if id, ok := matchPolitician("LULA", buildPoliticianIndex([]memgraph.PoliticianNames{pol})); !ok || id != "pol_1" {
		t.Fatal("aliases must remain matchable in the politician index")
	}
}
