package sanctions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestCGUClient(baseURL string) *cguClient {
	c := newCGUClient(baseURL, "test-key")
	c.minInterval = 0 // no pacing sleeps in tests
	c.maxRetries = 0  // do not exercise the backoff-sleep retry loop
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

// A page that keeps failing must not take down the whole run. CGU is four
// registries plus the keyless TCU lists, and a run that dies on ceis page 15
// discards cnep, ceaf and the leniency agreements, which never even started.
// The registry stops, keeps what it ingested, and the caller moves on.
func TestRunCGURegistry_PageFailureStopsRegistryNotRun(t *testing.T) {
	// Wind the retry pacing down: this test exercises the give-up path, and the
	// production backoff would make it sit for a minute.
	defer withFastRetries(t)()

	pagesServed := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pagesServed++
		if r.URL.Query().Get("pagina") == "1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"pessoa":{"cnpjFormatado":"11.222.333/0001-81","nome":"EMPRESA X"},"dataInicioSancao":"01/01/2020"}]`))
			return
		}
		// Every later page is a hard, unrecoverable failure.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	w := &Worker{opts: Options{
		APIKey:     "k",
		CGUBaseURL: server.URL,
		DryRun:     true, // no graph writes; we only care about control flow
	}}
	stats := &Stats{PerRegistry: map[string]int{}}

	if err := w.runCGURegistry(context.Background(), "ceis", stats); err != nil {
		t.Fatalf("a failing page must not abort the run, got error: %v", err)
	}
	if len(stats.FailedRegistries) != 1 {
		t.Fatalf("expected the aborted registry to be reported, got %v", stats.FailedRegistries)
	}
	if stats.RecordsProcessed != 1 {
		t.Fatalf("expected page 1's record to be kept, got %d", stats.RecordsProcessed)
	}
	if pagesServed < 2 {
		t.Fatalf("expected page 2 to have been attempted, served %d", pagesServed)
	}
}

// withFastRetries shrinks the retry pacing for the duration of a test.
func withFastRetries(t *testing.T) func() {
	t.Helper()
	interval, retries, timeout := cguMinInterval, cguMaxRetries, cguHTTPTimeout
	cguMinInterval, cguMaxRetries, cguHTTPTimeout = 0, 1, 2*time.Second
	return func() { cguMinInterval, cguMaxRetries, cguHTTPTimeout = interval, retries, timeout }
}

// CGU answers a valid request with 400 {"Erro na API":"Erro ao executar a consulta"}
// when its own backend query fails. That is their fault, not ours, and the same
// page succeeds moments later: retry it instead of abandoning the registry.
func TestCGUGetPage_RetriesTransient400(t *testing.T) {
	defer withFastRetries(t)()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"Erro na API":"Erro ao executar a consulta"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()

	c := newCGUClient(server.URL, "k")
	body, err := c.getPage(context.Background(), "ceis", 87)
	if err != nil {
		t.Fatalf("expected the transient 400 to be retried, got: %v", err)
	}
	if string(body) != `[{"id":1}]` {
		t.Fatalf("unexpected body: %s", body)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly one retry, got %d attempts", attempts)
	}
}

// A genuine client error (a real 400, a 404) must still fail fast.
func TestCGUGetPage_RealClientErrorDoesNotRetry(t *testing.T) {
	defer withFastRetries(t)()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"Erro na API":"Parametro invalido"}`))
	}))
	defer server.Close()

	c := newCGUClient(server.URL, "k")
	if _, err := c.getPage(context.Background(), "ceis", 1); err == nil {
		t.Fatal("expected a real 400 to fail")
	}
	if attempts != 1 {
		t.Fatalf("a real client error must not be retried, got %d attempts", attempts)
	}
}
