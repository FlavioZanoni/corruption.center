package handlers

import (
	"net/http"

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
// @Description  Returns full scandal profile with all politician and organization connections
// @Tags         scandal
// @Produce      json
// @Param        id   path      string  true  "Scandal ID"
// @Success      200  {object}  models.Scandal
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

	c.JSON(http.StatusOK, scandal)
}
