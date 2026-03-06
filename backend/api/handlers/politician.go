package handlers

import (
	"net/http"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type PoliticianHandler struct {
	service services.GraphService
}

func NewPoliticianHandler(service services.GraphService) *PoliticianHandler {
	return &PoliticianHandler{service: service}
}

// GetPolitician godoc
// @Summary      Politician profile
// @Description  Returns full politician profile with all scandal and proceeding connections
// @Tags         politician
// @Produce      json
// @Param        id   path      string  true  "Politician ID"
// @Success      200  {object}  models.Politician
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /politician/{id} [get]
func (h *PoliticianHandler) GetPolitician(c *gin.Context) {
	id := c.Param("id")

	politician, err := h.service.GetPolitician(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if politician == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "politician not found"})
		return
	}

	c.JSON(http.StatusOK, politician)
}
