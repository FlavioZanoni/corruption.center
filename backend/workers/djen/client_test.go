package djen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// A full page (100 items) triggers a second page fetch; a short second page
// stops pagination. Uses the disabled limiter so no sleep occurs.
func TestSearchByCaseNumber_Paginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("pagina")
		switch page {
		case "1":
			_ = json.NewEncoder(w).Encode(apiResponse{Items: makeItems(itemsPerPage, "100")})
		case "2":
			_ = json.NewEncoder(w).Encode(apiResponse{Items: makeItems(3, "100")})
		default:
			_ = json.NewEncoder(w).Encode(apiResponse{Items: nil})
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	items, err := c.SearchByCaseNumber(context.Background(), "100")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if len(items) != itemsPerPage+3 {
		t.Fatalf("expected %d items across two pages, got %d", itemsPerPage+3, len(items))
	}
}

// SearchByPartyName honors the cap across pages.
func TestSearchByPartyName_Cap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return a full page so only the cap stops the loop.
		_ = json.NewEncoder(w).Encode(apiResponse{Items: makeItems(itemsPerPage, "100")})
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
