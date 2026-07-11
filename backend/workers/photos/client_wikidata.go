package photos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultSPARQLEndpoint = "https://query.wikidata.org/sparql"
	ptWikipediaAPI        = "https://pt.wikipedia.org/w/api.php"
	wikidataEntityData    = "https://www.wikidata.org/wiki/Special:EntityData/%s.json"

	// commonsFilePath builds a stable hotlink to the Commons file bytes. Adding
	// ?width=512 asks Commons to serve a thumbnail. This is a hotlink — we never
	// copy the bytes.
	commonsFilePath = "https://commons.wikimedia.org/wiki/Special:FilePath/%s?width=512"

	// commonsFilePage is the human-readable file description page used for the
	// legally-required attribution string.
	commonsFilePage = "https://commons.wikimedia.org/wiki/File:%s"

	photoSourceCommons = "Wikimedia Commons"
)

// Test-only endpoint overrides. Empty in production; set by tests to point the
// pt.wikipedia and Wikidata EntityData calls at an httptest server.
var (
	ptWikipediaAPIOverride string
	entityDataOverride     string
)

func wikipediaAPIURL() string {
	if ptWikipediaAPIOverride != "" {
		return ptWikipediaAPIOverride
	}
	return ptWikipediaAPI
}

func entityDataURL(qid string) string {
	if entityDataOverride != "" {
		return fmt.Sprintf(entityDataOverride, qid)
	}
	return fmt.Sprintf(wikidataEntityData, qid)
}

// wikidataClient is a polite client for the public Wikidata/Wikipedia
// endpoints: ≤ 1 req/s, descriptive User-Agent with contact, backoff on 429.
type wikidataClient struct {
	http           *http.Client
	sparqlEndpoint string
	userAgent      string
	minInterval    time.Duration
	last           time.Time
}

func newWikidataClient(httpClient *http.Client, sparqlEndpoint, userAgent string) *wikidataClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	ep := strings.TrimSpace(sparqlEndpoint)
	if ep == "" {
		ep = defaultSPARQLEndpoint
	}
	return &wikidataClient{
		http:           httpClient,
		sparqlEndpoint: ep,
		userAgent:      userAgent,
		minInterval:    time.Second,
	}
}

func (c *wikidataClient) throttle(ctx context.Context) {
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

// doGET issues a rate-limited GET, retrying on 429/5xx with linear backoff.
func (c *wikidataClient) doGET(ctx context.Context, rawURL, accept string) ([]byte, error) {
	backoff := time.Second
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		c.throttle(ctx)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		res, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if !sleepFor(ctx, &backoff) {
				return nil, err
			}
			continue
		}
		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusOK:
			if readErr != nil {
				return nil, readErr
			}
			return body, nil
		case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
			lastErr = fmt.Errorf("wikidata: status %d for %s", res.StatusCode, rawURL)
			if !sleepFor(ctx, &backoff) {
				return nil, lastErr
			}
			continue
		default:
			return nil, fmt.Errorf("wikidata: status %d for %s", res.StatusCode, rawURL)
		}
	}
	return nil, lastErr
}

func sleepFor(ctx context.Context, backoff *time.Duration) bool {
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

// ─── Organizations: CNPJ (P6204) → image (P18) ────────────────────────────────

// orgImageSPARQL queries for an org whose P6204 (CNPJ) matches either the raw
// 14-digit or the formatted CNPJ, returning its P18 image. P154 (logo) is NEVER
// requested — only P18 — for legal reasons.
func orgImageSPARQL(cnpj14 string) string {
	formatted := formatCNPJ(cnpj14)
	return fmt.Sprintf(`SELECT ?image WHERE {
  VALUES ?cnpj { "%s" "%s" }
  ?item wdt:P6204 ?cnpj .
  ?item wdt:P18 ?image .
} LIMIT 1`, formatted, cnpj14)
}

// FindOrgImageByCNPJ returns the Commons file name for an organization's P18
// image, or ok=false when there is no Wikidata entity / image.
func (c *wikidataClient) FindOrgImageByCNPJ(ctx context.Context, cnpj14 string) (file string, ok bool, err error) {
	q := orgImageSPARQL(cnpj14)
	u, _ := url.Parse(c.sparqlEndpoint)
	qs := u.Query()
	qs.Set("query", q)
	qs.Set("format", "json")
	u.RawQuery = qs.Encode()

	body, err := c.doGET(ctx, u.String(), "application/sparql-results+json")
	if err != nil {
		return "", false, err
	}
	imgURL, ok := parseSPARQLImage(body)
	if !ok {
		return "", false, nil
	}
	return commonsFilenameFromP18(imgURL), true, nil
}

// parseSPARQLImage extracts the first ?image binding value from a SPARQL JSON
// result set. Pure and unit-tested.
func parseSPARQLImage(body []byte) (string, bool) {
	var r struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false
	}
	for _, b := range r.Results.Bindings {
		if img, ok := b["image"]; ok && strings.TrimSpace(img.Value) != "" {
			return img.Value, true
		}
	}
	return "", false
}

// ─── Politician fallback: pt.wikipedia title → Wikidata entity → P18 ──────────

// FindPoliticianImage attempts to resolve a single P18 image for a politician
// from the given candidate names (primary name + aliases). It NEVER accepts a
// name-only match with more than one distinct entity: only when every hit
// resolves to a single Wikidata entity carrying an image is the image returned.
func (c *wikidataClient) FindPoliticianImage(ctx context.Context, names []string) (file string, ok bool, err error) {
	imagesByQID := map[string]string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		qid, found, err := c.wikibaseItemForTitle(ctx, name)
		if err != nil {
			return "", false, err
		}
		if !found {
			continue
		}
		if _, seen := imagesByQID[qid]; seen {
			continue
		}
		img, hasImg, err := c.entityImage(ctx, qid)
		if err != nil {
			return "", false, err
		}
		if hasImg {
			imagesByQID[qid] = img
		} else {
			imagesByQID[qid] = "" // record the entity even without an image
		}
	}

	// Exactly one distinct entity, and it must carry an image.
	if len(imagesByQID) != 1 {
		return "", false, nil
	}
	for _, img := range imagesByQID {
		if strings.TrimSpace(img) == "" {
			return "", false, nil
		}
		return img, true, nil
	}
	return "", false, nil
}

// wikibaseItemForTitle resolves an exact pt.wikipedia article title to its
// Wikidata QID. A missing page or a title-normalization mismatch yields
// found=false (we require an exact-title hit, not a fuzzy search).
func (c *wikidataClient) wikibaseItemForTitle(ctx context.Context, title string) (qid string, found bool, err error) {
	u, _ := url.Parse(wikipediaAPIURL())
	qs := u.Query()
	qs.Set("action", "query")
	qs.Set("prop", "pageprops")
	qs.Set("ppprop", "wikibase_item")
	qs.Set("redirects", "1")
	qs.Set("format", "json")
	qs.Set("titles", title)
	u.RawQuery = qs.Encode()

	body, err := c.doGET(ctx, u.String(), "application/json")
	if err != nil {
		return "", false, err
	}
	return parseWikibaseItem(body)
}

// parseWikibaseItem extracts a single wikibase_item QID from a pt.wikipedia
// pageprops response, requiring exactly one existing (non-missing) page. Pure
// and unit-tested.
func parseWikibaseItem(body []byte) (qid string, found bool, err error) {
	var r struct {
		Query struct {
			Pages map[string]struct {
				Missing   *string `json:"missing"`
				PageProps struct {
					WikibaseItem string `json:"wikibase_item"`
				} `json:"pageprops"`
			} `json:"pages"`
		} `json:"query"`
	}
	if e := json.Unmarshal(body, &r); e != nil {
		return "", false, e
	}
	found = false
	for _, p := range r.Query.Pages {
		if p.Missing != nil {
			continue
		}
		id := strings.TrimSpace(p.PageProps.WikibaseItem)
		if id == "" {
			continue
		}
		if found {
			// More than one existing page → ambiguous, refuse.
			return "", false, nil
		}
		qid = id
		found = true
	}
	return qid, found, nil
}

// entityImage fetches Special:EntityData for a QID and returns its P18 image
// file name (without the "File:" prefix).
func (c *wikidataClient) entityImage(ctx context.Context, qid string) (file string, ok bool, err error) {
	body, err := c.doGET(ctx, entityDataURL(qid), "application/json")
	if err != nil {
		return "", false, err
	}
	return parseEntityDataP18(body, qid)
}

// parseEntityDataP18 extracts the P18 image file name from a Special:EntityData
// JSON document. Pure and unit-tested.
func parseEntityDataP18(body []byte, qid string) (file string, ok bool, err error) {
	var r struct {
		Entities map[string]struct {
			Claims map[string][]struct {
				Mainsnak struct {
					DataValue struct {
						Value string `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	if e := json.Unmarshal(body, &r); e != nil {
		return "", false, e
	}
	ent, ok := r.Entities[qid]
	if !ok {
		return "", false, nil
	}
	claims, ok := ent.Claims["P18"]
	if !ok || len(claims) == 0 {
		return "", false, nil
	}
	name := strings.TrimSpace(claims[0].Mainsnak.DataValue.Value)
	if name == "" {
		return "", false, nil
	}
	return name, true, nil
}

// ─── Commons URL / attribution helpers (pure) ─────────────────────────────────

// formatCNPJ turns 14 digits into the punctuated form Wikidata stores for P6204
// (XX.XXX.XXX/XXXX-XX). A non-14-digit input is returned unchanged.
func formatCNPJ(cnpj14 string) string {
	d := cnpj14
	if len(d) != 14 {
		return cnpj14
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

// commonsFilenameFromP18 extracts the Commons file name from a P18 value that
// may be a Special:FilePath URL (SPARQL wdt:P18) or already a bare file name
// (EntityData claim). The returned name has spaces (not underscores) and is not
// URL-encoded. Pure and unit-tested.
func commonsFilenameFromP18(value string) string {
	v := strings.TrimSpace(value)
	if idx := strings.LastIndex(v, "Special:FilePath/"); idx != -1 {
		v = v[idx+len("Special:FilePath/"):]
	}
	if dec, err := url.PathUnescape(v); err == nil {
		v = dec
	}
	v = strings.ReplaceAll(v, "_", " ")
	return strings.TrimSpace(v)
}

// buildCommonsThumbURL builds the hotlink to a 512px Commons thumbnail. Pure.
func buildCommonsThumbURL(file string) string {
	return fmt.Sprintf(commonsFilePath, url.PathEscape(strings.ReplaceAll(strings.TrimSpace(file), " ", "_")))
}

// buildCommonsAttribution builds the legally-required attribution string plus
// the file description page URL. Pure and unit-tested.
func buildCommonsAttribution(file string) string {
	f := strings.TrimSpace(file)
	page := fmt.Sprintf(commonsFilePage, url.PathEscape(strings.ReplaceAll(f, " ", "_")))
	return fmt.Sprintf("%s — Wikimedia Commons (%s)", f, page)
}
