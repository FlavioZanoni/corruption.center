package sanctions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestCGUClient(baseURL string) *cguClient {
	c := newCGUClient(baseURL, "test-key")
	c.minInterval = 0 // no pacing sleeps in tests
	c.maxRetries = 0   // do not exercise the backoff-sleep retry loop
	return c
}

func TestNewCGUClient_Defaults(t *testing.T) {
	c := newCGUClient("", "  k  ")
	if c.baseURL != defaultCGUBaseURL {
		t.Errorf("baseURL = %q, want default", c.baseURL)
	}
	if c.apiKey != "k" {
		t.Errorf("apiKey = %q, want trimmed", c.apiKey)
	}
	// Trailing slash trimmed.
	c2 := newCGUClient("https://x/api/", "k")
	if c2.baseURL != "https://x/api" {
		t.Errorf("baseURL = %q, want no trailing slash", c2.baseURL)
	}
}

func TestCGUGetPage_Success(t *testing.T) {
	var gotKey, gotPage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("chave-api-dados")
		gotPage = r.URL.Query().Get("pagina")
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()

	c := newTestCGUClient(server.URL)
	body, err := c.getPage(context.Background(), "ceis", 3)
	if err != nil {
		t.Fatalf("getPage: %v", err)
	}
	if string(body) != `[{"id":1}]` {
		t.Fatalf("body = %q", string(body))
	}
	if gotKey != "test-key" {
		t.Fatalf("chave-api-dados = %q", gotKey)
	}
	if gotPage != "3" {
		t.Fatalf("pagina = %q, want 3", gotPage)
	}
}

func TestCGUGetPage_NonRetryableStatus(t *testing.T) {
	// 400 is not 429/5xx → returned immediately without any backoff sleep.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()

	c := newTestCGUClient(server.URL)
	if _, err := c.getPage(context.Background(), "ceis", 1); err == nil {
		t.Fatalf("expected non-retryable status error")
	}
}

func TestCGUGetPage_RequestErrorNoSleep(t *testing.T) {
	// A cancelled context makes the HTTP request fail immediately and makes
	// sleepBackoff return without waiting, so the retry loop exits without any
	// wall-clock sleep. Exercises the transport-error branch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestCGUClient(server.URL)
	if _, err := c.getPage(ctx, "ceis", 1); err == nil {
		t.Fatalf("expected request error with cancelled context")
	}
}

func TestCGUThrottle_NoIntervalReturns(t *testing.T) {
	c := newTestCGUClient("http://example.com")
	// minInterval 0 → returns immediately, no sleep.
	c.throttle(context.Background())
}
