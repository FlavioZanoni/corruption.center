package datajud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	ctx := context.Background()
	if _, err := NewClient(ctx, "", ""); err == nil {
		t.Fatalf("expected error when API key missing")
	}
	c, err := NewClient(ctx, "https://example.com/", "key123")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.apiBase != "https://example.com" {
		t.Fatalf("apiBase = %q, want trailing slash trimmed", c.apiBase)
	}
	// Empty base falls back to the default.
	c2, err := NewClient(ctx, "", "key123")
	if err != nil {
		t.Fatalf("NewClient default: %v", err)
	}
	if c2.apiBase != defaultAPIBase {
		t.Fatalf("apiBase = %q, want default", c2.apiBase)
	}
}

func TestSearchByCaseNumber_Success(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{
			"hits": {"hits": [
				{"_source": {
					"numeroProcesso": "5046512-94.2016.4.04.7000",
					"nivelSigilo": 0,
					"movimentos": [{"codigo": "51"}]
				}}
			]}
		}`))
	}))
	defer server.Close()

	c, err := NewClient(context.Background(), server.URL, "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	src, err := c.SearchByCaseNumber(context.Background(), "api_publica_trf4", "5046512-94.2016.4.04.7000")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if src == nil {
		t.Fatalf("expected a source, got nil")
	}
	if src.NumeroProcesso != "5046512-94.2016.4.04.7000" {
		t.Fatalf("numeroProcesso = %q", src.NumeroProcesso)
	}
	if src.Raw == nil {
		t.Fatalf("expected Raw populated")
	}
	if gotAuth != "APIKey secret" {
		t.Fatalf("Authorization = %q, want APIKey secret", gotAuth)
	}
	if !strings.HasSuffix(gotPath, "/api_publica_trf4/_search") {
		t.Fatalf("path = %q, want tribunal endpoint suffix", gotPath)
	}
}

func TestSearchByCaseNumber_NoHits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hits": {"hits": []}}`))
	}))
	defer server.Close()

	c, _ := NewClient(context.Background(), server.URL, "secret")
	src, err := c.SearchByCaseNumber(context.Background(), "trf4", "x")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if src != nil {
		t.Fatalf("expected nil source for empty hits, got %+v", src)
	}
}

func TestSearchByCaseNumber_NilSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// hit present but _source missing → nil, nil.
		_, _ = w.Write([]byte(`{"hits": {"hits": [{"_id": "1"}]}}`))
	}))
	defer server.Close()

	c, _ := NewClient(context.Background(), server.URL, "secret")
	src, err := c.SearchByCaseNumber(context.Background(), "trf4", "x")
	if err != nil {
		t.Fatalf("SearchByCaseNumber: %v", err)
	}
	if src != nil {
		t.Fatalf("expected nil source when _source missing")
	}
}

func TestSearchByCaseNumber_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()

	c, _ := NewClient(context.Background(), server.URL, "secret")
	if _, err := c.SearchByCaseNumber(context.Background(), "trf4", "x"); err == nil {
		t.Fatalf("expected status error")
	}
}

func TestSearchByCaseNumber_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c, _ := NewClient(context.Background(), server.URL, "secret")
	if _, err := c.SearchByCaseNumber(context.Background(), "trf4", "x"); err == nil {
		t.Fatalf("expected decode error")
	}
}
