package handlers

import (
	"net/http"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type EntityHandler struct {
	service services.GraphService
}

func NewEntityHandler(service services.GraphService) *EntityHandler {
	return &EntityHandler{service: service}
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

	person, err := h.service.GetPerson(c.Request.Context(), id)
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

	org, err := h.service.GetOrganization(c.Request.Context(), id)
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
