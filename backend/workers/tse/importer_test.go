package tse

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportYear_FiltersTurnAliasesAndActiveFalse(t *testing.T) {
	elections := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;123;ELEITO;SP;ABC;JOSE DA SILVA;JOSE;#NE",
		"2022;2;PRESIDENTE;123;ELEITO;SP;ABC;JOSE DA SILVA;JOSE;J. SILVA",
		"2022;1;DEPUTADO ESTADUAL;999;ELEITO;RJ;DEF;IGNORA;IGNORA;#NE",
	}, "\r\n") + "\r\n"

	candidates := strings.Join([]string{
		"ANO_ELEICAO;SQ_CANDIDATO;NR_CPF_CANDIDATO",
		"2022;123;12345678900",
	}, "\r\n") + "\r\n"

	result, err := ImportYear(strings.NewReader(elections), strings.NewReader(candidates))
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

func TestImportYear_FixturesProduceRows(t *testing.T) {
	root := filepath.Join("..", "..", "..", "docs", "workerDetails", "tse_mock")
	tests := []struct {
		name       string
		elections  string
		candidates string
	}{
		{name: "2006", elections: "elections_2006.csv", candidates: "candidates_2006.csv"},
		{name: "2022", elections: "elections_2022.csv", candidates: "candidates_2022.csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			electionsBytes, err := os.ReadFile(filepath.Join(root, tt.elections))
			if err != nil {
				t.Fatalf("read elections fixture: %v", err)
			}
			candidatesBytes, err := os.ReadFile(filepath.Join(root, tt.candidates))
			if err != nil {
				t.Fatalf("read candidates fixture: %v", err)
			}
			result, err := ImportYear(strings.NewReader(string(electionsBytes)), strings.NewReader(string(candidatesBytes)))
			if err != nil {
				t.Fatalf("ImportYear returned error: %v", err)
			}
			if result.Stats.ElectionRowsRead == 0 {
				t.Fatalf("expected fixture rows to be read")
			}
			if result.Stats.CandidateRowsRead == 0 {
				t.Fatalf("expected candidate fixture rows to be read")
			}
		})
	}
}

func TestImportYearFromZipFiles_DeletesBrasilAndProcessedFiles(t *testing.T) {
	base := t.TempDir()
	votacaoZip := filepath.Join(base, "votacao.zip")
	consultaZip := filepath.Join(base, "consulta.zip")

	votacaoRows := strings.Join([]string{
		"ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO",
		"2022;1;PRESIDENTE;10;ELEITO;BR;P1;A;A;#NE",
		"2022;2;PRESIDENTE;10;ELEITO;BR;P1;A;A;#NE",
		"2022;1;DEPUTADO FEDERAL;20;ELEITO POR QP;SP;P2;B;B;#NE",
	}, "\r\n") + "\r\n"
	consultaRows := strings.Join([]string{
		"ANO_ELEICAO;SQ_CANDIDATO;NR_CPF_CANDIDATO",
		"2022;10;11111111111",
		"2022;20;22222222222",
	}, "\r\n") + "\r\n"

	if err := createZip(votacaoZip, map[string]string{
		"votacao_candidato_munzona_2022_BR.csv":     votacaoRows,
		"votacao_candidato_munzona_2022_SP.csv":     votacaoRows,
		"votacao_candidato_munzona_2022_BRASIL.csv": "BIG;UNUSED\r\n",
	}); err != nil {
		t.Fatalf("create votacao zip: %v", err)
	}
	if err := createZip(consultaZip, map[string]string{
		"consulta_cand_2022_SP.csv":     consultaRows,
		"consulta_cand_2022_BR.csv":     consultaRows,
		"consulta_cand_2022_BRASIL.csv": "BIG;UNUSED\r\n",
	}); err != nil {
		t.Fatalf("create consulta zip: %v", err)
	}

	result, err := ImportYearFromZipFiles(2022, votacaoZip, consultaZip, base, ImportOptions{})
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
	votacaoZip := filepath.Join(base, "votacao.zip")
	consultaZip := filepath.Join(base, "consulta.zip")

	content := "ANO_ELEICAO;NR_TURNO;DS_CARGO;SQ_CANDIDATO;DS_SIT_TOT_TURNO;SG_UF;SG_PARTIDO;NM_CANDIDATO;NM_URNA_CANDIDATO;NM_SOCIAL_CANDIDATO\r\n"
	if err := createZip(votacaoZip, map[string]string{"votacao_candidato_munzona_2022_SP.csv": content}); err != nil {
		t.Fatalf("create votacao zip: %v", err)
	}
	if err := createZip(consultaZip, map[string]string{"consulta_cand_2022_SP.csv": "SQ_CANDIDATO;NR_CPF_CANDIDATO\r\n"}); err != nil {
		t.Fatalf("create consulta zip: %v", err)
	}

	_, err := ImportYearFromZipFiles(2022, votacaoZip, consultaZip, base, ImportOptions{
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
