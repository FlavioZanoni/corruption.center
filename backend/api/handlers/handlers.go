package handlers

import (
	"corruption-center/api/services"
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

func NewHandlers(graph services.GraphService, search services.SearchService) *Handlers {
	return &Handlers{
		Health:     NewHealthHandler(),
		Graph:      NewGraphHandler(graph),
		Search:     NewSearchHandler(search),
		Timeline:   NewTimelineHandler(graph),
		Politician: NewPoliticianHandler(graph),
		Scandal:    NewScandalHandler(graph),
		Sanction:   NewSanctionHandler(graph),
		Entity:     NewEntityHandler(graph),
		Proceeding: NewProceedingHandler(graph),
	}
}
