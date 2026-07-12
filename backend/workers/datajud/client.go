package datajud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

	// A 429 is not a failure, it is "come back later", so it gets its own, more
	// patient ladder. With the 5xx ladder (4 tries inside ~15s) a run bled cases:
	// the 429s came in clusters on a single tribunal — tjba, tjap — which is a
	// per-tribunal quota refusing a burst, not an outage. Giving up 15s into a
	// quota window and skipping the case is how a run "succeeds" while quietly
	// never reading whole tribunals.
	maxRetries429 = 6
)

// var, not const: the tests wind the ladder down so they do not sleep.
var (
	backoffInitial = 1 * time.Second
	backoffMax     = 16 * time.Second
	backoffMax429  = 90 * time.Second
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
	backoff429 := backoffInitial
	var lastErr error

	// Counted separately: a server that is refusing a burst and a server that is
	// broken deserve different amounts of patience, and mixing the two means a
	// stretch of 429s eats the budget a real outage needed.
	fails, rateLimits := 0, 0

	for {
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
			if fails++; fails > maxRetries {
				break
			}
			if err := sleepBackoff(ctx, &backoff, backoffMax); err != nil {
				return nil, err
			}
			continue
		}

		if res.StatusCode == http.StatusTooManyRequests {
			// The server usually says exactly how long to wait. Guessing when it has
			// already told us is how you get another 429.
			wait := retryAfter(res.Header.Get("Retry-After"))
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			lastErr = fmt.Errorf("datajud: status %d", res.StatusCode)
			if rateLimits++; rateLimits > maxRetries429 {
				break
			}
			if wait <= 0 {
				wait = nextBackoff(&backoff429, backoffMax429)
			}
			if err := sleepFor(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}

		if res.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			lastErr = fmt.Errorf("datajud: status %d", res.StatusCode)
			if fails++; fails > maxRetries {
				break
			}
			if err := sleepBackoff(ctx, &backoff, backoffMax); err != nil {
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

func sleepBackoff(ctx context.Context, backoff *time.Duration, max time.Duration) error {
	return sleepFor(ctx, nextBackoff(backoff, max))
}

// nextBackoff returns the current delay and doubles it for the next call.
func nextBackoff(backoff *time.Duration, max time.Duration) time.Duration {
	d := *backoff
	if d > max {
		d = max
	}
	*backoff *= 2
	if *backoff > max {
		*backoff = max
	}
	return d
}

func sleepFor(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	return nil
}

// retryAfter parses the Retry-After header, which is either a delay in seconds or
// an HTTP date. An unparseable or absurd value returns 0 so the caller falls back
// to its own ladder: a server that asks us to wait an hour is not one we honour
// blindly mid-run.
func retryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs < 0 {
			return 0
		}
		return capDuration(time.Duration(secs) * time.Second)
	}
	if t, err := http.ParseTime(h); err == nil {
		return capDuration(time.Until(t))
	}
	return 0
}

func capDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if d > backoffMax429 {
		return backoffMax429
	}
	return d
}
