package djen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://comunicaapi.pje.jus.br/api/v1/comunicacao"
	userAgent      = "corruption.center-djen/1.0 (contato@corruption.center)"

	itemsPerPage    = 100
	maxRetries      = 6
	backoffInitial  = 1 * time.Second
	backoffMax      = 60 * time.Second
	requestInterval = time.Minute / 60 // ≤ 60 req/min self-imposed limit
)

// Destinatario is a single party of a communication. polo is "A" (ativo /
// plaintiff side) or "P" (passivo / defendant side). Names only: no document.
type Destinatario struct {
	Nome          string `json:"nome"`
	Polo          string `json:"polo"`
	ComunicacaoID int64  `json:"comunicacao_id"`
}

// Item is one DJEN communication as returned by the gazette API.
type Item struct {
	ID                    int64          `json:"id"`
	SiglaTribunal         string         `json:"siglaTribunal"`
	TipoComunicacao       string         `json:"tipoComunicacao"`
	TipoDocumento         string         `json:"tipoDocumento"`
	NomeOrgao             string         `json:"nomeOrgao"`
	NumeroProcesso        string         `json:"numero_processo"`
	NumeroProcessoMascara string         `json:"numeroprocessocommascara"`
	NomeClasse            string         `json:"nomeClasse"`
	CodigoClasse          string         `json:"codigoClasse"`
	DataDisponibilizacao  string         `json:"data_disponibilizacao"`
	Link                  string         `json:"link"`
	Texto                 string         `json:"texto"`
	Destinatarios         []Destinatario `json:"destinatarios"`
}

type apiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Count   int    `json:"count"`
	Items   []Item `json:"items"`
}

// Client is a polite HTTP client for the keyless DJEN API. It self-limits to
// ≤ 60 req/min and backs off exponentially on 429/5xx.
type Client struct {
	baseURL string
	http    *http.Client
	limiter *limiter
}

func NewClient(baseURL string) *Client {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = defaultBaseURL
	}
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: 45 * time.Second},
		limiter: &limiter{min: requestInterval},
	}
}

// SearchByCaseNumber fetches every communication for a 20-digit case number,
// following pagination. Old concluded cases legitimately return zero items.
func (c *Client) SearchByCaseNumber(ctx context.Context, caseNumber string) ([]Item, error) {
	return c.paginate(ctx, "numeroProcesso", caseNumber, 0)
}

// SearchByPartyName fetches communications for a party name, following
// pagination up to a cap (items across all pages) to bound work per name.
func (c *Client) SearchByPartyName(ctx context.Context, name string, cap int) ([]Item, error) {
	return c.paginate(ctx, "nomeParte", name, cap)
}

func (c *Client) paginate(ctx context.Context, param, value string, cap int) ([]Item, error) {
	out := make([]Item, 0)
	for page := 1; ; page++ {
		items, err := c.fetchPage(ctx, param, value, page)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if cap > 0 && len(out) >= cap {
			return out[:cap], nil
		}
		if len(items) < itemsPerPage {
			break
		}
	}
	return out, nil
}

func (c *Client) fetchPage(ctx context.Context, param, value string, page int) ([]Item, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("djen: parse url: %w", err)
	}
	q := u.Query()
	q.Set("pagina", strconv.Itoa(page))
	q.Set("itensPorPagina", strconv.Itoa(itemsPerPage))
	q.Set(param, value)
	u.RawQuery = q.Encode()

	body, err := c.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("djen: decode response: %w", err)
	}
	return resp.Items, nil
}

// get performs a rate-limited GET with exponential backoff on 429/5xx.
func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	backoff := backoffInitial
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, fmt.Errorf("djen: build request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("djen: request: %w", err)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, err
			}
			continue
		}

		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			lastErr = fmt.Errorf("djen: status %d", res.StatusCode)
			if err := sleepBackoff(ctx, &backoff); err != nil {
				return nil, err
			}
			continue
		}

		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
			return nil, fmt.Errorf("djen: status=%d body=%s", res.StatusCode, string(raw))
		}
		return io.ReadAll(res.Body)
	}
	return nil, fmt.Errorf("djen: exhausted retries: %w", lastErr)
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
// so holding the lock across the wait simply serializes calls, which is exactly
// the intended pacing.
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
