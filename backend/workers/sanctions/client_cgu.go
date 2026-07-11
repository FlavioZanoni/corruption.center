package sanctions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCGUBaseURL = "https://api.portaldatransparencia.gov.br/api-de-dados"
	// Daytime cap is 90 req/min; keep a small margin. Interval ~0.7s/req.
	cguMinInterval = 700 * time.Millisecond
	cguMaxRetries  = 5
)

// cguClient talks to the Portal da Transparência "API de Dados". The chave-api-
// dados key is free (self-service signup) and sent on every request.
type cguClient struct {
	baseURL     string
	apiKey      string
	http        *http.Client
	minInterval time.Duration
	maxRetries  int
	last        time.Time
}

func newCGUClient(baseURL, apiKey string) *cguClient {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = defaultCGUBaseURL
	}
	return &cguClient{
		baseURL:     strings.TrimRight(base, "/"),
		apiKey:      strings.TrimSpace(apiKey),
		http:        &http.Client{Timeout: 45 * time.Second},
		minInterval: cguMinInterval,
		maxRetries:  cguMaxRetries,
	}
}

// cguRegistryPath maps a registry group to its API path segment.
func cguRegistryPath(group string) (string, string, bool) {
	switch group {
	case "ceis":
		return "ceis", RegistryCEIS, true
	case "cnep":
		return "cnep", RegistryCNEP, true
	case "ceaf":
		return "ceaf", RegistryCEAF, true
	case "leniencia":
		return "acordos-leniencia", RegistryLeniencia, true
	default:
		return "", "", false
	}
}

// getPage fetches one page of a registry as a raw JSON array, honoring the rate
// limit and retrying on 429/5xx with exponential backoff.
func (c *cguClient) getPage(ctx context.Context, path string, page int) ([]byte, error) {
	u, err := url.Parse(c.baseURL + "/" + path)
	if err != nil {
		return nil, fmt.Errorf("cgu: parse url: %w", err)
	}
	q := u.Query()
	q.Set("pagina", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		c.throttle(ctx)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("cgu: build request: %w", err)
		}
		req.Header.Set("chave-api-dados", c.apiKey)
		req.Header.Set("Accept", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("cgu: request %s p%d: %w", path, page, err)
			if !sleepBackoff(ctx, &backoff) {
				return nil, lastErr
			}
			continue
		}

		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()

		switch {
		case res.StatusCode == http.StatusOK:
			if readErr != nil {
				return nil, fmt.Errorf("cgu: read body %s p%d: %w", path, page, readErr)
			}
			return body, nil
		case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
			lastErr = fmt.Errorf("cgu: %s p%d status=%d body=%s", path, page, res.StatusCode, truncate(body, 200))
			if !sleepBackoff(ctx, &backoff) {
				return nil, lastErr
			}
			continue
		default:
			return nil, fmt.Errorf("cgu: %s p%d status=%d body=%s", path, page, res.StatusCode, truncate(body, 200))
		}
	}
	return nil, lastErr
}

func (c *cguClient) throttle(ctx context.Context) {
	if c.minInterval <= 0 {
		return
	}
	wait := c.minInterval - time.Since(c.last)
	if wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	c.last = time.Now()
}

func sleepBackoff(ctx context.Context, backoff *time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(*backoff):
	}
	*backoff *= 2
	if *backoff > 30*time.Second {
		*backoff = 30 * time.Second
	}
	return true
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}

// runCGURegistry paginates a CGU registry until an empty page and applies the
// matching policy to every record.
func (w *Worker) runCGURegistry(ctx context.Context, group string, stats *Stats) error {
	path, registry, ok := cguRegistryPath(group)
	if !ok {
		return fmt.Errorf("unknown cgu registry %q", group)
	}
	if strings.TrimSpace(w.opts.APIKey) == "" {
		return fmt.Errorf("TRANSPARENCIA_API_KEY is required for registry %s", group)
	}

	client := newCGUClient(w.opts.CGUBaseURL, w.opts.APIKey)
	page := 1
	seen := 0
	for {
		if w.opts.MaxPages > 0 && page > w.opts.MaxPages {
			break
		}
		body, err := client.getPage(ctx, path, page)
		if err != nil {
			return err
		}
		records, err := mapCGUPage(registry, body)
		if err != nil {
			return fmt.Errorf("map %s page %d: %w", group, page, err)
		}
		if len(records) == 0 {
			break
		}
		for _, rec := range records {
			if err := w.apply(ctx, rec, stats); err != nil {
				return err
			}
			seen++
		}
		page++
	}

	if !w.opts.DryRun && w.pg != nil {
		if err := w.pg.UpsertSanctionImportState(ctx, registry, page-1, seen, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// mapCGUPage decodes a raw JSON array for a registry and maps it to normalized
// records. Exported indirectly via per-registry mappers for unit testing.
func mapCGUPage(registry string, body []byte) ([]SanctionRecord, error) {
	switch registry {
	case RegistryCEIS, RegistryCNEP:
		var dtos []ceisCnepDTO
		if err := json.Unmarshal(body, &dtos); err != nil {
			return nil, err
		}
		out := make([]SanctionRecord, 0, len(dtos))
		for _, d := range dtos {
			out = append(out, mapCeisCnep(registry, d))
		}
		return out, nil
	case RegistryCEAF:
		var dtos []ceafDTO
		if err := json.Unmarshal(body, &dtos); err != nil {
			return nil, err
		}
		out := make([]SanctionRecord, 0, len(dtos))
		for _, d := range dtos {
			out = append(out, mapCeaf(d))
		}
		return out, nil
	case RegistryLeniencia:
		var dtos []lenienciaDTO
		if err := json.Unmarshal(body, &dtos); err != nil {
			return nil, err
		}
		out := make([]SanctionRecord, 0, len(dtos))
		for _, d := range dtos {
			out = append(out, mapLeniencia(d)...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown registry %q", registry)
	}
}

// ─── DTOs (only the fields the worker consumes) ───────────────────────────────

type pessoaDTO struct {
	CPFFormatado  string `json:"cpfFormatado"`
	CNPJFormatado string `json:"cnpjFormatado"`
	Nome          string `json:"nome"`
	Tipo          string `json:"tipo"`
}

type ceisCnepDTO struct {
	ID               int64  `json:"id"`
	DataInicioSancao string `json:"dataInicioSancao"`
	DataFimSancao    string `json:"dataFimSancao"`
	TipoSancao       struct {
		DescricaoResumida string `json:"descricaoResumida"`
		DescricaoPortal   string `json:"descricaoPortal"`
	} `json:"tipoSancao"`
	OrgaoSancionador struct {
		Nome string `json:"nome"`
	} `json:"orgaoSancionador"`
	Sancionado struct {
		Nome            string `json:"nome"`
		CodigoFormatado string `json:"codigoFormatado"`
	} `json:"sancionado"`
	Pessoa         pessoaDTO `json:"pessoa"`
	LinkPublicacao string    `json:"linkPublicacao"`
	NumeroProcesso string    `json:"numeroProcesso"`
}

type ceafDTO struct {
	ID             int64  `json:"id"`
	DataPublicacao string `json:"dataPublicacao"`
	Punicao        struct {
		CPFPunidoFormatado string `json:"cpfPunidoFormatado"`
		NomePunido         string `json:"nomePunido"`
		Processo           string `json:"processo"`
	} `json:"punicao"`
	TipoPunicao struct {
		Descricao string `json:"descricao"`
	} `json:"tipoPunicao"`
	Pessoa       pessoaDTO `json:"pessoa"`
	OrgaoLotacao struct {
		Nome string `json:"nome"`
	} `json:"orgaoLotacao"`
}

type lenienciaDTO struct {
	ID               int64  `json:"id"`
	DataInicioAcordo string `json:"dataInicioAcordo"`
	DataFimAcordo    string `json:"dataFimAcordo"`
	OrgaoResponsavel string `json:"orgaoResponsavel"`
	SituacaoAcordo   string `json:"situacaoAcordo"`
	Sancoes          []struct {
		NomeInformado string `json:"nomeInformadoOrgaoResponsavel"`
		RazaoSocial   string `json:"razaoSocial"`
		CNPJFormatado string `json:"cnpjFormatado"`
		CNPJ          string `json:"cnpj"`
	} `json:"sancoes"`
}

// ─── Mappers ──────────────────────────────────────────────────────────────────

func mapCeisCnep(registry string, d ceisCnepDTO) SanctionRecord {
	rec := SanctionRecord{
		Registry:     registry,
		EntryID:      strconv.FormatInt(d.ID, 10),
		SanctionType: firstNonEmpty(d.TipoSancao.DescricaoResumida, d.TipoSancao.DescricaoPortal),
		Organ:        d.OrgaoSancionador.Nome,
		DateStart:    normalizeDate(d.DataInicioSancao),
		DateEnd:      normalizeDate(d.DataFimSancao),
		ProcessRef:   d.NumeroProcesso,
		SourceURL:    cguSourceURL(d.LinkPublicacao, registry, d.ID),
		Name:         firstNonEmpty(d.Pessoa.Nome, d.Sancionado.Nome),
	}
	doc := firstNonEmpty(d.Pessoa.CNPJFormatado, d.Pessoa.CPFFormatado, d.Sancionado.CodigoFormatado)
	rec.CPF, rec.CNPJ, rec.MaskedCPF = classifyDocument(doc)
	return rec
}

func mapCeaf(d ceafDTO) SanctionRecord {
	rec := SanctionRecord{
		Registry:     RegistryCEAF,
		EntryID:      strconv.FormatInt(d.ID, 10),
		SanctionType: d.TipoPunicao.Descricao,
		Organ:        d.OrgaoLotacao.Nome,
		DateStart:    normalizeDate(d.DataPublicacao),
		ProcessRef:   d.Punicao.Processo,
		SourceURL:    cguSourceURL("", RegistryCEAF, d.ID),
		Name:         firstNonEmpty(d.Pessoa.Nome, d.Punicao.NomePunido),
	}
	doc := firstNonEmpty(d.Pessoa.CPFFormatado, d.Punicao.CPFPunidoFormatado)
	rec.CPF, rec.CNPJ, rec.MaskedCPF = classifyDocument(doc)
	return rec
}

// mapLeniencia emits one Sanction per sanctioned company in the agreement, so
// each CNPJ gets its own deterministic node + edge.
func mapLeniencia(d lenienciaDTO) []SanctionRecord {
	base := SanctionRecord{
		Registry:     RegistryLeniencia,
		SanctionType: strings.TrimSpace("Acordo de Leniência " + d.SituacaoAcordo),
		Organ:        d.OrgaoResponsavel,
		DateStart:    normalizeDate(d.DataInicioAcordo),
		DateEnd:      normalizeDate(d.DataFimAcordo),
		SourceURL:    cguSourceURL("", RegistryLeniencia, d.ID),
	}
	if len(d.Sancoes) == 0 {
		base.EntryID = strconv.FormatInt(d.ID, 10)
		return []SanctionRecord{base}
	}
	out := make([]SanctionRecord, 0, len(d.Sancoes))
	for i, s := range d.Sancoes {
		rec := base
		rec.Name = firstNonEmpty(s.RazaoSocial, s.NomeInformado)
		doc := firstNonEmpty(s.CNPJFormatado, s.CNPJ)
		rec.CPF, rec.CNPJ, rec.MaskedCPF = classifyDocument(doc)
		// Sanction.id merges on registry+EntryID, so the discriminator after the
		// agreement id must be distinct per company. Prefer the CNPJ; when the
		// company has no document, fall back to a stable name slug, then to the
		// company's index within the agreement: otherwise several document-less
		// companies in one agreement would all collide on "<id>-" and merge into
		// a single Sanction node with wrong entity attribution.
		key := rec.CNPJ
		if key == "" {
			key = digitsOnly(doc)
		}
		if key == "" {
			key = companyNameSlug(rec.Name)
		}
		if key == "" {
			key = strconv.Itoa(i)
		}
		rec.EntryID = strconv.FormatInt(d.ID, 10) + "-" + key
		out = append(out, rec)
	}
	return out
}

// cguSourceURL returns the publication deep link when present, else a best-effort
// public Portal da Transparência URL for the registry entry.
func cguSourceURL(linkPublicacao, registry string, id int64) string {
	if l := strings.TrimSpace(linkPublicacao); l != "" {
		return l
	}
	idStr := strconv.FormatInt(id, 10)
	switch registry {
	case RegistryCEIS:
		return "https://portaldatransparencia.gov.br/sancoes/ceis?id=" + idStr
	case RegistryCNEP:
		return "https://portaldatransparencia.gov.br/sancoes/cnep?id=" + idStr
	case RegistryCEAF:
		return "https://portaldatransparencia.gov.br/sancoes/ceaf?id=" + idStr
	case RegistryLeniencia:
		return "https://portaldatransparencia.gov.br/acordos-leniencia?id=" + idStr
	default:
		return "https://portaldatransparencia.gov.br/sancoes"
	}
}

// companyNameSlug produces a stable, deterministic discriminator from a company
// name: accents folded to ASCII, uppercased, runs of non-alphanumerics collapsed
// to a single '-', leading/trailing '-' trimmed. Empty input yields "". It is
// used as a fallback EntryID component for document-less sanctioned companies so
// distinct records in the same agreement do not collide (same input → same id).
func companyNameSlug(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32) // uppercase
			lastDash = false
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if f, ok := foldAccent(r); ok {
				b.WriteRune(f)
				lastDash = false
				continue
			}
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// foldAccent maps a common Latin accented letter to its uppercase ASCII base,
// covering the Portuguese/Spanish accent set seen in Brazilian company names.
func foldAccent(r rune) (rune, bool) {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä', 'å', 'Á', 'À', 'Â', 'Ã', 'Ä', 'Å':
		return 'A', true
	case 'é', 'è', 'ê', 'ë', 'É', 'È', 'Ê', 'Ë':
		return 'E', true
	case 'í', 'ì', 'î', 'ï', 'Í', 'Ì', 'Î', 'Ï':
		return 'I', true
	case 'ó', 'ò', 'ô', 'õ', 'ö', 'Ó', 'Ò', 'Ô', 'Õ', 'Ö':
		return 'O', true
	case 'ú', 'ù', 'û', 'ü', 'Ú', 'Ù', 'Û', 'Ü':
		return 'U', true
	case 'ç', 'Ç':
		return 'C', true
	case 'ñ', 'Ñ':
		return 'N', true
	case 'ý', 'ÿ', 'Ý':
		return 'Y', true
	}
	return 0, false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// normalizeDate converts the date formats seen across CGU/TCU sources into
// yyyy-mm-dd, returning "" when the value is empty or unparseable.
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	layouts := []string{"2006-01-02", "02/01/2006", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
