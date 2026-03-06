package handlers

import (
	"net/http"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	service services.SearchService
}

func NewSearchHandler(service services.SearchService) *SearchHandler {
	return &SearchHandler{service: service}
}

// Search godoc
// @Summary      Full-text search
// @Description  Search across politicians, scandals and organizations
// @Tags         search
// @Produce      json
// @Param        q     query     string  true   "Search query"
// @Param        type  query     string  false  "Node type filter: politician|scandal|organization"
// @Success      200   {object}  models.GraphResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /search [get]
func (h *SearchHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}

	nodeType := c.Query("type")

	results, err := h.service.Search(c.Request.Context(), q, nodeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}
