package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// HealthCheck godoc
// @Summary      Health check endpoint
// @Description  Returns the health status of the API
// @Tags         health
// @Produce      plain
// @Success      200  {string}  string  "ok"
// @Router       /health [get]
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}
