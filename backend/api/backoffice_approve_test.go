package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"

	"github.com/gin-gonic/gin"
)

// postApprove builds a POST /backoffice/reviews/:id/approve context.
func postApprove(id string, form url.Values) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := ""
	if form != nil {
		body = form.Encode()
	}
	req := httptest.NewRequest("POST", "/backoffice/reviews/"+id+"/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: id}}
	return c, w
}

// ── payload → edge mapping (happy path) ──────────────────────────────────────

func TestApprove_DJENPartyMatch_CreatesDefendantEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := `{"politician_id":"pol_9","proceeding_id":"lp_7","case_number":"50465129420168040001","polo":"P","tribunal":"TRF4"}`
	ps := &stubPsql{review: psql.PendingReviewItem{ID: "rev1", Type: reviewTypeDJENPartyMatch, Status: "pending", Payload: payload}}
	mg := &stubMemgraph{}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postApprove("rev1", nil)
	h.reviewApprove(c)

	if len(mg.edges) != 1 || mg.edges[0] != (edgeCall{"defendant", "pol_9", "lp_7"}) {
		t.Fatalf("expected DEFENDANT_IN(pol_9→lp_7), got %+v", mg.edges)
	}
	if len(ps.statusUpdates) != 1 || ps.statusUpdates[0] != "approved" {
		t.Fatalf("review must be marked approved, got %+v", ps.statusUpdates)
	}
	if loc := w.Header().Get("Location"); loc != "/backoffice/reviews" {
		t.Fatalf("expected redirect to reviews, got %q", loc)
	}
}

func TestApprove_PoliticianInQSA_CreatesControlsEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := `{"politician_id":"pol_1","organization_id":"org_123","socio_name":"X"}`
	ps := &stubPsql{review: psql.PendingReviewItem{ID: "rev2", Type: reviewTypePoliticianInQSA, Status: "pending", Payload: payload}}
	mg := &stubMemgraph{}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, _ := postApprove("rev2", nil)
	h.reviewApprove(c)

	if len(mg.edges) != 1 || mg.edges[0] != (edgeCall{"controls", "pol_1", "org_123"}) {
		t.Fatalf("expected CONTROLS(pol_1→org_123), got %+v", mg.edges)
	}
	if len(ps.statusUpdates) != 1 {
		t.Fatalf("review must be marked approved, got %+v", ps.statusUpdates)
	}
}

func TestApprove_PoliticianInQSA_FormOverridesCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Multi-candidate payload: no politician_id; operator disambiguates via form.
	payload := `{"organization_id":"org_5","candidates":[{"id":"pol_a"},{"id":"pol_b"}]}`
	ps := &stubPsql{review: psql.PendingReviewItem{ID: "rev3", Type: reviewTypePoliticianInQSA, Status: "pending", Payload: payload}}
	mg := &stubMemgraph{}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, _ := postApprove("rev3", url.Values{"politician_id": {"pol_b"}})
	h.reviewApprove(c)

	if len(mg.edges) != 1 || mg.edges[0] != (edgeCall{"controls", "pol_b", "org_5"}) {
		t.Fatalf("expected CONTROLS(pol_b→org_5) from form override, got %+v", mg.edges)
	}
}

func TestApprove_PoliticianSanction_CreatesSanctionedEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := `{"politician_id":"pol_2","sanction_id":"CEIS:42","registry":"CEIS"}`
	ps := &stubPsql{review: psql.PendingReviewItem{ID: "rev4", Type: reviewTypePoliticianSanction, Status: "pending", Payload: payload}}
	mg := &stubMemgraph{}
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, _ := postApprove("rev4", nil)
	h.reviewApprove(c)

	if len(mg.edges) != 1 || mg.edges[0] != (edgeCall{"sanctioned", "pol_2", "CEIS:42"}) {
		t.Fatalf("expected SANCTIONED_IN(pol_2→CEIS:42), got %+v", mg.edges)
	}
}

// ── missing required ids → visible failure, review stays pending ─────────────

func TestApprove_MissingIDs_FailVisibly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		typ     string
		payload string
		wantMsg string
	}{
		{"djen missing proceeding", reviewTypeDJENPartyMatch, `{"politician_id":"pol_9"}`, "proceeding_id"},
		{"djen missing politician", reviewTypeDJENPartyMatch, `{"proceeding_id":"lp_1"}`, "politician_id"},
		{"qsa missing politician", reviewTypePoliticianInQSA, `{"organization_id":"org_1"}`, "politician_id"},
		{"qsa missing org", reviewTypePoliticianInQSA, `{"politician_id":"pol_1"}`, "organization_id"},
		{"sanction missing sanction", reviewTypePoliticianSanction, `{"politician_id":"pol_2"}`, "sanction_id"},
		{"sanction missing politician", reviewTypePoliticianSanction, `{"sanction_id":"CEIS:1"}`, "politician_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps := &stubPsql{review: psql.PendingReviewItem{ID: "x", Type: tc.typ, Status: "pending", Payload: tc.payload}}
			mg := &stubMemgraph{}
			h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

			c, w := postApprove("x", nil)
			h.reviewApprove(c)

			if len(mg.edges) != 0 {
				t.Fatalf("no edge may be written on missing ids, got %+v", mg.edges)
			}
			if len(ps.statusUpdates) != 0 {
				t.Fatalf("review must stay pending on failure, got status updates %+v", ps.statusUpdates)
			}
			if body := w.Body.String(); !strings.Contains(body, tc.wantMsg) {
				t.Fatalf("expected error banner mentioning %q, body: %s", tc.wantMsg, body)
			}
			if w.Code != 200 {
				t.Fatalf("expected 200 re-render (not redirect), got %d", w.Code)
			}
		})
	}
}

// TestApprove_EdgeWriteFails_StaysPending covers the "edge write fails" branch:
// the graph write errors, so the review must remain pending with a banner.
func TestApprove_EdgeWriteFails_StaysPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := `{"politician_id":"pol_9","proceeding_id":"lp_7"}`
	ps := &stubPsql{review: psql.PendingReviewItem{ID: "rev1", Type: reviewTypeDJENPartyMatch, Status: "pending", Payload: payload}}
	mg := &stubMemgraph{edgeErr: memgraph.ErrNodeNotPurgeable} // any non-nil error
	h := &backofficeHandler{server: &ApiServer{psql: ps, memgraph: mg}}

	c, w := postApprove("rev1", nil)
	h.reviewApprove(c)

	if len(ps.statusUpdates) != 0 {
		t.Fatalf("failed edge write must leave review pending, got %+v", ps.statusUpdates)
	}
	if body := w.Body.String(); !strings.Contains(body, "failed to create DEFENDANT_IN edge") {
		t.Fatalf("expected edge-failure banner, body: %s", body)
	}
}

// Guard the case-number seam: digitsOnly strips CNJ formatting to bare digits so
// a backoffice-seeded case matches the DJEN worker's 20-digit normalized form.
func TestDigitsOnly_StripsCNJFormatting(t *testing.T) {
	if got := digitsOnly("5046512-94.2016.4.04.7000"); got != "50465129420164047000" {
		t.Fatalf("digitsOnly = %q (len %d), want 20-digit bare form", got, len(got))
	}
	if got := digitsOnly("abc-12.3"); got != "123" {
		t.Fatalf("digitsOnly should keep only digits, got %q", got)
	}
}
