package photos

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// The photos worker NEVER downloads or stores photo bytes locally: photo_url
// always hotlinks to an official server. For TSE candidates the only thing we
// fetch from TSE is the consulta_cand CSV (candidate metadata, small) so we can
// map CPF → SQ_CANDIDATO; the photo itself is a runtime-verified hotlink.

const (
	// consultaZipURLTemplate is the official TSE bulk candidate metadata zip.
	// Same host/path the TSE importer uses. %d = election year.
	consultaZipURLTemplate = "https://cdn.tse.jus.br/estatistica/sead/odsele/consulta_cand/consulta_cand_%d.zip"

	// tseURLTemplate builds a candidate photo hotlink from the placeholders
	// {year}, {uf} and {sq}. It is intentionally the ONLY information we can
	// derive from (year, uf, SQ_CANDIDATO) alone. Every constructed URL is
	// runtime-verified (must return image bytes) before it is ever written, so a
	// wrong template can never produce bad data — it only produces skips.
	//
	// NOTE (2026-07-10): the divulgacandcontas service that serves individual
	// candidate photos was under scheduled maintenance during development, so no
	// per-candidate hotlink pattern could be empirically confirmed. This default
	// is a best-effort guess; operators can override it with --tse-url-template
	// once a pattern is verified. Until a probe returns real image bytes, TSE
	// mode fails fast with a clear error (see Worker.runTSE).
	defaultTSEURLTemplate = "https://divulgacandcontas.tse.jus.br/divulga/rest/arquivo/img/{year}/{sq}"

	// Sample (UF, SQ) confirmed to exist in the 2022 RR candidate photo bundle
	// (foto_cand2022_RR_div.zip → FRR230002529954_div.jpg). Used as the default
	// pre-flight probe so TSE mode can verify the hotlink pattern without first
	// downloading the (large) consulta zip.
	defaultProbeYear = 2022
	defaultProbeUF   = "RR"
	defaultProbeSQ   = "230002529954"
)

// tseClient resolves and verifies TSE candidate photo hotlinks. It stores no
// bytes; verifyImageURL streams a small prefix only to confirm the response is
// an image.
type tseClient struct {
	http        *http.Client
	urlTemplate string
	userAgent   string
}

func newTSEClient(httpClient *http.Client, urlTemplate, userAgent string) *tseClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	tmpl := strings.TrimSpace(urlTemplate)
	if tmpl == "" {
		tmpl = defaultTSEURLTemplate
	}
	return &tseClient{http: httpClient, urlTemplate: tmpl, userAgent: userAgent}
}

// buildPhotoURL substitutes the {year}/{uf}/{sq} placeholders in the template.
// Pure and unit-tested.
func buildPhotoURL(template string, year int, uf, sq string) string {
	r := strings.NewReplacer(
		"{year}", fmt.Sprintf("%d", year),
		"{uf}", strings.ToUpper(strings.TrimSpace(uf)),
		"{sq}", strings.TrimSpace(sq),
	)
	return r.Replace(template)
}

// verifyImageURL performs a GET and reports whether the response is an image,
// without persisting any bytes. It reads at most a small prefix to sniff the
// content type / magic bytes.
func (c *tseClient) verifyImageURL(ctx context.Context, rawURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	req.Header.Set("Accept", "image/*")
	res, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, nil
	}
	prefix, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	return looksLikeImage(res.Header.Get("Content-Type"), prefix), nil
}

// looksLikeImage reports whether a content type or the leading bytes identify a
// JPEG/PNG/GIF/WebP image. Pure and unit-tested.
func looksLikeImage(contentType string, prefix []byte) bool {
	if ct := strings.ToLower(strings.TrimSpace(contentType)); strings.HasPrefix(ct, "image/") {
		return true
	}
	switch {
	case len(prefix) >= 3 && prefix[0] == 0xFF && prefix[1] == 0xD8 && prefix[2] == 0xFF:
		return true // JPEG
	case len(prefix) >= 8 && prefix[0] == 0x89 && prefix[1] == 'P' && prefix[2] == 'N' && prefix[3] == 'G':
		return true // PNG
	case len(prefix) >= 6 && string(prefix[:6]) == "GIF89a":
		return true
	case len(prefix) >= 12 && string(prefix[:4]) == "RIFF" && string(prefix[8:12]) == "WEBP":
		return true
	}
	return false
}

// ─── consulta_cand CPF→SQ mapping ─────────────────────────────────────────────

// photoFilenameRe matches the candidate photo file names found inside the TSE
// per-UF photo bundles, e.g. "FRR230002529954_div.jpg" / "..._div.jpeg". Group 1
// is the UF, group 2 the SQ_CANDIDATO.
var photoFilenameRe = regexp.MustCompile(`(?i)^F([A-Z]{2})(\d+)_div\.(jpe?g)$`)

// parsePhotoFilename extracts the UF and SQ_CANDIDATO from a TSE photo bundle
// file name. Verified against foto_cand2022_RR_div.zip. Pure and unit-tested.
func parsePhotoFilename(name string) (uf, sq string, ok bool) {
	m := photoFilenameRe.FindStringSubmatch(strings.TrimSpace(filepath.Base(name)))
	if m == nil {
		return "", "", false
	}
	return strings.ToUpper(m[1]), m[2], true
}

// normalizeCPF strips non-digits and left-pads to 11 digits so consulta CSV and
// graph CPF keys compare equal. Returns "" when the value is not a CPF.
func normalizeCPF(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if d == "" || len(d) > 11 {
		return ""
	}
	if len(d) < 11 {
		d = strings.Repeat("0", 11-len(d)) + d
	}
	if d == "00000000000" {
		return ""
	}
	return d
}

// parseConsultaCPFtoSQ reads a consulta_cand CSV (ISO-8859-1, ';'-separated) and
// returns a normalized-CPF → SQ_CANDIDATO map. Pure and unit-tested.
func parseConsultaCPFtoSQ(r io.Reader) (map[string]string, error) {
	decoded := transform.NewReader(r, charmap.ISO8859_1.NewDecoder())
	cr := csv.NewReader(bufio.NewReader(decoded))
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("photos: empty consulta csv")
		}
		return nil, err
	}
	idx := map[string]int{}
	for i, col := range header {
		idx[strings.TrimSpace(col)] = i
	}
	cpfCol, okCPF := idx["NR_CPF_CANDIDATO"]
	sqCol, okSQ := idx["SQ_CANDIDATO"]
	if !okCPF || !okSQ {
		return nil, fmt.Errorf("photos: consulta csv missing NR_CPF_CANDIDATO/SQ_CANDIDATO columns")
	}

	out := map[string]string{}
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("photos: read consulta row: %w", err)
		}
		if cpfCol >= len(row) || sqCol >= len(row) {
			continue
		}
		cpf := normalizeCPF(row[cpfCol])
		sq := strings.TrimSpace(row[sqCol])
		if cpf == "" || sq == "" {
			continue
		}
		// First SQ wins; a candidate can appear across turns but SQ is stable
		// per (year, candidate).
		if _, exists := out[cpf]; !exists {
			out[cpf] = sq
		}
	}
	return out, nil
}

// buildCPFtoSQFromZip opens a downloaded consulta_cand_{year}.zip and merges the
// CPF→SQ maps from every per-UF CONSULTA_CAND CSV, optionally filtered to a
// single UF. The _BRASIL aggregate file (if any) is skipped to avoid double
// counting.
func buildCPFtoSQFromZip(zipPath, ufFilter string) (map[string]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("photos: open consulta zip: %w", err)
	}
	defer zr.Close()

	uf := strings.ToUpper(strings.TrimSpace(ufFilter))
	out := map[string]string{}
	for _, f := range zr.File {
		name := strings.ToUpper(filepath.Base(f.Name))
		if !strings.HasPrefix(name, "CONSULTA_CAND_") || !strings.HasSuffix(name, ".CSV") {
			continue
		}
		if strings.Contains(name, "_BRASIL") {
			continue
		}
		if uf != "" && !strings.HasSuffix(strings.TrimSuffix(name, ".CSV"), "_"+uf) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("photos: open consulta entry %s: %w", f.Name, err)
		}
		m, err := parseConsultaCPFtoSQ(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}
	return out, nil
}

// downloadConsultaZip fetches the official consulta_cand_{year}.zip into dir and
// returns the local path. It is metadata (not photos); nothing else is stored.
func (c *tseClient) downloadConsultaZip(ctx context.Context, year int, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("photos: create consulta dir: %w", err)
	}
	url := fmt.Sprintf(consultaZipURLTemplate, year)
	dest := filepath.Join(dir, fmt.Sprintf("consulta_cand_%d.zip", year))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	// The consulta zip can be tens of MB; use a generous timeout independent of
	// the short per-request verification client.
	dl := &http.Client{Timeout: 30 * time.Minute}
	res, err := dl.Do(req)
	if err != nil {
		return "", fmt.Errorf("photos: download consulta zip: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("photos: consulta zip status %d for %s", res.StatusCode, url)
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, res.Body); err != nil {
		return "", fmt.Errorf("photos: write consulta zip: %w", err)
	}
	return dest, nil
}
