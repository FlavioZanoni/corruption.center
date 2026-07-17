package handlers

import (
	"net/http"

	"corruption-center/db/memgraph"
	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	repo memgraph.Repository
}

func NewSearchHandler(repo memgraph.Repository) *SearchHandler {
	return &SearchHandler{repo: repo}
}

// Search godoc
// @Summary      Full-text search
// @Description  Search across politicians, persons, scandals, organizations, sanctions and legal proceedings
// @Tags         search
// @Produce      json
// @Param        q     query     string  true   "Search query"
// @Param        type  query     string  false  "Node type filter: politician|person|scandal|organization|sanction|legal_proceeding"
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

	results, err := h.repo.QuerySearch(c.Request.Context(), q, nodeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}
