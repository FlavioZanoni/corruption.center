package sanctions

import (
	"strings"
	"testing"
)

func TestClassifyDocument(t *testing.T) {
	cases := []struct {
		in                     string
		wantCPF, wantCNPJ, wMk string
	}{
		{"12.345.678/0001-95", "", "12345678000195", ""}, // full CNPJ
		{"72632078000130", "", "72632078000130", ""},     // raw CNPJ
		{"215.805.453-00", "21580545300", "", ""},        // full CPF
		{"***.456.789-**", "", "", "456789"},             // masked CPF (portal)
		{"***456789**", "", "", "456789"},                // masked, no punctuation
		{"", "", "", ""},                                 // empty
		{"JOSE DA SILVA", "", "", ""},                    // name only
	}
	for _, c := range cases {
		cpf, cnpj, mk := classifyDocument(c.in)
		if cpf != c.wantCPF || cnpj != c.wantCNPJ || mk != c.wMk {
			t.Errorf("classifyDocument(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, cpf, cnpj, mk, c.wantCPF, c.wantCNPJ, c.wMk)
		}
	}
}

// TestMaskedMatchesFullCPF pins the contract the Cypher matcher implements:
// the 6 visible middle digits of a masked CPF equal digits [3:9] of the full
// 11-digit CPF stored on a Politician node.
func TestMaskedMatchesFullCPF(t *testing.T) {
	fullCPF := "21580545300"
	masked := "***." + fullCPF[3:6] + "." + fullCPF[6:9] + "-**"
	_, _, mk := classifyDocument(masked)
	if mk != fullCPF[3:9] {
		t.Fatalf("masked middle = %q, want %q", mk, fullCPF[3:9])
	}
}

func TestMapCeisCnep(t *testing.T) {
	body := []byte(`[
      {
        "id": 1001,
        "dataInicioSancao": "01/02/2022",
        "dataFimSancao": "01/02/2024",
        "tipoSancao": {"descricaoResumida": "Inidônea", "descricaoPortal": "Declaração de inidoneidade"},
        "orgaoSancionador": {"nome": "Ministério da Saúde"},
        "sancionado": {"nome": "EMPRESA FANTASMA LTDA", "codigoFormatado": "12.345.678/0001-95"},
        "pessoa": {"cnpjFormatado": "12.345.678/0001-95", "nome": "EMPRESA FANTASMA LTDA", "tipo": "JURIDICA"},
        "linkPublicacao": "https://portaldatransparencia.gov.br/sancoes/ceis/1001",
        "numeroProcesso": "25000.123456/2021-99"
      },
      {
        "id": 1002,
        "dataInicioSancao": "2023-05-10",
        "tipoSancao": {"descricaoResumida": "Suspensa"},
        "orgaoSancionador": {"nome": "Prefeitura X"},
        "pessoa": {"cpfFormatado": "***.456.789-**", "nome": "FULANO", "tipo": "FISICA"},
        "numeroProcesso": "abc"
      }
    ]`)
	recs, err := mapCGUPage(RegistryCEIS, body)
	if err != nil {
		t.Fatalf("mapCGUPage: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}

	a := recs[0]
	if a.Registry != RegistryCEIS || a.EntryID != "1001" {
		t.Errorf("bad registry/entry: %+v", a)
	}
	if a.CNPJ != "12345678000195" {
		t.Errorf("cnpj = %q", a.CNPJ)
	}
	if a.SanctionType != "Inidônea" || a.Organ != "Ministério da Saúde" {
		t.Errorf("type/organ = %q / %q", a.SanctionType, a.Organ)
	}
	if a.DateStart != "2022-02-01" || a.DateEnd != "2024-02-01" {
		t.Errorf("dates = %q / %q", a.DateStart, a.DateEnd)
	}
	if a.ProcessRef != "25000.123456/2021-99" {
		t.Errorf("process = %q", a.ProcessRef)
	}
	if a.SourceURL != "https://portaldatransparencia.gov.br/sancoes/ceis/1001" {
		t.Errorf("source_url = %q", a.SourceURL)
	}

	b := recs[1]
	if b.MaskedCPF != "456789" {
		t.Errorf("masked = %q", b.MaskedCPF)
	}
	if b.CNPJ != "" || b.CPF != "" {
		t.Errorf("expected masked only, got cpf=%q cnpj=%q", b.CPF, b.CNPJ)
	}
	if b.DateStart != "2023-05-10" {
		t.Errorf("dateStart = %q", b.DateStart)
	}
	if !strings.HasPrefix(b.SourceURL, "https://portaldatransparencia.gov.br/sancoes/ceis?id=1002") {
		t.Errorf("fallback source_url = %q", b.SourceURL)
	}
}

func TestMapCeaf(t *testing.T) {
	body := []byte(`[
      {
        "id": 55,
        "dataPublicacao": "15/03/2021",
        "punicao": {"cpfPunidoFormatado": "***.111.222-**", "nomePunido": "SERVIDOR PUBLICO", "processo": "00190.000111/2020-10"},
        "tipoPunicao": {"descricao": "Demissão"},
        "pessoa": {"cpfFormatado": "***.111.222-**", "nome": "SERVIDOR PUBLICO", "tipo": "FISICA"},
        "orgaoLotacao": {"nome": "INSS"}
      }
    ]`)
	recs, err := mapCGUPage(RegistryCEAF, body)
	if err != nil {
		t.Fatalf("mapCGUPage ceaf: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1, got %d", len(recs))
	}
	r := recs[0]
	if r.Registry != RegistryCEAF || r.EntryID != "55" {
		t.Errorf("registry/entry: %+v", r)
	}
	if r.MaskedCPF != "111222" {
		t.Errorf("masked = %q", r.MaskedCPF)
	}
	if r.SanctionType != "Demissão" || r.Organ != "INSS" {
		t.Errorf("type/organ = %q / %q", r.SanctionType, r.Organ)
	}
	if r.ProcessRef != "00190.000111/2020-10" {
		t.Errorf("process = %q", r.ProcessRef)
	}
	if r.SourceURL == "" {
		t.Errorf("ceaf must still carry a source_url")
	}
}

func TestMapLeniencia(t *testing.T) {
	body := []byte(`[
      {
        "id": 7,
        "dataInicioAcordo": "10/01/2020",
        "dataFimAcordo": "10/01/2025",
        "orgaoResponsavel": "CGU",
        "situacaoAcordo": "Em vigor",
        "sancoes": [
          {"razaoSocial": "CONSTRUTORA A", "cnpjFormatado": "11.111.111/0001-11"},
          {"razaoSocial": "CONSTRUTORA B", "cnpj": "22222222000122"}
        ]
      }
    ]`)
	recs, err := mapCGUPage(RegistryLeniencia, body)
	if err != nil {
		t.Fatalf("mapCGUPage leniencia: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].EntryID == recs[1].EntryID {
		t.Errorf("entry ids must be unique: %q", recs[0].EntryID)
	}
	if recs[0].CNPJ != "11111111000111" || recs[1].CNPJ != "22222222000122" {
		t.Errorf("cnpjs = %q / %q", recs[0].CNPJ, recs[1].CNPJ)
	}
	if recs[0].EntryID != "7-11111111000111" {
		t.Errorf("entry id = %q", recs[0].EntryID)
	}
	if !strings.Contains(recs[0].SanctionType, "Acordo de Leniência") {
		t.Errorf("type = %q", recs[0].SanctionType)
	}
	if recs[0].DateStart != "2020-01-10" || recs[0].DateEnd != "2025-01-10" {
		t.Errorf("dates = %q / %q", recs[0].DateStart, recs[0].DateEnd)
	}
}

// Two document-less companies in the SAME agreement must produce distinct,
// deterministic EntryIDs (not both "7-"), so they map to separate Sanction nodes
// instead of merging into one with wrong entity attribution.
func TestMapLenienciaDocumentlessDistinct(t *testing.T) {
	body := []byte(`[
      {
        "id": 7,
        "orgaoResponsavel": "CGU",
        "situacaoAcordo": "Em vigor",
        "sancoes": [
          {"razaoSocial": "Construtora Ação Ltda"},
          {"razaoSocial": "Empresa Beta S.A."}
        ]
      }
    ]`)
	recs, err := mapCGUPage(RegistryLeniencia, body)
	if err != nil {
		t.Fatalf("mapCGUPage leniencia: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].EntryID == recs[1].EntryID {
		t.Fatalf("document-less companies collided on EntryID %q", recs[0].EntryID)
	}
	if recs[0].EntryID != "7-CONSTRUTORA-ACAO-LTDA" {
		t.Errorf("entry id[0] = %q, want %q", recs[0].EntryID, "7-CONSTRUTORA-ACAO-LTDA")
	}
	if recs[1].EntryID != "7-EMPRESA-BETA-S-A" {
		t.Errorf("entry id[1] = %q, want %q", recs[1].EntryID, "7-EMPRESA-BETA-S-A")
	}

	// Stable across a second mapper invocation (Sanction.id is the merge key).
	recs2, err := mapCGUPage(RegistryLeniencia, body)
	if err != nil {
		t.Fatalf("mapCGUPage leniencia (2nd): %v", err)
	}
	if recs2[0].EntryID != recs[0].EntryID || recs2[1].EntryID != recs[1].EntryID {
		t.Errorf("EntryIDs not stable across invocations: %q/%q vs %q/%q",
			recs[0].EntryID, recs[1].EntryID, recs2[0].EntryID, recs2[1].EntryID)
	}
}

// When both document AND name are empty, distinct records still get distinct
// EntryIDs via their index within the agreement.
func TestMapLenienciaDocumentlessNamelessFallback(t *testing.T) {
	body := []byte(`[
      {
        "id": 7,
        "orgaoResponsavel": "CGU",
        "situacaoAcordo": "Em vigor",
        "sancoes": [ {}, {} ]
      }
    ]`)
	recs, err := mapCGUPage(RegistryLeniencia, body)
	if err != nil {
		t.Fatalf("mapCGUPage leniencia: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].EntryID == recs[1].EntryID {
		t.Fatalf("nameless companies collided on EntryID %q", recs[0].EntryID)
	}
	if recs[0].EntryID != "7-0" || recs[1].EntryID != "7-1" {
		t.Errorf("index-fallback entry ids = %q / %q, want 7-0 / 7-1", recs[0].EntryID, recs[1].EntryID)
	}
}

func TestCompanyNameSlug(t *testing.T) {
	cases := map[string]string{
		"Construtora Ação Ltda": "CONSTRUTORA-ACAO-LTDA",
		"Empresa Beta S.A.":     "EMPRESA-BETA-S-A",
		"  José & Filhos  ":     "JOSE-FILHOS",
		"RAZÃO SOCIAL":          "RAZAO-SOCIAL",
		"":                      "",
		"...":                   "",
	}
	for in, want := range cases {
		if got := companyNameSlug(in); got != want {
			t.Errorf("companyNameSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// Real header + rows from resp-contas-julgadas-irregulares.csv (7 columns,
// DELIBERACAO is an acórdão URL).
const tcuIrregularCSV = `"NOME"|"CPF_CNPJ"|"PROCESSO"|"DELIBERACAO"|"DATA TRANSITO JULGADO"|"UF"|"MUNICIPIO"
"/VR TRANSPORTES E LOCACAO DE VEICULOS LTDA"|"72632078000130"|"029.212/2019-7"|"https://contas.tcu.gov.br/pesquisaJurisprudencia/#/resultado/acordao-completo/2921220197.PROC"|"29/09/2022"|""|""
"JOAO DA SILVA"|"215.805.453-00"|"005.000/2018-1"|"https://contas.tcu.gov.br/x"|"01/01/2020"|"MA"|"SANTA INES"`

func TestParseTCUIrregular(t *testing.T) {
	f := tcuFile{Registry: RegistryTCUIrregular, Filename: "resp-contas-julgadas-irregulares.csv", SanctionType: "Contas julgadas irregulares"}
	recs, err := parseTCUCSV(strings.NewReader(tcuIrregularCSV), f, "https://sites.tcu.gov.br/file.csv")
	if err != nil {
		t.Fatalf("parseTCUCSV: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2, got %d", len(recs))
	}
	org := recs[0]
	if org.CNPJ != "72632078000130" {
		t.Errorf("cnpj = %q", org.CNPJ)
	}
	if org.Organ != "Tribunal de Contas da União" {
		t.Errorf("organ = %q", org.Organ)
	}
	if org.ProcessRef != "029.212/2019-7" {
		t.Errorf("process = %q", org.ProcessRef)
	}
	if !strings.HasPrefix(org.SourceURL, "https://contas.tcu.gov.br/pesquisaJurisprudencia") {
		t.Errorf("source_url should use the deliberação URL: %q", org.SourceURL)
	}
	if org.EntryID != "0292122019"+"7"+"-"+"72632078000130" && org.EntryID != "02921220197-72632078000130" {
		t.Errorf("entry id = %q", org.EntryID)
	}

	person := recs[1]
	if person.CPF != "21580545300" {
		t.Errorf("cpf = %q", person.CPF)
	}
	if person.DateStart != "2020-01-01" {
		t.Errorf("dateStart = %q", person.DateStart)
	}
}

// Real header from inabilitados-funcao-publica.csv (9 columns; DELIBERACAO is an
// acórdão number, not a URL, and there are DATA FINAL / DATA ACORDAO columns).
const tcuInabilitadoCSV = `"NOME"|"CPF"|"PROCESSO"|"DELIBERACAO"|"DATA TRANSITO JULGADO"|"DATA FINAL"|"DATA ACORDAO"|"UF"|"MUNICIPIO"
"ABDALA GOMES SANTOS"|"215.805.453-00"|"026.615/2020-7"|"AC-000738/2022-PL"|"16/07/2022"|"16/07/2027"|"06/04/2022"|"MA"|"SANTA INES"`

func TestParseTCUInabilitado(t *testing.T) {
	f := tcuFile{Registry: RegistryTCUInabilitado, Filename: "inabilitados-funcao-publica.csv", SanctionType: "Inabilitado para função pública"}
	recs, err := parseTCUCSV(strings.NewReader(tcuInabilitadoCSV), f, "https://sites.tcu.gov.br/file.csv")
	if err != nil {
		t.Fatalf("parseTCUCSV: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1, got %d", len(recs))
	}
	r := recs[0]
	if r.CPF != "21580545300" {
		t.Errorf("cpf = %q", r.CPF)
	}
	if r.DateStart != "2022-04-06" { // DATA ACORDAO preferred as start
		t.Errorf("dateStart = %q", r.DateStart)
	}
	if r.DateEnd != "2027-07-16" { // DATA FINAL
		t.Errorf("dateEnd = %q", r.DateEnd)
	}
	if r.SanctionType != "Inabilitado para função pública" {
		t.Errorf("type = %q", r.SanctionType)
	}
	// DELIBERACAO is not a URL -> source_url constructed from process number.
	if !strings.HasPrefix(r.SourceURL, "https://contas.tcu.gov.br/pesquisaJurisprudencia") {
		t.Errorf("constructed source_url = %q", r.SourceURL)
	}
	if r.SourceURL == "https://sites.tcu.gov.br/file.csv" {
		t.Errorf("should not fall back to file url when process present")
	}
}

func TestNormalizeDate(t *testing.T) {
	cases := map[string]string{
		"29/09/2022":           "2022-09-29",
		"2023-05-10":           "2023-05-10",
		"":                     "",
		"garbage":              "",
		"2021-01-02T10:00:00Z": "2021-01-02",
	}
	for in, want := range cases {
		if got := normalizeDate(in); got != want {
			t.Errorf("normalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectedRegistries(t *testing.T) {
	if got := selectedRegistries(nil); len(got) != 5 {
		t.Errorf("default should be 5 registries, got %v", got)
	}
	got := selectedRegistries([]string{"tcu", "ceis", "ceis"})
	if len(got) != 2 || got[0] != "tcu" || got[1] != "ceis" {
		t.Errorf("dedup/order failed: %v", got)
	}
}
