package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/api/models"
	"corruption-center/db/memgraph"
	"github.com/gin-gonic/gin"
)

type PoliticianHandler struct {
	repo memgraph.Repository
}

func NewPoliticianHandler(repo memgraph.Repository) *PoliticianHandler {
	return &PoliticianHandler{repo: repo}
}

// GetPolitician godoc
// @Summary      Politician profile
// @Description  Returns full politician profile with all graph connections
// @Tags         politician
// @Produce      json
// @Param        id   path      string  true  "Politician ID"
// @Success      200  {object}  models.PoliticianProfileResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /politician/{id} [get]
func (h *PoliticianHandler) GetPolitician(c *gin.Context) {
	id := c.Param("id")

	politician, err := h.repo.QueryPolitician(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if politician == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "politician not found"})
		return
	}

	connections, err := h.repo.QueryPoliticianGraph(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.PoliticianProfileResponse{
		Politician:  politician,
		Connections: connections,
	})
}

// ListPoliticians godoc
// @Summary      Browse politicians
// @Description  Paginated, filterable list of politicians. Gives a fresh install content to explore even before any scandals exist.
// @Tags         politician
// @Produce      json
// @Param        filter  query     string  false  "Case-insensitive name substring"
// @Param        party   query     string  false  "Exact party filter (party_current)"
// @Param        uf      query     string  false  "Exact state/UF filter (state)"
// @Param        page    query     int     false  "Page number (1-based, default 1)"
// @Param        page_size query   int     false  "Items per page (default 24, max 100)"
// @Param        sort    query     string  false  "Sort order: connections (default, most linked first) or name"
// @Success      200  {object}  models.PoliticianListResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /politicians [get]
func (h *PoliticianHandler) ListPoliticians(c *gin.Context) {
	filter := c.Query("filter")
	party := c.Query("party")
	uf := c.Query("uf")

	sort := c.Query("sort")
	if sort != "name" {
		sort = "connections"
	}

	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	list, err := h.repo.QueryPoliticians(c.Request.Context(), filter, party, uf, sort, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}
