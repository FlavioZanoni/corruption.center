package services

import (
	"context"

	"corruption-center/api/models"
	"corruption-center/db/memgraph"
)

type SearchService interface {
	Search(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error)
}

type searchService struct {
	memgraph memgraph.Repository
}

func NewSearchService(memgraph memgraph.Repository) SearchService {
	return &searchService{memgraph: memgraph}
}

func (s *searchService) Search(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error) {
	return s.memgraph.QuerySearch(ctx, q, nodeType)
}
