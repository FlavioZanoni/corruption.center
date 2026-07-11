package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/api/models"
	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type ScandalHandler struct {
	service services.GraphService
}

func NewScandalHandler(service services.GraphService) *ScandalHandler {
	return &ScandalHandler{service: service}
}

// GetScandal godoc
// @Summary      Scandal profile
// @Description  Returns full scandal profile with all graph connections
// @Tags         scandal
// @Produce      json
// @Param        id   path      string  true  "Scandal ID"
// @Success      200  {object}  models.ScandalProfileResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /scandal/{id} [get]
func (h *ScandalHandler) GetScandal(c *gin.Context) {
	id := c.Param("id")

	scandal, err := h.service.GetScandal(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if scandal == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scandal not found"})
		return
	}

	connections, err := h.service.GetScandalGraph(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.ScandalProfileResponse{
		Scandal:     scandal,
		Connections: connections,
	})
}

// ListScandals godoc
// @Summary      Browse scandals
// @Description  Paginated list of scandals with involved-politician and linked-proceeding counts. Walking every page enumerates the whole corpus, which is what the SEO sitemap does.
// @Tags         scandal
// @Produce      json
// @Param        page      query     int     false  "Page number (1-based, default 1)"
// @Param        page_size query     int     false  "Items per page (default 24, max 100)"
// @Param        sort      query     string  false  "Sort order: date (default, newest first) or name"
// @Success      200  {object}  models.ScandalListResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /scandals [get]
func (h *ScandalHandler) ListScandals(c *gin.Context) {
	sort := c.Query("sort")
	if sort != "name" {
		sort = "date"
	}

	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	list, err := h.service.ListScandals(c.Request.Context(), sort, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}
