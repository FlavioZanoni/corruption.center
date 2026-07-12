package djen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a Client pointed at a test server with the rate limiter
// disabled so pagination does not introduce wall-clock sleeps.
func newTestClient(baseURL string) *Client {
	c := NewClient(baseURL)
	c.limiter.min = 0
	return c
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	if c := NewClient("  "); c.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want default", c.baseURL)
	}
	if c := NewClient("https://x/api"); c.baseURL != "https://x/api" {
		t.Fatalf("baseURL = %q, want passthrough", c.baseURL)
	}
}

func makeItems(n int, caseNumber string) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: int64(i), NumeroProcesso: caseNumber, SiglaTribunal: "TRF1"}
	}
	return out
}

func TestSearchByCaseNumber_SinglePage(t *testing.T) {
	var gotParam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParam = r.URL.Query().Get("numeroProcesso")
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "success", Items: makeItems(5, "100")})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items, err := c.SearchByCaseNumber(context.Background(), "10000000020234013700")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}
	if gotParam != "10000000020234013700" {
		t.Fatalf("numeroProcesso param = %q", gotParam)
	}
}

// A full page triggers a second fetch; pagination stops at the count DJEN reports.
func TestSearchByCaseNumber_Paginates(t *testing.T) {
	const total = itemsPerPage + 3

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pagina") {
		case "1":
			_ = json.NewEncoder(w).Encode(apiResponse{Count: total, Items: makeItems(itemsPerPage, "100")})
		case "2":
			_ = json.NewEncoder(w).Encode(apiResponse{Count: total, Items: makeItems(3, "100")})
		default:
			_ = json.NewEncoder(w).Encode(apiResponse{Count: total, Items: nil})
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items, err := c.SearchByCaseNumber(context.Background(), "100")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if len(items) != total {
		t.Fatalf("expected %d items across two pages, got %d", total, len(items))
	}
}

// SearchByPartyName honors the cap across pages and windows.
func TestSearchByPartyName_Cap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A big result set, so only the cap stops the loop.
		_ = json.NewEncoder(w).Encode(apiResponse{Count: 10000, Items: makeItems(itemsPerPage, "100")})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items, err := c.SearchByPartyName(context.Background(), "FULANO", 150)
	if err != nil {
		t.Fatalf("SearchByPartyName: %v", err)
	}
	if len(items) != 150 {
		t.Fatalf("expected cap of 150 items, got %d", len(items))
	}
}

func TestFetchPage_NonRetryableStatus(t *testing.T) {
	// 404 is neither 429 nor 5xx → returned immediately, no backoff sleep.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.SearchByCaseNumber(context.Background(), "100"); err == nil {
		t.Fatalf("expected 404 error")
	}
}

func TestFetchPage_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.SearchByCaseNumber(context.Background(), "100"); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestGet_RequestErrorNoSleep(t *testing.T) {
	// Cancelled context: the request fails and sleepBackoff returns immediately,
	// so the retry loop exits without a wall-clock sleep.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient(server.URL)
	if _, err := c.SearchByCaseNumber(ctx, "100"); err == nil {
		t.Fatalf("expected error with cancelled context")
	}
}

// tribunalEndpoint, namesFor, sortedKeys and stripTags are pure helpers not yet
// covered by the existing suite.
func TestTribunalEndpoint(t *testing.T) {
	cases := map[string]string{
		"TRF4":   "api_publica_trf4",
		" trt1 ": "api_publica_trt1",
		"":       "",
		"   ":    "",
	}
	for in, want := range cases {
		if got := tribunalEndpoint(in); got != want {
			t.Errorf("tribunalEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortedKeys(t *testing.T) {
	got := sortedKeys(map[string]bool{"b": true, "a": true, "c": true})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("sortedKeys = %v, want sorted", got)
	}
	if len(sortedKeys(map[string]bool{})) != 0 {
		t.Fatalf("empty map should yield empty slice")
	}
}

func TestStripTags(t *testing.T) {
	if got := stripTags("<p>Hello <b>World</b></p>"); got != "Hello World" {
		t.Fatalf("stripTags = %q", got)
	}
	if got := stripTags("no tags"); got != "no tags" {
		t.Fatalf("stripTags plain = %q", got)
	}
}

func TestSnippet_Empty(t *testing.T) {
	if got := snippet("", 10); got != "" {
		t.Fatalf("snippet empty = %q", got)
	}
}

// Ranges are halved to isolate a record DJEN cannot serve, so the page size must
// be a power of two — otherwise a halved range lands off DJEN's fixed page grid
// and silently reads the wrong items.
func TestItemsPerPage_IsAPowerOfTwo(t *testing.T) {
	if itemsPerPage <= 0 || itemsPerPage&(itemsPerPage-1) != 0 {
		t.Fatalf("itemsPerPage = %d, must be a power of two so halving stays page-aligned", itemsPerPage)
	}

	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("itensPorPagina")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.fetchPage(context.Background(), url.Values{"nomeParte": {"FULANO"}}, 1, itemsPerPage); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if got != strconv.Itoa(itemsPerPage) {
		t.Fatalf("sent itensPorPagina=%q, want %d", got, itemsPerPage)
	}
}

// The bug this pins cost two entire runs. DJEN 500s on a nomeParte search with
// no date range, so a name lookup without one does not return fewer results — it
// returns none, ever. Both prior runs made every lookup this way and matched
// nothing, and it read as an upstream outage rather than a malformed request.
func TestSearchByPartyName_AlwaysSendsADateRange(t *testing.T) {
	var windows [][2]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, to := q.Get("dataDisponibilizacaoInicio"), q.Get("dataDisponibilizacaoFim")
		if from == "" || to == "" {
			t.Errorf("name search sent no date range (from=%q to=%q): DJEN answers 500", from, to)
		}
		windows = append(windows, [2]string{from, to})
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	restore := nowFunc
	t.Cleanup(func() { nowFunc = restore })
	nowFunc = func() time.Time { return time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC) }

	c := newTestClient(server.URL)
	if _, err := c.SearchByPartyName(context.Background(), "FULANO", 0); err != nil {
		t.Fatalf("SearchByPartyName: %v", err)
	}

	// 2021..2026 inclusive, one window per covered year, the last clipped to today
	// so we never ask DJEN for a future date.
	want := [][2]string{
		{"2021-01-01", "2021-12-31"},
		{"2022-01-01", "2022-12-31"},
		{"2023-01-01", "2023-12-31"},
		{"2024-01-01", "2024-12-31"},
		{"2025-01-01", "2025-12-31"},
		{"2026-01-01", "2026-07-12"},
	}
	if len(windows) != len(want) {
		t.Fatalf("got %d windows, want %d: %v", len(windows), len(want), windows)
	}
	for i, w := range want {
		if windows[i] != w {
			t.Errorf("window %d = %v, want %v", i, windows[i], w)
		}
	}
}

// A case-number search must NOT carry a date range: DJEN answers it without one,
// and a window would silently hide communications published outside it.
func TestSearchByCaseNumber_SendsNoDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if from := r.URL.Query().Get("dataDisponibilizacaoInicio"); from != "" {
			t.Errorf("case search sent a date range (%q); it would hide older publications", from)
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	if _, err := newTestClient(server.URL).SearchByCaseNumber(context.Background(), "100"); err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
}

// A single record DJEN cannot serve poisons every page that overlaps it, at any
// page size. The client must isolate that record and step over it, keeping the
// rest of the results — abandoning the whole search (what it used to do) loses a
// politician's entire history to one bad row.
func TestPaginate_StepsOverARecordDJENCannotServe(t *testing.T) {
	const total, poison = 100, 36 // 0-based index of the unservable record

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size, _ := strconv.Atoi(r.URL.Query().Get("itensPorPagina"))
		page, _ := strconv.Atoi(r.URL.Query().Get("pagina"))
		lo := (page - 1) * size
		hi := min(lo+size, total)
		if lo <= poison && poison < hi {
			http.Error(w, "<html>internal server error</html>", http.StatusInternalServerError)
			return
		}
		items := []Item{}
		for i := lo; i < hi; i++ {
			items = append(items, Item{ID: int64(i), NumeroProcesso: "100"})
		}
		_ = json.NewEncoder(w).Encode(apiResponse{Count: total, Items: items})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items, err := c.SearchByCaseNumber(context.Background(), "100")
	if err != nil {
		t.Fatalf("one bad record must not fail the whole search: %v", err)
	}
	if len(items) != total-1 {
		t.Fatalf("got %d items, want %d (everything but the unservable record)", len(items), total-1)
	}
	for _, it := range items {
		if it.ID == poison {
			t.Fatalf("item %d is unservable and cannot have been returned", poison)
		}
	}
	if c.SkippedRecords() != 1 {
		t.Errorf("SkippedRecords = %d, want 1: a dropped record must be counted, not absorbed", c.SkippedRecords())
	}
}

// A range where NOTHING answers is an outage, not a case with no publications.
// The two are indistinguishable by result — both yield zero items — so the client
// must raise the error rather than hand back an empty list that reads as fact.
func TestPaginate_TotalOutageSurfacesRatherThanLookingEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).SearchByCaseNumber(context.Background(), "100")
	if err == nil {
		t.Fatal("an outage must not be reported as a case with no publications")
	}
	if !errors.Is(err, errServerRejected) {
		t.Fatalf("err = %v, want it to wrap errServerRejected", err)
	}
}

// The window width is not a tuning knob: 18 months answers, 24 months 500s.
func TestPartyWindows_StayWithinWhatDJENAnswers(t *testing.T) {
	const djenObservedCeiling = 18 * 31 * 24 * time.Hour // 18 months answers; 24 does not

	for _, w := range partyWindows(time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC)) {
		from, err := time.Parse(dateLayout, w.from)
		if err != nil {
			t.Fatalf("parse %q: %v", w.from, err)
		}
		to, err := time.Parse(dateLayout, w.to)
		if err != nil {
			t.Fatalf("parse %q: %v", w.to, err)
		}
		if span := to.Sub(from); span > djenObservedCeiling {
			t.Errorf("window %v spans %v, wider than DJEN answers: it will 500", w, span)
		}
		if to.Before(from) {
			t.Errorf("window %v ends before it starts", w)
		}
	}
}
