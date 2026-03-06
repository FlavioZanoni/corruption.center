package api

import (
	"corruption-center/api/handlers"
	"corruption-center/api/services"
	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type ApiServer struct {
	psql     psql.Repository
	memgraph memgraph.Repository
}

func NewApiServer(psql psql.Repository, memgraph memgraph.Repository) *ApiServer {
	return &ApiServer{psql: psql, memgraph: memgraph}
}

// @title         ⌬ API
// @version       0.1.0
// @description   Corruption graph api
// @BasePath      /api/v1
func (s *ApiServer) SetupRouter() *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	graphSvc := services.NewGraphService(s.memgraph)
	searchSvc := services.NewSearchService(s.memgraph)

	h := handlers.NewHandlers(graphSvc, searchSvc)

	r.GET("/health", h.Health.HealthCheck)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/graph/scandal/:id", h.Graph.GetScandalGraph)
		v1.GET("/graph/politician/:id", h.Graph.GetPoliticianGraph)
		v1.GET("/graph/expand/:id", h.Graph.ExpandNode)
		v1.GET("/search", h.Search.Search)
		v1.GET("/politician/:id", h.Politician.GetPolitician)
		v1.GET("/scandal/:id", h.Scandal.GetScandal)
		v1.GET("/timeline", h.Timeline.GetTimeline)
	}

	return r
}

func (s *ApiServer) Start(port string) {
	dev := os.Getenv("DEV")
	if dev != "true" {
		gin.SetMode(gin.ReleaseMode)
	}

	log.Default().Printf("Starting API server on port %s", port)
	r := s.SetupRouter()
	r.Run(fmt.Sprintf(":%s", port))
}

