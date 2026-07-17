package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/db/memgraph"
	"github.com/gin-gonic/gin"
)

type ProceedingHandler struct {
	repo memgraph.Repository
}

func NewProceedingHandler(repo memgraph.Repository) *ProceedingHandler {
	return &ProceedingHandler{repo: repo}
}

// ListProceedings godoc
// @Summary      Browse legal proceedings
// @Description  Paginated list of legal proceedings with support for filtering and sorting. Walking every page enumerates the whole corpus (when no filters applied), which is what the SEO sitemap does.
// @Tags         proceeding
// @Produce      json
// @Param        page      query     int     false  "Page number (1-based, default 1)"
// @Param        page_size query     int     false  "Items per page (default 24, max 100)"
// @Param        court     query     string  false  "Filter by court (exact match)"
// @Param        has_conviction query string false "Conviction state: true (a court convicted), false (DataJud looked, no conviction), unknown (DataJud has never looked), or absent (all)"
// @Param        q         query     string  false  "Search query (case/accent-insensitive substring on case_number or class_name)"
// @Param        sort      query     string  false  "Sort order: case_number (default) or court"
// @Success      200  {object}  models.ProceedingListResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /proceedings [get]
func (h *ProceedingHandler) ListProceedings(c *gin.Context) {
	court := c.Query("court")
	hasConviction := c.Query("has_conviction")
	q := c.Query("q")
	sort := c.Query("sort")
	if sort != "court" {
		sort = "case_number"
	}

	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	list, err := h.repo.QueryProceedings(c.Request.Context(), court, hasConviction, q, sort, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}

// GetProceeding godoc
// @Summary      Legal proceeding detail
// @Description  Returns one legal proceeding by node id, with the scandal it investigates and every defendant linked to it (including the DEFENDANT_IN edge's provenance).
// @Tags         proceeding
// @Produce      json
// @Param        id   path      string  true  "Legal proceeding node ID"
// @Success      200  {object}  models.ProceedingDetailResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /proceeding/{id} [get]
func (h *ProceedingHandler) GetProceeding(c *gin.Context) {
	id := c.Param("id")

	proceeding, err := h.repo.QueryProceeding(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if proceeding == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proceeding not found"})
		return
	}

	c.JSON(http.StatusOK, proceeding)
}
