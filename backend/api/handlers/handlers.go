package handlers

import (
	"corruption-center/db/memgraph"
)

type Handlers struct {
	Health     *HealthHandler
	Graph      *GraphHandler
	Search     *SearchHandler
	Timeline   *TimelineHandler
	Politician *PoliticianHandler
	Scandal    *ScandalHandler
	Sanction   *SanctionHandler
	Entity     *EntityHandler
	Proceeding *ProceedingHandler
}

func NewHandlers(repo memgraph.Repository) *Handlers {
	return &Handlers{
		Health:     NewHealthHandler(),
		Graph:      NewGraphHandler(repo),
		Search:     NewSearchHandler(repo),
		Timeline:   NewTimelineHandler(repo),
		Politician: NewPoliticianHandler(repo),
		Scandal:    NewScandalHandler(repo),
		Sanction:   NewSanctionHandler(repo),
		Entity:     NewEntityHandler(repo),
		Proceeding: NewProceedingHandler(repo),
	}
}
