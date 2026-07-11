package datajud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultAPIBase = "https://api-publica.datajud.cnj.jus.br"

type Client struct {
	apiBase string
	apiKey  string
	http    *http.Client
}

// CaseSource is the subset of the DataJud public API _source we consume.
// Per Portaria CNJ 160/2020 the public API never returns partes[] or
// processoRelacionado[], so those are intentionally absent: the watcher is a
// case-level status engine only (see docs/workerDetails/DATAJUD.md).
type CaseSource struct {
	NumeroProcesso  string           `json:"numeroProcesso"`
	NivelSigilo     int              `json:"nivelSigilo"`
	Classe          map[string]any   `json:"classe"`
	Assuntos        []map[string]any `json:"assuntos"`
	DataAjuizamento string           `json:"dataAjuizamento"`
	OrgaoJulgador   map[string]any   `json:"orgaoJulgador"`
	Movimentos      []map[string]any `json:"movimentos"`
	Raw             map[string]any   `json:"-"`
}

// NewClient builds a DataJud client. The API key comes only from the caller
// (wired from the DATAJUD_API_KEY env var); it is never hardcoded or fetched
// from the wiki. The public key rotates and is published at
// https://datajud-wiki.cnj.jus.br/api-publica/acesso/.
func NewClient(ctx context.Context, apiBase, apiKey string) (*Client, error) {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		base = defaultAPIBase
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("datajud: DATAJUD_API_KEY is required")
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
