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
}

func NewHandlers(graph services.GraphService, search services.SearchService) *Handlers {
	return &Handlers{
		Health:     NewHealthHandler(),
		Graph:      NewGraphHandler(graph),
		Search:     NewSearchHandler(search),
		Timeline:   NewTimelineHandler(graph),
		Politician: NewPoliticianHandler(graph),
		Scandal:    NewScandalHandler(graph),
	}
}
