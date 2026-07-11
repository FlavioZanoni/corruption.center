package tse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeNull(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"   ":             "",
		"#NULO":           "",
		"#NE":             "",
		"-1":              "",
		"-3":              "",
		"  #NULO  ":       "",
		"12345678900":     "12345678900",
		"  12345678900  ": "12345678900",
		"#NULOX":          "#NULOX", // only the exact sentinels are nulled
		"-13":             "-13",
	}
	for in, want := range cases {
		if got := normalizeNull(in); got != want {
			t.Errorf("normalizeNull(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddAlias(t *testing.T) {
	t.Run("appends non-empty distinct alias", func(t *testing.T) {
		var aliases []string
		addAlias(&aliases, "JOSE", "JOSE DA SILVA")
		if len(aliases) != 1 || aliases[0] != "JOSE" {
			t.Fatalf("expected [JOSE], got %v", aliases)
		}
	})
	t.Run("null sentinel is skipped", func(t *testing.T) {
		var aliases []string
		addAlias(&aliases, "#NE", "JOSE DA SILVA")
		addAlias(&aliases, "-1", "JOSE DA SILVA")
		addAlias(&aliases, "", "JOSE DA SILVA")
		if len(aliases) != 0 {
			t.Fatalf("expected no aliases, got %v", aliases)
		}
	})
	t.Run("alias equal to legal name is skipped case-insensitively", func(t *testing.T) {
		var aliases []string
		addAlias(&aliases, "jose da silva", "JOSE DA SILVA")
		if len(aliases) != 0 {
			t.Fatalf("expected legal-name alias skipped, got %v", aliases)
		}
	})
	t.Run("duplicate alias is skipped case-insensitively", func(t *testing.T) {
		aliases := []string{"JOSE"}
		addAlias(&aliases, "jose", "JOSE DA SILVA")
		if len(aliases) != 1 {
			t.Fatalf("expected duplicate skipped, got %v", aliases)
		}
	})
}

func TestBuildProfileURL(t *testing.T) {
	got := buildProfileURL("2022", "123")
	want := "https://divulgacandcontas.tse.jus.br/divulga/#/candidato/2022/123"
	if got != want {
		t.Fatalf("buildProfileURL = %q, want %q", got, want)
	}
}

func TestRowToIndexAndCell(t *testing.T) {
	idx := rowToIndex([]string{" A ", "B", "C"})
	if idx["A"] != 0 || idx["B"] != 1 || idx["C"] != 2 {
		t.Fatalf("rowToIndex trimmed keys wrong: %v", idx)
	}
	row := []string{"x", "y"}
	if got := cell(row, idx, "A"); got != "x" {
		t.Fatalf("cell A = %q, want x", got)
	}
	// Column present in header but beyond the row length returns "".
	if got := cell(row, idx, "C"); got != "" {
		t.Fatalf("cell out-of-range = %q, want empty", got)
	}
	// Unknown key maps to index 0 (Go zero value); still bounded read.
	if got := cell([]string{"z"}, idx, "MISSING"); got != "z" {
		t.Fatalf("cell missing-key = %q", got)
	}
}

func TestEnsureHeaders(t *testing.T) {
	idx := map[string]int{"A": 0, "B": 1}
	if err := ensureHeaders(idx, []string{"A", "B"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	err := ensureHeaders(idx, []string{"A", "C"})
	if err == nil || !strings.Contains(err.Error(), "C") {
		t.Fatalf("expected missing-header error mentioning C, got %v", err)
	}
}

func TestNewCSVReaderLatin1_EmptyIsError(t *testing.T) {
	_, _, err := newCSVReaderLatin1(strings.NewReader(""))
	if err == nil {
		t.Fatalf("expected empty csv error")
	}
}

func TestNewCSVReaderLatin1_DecodesLatin1(t *testing.T) {
	// 0xE7 is 'ç' and 0xE3 is 'ã' in ISO-8859-1.
	raw := []byte{'N', 'O', 'M', 'E', ';', 'X', '\n', 0xE7, 0xE3, 'o', ';', 'y', '\n'}
	reader, headers, err := newCSVReaderLatin1(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := headers["NOME"]; !ok {
		t.Fatalf("expected NOME header, got %v", headers)
	}
	row, err := reader.Read()
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row[0] != "ção" {
		t.Fatalf("latin-1 decode = %q, want ção", row[0])
	}
}

func TestImportYear_MissingHeaderErrors(t *testing.T) {
	// Elections CSV missing DS_CARGO (required because enforceCargo is true).
	elections := "ANO_ELEICAO;NR_TURNO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO\r\n"
	candidates := "SQ_CANDIDATO;NR_CPF_CANDIDATO\r\n"
	if _, err := ImportYear(strings.NewReader(elections), strings.NewReader(candidates)); err == nil {
		t.Fatalf("expected missing DS_CARGO header error")
	}
}

func TestImportYear_MissingCandidateHeaderErrors(t *testing.T) {
	elections := "ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO\r\n"
	candidates := "SQ_CANDIDATO;WRONG\r\n"
	if _, err := ImportYear(strings.NewReader(elections), strings.NewReader(candidates)); err == nil {
		t.Fatalf("expected missing NR_CPF_CANDIDATO header error")
	}
}

func TestImportYear_StatsSkipCountersAndNullCPF(t *testing.T) {
	elections := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;1;ELEITO;SP;ABC;WINNER ONE;W1;#NE",   // winner, cpf present
		"2022;1;PRESIDENTE;2;ELEITO;SP;ABC;WINNER TWO;W2;#NE",   // winner, cpf is #NULO -> MissingCPF
		"2022;1;DEPUTADO ESTADUAL;3;ELEITO;RJ;DEF;X;X;#NE",      // skipped by cargo
		"2022;1;PRESIDENTE;4;NAO ELEITO;RJ;DEF;Y;Y;#NE",         // skipped by status
		"2022;;PRESIDENTE;5;ELEITO;RJ;DEF;Z;Z;#NE",              // invalid turn -> skipped invalid
	}, "\r\n") + "\r\n"
	candidates := strings.Join([]string{
		"SQ_CANDIDATO;NR_CPF_CANDIDATO",
		"1;12345678900",
		"2;#NULO",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(strings.NewReader(elections), strings.NewReader(candidates))
	if err != nil {
		t.Fatalf("ImportYear error: %v", err)
	}
	s := result.Stats
	if s.SkippedByCargo != 1 {
		t.Errorf("SkippedByCargo = %d, want 1", s.SkippedByCargo)
	}
	if s.SkippedByStatus != 1 {
		t.Errorf("SkippedByStatus = %d, want 1", s.SkippedByStatus)
	}
	if s.SkippedByInvalidRow != 1 {
		t.Errorf("SkippedByInvalidRow = %d, want 1", s.SkippedByInvalidRow)
	}
	if s.MissingCPF != 1 {
		t.Errorf("MissingCPF = %d, want 1 (SQ 2 has #NULO cpf)", s.MissingCPF)
	}
	if len(result.Records) != 1 || result.Records[0].CandidateSQ != "1" {
		t.Fatalf("expected only SQ 1 with valid CPF, got %+v", result.Records)
	}
	if result.Records[0].ElectionYear != 2022 {
		t.Errorf("ElectionYear = %d, want 2022", result.Records[0].ElectionYear)
	}
}

func TestCollectVotacaoFiles(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"VOTACAO_CANDIDATO_MUNZONA_2022_SP.csv",
		"VOTACAO_CANDIDATO_MUNZONA_2022_RJ.csv",
		"VOTACAO_CANDIDATO_MUNZONA_2022_BR.csv",
		"VOTACAO_CANDIDATO_MUNZONA_2018_SP.csv", // wrong year
		"UNRELATED.csv",
		"VOTACAO_CANDIDATO_MUNZONA_2022_SP.txt", // wrong ext
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ufFiles, brFile, err := collectVotacaoFiles(2022, dir)
	if err != nil {
		t.Fatalf("collectVotacaoFiles: %v", err)
	}
	if len(ufFiles) != 2 {
		t.Fatalf("expected 2 UF files, got %d: %v", len(ufFiles), ufFiles)
	}
	if !strings.HasSuffix(strings.ToUpper(brFile), "_BR.CSV") {
		t.Fatalf("expected BR file, got %q", brFile)
	}
}

func TestCollectVotacaoFiles_MissingDir(t *testing.T) {
	if _, _, err := collectVotacaoFiles(2022, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatalf("expected error for missing dir")
	}
}

func TestCollectConsultaFiles(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"CONSULTA_CAND_2022_SP.csv",
		"CONSULTA_CAND_2022_BR.csv",
		"CONSULTA_CAND_2020_SP.csv", // wrong year
		"OTHER.csv",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := collectConsultaFiles(2022, dir)
	if err != nil {
		t.Fatalf("collectConsultaFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 consulta files, got %d: %v", len(files), files)
	}
}

func TestCollectConsultaFiles_MissingDir(t *testing.T) {
	if _, err := collectConsultaFiles(2022, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatalf("expected error for missing dir")
	}
}
