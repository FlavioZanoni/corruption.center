package handlers

import (
	"net/http"

	"corruption-center/db/memgraph"
	"github.com/gin-gonic/gin"
)

type EntityHandler struct {
	repo memgraph.Repository
}

func NewEntityHandler(repo memgraph.Repository) *EntityHandler {
	return &EntityHandler{repo: repo}
}

// GetPerson godoc
// @Summary      Person profile
// @Description  Returns full person profile with proceedings and sanctions connections
// @Tags         entity
// @Produce      json
// @Param        id   path      string  true  "Person ID"
// @Success      200  {object}  models.PersonProfileResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /person/{id} [get]
func (h *EntityHandler) GetPerson(c *gin.Context) {
	id := c.Param("id")

	person, err := h.repo.QueryPerson(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if person == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "person not found"})
		return
	}

	c.JSON(http.StatusOK, person)
}

// GetOrganization godoc
// @Summary      Organization profile
// @Description  Returns full organization profile with ownership, control, proceedings and sanctions connections
// @Tags         entity
// @Produce      json
// @Param        id   path      string  true  "Organization ID"
// @Success      200  {object}  models.OrganizationProfileResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /organization/{id} [get]
func (h *EntityHandler) GetOrganization(c *gin.Context) {
	id := c.Param("id")

	org, err := h.repo.QueryOrganization(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if org == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
		return
	}

	c.JSON(http.StatusOK, org)
}
