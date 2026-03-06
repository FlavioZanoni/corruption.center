package handlers

import (
	"net/http"
	"time"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type TimelineHandler struct {
	service services.GraphService
}

func NewTimelineHandler(service services.GraphService) *TimelineHandler {
	return &TimelineHandler{service: service}
}

// GetTimeline godoc
// @Summary      Timeline-filtered graph
// @Description  Returns nodes and edges active within a date range
// @Tags         timeline
// @Produce      json
// @Param        from  query     string  true  "Start date (YYYY-MM-DD)"
// @Param        to    query     string  true  "End date (YYYY-MM-DD)"
// @Success      200   {object}  models.GraphResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /timeline [get]
func (h *TimelineHandler) GetTimeline(c *gin.Context) {
	fromRaw := c.Query("from")
	toRaw := c.Query("to")

	if fromRaw == "" || toRaw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to are required"})
		return
	}

	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be in YYYY-MM-DD format"})
		return
	}

	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to must be in YYYY-MM-DD format"})
		return
	}

	if to.Before(from) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to must be after from"})
		return
	}

	graph, err := h.service.GetTimeline(c.Request.Context(), from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, graph)
}
