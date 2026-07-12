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
	origInit, origMax, orig429 := backoffInitial, backoffMax, backoffMax429
	t.Cleanup(func() {
		backoffInitial, backoffMax, backoffMax429 = origInit, origMax, orig429
	})
	backoffInitial, backoffMax, backoffMax429 = time.Millisecond, 2*time.Millisecond, 4*time.Millisecond
}

// A 429 is a quota window, not an outage, so it must outlive the 5xx budget:
// four tries inside ~15s is how a run skips whole tribunals (tjba, tjap) and
// still reports success.
func TestSearchByCaseNumber_OutlastsAQuotaWindowThatExceedsThe5xxBudget(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// One more 429 than the 5xx ladder would ever tolerate.
		if calls <= maxRetries+1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"numeroProcesso":"123"}}]}}`))
	}))
	defer server.Close()

	c, err := NewClient(context.Background(), server.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	c.limiter.min = 0
	withFastBackoff(t)

	src, err := c.SearchByCaseNumber(context.Background(), "api_publica_tjba", "123")
	if err != nil {
		t.Fatalf("a quota window longer than the 5xx budget must not drop the case: %v", err)
	}
	if src == nil || src.NumeroProcesso != "123" {
		t.Fatalf("expected the case once the quota window passed, got %+v", src)
	}
}

// When the server says how long to wait, guessing is how you earn another 429.
func TestRetryAfter_HonoursTheServersDelayOverOurLadder(t *testing.T) {
	got := retryAfter("2")
	if got != 2*time.Second {
		t.Fatalf("seconds form: want 2s, got %v", got)
	}
	if d := retryAfter(""); d != 0 {
		t.Fatalf("absent header must fall back to our ladder (0), got %v", d)
	}
	if d := retryAfter("garbage"); d != 0 {
		t.Fatalf("unparseable header must fall back to our ladder (0), got %v", d)
	}
	// A server asking us to sit out an hour mid-run is not honoured blindly.
	if d := retryAfter("3600"); d != backoffMax429 {
		t.Fatalf("absurd delay must be capped at %v, got %v", backoffMax429, d)
	}
}

func TestSearchByCaseNumber_GivesUpOnAnEndlessQuotaWindow(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c, err := NewClient(context.Background(), server.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	c.limiter.min = 0
	withFastBackoff(t)
	// Retry-After is honoured, so cap it too or this test sleeps for 7 seconds.
	orig := backoffMax429
	backoffMax429 = 2 * time.Millisecond
	t.Cleanup(func() { backoffMax429 = orig })

	if _, err := c.SearchByCaseNumber(context.Background(), "api_publica_tjba", "123"); err == nil {
		t.Fatal("an endless quota window must surface as an error, not hang forever")
	}
	if calls != maxRetries429+1 {
		t.Fatalf("want %d attempts, got %d", maxRetries429+1, calls)
	}
}
