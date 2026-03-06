package handlers

import (
	"net/http"
	"strconv"

	"corruption-center/api/services"
	"github.com/gin-gonic/gin"
)

type GraphHandler struct {
	service services.GraphService
}

func NewGraphHandler(service services.GraphService) *GraphHandler {
	return &GraphHandler{service: service}
}

// GetScandalGraph godoc
// @Summary      Scandal-centric subgraph
// @Description  Returns nodes and edges centered on a scandal
// @Tags         graph
// @Produce      json
// @Param        id   path      string  true  "Scandal ID"
// @Success      200  {object}  models.GraphResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /graph/scandal/{id} [get]
func (h *GraphHandler) GetScandalGraph(c *gin.Context) {
	id := c.Param("id")

	graph, err := h.service.GetScandalGraph(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if graph == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scandal not found"})
		return
	}

	c.JSON(http.StatusOK, graph)
}

// GetPoliticianGraph godoc
// @Summary      Politician-centric subgraph
// @Description  Returns nodes and edges centered on a politician
// @Tags         graph
// @Produce      json
// @Param        id   path      string  true  "Politician ID"
// @Success      200  {object}  models.GraphResponse
// @Failure      404  {object}  models.ErrorResponse
// @Failure      500  {object}  models.ErrorResponse
// @Router       /graph/politician/{id} [get]
func (h *GraphHandler) GetPoliticianGraph(c *gin.Context) {
	id := c.Param("id")

	graph, err := h.service.GetPoliticianGraph(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if graph == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "politician not found"})
		return
	}

	c.JSON(http.StatusOK, graph)
}

// ExpandNode godoc
// @Summary      Expand a node N hops
// @Description  Returns nodes and edges reachable from a node within N hops
// @Tags         graph
// @Produce      json
// @Param        id    path      string  true   "Node ID"
// @Param        hops  query     int     false  "Traversal depth (default 2)"
// @Success      200   {object}  models.GraphResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /graph/expand/{id} [get]
func (h *GraphHandler) ExpandNode(c *gin.Context) {
	id := c.Param("id")

	hops := 2
	if raw := c.Query("hops"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "hops must be an integer between 1 and 5"})
			return
		}
		hops = parsed
	}

	graph, err := h.service.ExpandNode(c.Request.Context(), id, hops)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, graph)
}
