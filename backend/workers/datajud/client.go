package datajud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	defaultAPIBase = "https://api-publica.datajud.cnj.jus.br"
	wikiURL        = "https://datajud-wiki.cnj.jus.br/api-publica"
)

type Client struct {
	apiBase string
	apiKey  string
	http    *http.Client
}

type CaseSource struct {
	NumeroProcesso      string                     `json:"numeroProcesso"`
	NivelSigilo         int                        `json:"nivelSigilo"`
	Classe              map[string]any             `json:"classe"`
	Assuntos            []map[string]any           `json:"assuntos"`
	DataAjuizamento     string                     `json:"dataAjuizamento"`
	OrgaoJulgador       map[string]any             `json:"orgaoJulgador"`
	Partes              []map[string]any           `json:"partes"`
	Movimentos          []map[string]any           `json:"movimentos"`
	ProcessoRelacionado []map[string]any           `json:"processoRelacionado"`
	Raw                 map[string]any             `json:"-"`
	RawJSON             map[string]json.RawMessage `json:"-"`
}

func NewClient(ctx context.Context, apiBase, apiKey string) (*Client, error) {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		base = defaultAPIBase
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		var err error
		key, err = fetchAPIKeyFromWiki(ctx)
		if err != nil {
			return nil, fmt.Errorf("datajud: resolve api key (set DATAJUD_API_KEY or ensure wiki key is available): %w", err)
		}
	}
	return &Client{apiBase: strings.TrimRight(base, "/"), apiKey: key, http: &http.Client{Timeout: 45 * time.Second}}, nil
}

func (c *Client) SearchByCaseNumber(ctx context.Context, tribunalEndpoint, caseNumber string) (*CaseSource, error) {
	body := map[string]any{"size": 1, "query": map[string]any{"match": map[string]any{"numeroProcesso": caseNumber}}}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/"+strings.TrimSpace(tribunalEndpoint)+"/_search", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("datajud: build request: %w", err)
	}
	req.Header.Set("Authorization", "APIKey "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("datajud: request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("datajud: status=%d body=%s", res.StatusCode, string(raw))
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("datajud: decode response: %w", err)
	}
	hitsRoot, _ := payload["hits"].(map[string]any)
	hitsArrAny, _ := hitsRoot["hits"].([]any)
	if len(hitsArrAny) == 0 {
		return nil, nil
	}
	first, _ := hitsArrAny[0].(map[string]any)
	sourceRaw, _ := first["_source"].(map[string]any)
	if sourceRaw == nil {
		return nil, nil
	}
	sourceBytes, _ := json.Marshal(sourceRaw)
	var src CaseSource
	if err := json.Unmarshal(sourceBytes, &src); err != nil {
		return nil, fmt.Errorf("datajud: decode source: %w", err)
	}
	src.Raw = sourceRaw
	return &src, nil
}

func fetchAPIKeyFromWiki(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wikiURL, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?i)api\s*key\s*[:\s]+([A-Za-z0-9]{20,})`)
	m := re.FindStringSubmatch(string(b))
	if len(m) < 2 {
		return "", fmt.Errorf("api key not found on wiki page")
	}
	return m[1], nil
}
