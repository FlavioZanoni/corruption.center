package api

import (
	"net/http"
	"strconv"
	"strings"

	"corruption-center/api/middleware"
	"corruption-center/db/memgraph"

	"github.com/gin-gonic/gin"
)

type backofficeHandler struct {
	server *ApiServer
}

type seedCaseView struct {
	CaseNumber       string
	TribunalEndpoint string
	ScandalID        string
	Message          string
	Error            string
}

type pendingReviewView struct {
	ID        string
	Type      string
	Status    string
	Worker    string
	CreatedAt string
	Payload   string
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
		back.GET("/logs", h.logs)
	}
}

func (h *backofficeHandler) dashboard(c *gin.Context) {
	renderPage(c, "Backoffice", dashboardPage())
}

func (h *backofficeHandler) seedForm(c *gin.Context) {
	renderPage(c, "Seed DataJud case", seedCasePage(seedCaseView{}))
}

func (h *backofficeHandler) seedSubmit(c *gin.Context) {
	v := seedCaseView{
		CaseNumber:       strings.TrimSpace(c.PostForm("case_number")),
		TribunalEndpoint: strings.TrimSpace(c.PostForm("tribunal_endpoint")),
		ScandalID:        strings.TrimSpace(c.PostForm("scandal_id")),
	}
	if v.CaseNumber == "" || v.TribunalEndpoint == "" || v.ScandalID == "" {
		v.Error = "case_number, tribunal_endpoint and scandal_id are required"
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}

	lpID, err := h.server.memgraph.UpsertLegalProceedingByCase(c.Request.Context(), memgraph.DataJudProceedingUpsert{
		CaseNumber: v.CaseNumber,
		Status:     "ongoing",
		Type:       "criminal",
	})
	if err != nil {
		v.Error = "failed to create/update legal proceeding: " + err.Error()
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}
	if err := h.server.memgraph.EnsureInvestigatesEdge(c.Request.Context(), lpID, v.ScandalID); err != nil {
		v.Error = "failed to link proceeding to scandal: " + err.Error()
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}
	if err := h.server.psql.UpsertWatcherCase(c.Request.Context(), v.CaseNumber, v.TribunalEndpoint, v.ScandalID, lpID, "backoffice"); err != nil {
		v.Error = "failed to seed watcher tracking: " + err.Error()
		renderPage(c, "Seed DataJud case", seedCasePage(v))
		return
	}

	v.Message = "case seeded successfully"
	renderPage(c, "Seed DataJud case", seedCasePage(v))
}

func (h *backofficeHandler) reviewsList(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	items, err := h.server.psql.ListPendingReviews(c.Request.Context(), status, 200)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load reviews: %v", err)
		return
	}
	views := make([]pendingReviewView, 0, len(items))
	for _, it := range items {
		views = append(views, pendingReviewView{
			ID:        it.ID,
			Type:      it.Type,
			Status:    it.Status,
			Worker:    it.Worker,
			CreatedAt: it.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
			Payload:   it.Payload,
		})
	}
	renderPage(c, "Pending reviews", reviewsPage(status, views))
}

func (h *backofficeHandler) reviewApprove(c *gin.Context) {
	h.updateReview(c, "approved")
}

func (h *backofficeHandler) reviewReject(c *gin.Context) {
	h.updateReview(c, "rejected")
}

func (h *backofficeHandler) updateReview(c *gin.Context, status string) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.String(http.StatusBadRequest, "missing id")
		return
	}
	user, _, _ := c.Request.BasicAuth()
	if user == "" {
		user = "backoffice"
	}
	if err := h.server.psql.UpdatePendingReviewStatus(c.Request.Context(), id, status, user); err != nil {
		c.String(http.StatusInternalServerError, "failed to update review: %v", err)
		return
	}
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
	renderPage(c, "Worker logs", logsPage(view))
}
