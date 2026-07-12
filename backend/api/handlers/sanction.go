package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type SanctionHandler struct {
	service services.GraphService
}

func NewSanctionHandler(service services.GraphService) *SanctionHandler {
	return &SanctionHandler{service: service}
}

// GetSanction godoc
// @Summary      Sanction profile
// @Description  Returns full sanction with all sanctioned parties
// @Tags         sanction
// @Produce      json
// @Param        id   path      string  true  "Sanction ID"
// @Success      200  {object}  models.SanctionDetailResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /sanction/{id} [get]
func (h *SanctionHandler) GetSanction(c *gin.Context) {
	id := c.Param("id")

	sanction, err := h.service.GetSanction(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sanction == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sanction not found"})
		return
	}

	c.JSON(http.StatusOK, sanction)
}

// ListSanctions godoc
// @Summary      Browse sanctions
// @Description  Paginated list of sanctions with sanctioned party information. Supports filtering and sorting.
// @Tags         sanction
// @Produce      json
// @Param        page      query     int     false  "Page number (1-based, default 1)"
// @Param        page_size query     int     false  "Items per page (default 24, max 100)"
// @Param        registry  query     string  false  "Filter by registry (exact match)"
// @Param        organ     query     string  false  "Filter by organ (exact match)"
// @Param        q         query     string  false  "Search query (case/accent-insensitive substring on name/registry/type)"
// @Param        sort      query     string  false  "Sort order: date (default, newest first) or registry"
// @Success      200  {object}  models.SanctionListResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /sanctions [get]
func (h *SanctionHandler) ListSanctions(c *gin.Context) {
	registry := c.Query("registry")
	organ := c.Query("organ")
	q := c.Query("q")
	sort := c.Query("sort")
	if sort != "registry" {
		sort = "date"
	}

	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	list, err := h.service.ListSanctions(c.Request.Context(), registry, organ, q, sort, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}

// GetSanctionRegistries godoc
// @Summary      Sanction registries
// @Description  Returns distinct registry values with their counts for filter dropdowns
// @Tags         sanction
// @Produce      json
// @Success      200  {object}  models.SanctionRegistriesResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /sanction-registries [get]
func (h *SanctionHandler) GetSanctionRegistries(c *gin.Context) {
	registries, err := h.service.GetSanctionRegistries(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, registries)
}
