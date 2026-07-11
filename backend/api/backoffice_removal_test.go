package api

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"

	"github.com/gin-gonic/gin"
)

// ── stubs ──────────────────────────────────────────────────────────────────
// Both stubs embed the repository interface (nil) so only the methods exercised
// by the handler need to be implemented; any other call panics loudly.

type resolveCall struct {
	id, status, resolution, by string
}

type stubPsql struct {
	psql.Repository
	removal       psql.RemovalRequest
	getErr        error
	resolveErr    error
	resolveCalls  []resolveCall
	auditActions  []psql.AuditAction
	tombstoneKeys [][]string
	// review approval bookkeeping
	review        psql.PendingReviewItem
	statusUpdates []string
}

func (s *stubPsql) GetRemovalRequest(_ context.Context, _ string) (psql.RemovalRequest, error) {
	return s.removal, s.getErr
}

func (s *stubPsql) GetPendingReview(_ context.Context, _ string) (psql.PendingReviewItem, error) {
	return s.review, nil
}

func (s *stubPsql) UpdatePendingReviewStatus(_ context.Context, _ string, status string, _ string) error {
	s.statusUpdates = append(s.statusUpdates, status)
	return nil
}

func (s *stubPsql) CreatePurgeTombstones(_ context.Context, keys []string, _ string, _ string) error {
	s.tombstoneKeys = append(s.tombstoneKeys, keys)
	return nil
}

// ListPendingReviews is called when a failed approval re-renders the queue.
func (s *stubPsql) ListPendingReviews(_ context.Context, _, _ string, _ int) ([]psql.PendingReviewItem, error) {
	return nil, nil
}

func (s *stubPsql) ListRemovalRequests(_ context.Context, _ string, _ int) ([]psql.RemovalRequest, error) {
	return []psql.RemovalRequest{s.removal}, nil
}

func (s *stubPsql) ResolveRemovalRequest(_ context.Context, id, status, resolution, by string) error {
	if s.resolveErr != nil {
		return s.resolveErr
	}
	s.resolveCalls = append(s.resolveCalls, resolveCall{id, status, resolution, by})
	return nil
}

func (s *stubPsql) LogAudit(_ context.Context, _ string, action psql.AuditAction, _ string, _ string, _ map[string]any) error {
	s.auditActions = append(s.auditActions, action)
	return nil
}

type edgeCall struct {
	kind             string // "defendant" | "controls" | "sanctioned"
	politician, dest string
}

type stubMemgraph struct {
	memgraph.Repository
	prov     *memgraph.NodeProvenance
	purgeRet *memgraph.NodeProvenance
	purgeErr error
	purged   bool
	edgeErr  error // returned by every edge writer when set
	edges    []edgeCall
}

func (s *stubMemgraph) GetNodeProvenance(_ context.Context, _ string) (*memgraph.NodeProvenance, error) {
	return s.prov, nil
}

// ListScandals is called by the reviews re-render (loadScandals).
func (s *stubMemgraph) ListScandals(_ context.Context) ([]memgraph.ScandalOption, error) {
	return nil, nil
}

func (s *stubMemgraph) PurgePersonNode(_ context.Context, _ string) (*memgraph.NodeProvenance, error) {
	if s.purgeErr != nil {
		return s.purgeRet, s.purgeErr
	}
	s.purged = true
	return s.purgeRet, nil
}

func (s *stubMemgraph) EnsurePoliticianDefendantEdge(_ context.Context, pol, proc string) error {
	if s.edgeErr != nil {
		return s.edgeErr
	}
	s.edges = append(s.edges, edgeCall{"defendant", pol, proc})
	return nil
}

func (s *stubMemgraph) EnsurePoliticianControlsOrganization(_ context.Context, pol, org string) error {
	if s.edgeErr != nil {
		return s.edgeErr
	}
	s.edges = append(s.edges, edgeCall{"controls", pol, org})
	return nil
}

func (s *stubMemgraph) EnsurePoliticianSanctionedInEdge(_ context.Context, pol, sanc string) error {
	if s.edgeErr != nil {
		return s.edgeErr
	}
	s.edges = append(s.edges, edgeCall{"sanctioned", pol, sanc})
	return nil
}

func postResolve(id string, form url.Values) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/backoffice/removals/"+id+"/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: id}}
	return c, w
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestRemovalResolve_PurgePolitician_Refused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pol := &memgraph.NodeProvenance{ID: "pol_1", Label: "Politician", Name: "Fulano", IsPolitician: true}
	ps := &stubPsql{removal: psql.RemovalRequest{ID: "r1", TargetID: "pol_1", TargetType: "Politician", Status: "pending", Requester: "x"}}
	mg := &stubMemgraph{prov: pol, purgeRet: pol, purgeErr: memgraph.ErrPoliticianNotPurgeable}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postResolve("r1", url.Values{"action": {"purge"}})
	h.removalResolve(c)

	if len(ps.resolveCalls) != 0 {
		t.Fatalf("politician purge must NOT resolve the request, got resolve calls: %+v", ps.resolveCalls)
	}
	if mg.purged {
		t.Fatalf("politician node must not be reported as purged")
	}
	body := w.Body.String()
	if !strings.Contains(body, "purge refused") {
		t.Fatalf("expected refusal message in response, body did not contain it")
	}
	// The refusal page re-renders the queue (200), it does not redirect away.
	if w.Code != 200 {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
}

func TestRemovalResolve_PurgePerson_ResolvesAndAudits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	person := &memgraph.NodeProvenance{ID: "person_1", Label: "Person", Name: "Beltrano", IsPolitician: false, EdgeCount: 3}
	ps := &stubPsql{removal: psql.RemovalRequest{ID: "r2", TargetID: "person_1", TargetType: "Person", Status: "pending", Requester: "x"}}
	mg := &stubMemgraph{prov: person, purgeRet: person}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postResolve("r2", url.Values{"action": {"purge"}, "resolution": {"done"}})
	h.removalResolve(c)

	if !mg.purged {
		t.Fatalf("person node should have been purged")
	}
	if len(ps.resolveCalls) != 1 || ps.resolveCalls[0].status != "resolved" {
		t.Fatalf("expected one 'resolved' transition, got %+v", ps.resolveCalls)
	}
	var sawDelete bool
	for _, a := range ps.auditActions {
		if a == psql.AuditActionDelete {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Fatalf("purge must write a delete audit record, actions: %+v", ps.auditActions)
	}
	if loc := w.Header().Get("Location"); loc != "/backoffice/removals" {
		t.Fatalf("expected redirect to removals queue, got Location=%q", loc)
	}
}

func TestRemovalResolve_PurgeOnRejectedRequest_Refused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	person := &memgraph.NodeProvenance{ID: "person_1", Label: "Person", Purgeable: true}
	// A replayed purge POST on an ALREADY REJECTED request must not delete.
	ps := &stubPsql{removal: psql.RemovalRequest{ID: "r5", TargetID: "person_1", TargetType: "Person", Status: "rejected", Requester: "x"}}
	mg := &stubMemgraph{prov: person, purgeRet: person}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postResolve("r5", url.Values{"action": {"purge"}})
	h.removalResolve(c)

	if mg.purged {
		t.Fatalf("purge on a non-pending request must NOT delete the node")
	}
	if len(ps.resolveCalls) != 0 {
		t.Fatalf("purge on a non-pending request must not resolve, got %+v", ps.resolveCalls)
	}
	if len(ps.tombstoneKeys) != 0 {
		t.Fatalf("no tombstones may be written when the purge is refused")
	}
	if body := w.Body.String(); !strings.Contains(body, "not pending") {
		t.Fatalf("expected 'not pending' refusal in body")
	}
}

func TestRemovalResolve_Purge_WritesTombstones(t *testing.T) {
	gin.SetMode(gin.TestMode)
	person := &memgraph.NodeProvenance{
		ID: "person_1", Label: "Person", Name: "João Da Silva",
		CPF: "12345678901", Purgeable: true,
	}
	ps := &stubPsql{removal: psql.RemovalRequest{ID: "r6", TargetID: "person_1", TargetType: "Person", Status: "pending", Requester: "x"}}
	mg := &stubMemgraph{prov: person, purgeRet: person}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, _ := postResolve("r6", url.Values{"action": {"purge"}})
	h.removalResolve(c)

	if !mg.purged {
		t.Fatalf("pending person should be purged")
	}
	if len(ps.tombstoneKeys) != 1 {
		t.Fatalf("expected one tombstone write, got %d", len(ps.tombstoneKeys))
	}
	keys := ps.tombstoneKeys[0]
	// A CPF key and a NAME key (normalized): no CNPJ key for a Person.
	wantCPF := psql.TombstoneKeyCPF("12345678901")
	wantName := psql.TombstoneKeyName("João Da Silva")
	var sawCPF, sawName bool
	for _, k := range keys {
		switch k {
		case wantCPF:
			sawCPF = true
		case wantName:
			sawName = true
		}
	}
	if !sawCPF || !sawName {
		t.Fatalf("tombstone keys %v missing cpf(%q) or name(%q)", keys, wantCPF, wantName)
	}
}

func TestRemovalResolve_Reject_TransitionsToRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	person := &memgraph.NodeProvenance{ID: "person_9", Label: "Person"}
	ps := &stubPsql{removal: psql.RemovalRequest{ID: "r3", TargetID: "person_9", TargetType: "Person", Status: "pending", Requester: "x"}}
	mg := &stubMemgraph{prov: person}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postResolve("r3", url.Values{"action": {"reject"}, "resolution": {"no basis"}})
	h.removalResolve(c)

	if len(ps.resolveCalls) != 1 || ps.resolveCalls[0].status != "rejected" {
		t.Fatalf("expected one 'rejected' transition, got %+v", ps.resolveCalls)
	}
	if mg.purged {
		t.Fatalf("reject must not purge the node")
	}
	if loc := w.Header().Get("Location"); loc != "/backoffice/removals" {
		t.Fatalf("expected redirect to removals queue, got Location=%q", loc)
	}
}
