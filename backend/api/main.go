package api

import (
	"corruption-center/api/handlers"
	"corruption-center/api/middleware"
	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

type ApiServer struct {
	psql     psql.Repository
	memgraph memgraph.Repository
}

func NewApiServer(psql psql.Repository, memgraph memgraph.Repository) *ApiServer {
	return &ApiServer{psql: psql, memgraph: memgraph}
}

func (s *ApiServer) SetupRouter() *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)
	r.Use(middleware.CORS())

	h := handlers.NewHandlers(s.memgraph)

	r.GET("/health", h.Health.HealthCheck)

	// The public API is read-only over data that changes at worker cadence
	// (nightly syncs, occasional backoffice edits), so let browsers and any
	// reverse proxy in front reuse responses briefly instead of re-running the
	// same Memgraph queries. 5 min matches the frontend's client-side staleTime;
	// stale-while-revalidate lets a proxy serve the old answer while it refreshes.
	// The backoffice group below is authenticated and state-changing — it gets no
	// cache headers, deliberately.
	v1 := r.Group("/api/v1")
	v1.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=300, stale-while-revalidate=3600")
		c.Next()
	})
	{
		v1.GET("/graph/scandal/:id", h.Graph.GetScandalGraph)
		v1.GET("/graph/politician/:id", h.Graph.GetPoliticianGraph)
		v1.GET("/graph/expand/:id", h.Graph.ExpandNode)
		v1.GET("/search", h.Search.Search)
		v1.GET("/politicians", h.Politician.ListPoliticians)
		v1.GET("/politician/:id", h.Politician.GetPolitician)
		v1.GET("/scandals", h.Scandal.ListScandals)
		v1.GET("/scandal/:id", h.Scandal.GetScandal)
		v1.GET("/sanctions", h.Sanction.ListSanctions)
		v1.GET("/sanction/:id", h.Sanction.GetSanction)
		v1.GET("/sanction-registries", h.Sanction.GetSanctionRegistries)
		v1.GET("/proceedings", h.Proceeding.ListProceedings)
		v1.GET("/proceeding/:id", h.Proceeding.GetProceeding)
		v1.GET("/person/:id", h.Entity.GetPerson)
		v1.GET("/organization/:id", h.Entity.GetOrganization)
		v1.GET("/timeline", h.Timeline.GetTimeline)
	}

	s.registerBackoffice(r)

	return r
}

func (s *ApiServer) Start(port string) {
	dev := os.Getenv("DEV")
	if dev != "true" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Fail fast if production is using weak/default backoffice credentials.
	// This prevents silent security misconfigurations like the old ENABLE_WRITES bug.
	if err := middleware.ValidateBackofficeCredentials(); err != nil {
		log.Fatal(err)
	}

	log.Default().Printf("Starting API server on port %s", port)
	r := s.SetupRouter()
	r.Run(fmt.Sprintf(":%s", port))
}
