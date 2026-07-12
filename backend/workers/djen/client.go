package djen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBaseURL = "https://comunicaapi.pje.jus.br/api/v1/comunicacao"
	userAgent      = "corruption.center-djen/1.0 (contato@corruption.center)"

	// DJEN's data contains individual records it cannot serve. Every page that
	// overlaps one answers 500, whatever the page size, and nothing about the
	// request is wrong — see fetchRange.
	//
	// A power of two, so halving a range keeps its offset aligned to DJEN's fixed
	// page grid.
	itemsPerPage = 32

	// Retries are for blips only — a 429, a dropped connection, or a single record
	// that turns out to be readable after all. They are deliberately cheap, because
	// a 500 on a page is nearly always a bad record, and no amount of repeating the
	// identical request will move one.
	maxRetries      = 3
	backoffInitial  = 1 * time.Second
	backoffMax      = 8 * time.Second
	requestInterval = time.Minute / 60 // ≤ 60 req/min self-imposed limit

	// A nomeParte search MUST carry a date range. Without one DJEN answers 500 —
	// not an empty list, not a 400 — and the body is an HTML error page rather
	// than JSON. Nothing in their docs says so, and numeroProcesso needs no range
	// at all, so the requirement is invisible until you diff the two. We shipped
	// without it and every name lookup we ever made failed: two full runs, 100%
	// 500s, zero matches. It reads exactly like an API outage, which is why it
	// survived a page-size fix that was real but fixed a different bug.
	//
	// The range also has a width limit, equally undocumented: 18 months answers,
	// 24 months 500s. One calendar year per request keeps a margin.
	dateLayout = "2006-01-02"

	// DJEN publishes nothing before 2021. The same name that returns 25 hits for
	// 2021 returns 0 for 2020, 2019 and 2018, so earlier windows only buy empty
	// answers at one request each.
	partyCoverageStartYear = 2021
)

// nowFunc is a var so the window split is testable without freezing the clock.
var nowFunc = time.Now

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
	// skipped counts individual communications DJEN refused to serve at any page
	// size. Dropping a record is a real (if small) hole in the data, so the run
	// reports it rather than absorbing it.
	skipped atomic.Int64
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
// A case-number search takes no date range and must not be given one.
func (c *Client) SearchByCaseNumber(ctx context.Context, caseNumber string) ([]Item, error) {
	return c.paginate(ctx, url.Values{"numeroProcesso": {caseNumber}}, 0)
}

// SearchByPartyName fetches communications for a party name, up to a cap of
// items across the whole search.
//
// The search is split into one-year windows because DJEN rejects a name search
// that carries no date range, and rejects one whose range is too wide (see
// dateLayout). So a single name costs one request per covered year, not one
// request: the windows are the API's price for answering at all, not a
// throughput choice we made.
func (c *Client) SearchByPartyName(ctx context.Context, name string, cap int) ([]Item, error) {
	out := make([]Item, 0)
	for _, w := range partyWindows(nowFunc()) {
		remaining := 0
		if cap > 0 {
			if remaining = cap - len(out); remaining <= 0 {
				break
			}
		}
		items, err := c.paginate(ctx, url.Values{
			"nomeParte":                  {name},
			"dataDisponibilizacaoInicio": {w.from},
			"dataDisponibilizacaoFim":    {w.to},
		}, remaining)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

// dateWindow is an inclusive [from, to] slice of DJEN's covered span.
type dateWindow struct{ from, to string }

// partyWindows splits DJEN's coverage (2021 → today) into calendar years, the
// widest slice the API reliably answers. The final window stops at today rather
// than at 31 December, so a future end date is never sent.
func partyWindows(now time.Time) []dateWindow {
	end := now.UTC()
	out := make([]dateWindow, 0, end.Year()-partyCoverageStartYear+1)
	for y := partyCoverageStartYear; y <= end.Year(); y++ {
		to := time.Date(y, time.December, 31, 0, 0, 0, 0, time.UTC)
		if to.After(end) {
			to = end
		}
		out = append(out, dateWindow{
			from: time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC).Format(dateLayout),
			to:   to.Format(dateLayout),
		})
	}
	return out
}

// errServerRejected marks a page DJEN answered 5xx to. It is a fact about the
// records on that page, not about the search: see fetchRange.
//
// It replaced a belief that a 500 meant DJEN had no coverage of a case and would
// never answer — "the same case number 500s on all four attempts, while a case it
// does cover answers 200 on all of them". Both halves of that were true and the
// conclusion was still wrong: the 500ing cases were the ones holding a record DJEN
// cannot serve, and no number of identical retries was ever going to shake one
// loose.
var errServerRejected = errors.New("djen: server rejected the request")

// paginate walks a search in fixed-width ranges, stepping over any record DJEN
// cannot serve. It stops at the result count DJEN reports, or at cap.
func (c *Client) paginate(ctx context.Context, params url.Values, cap int) ([]Item, error) {
	out := make([]Item, 0)
	total := -1 // unknown until the first answer
	for offset := 0; ; offset += itemsPerPage {
		if cap > 0 && len(out) >= cap {
			break
		}
		if total >= 0 && offset >= total {
			break
		}
		r, err := c.fetchRange(ctx, params, offset, itemsPerPage)
		if err != nil {
			return nil, err
		}
		if total < 0 {
			// Not one page of the first range came back, so DJEN never told us how
			// many results there are. A handful of bad records is one thing; a range
			// that answers nothing at all is an outage, and continuing would mean
			// guessing where the results end. Say so instead of returning a short
			// list that looks like a complete one.
			if !r.served {
				return nil, fmt.Errorf("%w: no page of the first range answered", errServerRejected)
			}
			total = r.total
		}
		out = append(out, r.items...)
		// A range that yielded neither an item nor a skip is the end of the results;
		// without this a search whose count DJEN never reports would loop forever.
		if len(r.items)+r.skipped == 0 {
			break
		}
	}
	if cap > 0 && len(out) > cap {
		out = out[:cap]
	}
	return out, nil
}

// rangeResult is what one range of a search yielded: the items DJEN served, the
// total it says match, and how many records it refused to serve at all.
type rangeResult struct {
	items []Item
	total int
	// served records whether ANY page in this range was answered. It is what tells
	// a few bad records apart from an outage: total would be 0 in both cases, and
	// only one of them means "there are no results".
	served  bool
	skipped int
}

// fetchRange reads size items starting at offset, halving the range around any
// record DJEN refuses to serve.
//
// Some individual records in DJEN's data are unservable: every page overlapping
// one answers 500, at every page size. Lula's 2025 results have one at index 36 —
// pages 1-7 at size 5 are fine and page 8 is not, and the same record kills a
// size-50 page and a size-12 page alike. Nothing about the request is wrong, so
// neither retrying it nor shrinking the whole search can help. (Shrinking looked
// like it helped, which is how this hid: a 30-item page stops at index 29, just
// short of the bad record.) The only way through is to find it and step over it.
//
// Halving isolates it in log2(size) requests and costs the caller that one record
// instead of the entire politician's results — which is what abandoning the search
// used to cost. offset stays aligned to DJEN's page grid because size is a power
// of two, so page = offset/size + 1 is always exact.
func (c *Client) fetchRange(ctx context.Context, params url.Values, offset, size int) (rangeResult, error) {
	resp, err := c.fetchPage(ctx, params, offset/size+1, size)
	if err == nil {
		return rangeResult{items: resp.Items, total: resp.Count, served: true}, nil
	}
	if !errors.Is(err, errServerRejected) {
		return rangeResult{}, err
	}

	if size > 1 {
		half := size / 2
		lo, err := c.fetchRange(ctx, params, offset, half)
		if err != nil {
			return rangeResult{}, err
		}
		hi, err := c.fetchRange(ctx, params, offset+half, half)
		if err != nil {
			return rangeResult{}, err
		}
		return rangeResult{
			items:   append(lo.items, hi.items...),
			total:   max(lo.total, hi.total),
			served:  lo.served || hi.served,
			skipped: lo.skipped + hi.skipped,
		}, nil
	}

	// One record, alone on its page, and DJEN still will not serve it. That is
	// almost certainly a bad record — but a transient blip looks identical, and
	// silently dropping a real communication is the worse mistake. Give it one more
	// chance before writing it off.
	//
	// Just one, and on a flat delay: a whole range of unservable records subdivides
	// into itemsPerPage of these, so an exponential ladder here would turn a DJEN
	// outage into minutes of sleeping per range. An outage is caught by paginate,
	// which sees that nothing in the range answered.
	backoff := backoffInitial
	if err := sleepBackoff(ctx, &backoff); err != nil {
		return rangeResult{}, err
	}
	if resp, err := c.fetchPage(ctx, params, offset+1, 1); err == nil {
		return rangeResult{items: resp.Items, total: resp.Count, served: true}, nil
	} else if !errors.Is(err, errServerRejected) {
		return rangeResult{}, err
	}

	c.skipped.Add(1)
	return rangeResult{skipped: 1}, nil
}

// SkippedRecords is the number of individual communications DJEN refused to serve
// across this client's lifetime. Surfaced in the worker's run stats: a run that
// quietly drops records should say how many.
func (c *Client) SkippedRecords() int64 { return c.skipped.Load() }

func (c *Client) fetchPage(ctx context.Context, params url.Values, page, size int) (*apiResponse, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("djen: parse url: %w", err)
	}
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	q.Set("pagina", strconv.Itoa(page))
	q.Set("itensPorPagina", strconv.Itoa(size))
	u.RawQuery = q.Encode()

	body, err := c.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("djen: decode response: %w", err)
	}
	return &resp, nil
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

		// A 5xx is almost always DJEN refusing an oversized response, so it is not
		// retried here: the identical request would be refused identically. Hand it
		// to paginate, which answers by asking for a smaller page — the only thing
		// that changes the outcome. paginate still backs off and retries once the
		// page cannot shrink further, which is where a genuine blip shows up.
		if res.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 512))
			res.Body.Close()
			return nil, fmt.Errorf("%w: status %d", errServerRejected, res.StatusCode)
		}

		// A 429 is the opposite case: the request was fine, we were merely too fast.
		if res.StatusCode == http.StatusTooManyRequests {
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
