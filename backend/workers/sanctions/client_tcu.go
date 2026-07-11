package sanctions

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTCUBaseURL = "https://sites.tcu.gov.br/dados-abertos/inidoneos-irregulares/arquivos"

// tcuFile describes one downloadable TCU registry CSV.
type tcuFile struct {
	Registry     string
	Filename     string
	SanctionType string
}

// tcuFiles are the three registries in scope. The "implicação eleitoral" list is
// a filtered view of the irregular-accounts list and is intentionally omitted.
var tcuFiles = []tcuFile{
	{Registry: RegistryTCUIrregular, Filename: "resp-contas-julgadas-irregulares.csv", SanctionType: "Contas julgadas irregulares"},
	{Registry: RegistryTCUInabilitado, Filename: "inabilitados-funcao-publica.csv", SanctionType: "Inabilitado para função pública"},
	{Registry: RegistryTCUInidoneo, Filename: "licitantes-inidoneos.csv", SanctionType: "Licitante inidôneo"},
}

// runTCU downloads and applies each TCU registry CSV.
func (w *Worker) runTCU(ctx context.Context, stats *Stats) error {
	base := strings.TrimRight(strings.TrimSpace(w.opts.TCUBaseURL), "/")
	if base == "" {
		base = defaultTCUBaseURL
	}
	client := &http.Client{Timeout: 120 * time.Second}

	for _, f := range tcuFiles {
		fileURL := base + "/" + f.Filename
		records, err := downloadAndParseTCU(ctx, client, fileURL, f)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Registry, err)
		}
		seen := 0
		for _, rec := range records {
			if err := w.apply(ctx, rec, stats); err != nil {
				return err
			}
			seen++
		}
		if !w.opts.DryRun && w.pg != nil {
			if err := w.pg.UpsertSanctionImportState(ctx, f.Registry, 0, seen, time.Now().UTC()); err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadAndParseTCU(ctx context.Context, client *http.Client, fileURL string, f tcuFile) ([]SanctionRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tcu: build request: %w", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tcu: request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tcu: %s status=%d", fileURL, res.StatusCode)
	}
	return parseTCUCSV(res.Body, f, fileURL)
}

// parseTCUCSV parses a pipe-delimited, double-quoted, UTF-8 TCU CSV using its
// header row (column layout differs between the three files), producing
// normalized records. fileURL is the deep-link fallback for source_url.
func parseTCUCSV(r io.Reader, f tcuFile, fileURL string) ([]SanctionRecord, error) {
	cr := csv.NewReader(r)
	cr.Comma = '|'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("tcu: read header: %w", err)
	}
	cols := map[string]int{}
	for i, h := range header {
		cols[normalizeHeader(h)] = i
	}
	get := func(row []string, name string) string {
		if idx, ok := cols[name]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	var out []SanctionRecord
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tcu: read row: %w", err)
		}

		name := get(row, "NOME")
		doc := firstNonEmpty(get(row, "CPF_CNPJ"), get(row, "CPF"))
		processo := get(row, "PROCESSO")
		deliberacao := get(row, "DELIBERACAO")
		dateStart := firstNonEmpty(normalizeDate(get(row, "DATA ACORDAO")), normalizeDate(get(row, "DATA TRANSITO JULGADO")))
		dateEnd := normalizeDate(get(row, "DATA FINAL"))

		if name == "" && doc == "" && processo == "" {
			continue
		}

		rec := SanctionRecord{
			Registry:     f.Registry,
			EntryID:      tcuEntryID(processo, doc, name),
			SanctionType: f.SanctionType,
			Organ:        "Tribunal de Contas da União",
			DateStart:    dateStart,
			DateEnd:      dateEnd,
			ProcessRef:   processo,
			SourceURL:    tcuSourceURL(deliberacao, processo, fileURL),
			Name:         name,
		}
		rec.CPF, rec.CNPJ, rec.MaskedCPF = classifyDocument(doc)
		out = append(out, rec)
	}
	return out, nil
}

func normalizeHeader(h string) string {
	h = strings.TrimSpace(h)
	h = strings.TrimPrefix(h, "\ufeff") // strip UTF-8 BOM on first header cell
	return strings.ToUpper(h)
}

// tcuEntryID builds a registry-unique key from the process number plus the
// document (or normalized name when no document is present).
func tcuEntryID(processo, doc, name string) string {
	proc := digitsOnly(processo)
	subject := digitsOnly(doc)
	if subject == "" {
		subject = slugName(name)
	}
	switch {
	case proc != "" && subject != "":
		return proc + "-" + subject
	case proc != "":
		return proc
	default:
		return subject
	}
}

func slugName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}

// tcuSourceURL prefers an explicit deliberação URL, then a constructed TCU
// jurisprudence deep link from the process number, then the dataset file URL.
func tcuSourceURL(deliberacao, processo, fileURL string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(deliberacao)), "http") {
		return strings.TrimSpace(deliberacao)
	}
	if p := stripLeadingZeros(digitsOnly(processo)); p != "" {
		return "https://contas.tcu.gov.br/pesquisaJurisprudencia/#/resultado/acordao-completo/" + p + ".PROC"
	}
	return fileURL
}

func stripLeadingZeros(s string) string {
	return strings.TrimLeft(s, "0")
}
