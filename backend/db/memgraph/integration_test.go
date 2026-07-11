//go:build integration

package memgraph

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"corruption-center/workers/tse"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestUpsertPoliticiansFromTSE_DedupAndMerge(t *testing.T) {
	ctx := context.Background()

	container, uri := startMemgraphContainer(t, ctx)
	defer func() { _ = container.Terminate(context.Background()) }()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth("", "", ""))
	if err != nil {
		t.Fatalf("new memgraph driver: %v", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		t.Fatalf("verify memgraph connectivity: %v", err)
	}
	db := &DB{driver: driver, log: log}
	defer db.Close(ctx)

	first := []tse.PoliticianRecord{{
		CPF:            "12345678900",
		Name:           "JOAO SILVA",
		NameAliases:    []string{"JOAO", "J. SILVA"},
		PartyCurrent:   "ABC",
		State:          "SP",
		TSEProfileURLs: []string{"https://tse/2022/1"},
		Active:         false,
	}}
	if _, err := db.UpsertPoliticiansFromTSE(ctx, first, 100); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []tse.PoliticianRecord{{
		CPF:            "12345678900",
		Name:           "JOAO DA SILVA",
		NameAliases:    []string{"JOAO", "JOAOZINHO"},
		PartyCurrent:   "DEF",
		State:          "SP",
		TSEProfileURLs: []string{"https://tse/2022/1", "https://tse/2006/2"},
		Active:         false,
	}}
	if _, err := db.UpsertPoliticiansFromTSE(ctx, second, 100); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.Run(ctx, `
    MATCH (p:Politician {cpf: $cpf})
    RETURN p.name AS name, p.party_current AS party, p.active AS active,
           p.name_aliases AS aliases, p.tse_profile_urls AS urls
  `, map[string]any{"cpf": "12345678900"})
	if err != nil {
		t.Fatalf("query politician: %v", err)
	}
	if !res.Next(ctx) {
		t.Fatalf("expected politician node")
	}
	rec := res.Record()

	partyVal, _ := rec.Get("party")
	if party, ok := partyVal.(string); !ok || party != "DEF" {
		t.Fatalf("expected updated party DEF, got %#v", partyVal)
	}

	activeVal, _ := rec.Get("active")
	if active, ok := activeVal.(bool); !ok || active {
		t.Fatalf("expected active=false from TSE default, got %#v", activeVal)
	}

	aliasesVal, _ := rec.Get("aliases")
	aliases := toStringSlice(t, aliasesVal)
	if !containsAll(aliases, []string{"JOAO", "J. SILVA", "JOAOZINHO"}) {
		t.Fatalf("expected merged aliases, got %#v", aliases)
	}

	urlsVal, _ := rec.Get("urls")
	urls := toStringSlice(t, urlsVal)
	if !containsAll(urls, []string{"https://tse/2022/1", "https://tse/2006/2"}) || len(urls) != 2 {
		t.Fatalf("expected deduped urls, got %#v", urls)
	}
}

// The baseline seed re-registers its cases on every API boot, and registration
// carries no facts. An empty field must therefore preserve what the DataJud
// watcher wrote, not blank it.
func TestUpsertLegalProceedingByCase_RegistrationPreservesWatcherState(t *testing.T) {
	ctx := context.Background()

	container, uri := startMemgraphContainer(t, ctx)
	defer func() { _ = container.Terminate(context.Background()) }()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth("", "", ""))
	if err != nil {
		t.Fatalf("new memgraph driver: %v", err)
	}
	db := &DB{driver: driver, log: log}
	defer db.Close(ctx)

	const caseNumber = "50465129420164047000"

	// Registration (backoffice / baseline seed): number only.
	if _, err := db.UpsertLegalProceedingByCase(ctx, DataJudProceedingUpsert{CaseNumber: caseNumber}); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Watcher poll: the real facts.
	if _, err := db.UpsertLegalProceedingByCase(ctx, DataJudProceedingUpsert{
		CaseNumber: caseNumber,
		Court:      "13ª Vara Federal de Curitiba",
		Type:       "criminal",
		Status:     "concluded",
		Assuntos:   []string{"Corrupção passiva"},
	}); err != nil {
		t.Fatalf("watcher upsert: %v", err)
	}
	// API restarts → the seed registers the same case again.
	if _, err := db.UpsertLegalProceedingByCase(ctx, DataJudProceedingUpsert{CaseNumber: caseNumber}); err != nil {
		t.Fatalf("re-register: %v", err)
	}

	session := db.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
    MATCH (lp:LegalProceeding {case_number: $c})
    RETURN lp.status AS status, lp.court AS court, lp.assuntos AS assuntos
  `, map[string]any{"c": caseNumber})
	if err != nil {
		t.Fatalf("query proceeding: %v", err)
	}
	if !res.Next(ctx) {
		t.Fatalf("expected proceeding node")
	}
	rec := res.Record()

	statusVal, _ := rec.Get("status")
	if status, _ := statusVal.(string); status != "concluded" {
		t.Fatalf("re-registration reset status: got %#v, want concluded", statusVal)
	}
	courtVal, _ := rec.Get("court")
	if court, _ := courtVal.(string); court != "13ª Vara Federal de Curitiba" {
		t.Fatalf("re-registration cleared court: got %#v", courtVal)
	}
	assuntosVal, _ := rec.Get("assuntos")
	if assuntos := toStringSlice(t, assuntosVal); len(assuntos) != 1 {
		t.Fatalf("re-registration cleared assuntos: got %#v", assuntosVal)
	}
}

func startMemgraphContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "memgraph/memgraph:2.22.1",
		ExposedPorts: []string{"7687/tcp"},
		WaitingFor:   wait.ForListeningPort("7687/tcp"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start memgraph container: %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("memgraph host: %v", err)
	}
	port, err := container.MappedPort(ctx, "7687/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("memgraph port: %v", err)
	}
	return container, "bolt://" + host + ":" + port.Port()
}

func toStringSlice(t *testing.T, v any) []string {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %#v", v)
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		s, ok := it.(string)
		if !ok {
			t.Fatalf("non-string item %#v", it)
		}
		out = append(out, s)
	}
	return out
}

func containsAll(hay []string, needles []string) bool {
	m := map[string]struct{}{}
	for _, s := range hay {
		m[s] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := m[n]; !ok {
			return false
		}
	}
	return true
}
