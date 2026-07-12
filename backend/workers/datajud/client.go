package datajud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultAPIBase = "https://api-publica.datajud.cnj.jus.br"

const (
	// DataJud publishes no rate limit, so we impose one: the watcher used to send
	// requests as fast as it could loop, and the only thing holding it back was
	// DATAJUD_POLL_LIMIT — a cap on how many CASES a run touched, standing in for a
	// cap on how fast it touched them. That is why 480 of our 484 cases had never
	// been polled: the crude guard was the whole throttle.
	requestInterval = time.Minute / 60 // <= 60 req/min, self-imposed

	// A 429 or a 5xx used to end a case lookup outright: no retry at all, so one
	// blip silently dropped that case from the run and its conviction state simply
	// stayed unknown.
	maxRetries = 3
)

// var, not const: the tests wind the ladder down so they do not sleep.
var (
	backoffInitial = 1 * time.Second
	backoffMax     = 16 * time.Second
)

type Client struct {
	apiBase string
	apiKey  string
	http    *http.Client
	limiter *limiter
}

// limiter enforces a minimum interval between requests. Holding the lock across
// the wait serializes callers, which is exactly the intended pacing.
type limiter struct {
	mu   sync.Mutex
	min  time.Duration
	last time.Time
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.last.IsZero() {
		if d := l.min - time.Since(l.last); d > 0 {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	l.last = time.Now()
	return nil
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
	return &Client{
		apiBase: strings.TrimRight(base, "/"),
		apiKey:  key,
		http:    &http.Client{Timeout: 45 * time.Second},
		limiter: &limiter{min: requestInterval},
	}, nil
}

func (c *Client) SearchByCaseNumber(ctx context.Context, tribunalEndpoint, caseNumber string) (*CaseSource, error) {
	body := map[string]any{"size": 1, "query": map[string]any{"match": map[string]any{"numeroProcesso": caseNumber}}}
	b, _ := json.Marshal(body)
	url := c.apiBase + "/" + strings.TrimSpace(tribunalEndpoint) + "/_search"

	raw, err := c.post(ctx, url, b)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
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

// post sends one rate-limited request, retrying a 429 or a 5xx with exponential
// backoff. A 4xx other than 429 is our mistake and is returned immediately —
// retrying a bad request just annoys CNJ and cannot succeed.
func (c *Client) post(ctx context.Context, url string, body []byte) ([]byte, error) {
	backoff := backoffInitial
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("datajud: build request: %w", err)
		}
		req.Header.Set("Authorization", "APIKey "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("datajud: request: %w", err)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, err
			}
			continue
		}

		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			lastErr = fmt.Errorf("datajud: status %d", res.StatusCode)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, err
			}
			continue
		}

		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
			return nil, fmt.Errorf("datajud: status=%d body=%s", res.StatusCode, string(raw))
		}
		return io.ReadAll(res.Body)
	}
	return nil, fmt.Errorf("datajud: exhausted retries: %w", lastErr)
}

func sleepBackoff(ctx context.Context, backoff *time.Duration) error {
	t := time.NewTimer(*backoff)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	*backoff *= 2
	if *backoff > backoffMax {
		*backoff = backoffMax
	}
	return nil
}
