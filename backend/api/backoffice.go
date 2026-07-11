package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"corruption-center/api/middleware"
	"corruption-center/db/memgraph"
	"corruption-center/db/psql"

	"github.com/gin-gonic/gin"
)

// reviewTypeDJENCandidate is the pending_review.type filed by the DJEN worker
// for cases it discovered in name mode that need human approval before being
// registered in watcher_tracking.
const reviewTypeDJENCandidate = "djen_case_candidate"

// Review types whose approval must write the human-confirmed graph edge that is
// the whole point of the review queue: the workers flag these matches but never
// auto-create the link (docs/legal_compliance.md), so the edge only exists once
// an operator approves it here.
const (
	// reviewTypeDJENPartyMatch → DEFENDANT_IN (Politician → LegalProceeding).
	reviewTypeDJENPartyMatch = "djen_party_match"
	// reviewTypePoliticianInQSA → CONTROLS (Politician → Organization).
	reviewTypePoliticianInQSA = "possible_politician_in_qsa"
	// reviewTypePoliticianSanction → SANCTIONED_IN (Politician → Sanction).
	reviewTypePoliticianSanction = "possible_politician_sanction"
)

// nonDigit strips CNJ formatting so a case number collapses to bare digits.
var nonDigit = regexp.MustCompile(`\D`)

// digitsOnly returns s with every non-digit character removed.
func digitsOnly(s string) string { return nonDigit.ReplaceAllString(s, "") }

// djenCaseCandidate is the subset of a djen_case_candidate pending_review
// payload needed to register the case. The full payload also carries
// politician/name/class/link fields used only for display.
type djenCaseCandidate struct {
	CaseNumber       string `json:"case_number"`
	TribunalSigla    string `json:"tribunal_sigla"`
	TribunalEndpoint string `json:"tribunal_endpoint"`
}

// resolveTribunalEndpoint returns the DataJud endpoint for a candidate. It
// prefers the explicit tribunal_endpoint key and falls back to deriving it from
// tribunal_sigla as "api_publica_<sigla lowercase>" for older payloads. If both
// are missing it returns an error so the case is not approved.
func resolveTribunalEndpoint(c djenCaseCandidate) (string, error) {
	if ep := strings.TrimSpace(c.TribunalEndpoint); ep != "" {
		return ep, nil
	}
	if sigla := strings.TrimSpace(c.TribunalSigla); sigla != "" {
		return "api_publica_" + strings.ToLower(sigla), nil
	}
	return "", fmt.Errorf("payload is missing both tribunal_endpoint and tribunal_sigla; cannot resolve DataJud endpoint")
}

// registerWatcherCase performs the shared case-registration used by both the
// manual seed flow and the DJEN approval flow: upsert the LegalProceeding node,
// link it to the scandal, and record it in watcher_tracking. addedBy records
// provenance ("backoffice" for manual seeds, "djen" for approved candidates).
func (h *backofficeHandler) registerWatcherCase(ctx context.Context, caseNumber, tribunalEndpoint, scandalID, addedBy string) error {
	// Normalize to bare digits so a backoffice-seeded row (which may be typed in
	// formatted CNJ form, "5046512-94.2016.4.04.7000") matches the exact 20-digit
	// key the DJEN worker polls with (workers/djen normalizeCaseNumber). Without
	// this the seeded case and the worker's snapshot keys never line up.
	caseNumber = digitsOnly(caseNumber)
	if len(caseNumber) != 20 {
		return fmt.Errorf("case_number must be 20 digits after stripping formatting, got %d digit(s)", len(caseNumber))
	}
	lpID, err := h.server.memgraph.UpsertLegalProceedingByCase(ctx, memgraph.DataJudProceedingUpsert{
		CaseNumber: caseNumber,
		Status:     "ongoing",
		Type:       "criminal",
	})
	if err != nil {
		return fmt.Errorf("failed to create/update legal proceeding: %w", err)
	}
	if err := h.server.memgraph.EnsureInvestigatesEdge(ctx, lpID, scandalID); err != nil {
		return fmt.Errorf("failed to link proceeding to scandal: %w", err)
	}
	if err := h.server.psql.UpsertWatcherCase(ctx, caseNumber, tribunalEndpoint, scandalID, lpID, addedBy); err != nil {
		return fmt.Errorf("failed to seed watcher tracking: %w", err)
	}
	return nil
}

type backofficeHandler struct {
	server *ApiServer
}

type seedCaseView struct {
	CaseNumber       string
	TribunalEndpoint string
	ScandalID        string
	Message          string
	Error            string
	Scandals         []memgraph.ScandalOption
}

type reviewTypeCountView struct {
	Type  string
	Count int
}

type pendingReviewView struct {
	ID              string
	Type            string
	Status          string
	Worker          string
	CreatedAt       string
	Payload         string
	IsDJENCandidate bool
}

// removalRequestView is a removal_request row plus the live provenance of the
// targeted node so the operator sees the creation reason before deciding.
type removalRequestView struct {
	ID           string
	Requester    string
	TargetType   string
	TargetID     string
	Reason       string
	Status       string
	Resolution   string
	ReceivedAt   string
	ResolvedAt   string
	ResolvedBy   string
	Provenance   *memgraph.NodeProvenance
	ProvErr      string
	IsPending    bool
	IsPolitician bool
}

type auditView struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   string
	CreatedAt  string
}

type logsView struct {
	Source          string
	Status          string
	StartedAt       string
	FinishedAt      string
	RecordsUpserted int
	Details         string
	ErrorMessage    string
}

func (s *ApiServer) registerBackoffice(r *gin.Engine) {
	h := &backofficeHandler{server: s}
	back := r.Group("/backoffice", middleware.BackofficeBasicAuth())
	{
		back.GET("", h.dashboard)
		back.GET("/seed", h.seedForm)
		back.POST("/seed", h.seedSubmit)
		back.GET("/reviews", h.reviewsList)
		back.POST("/reviews/:id/approve", h.reviewApprove)
		back.POST("/reviews/:id/reject", h.reviewReject)
		back.GET("/removals", h.removalsList)
		back.POST("/removals", h.removalCreate)
		back.POST("/removals/:id/resolve", h.removalResolve)
		back.GET("/logs", h.logs)
	}
}

// currentUser returns the authenticated backoffice operator, falling back to
// "backoffice" when basic-auth is absent. Used as the audit actor.
func currentUser(c *gin.Context) string {
	user, _, _ := c.Request.BasicAuth()
	if strings.TrimSpace(user) == "" {
		return "backoffice"
	}
	return user
}

// resolveScandalID picks the scandal id from the form: a newly-typed id
// (new_scandal_id) always wins over the dropdown selection so operators can
// register a case against a scandal that does not exist as a node yet.
func resolveScandalID(c *gin.Context) string {
	if newID := strings.TrimSpace(c.PostForm("new_scandal_id")); newID != "" {
		return newID
	}
	return strings.TrimSpace(c.PostForm("scandal_id"))
}

// loadScandals fetches the scandal options for the selector, tolerating a
// memgraph error by returning an empty list (the free-text fallback still works).
func (h *backofficeHandler) loadScandals(ctx context.Context) []memgraph.ScandalOption {
	scandals, err := h.server.memgraph.ListScandals(ctx)
	if err != nil {
		return nil
	}
	return scandals
}

func (h *backofficeHandler) dashboard(c *gin.Context) {
	counts, err := h.server.psql.CountPendingReviewsByType(c.Request.Context())
	if err != nil {
		counts = nil
	}
	views := make([]reviewTypeCountView, 0, len(counts))
	total := 0
	for _, ct := range counts {
		views = append(views, reviewTypeCountView{Type: ct.Type, Count: ct.Count})
		total += ct.Count
	}
	renderPage(c, "Backoffice", dashboardPage(views, total))
}

func (h *backofficeHandler) seedForm(c *gin.Context) {
	renderPage(c, "Seed DataJud case", seedCasePage(seedCaseView{Scandals: h.loadScandals(c.Request.Context())}))
}

func (h *backofficeHandler) seedSubmit(c *gin.Context) {
	v := seedCaseView{
		CaseNumber:       strings.TrimSpace(c.PostForm("case_number")),
		TribunalEndpoint: strings.TrimSpace(c.PostForm("tribunal_endpoint")),
		ScandalID:        resolveScandalID(c),
		Scandals:         h.loadScandals(c.Request.Context()),
	}
	if v.CaseNumber == "" || v.TribunalEndpoint == "" || v.ScandalID == "" {
		v.Error = "case_number, tribunal_endpoint and scandal_id are required"
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}

	if err := h.registerWatcherCase(c.Request.Context(), v.CaseNumber, v.TribunalEndpoint, v.ScandalID, "backoffice"); err != nil {
		v.Error = err.Error()
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}

	user := currentUser(c)
	_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionCreate, "watcher_case", v.CaseNumber, map[string]any{
		"tribunal_endpoint": v.TribunalEndpoint,
		"scandal_id":        v.ScandalID,
		"via":               "backoffice_seed",
	})

	v.Message = "case seeded successfully"
	renderPage(c, "Seed DataJud case", seedCasePage(v))
}

func (h *backofficeHandler) reviewsList(c *gin.Context) {
	h.renderReviews(c, "")
}

// renderReviews lists pending reviews (honoring the ?status filter) and renders
// the reviews page, optionally with a top-level error banner. It is used both
// for the normal GET and to re-render after a failed approval so the operator
// sees the error and the item stays pending.
func (h *backofficeHandler) renderReviews(c *gin.Context, errMsg string) {
	status := strings.TrimSpace(c.Query("status"))
	typ := strings.TrimSpace(c.Query("type"))
	items, err := h.server.psql.ListPendingReviews(c.Request.Context(), status, typ, 200)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load reviews: %v", err)
		return
	}
	views := make([]pendingReviewView, 0, len(items))
	for _, it := range items {
		views = append(views, pendingReviewView{
			ID:              it.ID,
			Type:            it.Type,
			Status:          it.Status,
			Worker:          it.Worker,
			CreatedAt:       it.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
			Payload:         it.Payload,
			IsDJENCandidate: it.Type == reviewTypeDJENCandidate,
		})
	}
	renderPage(c, "Pending reviews", reviewsPage(status, typ, views, h.loadScandals(c.Request.Context()), errMsg))
}

func (h *backofficeHandler) reviewApprove(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.String(http.StatusBadRequest, "missing id")
		return
	}
	item, err := h.server.psql.GetPendingReview(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load review: %v", err)
		return
	}
	switch item.Type {
	case reviewTypeDJENCandidate:
		h.approveDJENCandidate(c, item)
	case reviewTypeDJENPartyMatch:
		h.approveDJENPartyMatch(c, item)
	case reviewTypePoliticianInQSA:
		h.approvePoliticianInQSA(c, item)
	case reviewTypePoliticianSanction:
		h.approvePoliticianSanction(c, item)
	default:
		h.updateReview(c, "approved")
	}
}

// finishEdgeApproval marks the review approved and audits the confirmed edge
// once its graph write has succeeded. On any failure it re-renders the queue
// with the error and leaves the review pending, mirroring approveDJENCandidate.
func (h *backofficeHandler) finishEdgeApproval(c *gin.Context, item psql.PendingReviewItem, edge string, meta map[string]any) {
	user := currentUser(c)
	if err := h.server.psql.UpdatePendingReviewStatus(c.Request.Context(), item.ID, "approved", user); err != nil {
		h.renderReviews(c, "edge created but failed to mark review approved: "+err.Error())
		return
	}
	// Audit the confirmed edge (LGPD who/what/when for a human-created link).
	auditMeta := map[string]any{"resolution": "approved", "type": item.Type, "edge": edge}
	for k, v := range meta {
		auditMeta[k] = v
	}
	_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionCreate, "graph_edge", item.ID, auditMeta)
	c.Redirect(http.StatusSeeOther, "/backoffice/reviews")
}

// approveDJENPartyMatch confirms a DEFENDANT_IN edge from the matched Politician
// to the LegalProceeding. Fields are written by workers/djen (djen_party_match).
func (h *backofficeHandler) approveDJENPartyMatch(c *gin.Context, item psql.PendingReviewItem) {
	var p struct {
		PoliticianID string `json:"politician_id"`
		ProceedingID string `json:"proceeding_id"`
		CaseNumber   string `json:"case_number"`
	}
	if err := json.Unmarshal([]byte(item.Payload), &p); err != nil {
		h.renderReviews(c, "failed to parse review payload: "+err.Error())
		return
	}
	polID := strings.TrimSpace(p.PoliticianID)
	procID := strings.TrimSpace(p.ProceedingID)
	if polID == "" || procID == "" {
		h.renderReviews(c, "review payload is missing politician_id or proceeding_id; cannot create DEFENDANT_IN edge")
		return
	}
	if err := h.server.memgraph.EnsurePoliticianDefendantEdge(c.Request.Context(), polID, procID); err != nil {
		h.renderReviews(c, "failed to create DEFENDANT_IN edge: "+err.Error())
		return
	}
	h.finishEdgeApproval(c, item, "DEFENDANT_IN", map[string]any{
		"politician_id": polID, "proceeding_id": procID, "case_number": strings.TrimSpace(p.CaseNumber),
	})
}

// approvePoliticianInQSA confirms a CONTROLS edge from the Politician to the
// Organization. Fields are written by workers/cnpj (possible_politician_in_qsa).
// politician_id is present only when the worker found a single candidate; a
// multi-candidate payload cannot be auto-approved and fails visibly.
func (h *backofficeHandler) approvePoliticianInQSA(c *gin.Context, item psql.PendingReviewItem) {
	var p struct {
		PoliticianID   string `json:"politician_id"`
		OrganizationID string `json:"organization_id"`
	}
	if err := json.Unmarshal([]byte(item.Payload), &p); err != nil {
		h.renderReviews(c, "failed to parse review payload: "+err.Error())
		return
	}
	// An operator may disambiguate a multi-candidate match by posting politician_id.
	polID := strings.TrimSpace(c.PostForm("politician_id"))
	if polID == "" {
		polID = strings.TrimSpace(p.PoliticianID)
	}
	orgID := strings.TrimSpace(p.OrganizationID)
	if polID == "" || orgID == "" {
		h.renderReviews(c, "review payload is missing politician_id or organization_id (multiple candidates need an explicit politician_id); cannot create CONTROLS edge")
		return
	}
	if err := h.server.memgraph.EnsurePoliticianControlsOrganization(c.Request.Context(), polID, orgID); err != nil {
		h.renderReviews(c, "failed to create CONTROLS edge: "+err.Error())
		return
	}
	h.finishEdgeApproval(c, item, "CONTROLS", map[string]any{
		"politician_id": polID, "organization_id": orgID,
	})
}

// approvePoliticianSanction confirms a SANCTIONED_IN edge from the Politician to
// the Sanction. Fields are written by workers/sanctions (queueReview:
// possible_politician_sanction).
func (h *backofficeHandler) approvePoliticianSanction(c *gin.Context, item psql.PendingReviewItem) {
	var p struct {
		PoliticianID string `json:"politician_id"`
		SanctionID   string `json:"sanction_id"`
	}
	if err := json.Unmarshal([]byte(item.Payload), &p); err != nil {
		h.renderReviews(c, "failed to parse review payload: "+err.Error())
		return
	}
	polID := strings.TrimSpace(p.PoliticianID)
	sancID := strings.TrimSpace(p.SanctionID)
	if polID == "" || sancID == "" {
		h.renderReviews(c, "review payload is missing politician_id or sanction_id; cannot create SANCTIONED_IN edge")
		return
	}
	if err := h.server.memgraph.EnsurePoliticianSanctionedInEdge(c.Request.Context(), polID, sancID); err != nil {
		h.renderReviews(c, "failed to create SANCTIONED_IN edge: "+err.Error())
		return
	}
	h.finishEdgeApproval(c, item, "SANCTIONED_IN", map[string]any{
		"politician_id": polID, "sanction_id": sancID,
	})
}

// approveDJENCandidate registers an approved djen_case_candidate in
// watcher_tracking (same path as the manual seed) and only then marks the
// review approved. On any failure it re-renders the reviews page with the error
// and leaves the review pending.
func (h *backofficeHandler) approveDJENCandidate(c *gin.Context, item psql.PendingReviewItem) {
	scandalID := resolveScandalID(c)
	if scandalID == "" {
		h.renderReviews(c, "scandal id is required to approve a DJEN case candidate")
		return
	}

	var cand djenCaseCandidate
	if err := json.Unmarshal([]byte(item.Payload), &cand); err != nil {
		h.renderReviews(c, "failed to parse review payload: "+err.Error())
		return
	}
	caseNumber := strings.TrimSpace(cand.CaseNumber)
	if caseNumber == "" {
		h.renderReviews(c, "review payload is missing case_number; cannot approve")
		return
	}
	endpoint, err := resolveTribunalEndpoint(cand)
	if err != nil {
		h.renderReviews(c, err.Error())
		return
	}

	if err := h.registerWatcherCase(c.Request.Context(), caseNumber, endpoint, scandalID, "djen"); err != nil {
		h.renderReviews(c, err.Error())
		return
	}

	user := currentUser(c)
	if err := h.server.psql.UpdatePendingReviewStatus(c.Request.Context(), item.ID, "approved", user); err != nil {
		h.renderReviews(c, "case registered but failed to mark review approved: "+err.Error())
		return
	}
	_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionUpdate, "pending_review", item.ID, map[string]any{
		"resolution":  "approved",
		"type":        item.Type,
		"case_number": caseNumber,
		"scandal_id":  scandalID,
	})
	c.Redirect(http.StatusSeeOther, "/backoffice/reviews")
}

func (h *backofficeHandler) reviewReject(c *gin.Context) {
	h.updateReview(c, "rejected")
}

// updateReview transitions a pending_review to the given terminal status,
// captures the operator's reason (rejections must be recorded so the watcher
// does not re-flag the same false match), and writes an audit record.
func (h *backofficeHandler) updateReview(c *gin.Context, status string) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.String(http.StatusBadRequest, "missing id")
		return
	}
	user := currentUser(c)
	item, err := h.server.psql.GetPendingReview(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load review: %v", err)
		return
	}
	if err := h.server.psql.UpdatePendingReviewStatus(c.Request.Context(), id, status, user); err != nil {
		c.String(http.StatusInternalServerError, "failed to update review: %v", err)
		return
	}
	meta := map[string]any{"resolution": status, "type": item.Type}
	if reason := strings.TrimSpace(c.PostForm("reason")); reason != "" {
		meta["reason"] = reason
	}
	_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionUpdate, "pending_review", id, meta)
	c.Redirect(http.StatusSeeOther, "/backoffice/reviews")
}

func (h *backofficeHandler) logs(c *gin.Context) {
	limit := 200
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	rows, err := h.server.psql.ListWorkerLogs(c.Request.Context(), limit)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load worker logs: %v", err)
		return
	}
	view := make([]logsView, 0, len(rows))
	for _, r := range rows {
		finished := "-"
		if r.FinishedAt != nil {
			finished = r.FinishedAt.UTC().Format("2006-01-02 15:04:05")
		}
		errMsg := ""
		if r.ErrorMessage != nil {
			errMsg = *r.ErrorMessage
		}
		view = append(view, logsView{
			Source:          r.Source,
			Status:          r.Status,
			StartedAt:       r.StartedAt.UTC().Format("2006-01-02 15:04:05"),
			FinishedAt:      finished,
			RecordsUpserted: r.RecordsUpserted,
			Details:         r.Details,
			ErrorMessage:    errMsg,
		})
	}

	filter := psql.AuditFilter{
		ActorID:    strings.TrimSpace(c.Query("actor")),
		Action:     strings.TrimSpace(c.Query("action")),
		TargetType: strings.TrimSpace(c.Query("target_type")),
	}
	auditRows, err := h.server.psql.ListAuditEntries(c.Request.Context(), filter, limit)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load audit log: %v", err)
		return
	}
	audit := make([]auditView, 0, len(auditRows))
	for _, a := range auditRows {
		meta := ""
		if len(a.Metadata) > 0 {
			if b, err := json.Marshal(a.Metadata); err == nil {
				meta = string(b)
			}
		}
		audit = append(audit, auditView{
			ActorID:    a.ActorID,
			Action:     a.Action,
			TargetType: a.TargetType,
			TargetID:   a.TargetID,
			Metadata:   meta,
			CreatedAt:  a.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
		})
	}
	renderPage(c, "Worker logs", logsPage(view, audit, filter))
}

// removalsList renders the LGPD data-removal request queue, honoring the
// ?status filter, with the live provenance of each targeted node.
func (h *backofficeHandler) removalsList(c *gin.Context) {
	h.renderRemovals(c, "", "")
}

func (h *backofficeHandler) renderRemovals(c *gin.Context, msg, errMsg string) {
	status := strings.TrimSpace(c.Query("status"))
	items, err := h.server.psql.ListRemovalRequests(c.Request.Context(), status, 200)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load removal requests: %v", err)
		return
	}
	views := make([]removalRequestView, 0, len(items))
	for _, it := range items {
		v := removalRequestView{
			ID:         it.ID,
			Requester:  it.Requester,
			TargetType: it.TargetType,
			TargetID:   it.TargetID,
			Reason:     it.Reason,
			Status:     it.Status,
			Resolution: it.Resolution,
			ReceivedAt: it.ReceivedAt.UTC().Format("2006-01-02 15:04:05"),
			IsPending:  it.Status == "pending",
		}
		if it.ResolvedAt != nil {
			v.ResolvedAt = it.ResolvedAt.UTC().Format("2006-01-02 15:04:05")
		}
		if it.ResolvedBy != nil {
			v.ResolvedBy = *it.ResolvedBy
		}
		// Show the live creation reason of the targeted node so the operator can
		// decide, and so a Politician is flagged as not purgeable up front.
		prov, provErr := h.server.memgraph.GetNodeProvenance(c.Request.Context(), it.TargetID)
		if provErr != nil {
			v.ProvErr = provErr.Error()
		} else if prov != nil {
			v.Provenance = prov
			v.IsPolitician = prov.IsPolitician
		}
		views = append(views, v)
	}
	renderPage(c, "Data removal requests", removalsPage(status, views, msg, errMsg))
}

func (h *backofficeHandler) removalCreate(c *gin.Context) {
	requester := strings.TrimSpace(c.PostForm("requester"))
	targetType := strings.TrimSpace(c.PostForm("target_type"))
	targetID := strings.TrimSpace(c.PostForm("target_id"))
	reason := strings.TrimSpace(c.PostForm("reason"))
	if requester == "" || targetType == "" || targetID == "" {
		h.renderRemovals(c, "", "requester, target_type and target_id are required")
		return
	}
	id, err := h.server.psql.CreateRemovalRequest(c.Request.Context(), requester, targetType, targetID, reason)
	if err != nil {
		h.renderRemovals(c, "", err.Error())
		return
	}
	_ = h.server.psql.LogAudit(c.Request.Context(), currentUser(c), psql.AuditActionCreate, "removal_request", id, map[string]any{
		"requester":   requester,
		"target_type": targetType,
		"target_id":   targetID,
	})
	c.Redirect(http.StatusSeeOther, "/backoffice/removals")
}

// removalResolve closes a removal request. action=purge deletes the targeted
// Person/Organization node and all its edges (refusing Politicians); action=
// reject records a documented refusal; action=manual marks it resolved without a
// graph change. Every path writes an audit record.
func (h *backofficeHandler) removalResolve(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.String(http.StatusBadRequest, "missing id")
		return
	}
	action := strings.TrimSpace(c.PostForm("action"))
	resolution := strings.TrimSpace(c.PostForm("resolution"))
	user := currentUser(c)

	req, err := h.server.psql.GetRemovalRequest(c.Request.Context(), id)
	if err != nil {
		h.renderRemovals(c, "", "failed to load removal request: "+err.Error())
		return
	}

	switch action {
	case "purge":
		// Verify the request is still pending BEFORE deleting anything. A replayed
		// POST against an already rejected/resolved request must not delete the
		// node — the status gate closes that resurrection-by-replay hole.
		if req.Status != "pending" {
			h.renderRemovals(c, "", "purge refused: removal request is not pending (already "+req.Status+")")
			return
		}
		prov, err := h.server.memgraph.PurgePersonNode(c.Request.Context(), req.TargetID)
		if err != nil {
			// Politician refusal (or any purge failure): leave the request pending
			// so it can be handled explicitly, and surface the justification.
			h.renderRemovals(c, "", "purge refused: "+err.Error())
			return
		}
		meta := map[string]any{"target_id": req.TargetID, "requester": req.Requester}
		if prov != nil {
			meta["label"] = prov.Label
			meta["name"] = prov.Name
			meta["creation_reason"] = prov.CreationReason
			meta["edges_deleted"] = prov.EdgeCount
		}
		// Deletion record in audit_log — the LGPD "why/what was removed" trail.
		_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionDelete, "graph_node", req.TargetID, meta)

		// Write purge tombstones so the weekly worker syncs cannot silently
		// resurrect the removed subject (LGPD art. 18). Keys are built from the
		// pre-deletion node properties; the removal request id is a UUID, which
		// the tombstone table's BIGINT removal_id cannot represent, so it is
		// recorded via node_id instead (removalID passed as 0 = NULL).
		if keys := purgeTombstoneKeys(prov); len(keys) > 0 {
			if err := h.server.psql.CreatePurgeTombstones(c.Request.Context(), keys, req.TargetID, req.ID); err != nil {
				h.renderRemovals(c, "", "node purged but failed to write purge tombstone: "+err.Error())
				return
			}
		}
		if resolution == "" {
			resolution = "purged targeted node and all edges from the graph"
		}
		if err := h.server.psql.ResolveRemovalRequest(c.Request.Context(), id, "resolved", resolution, user); err != nil {
			h.renderRemovals(c, "", err.Error())
			return
		}
	case "reject":
		if resolution == "" {
			resolution = "request refused"
		}
		if err := h.server.psql.ResolveRemovalRequest(c.Request.Context(), id, "rejected", resolution, user); err != nil {
			h.renderRemovals(c, "", err.Error())
			return
		}
		_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionUpdate, "removal_request", id, map[string]any{
			"resolution": "rejected", "note": resolution,
		})
	case "manual":
		if resolution == "" {
			resolution = "resolved manually"
		}
		if err := h.server.psql.ResolveRemovalRequest(c.Request.Context(), id, "resolved", resolution, user); err != nil {
			h.renderRemovals(c, "", err.Error())
			return
		}
		_ = h.server.psql.LogAudit(c.Request.Context(), user, psql.AuditActionUpdate, "removal_request", id, map[string]any{
			"resolution": "resolved", "note": resolution,
		})
	default:
		h.renderRemovals(c, "", "unknown action: "+action)
		return
	}
	c.Redirect(http.StatusSeeOther, "/backoffice/removals")
}

// purgeTombstoneKeys builds the anti-resurrection tombstone keys from a purged
// node's pre-deletion provenance: a cpf key when a CPF is known, a cnpj key when
// a CNPJ is known, and always a name key when a name is present. The workers
// consult these before recreating a node (see db/psql/tombstone.go).
func purgeTombstoneKeys(prov *memgraph.NodeProvenance) []string {
	if prov == nil {
		return nil
	}
	keys := make([]string, 0, 3)
	if cpf := digitsOnly(prov.CPF); cpf != "" {
		keys = append(keys, psql.TombstoneKeyCPF(cpf))
	}
	if cnpj := digitsOnly(prov.CNPJ); cnpj != "" {
		keys = append(keys, psql.TombstoneKeyCNPJ(cnpj))
	}
	if strings.TrimSpace(prov.Name) != "" {
		keys = append(keys, psql.TombstoneKeyName(prov.Name))
	}
	return keys
}
