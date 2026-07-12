package datajud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// A 429 or a transient 5xx used to end a case lookup outright — the client had no
// retry at all, so one blip silently dropped that case and its conviction state
// stayed unknown forever.
func TestSearchByCaseNumber_RetriesRateLimitAndServerErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls < 3 {
				w.WriteHeader(status)
				return
			}
			_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"numeroProcesso":"123"}}]}}`))
		}))

		c, err := NewClient(context.Background(), server.URL, "key")
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		c.limiter.min = 0 // no wall-clock pacing in tests
		withFastBackoff(t)

		src, err := c.SearchByCaseNumber(context.Background(), "api_publica_trf4", "123")
		server.Close()
		if err != nil {
			t.Fatalf("status %d should have been retried, got: %v", status, err)
		}
		if src == nil || src.NumeroProcesso != "123" {
			t.Fatalf("status %d: expected the case after retry, got %+v", status, src)
		}
		if calls != 3 {
			t.Errorf("status %d: expected 3 attempts, got %d", status, calls)
		}
	}
}

// A 4xx that is not 429 is our bad request. Retrying it cannot succeed and only
// adds load to a government API, so it must surface at once.
func TestSearchByCaseNumber_ClientErrorDoesNotRetry(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	c, _ := NewClient(context.Background(), server.URL, "key")
	c.limiter.min = 0
	withFastBackoff(t)

	if _, err := c.SearchByCaseNumber(context.Background(), "api_publica_trf4", "123"); err == nil {
		t.Fatal("expected a 400 to surface")
	}
	if calls != 1 {
		t.Errorf("a 400 must not be retried, got %d attempts", calls)
	}
}

// The watcher polls hundreds of cases in a run against an API that publishes no
// rate limit. Losing the self-imposed one would let it hammer CNJ as fast as it
// can loop, which is exactly what it used to do.
func TestClient_SelfLimitsItsRequestRate(t *testing.T) {
	c, err := NewClient(context.Background(), "https://example.test", "key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.limiter == nil || c.limiter.min <= 0 {
		t.Fatal("client must pace its own requests: DataJud publishes no limit, so we impose one")
	}
	if perMin := time.Minute / c.limiter.min; perMin > 60 {
		t.Errorf("client would send %d req/min; keep it at or under 60", perMin)
	}
}

// withFastBackoff winds the retry ladder down so the tests do not sleep.
func withFastBackoff(t *testing.T) {
	t.Helper()
	origInit, origMax := backoffInitial, backoffMax
	t.Cleanup(func() { backoffInitial, backoffMax = origInit, origMax })
	backoffInitial, backoffMax = time.Millisecond, 2*time.Millisecond
}
