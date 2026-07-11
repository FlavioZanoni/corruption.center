package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type ProceedingHandler struct {
	service services.GraphService
}

func NewProceedingHandler(service services.GraphService) *ProceedingHandler {
	return &ProceedingHandler{service: service}
}

// ListProceedings godoc
// @Summary      Browse legal proceedings
// @Description  Paginated list of legal proceedings. Walking every page enumerates the whole corpus, which is what the SEO sitemap does.
// @Tags         proceeding
// @Produce      json
// @Param        page      query     int  false  "Page number (1-based, default 1)"
// @Param        page_size query     int  false  "Items per page (default 24, max 100)"
// @Success      200  {object}  models.ProceedingListResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /proceedings [get]
func (h *ProceedingHandler) ListProceedings(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	list, err := h.service.ListProceedings(c.Request.Context(), page, pageSize)
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

	proceeding, err := h.service.GetProceeding(c.Request.Context(), id)
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
