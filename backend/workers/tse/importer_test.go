package tse

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// latin1 encodes a fixture the way TSE actually ships its CSVs (ISO-8859-1), so
// accented values like "MÉDIA" survive the importer's Latin-1 decoding. A UTF-8
// fixture would arrive mangled and silently fail to match.
func latin1(t *testing.T, s string) io.Reader {
	t.Helper()
	encoded, _, err := transform.String(charmap.ISO8859_1.NewEncoder(), s)
	if err != nil {
		t.Fatalf("encode fixture as latin-1: %v", err)
	}
	return strings.NewReader(encoded)
}

func TestImportYear_FiltersTurnAliasesAndActiveFalse(t *testing.T) {
	consulta := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;123;12345678900;ELEITO;SP;ABC;JOSE DA SILVA;JOSE;#NE",
		"2022;2;PRESIDENTE;123;12345678900;ELEITO;SP;ABC;JOSE DA SILVA;JOSE;J. SILVA",
		"2022;1;VEREADOR;999;99999999999;ELEITO;RJ;DEF;IGNORA;IGNORA;#NE",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(strings.NewReader(consulta), "")
	if err != nil {
		t.Fatalf("ImportYear returned error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	rec := result.Records[0]
	if rec.CandidateSQ != "123" {
		t.Fatalf("expected SQ 123, got %s", rec.CandidateSQ)
	}
	if len(rec.NameAliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(rec.NameAliases))
	}
	if len(rec.TSEProfileURLs) != 1 {
		t.Fatalf("expected 1 profile url, got %d", len(rec.TSEProfileURLs))
	}
	if rec.Active {
		t.Fatalf("expected active=false for TSE import")
	}
}

func TestImportYearFromZipFiles_DeletesBrasilAndProcessedFiles(t *testing.T) {
	base := t.TempDir()
	consultaZip := filepath.Join(base, "consulta.zip")

	consultaRows := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;10;11111111111;ELEITO;BR;P1;A;A;#NE",
		"2022;2;PRESIDENTE;10;11111111111;ELEITO;BR;P1;A;A;#NE",
		"2022;1;DEPUTADO FEDERAL;20;22222222222;ELEITO POR QP;SP;P2;B;B;#NE",
	}, "\r\n") + "\r\n"

	if err := createZip(consultaZip, map[string]string{
		"consulta_cand_2022_SP.csv":     consultaRows,
		"consulta_cand_2022_BR.csv":     consultaRows,
		"consulta_cand_2022_BRASIL.csv": "BIG;UNUSED\r\n",
	}); err != nil {
		t.Fatalf("create consulta zip: %v", err)
	}

	result, err := ImportYearFromZipFiles(2022, consultaZip, base, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportYearFromZipFiles returned error: %v", err)
	}

	if result.Stats.FilesDeleted == 0 {
		t.Fatalf("expected processed files to be deleted")
	}
	if len(result.Records) == 0 {
		t.Fatalf("expected records from zip import")
	}
}

func TestImportYearFromZipFiles_FailsWhenDiskThresholdTooHigh(t *testing.T) {
	base := t.TempDir()
	consultaZip := filepath.Join(base, "consulta.zip")

	content := "ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO\r\n"
	if err := createZip(consultaZip, map[string]string{"consulta_cand_2022_SP.csv": content}); err != nil {
		t.Fatalf("create consulta zip: %v", err)
	}

	_, err := ImportYearFromZipFiles(2022, consultaZip, base, ImportOptions{
		MinDiskBytes: ^uint64(0),
		MinMemBytes:  1,
	})
	if err == nil {
		t.Fatalf("expected disk threshold error")
	}
}

func createZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			_ = w.Close()
			return err
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			_ = w.Close()
			return err
		}
	}
	return w.Close()
}

// SQ_CANDIDATO is only unique per state in the older TSE files: in 2006, SQ 10204
// is a different elected deputy in BA, in PE and in AL. Keying on SQ alone drops
// two of the three, and joins the CPF of whichever row won the race, which would
// attach one politician's document to another and link their sanctions and cases
// to the wrong human.
func TestImportYear_SameSQInDifferentStatesAreDifferentPeople(t *testing.T) {
	consulta := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2006;1;DEPUTADO FEDERAL;10204;11111111111;ELEITO;BA;P1;CLAUDIO CAJADO;CAJADO;#NE",
		"2006;1;DEPUTADO FEDERAL;10204;22222222222;MÉDIA;AL;P2;GIVALDO CARIMBAO;CARIMBAO;#NE",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(latin1(t, consulta), "")
	if err != nil {
		t.Fatalf("ImportYear returned error: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected both deputies, got %d record(s): %+v", len(result.Records), result.Records)
	}

	cpfByName := map[string]string{}
	for _, r := range result.Records {
		cpfByName[r.Name] = r.CPF
	}
	if got := cpfByName["CLAUDIO CAJADO"]; got != "11111111111" {
		t.Fatalf("CLAUDIO CAJADO got CPF %q, want 11111111111 (a cross-state SQ collision handed him the wrong document)", got)
	}
	if got := cpfByName["GIVALDO CARIMBAO"]; got != "22222222222" {
		t.Fatalf("GIVALDO CARIMBAO got CPF %q, want 22222222222", got)
	}
}

// "MÉDIA" is how 2006 spells what later years call "ELEITO POR MÉDIA". Dropping
// it loses 79 of the 513 federal deputies elected that year.
func TestImportYear_AcceptsLegacyElectedLabels(t *testing.T) {
	consulta := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2006;1;DEPUTADO FEDERAL;1;33333333333;MÉDIA;SP;P1;WINNER BY AVERAGE;X;#NE",
		"2006;1;DEPUTADO FEDERAL;2;44444444444;SUPLENTE;SP;P1;NOT ELECTED;Y;#NE",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(latin1(t, consulta), "")
	if err != nil {
		t.Fatalf("ImportYear returned error: %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].CPF != "33333333333" {
		t.Fatalf("expected the MÉDIA winner only, got %+v", result.Records)
	}
}

// The photo URL keys on the candidate's own SQ and UF, not on the file's. Getting
// the pairing wrong is invisible: TSE answers 200 with a placeholder rather than
// 404, so the base would fill up with grey silhouettes and look merely photo-less.
func TestImportYear_BuildsPhotoURLFromCandidateSQAndUF(t *testing.T) {
	consulta := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;280001607829;12345678900;ELEITO;BR;PT;LUIZ INACIO LULA DA SILVA;LULA;#NULO#",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(latin1(t, consulta), "2040602022")
	if err != nil {
		t.Fatalf("ImportYear: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	got := result.Records[0].PhotoURL
	want := "https://divulgacandcontas.tse.jus.br/divulga/rest/arquivo/img/2040602022/280001607829/BR"
	if got != want {
		t.Fatalf("PhotoURL = %q, want %q", got, want)
	}
}

// Without an election id (2002 and earlier) the year still imports; it just has
// no photos. It must not emit a half-built URL.
func TestImportYear_NoElectionIDMeansNoPhoto(t *testing.T) {
	consulta := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;NR_CPF_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2002;1;PRESIDENTE;9574;12345678900;ELEITO;BR;PT;LUIZ INACIO LULA DA SILVA;LULA;#NULO#",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(latin1(t, consulta), "")
	if err != nil {
		t.Fatalf("ImportYear: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	if got := result.Records[0].PhotoURL; got != "" {
		t.Fatalf("expected no photo URL without an election id, got %q", got)
	}
}
