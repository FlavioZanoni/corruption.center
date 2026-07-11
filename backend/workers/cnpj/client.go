package cnpj

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://minhareceita.org"
	userAgent      = "corruption.center-cnpj/1.0 (contato@corruption.center)"

	// defaultRatePerMin is generous for a self-hosted minha receita instance.
	// Users pointing CNPJ_API_BASE at the shared PUBLIC instance must lower it
	// (see README) — the public instance is a courtesy service.
	defaultRatePerMin = 60

	maxRetries     = 6
	backoffInitial = 1 * time.Second
	backoffMax     = 60 * time.Second
)

// QSAEntry is one Quadro de Sócios e Administradores row. cnpj_cpf_do_socio is
// either a MASKED CPF (individual, "***641988**") or a full CNPJ (a company).
type QSAEntry struct {
	NomeSocio               string `json:"nome_socio"`
	CNPJCPFDoSocio          string `json:"cnpj_cpf_do_socio"`
	QualificacaoSocio       string `json:"qualificacao_socio"`
	CodigoQualificacaoSocio int    `json:"codigo_qualificacao_socio"`
}

// CNPJResponse is the subset of the minha receita / Receita Federal open-data
// response the enricher consumes. The provider returns many more fields.
type CNPJResponse struct {
	CNPJ                       string     `json:"cnpj"`
	RazaoSocial                string     `json:"razao_social"`
	DescricaoSituacaoCadastral string     `json:"descricao_situacao_cadastral"`
	NaturezaJuridica           string     `json:"natureza_juridica"`
	UF                         string     `json:"uf"`
	CapitalSocial              float64    `json:"capital_social"`
	CNAEFiscalDescricao        string     `json:"cnae_fiscal_descricao"`
	QSA                        []QSAEntry `json:"qsa"`
}

// Client is a polite HTTP client for the minha receita provider. It self-limits
// to a configurable rate and backs off exponentially on 429/5xx.
type Client struct {
	baseURL string
	http    *http.Client
	limiter *limiter
}

// NewClient builds a client. baseURL defaults to the public minha receita
// instance; ratePerMin <= 0 defaults to defaultRatePerMin.
func NewClient(baseURL string, ratePerMin int) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	if ratePerMin <= 0 {
		ratePerMin = defaultRatePerMin
	}
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: 45 * time.Second},
		limiter: &limiter{min: time.Minute / time.Duration(ratePerMin)},
	}
}

// FetchCNPJ looks up a single CNPJ. A 404 (unknown CNPJ) returns (nil, nil) so
// the caller can skip it without aborting the run.
func (c *Client) FetchCNPJ(ctx context.Context, cnpj string) (*CNPJResponse, error) {
	digits := digitsOnly(cnpj)
	if len(digits) != 14 {
		return nil, fmt.Errorf("cnpj: invalid cnpj %q", cnpj)
	}
	body, notFound, err := c.get(ctx, c.baseURL+"/"+digits)
	if err != nil {
		return nil, err
	}
	if notFound {
		return nil, nil
	}
	var resp CNPJResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cnpj: decode response: %w", err)
	}
	return &resp, nil
}

// get performs a rate-limited GET with exponential backoff on 429/5xx. The
// second return value reports a 404 (unknown CNPJ), which is not an error.
func (c *Client) get(ctx context.Context, target string) ([]byte, bool, error) {
	backoff := backoffInitial
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, false, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, false, fmt.Errorf("cnpj: build request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("cnpj: request: %w", err)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, false, err
			}
			continue
		}

		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			lastErr = fmt.Errorf("cnpj: status %d", res.StatusCode)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, false, err
			}
			continue
		}

		if res.StatusCode == http.StatusNotFound {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			return nil, true, nil
		}

		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
			return nil, false, fmt.Errorf("cnpj: status=%d body=%s", res.StatusCode, string(raw))
		}
		body, err := io.ReadAll(res.Body)
		return body, false, err
	}
	return nil, false, fmt.Errorf("cnpj: exhausted retries: %w", lastErr)
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

// limiter enforces a minimum interval between requests. The worker is sequential
// so holding the lock across the wait serializes calls, which is the intended
// pacing.
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
