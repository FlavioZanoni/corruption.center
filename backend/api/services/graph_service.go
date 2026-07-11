package services

import (
	"context"
	"time"

	"corruption-center/api/models"
	"corruption-center/db/memgraph"
)

type GraphService interface {
	GetScandalGraph(ctx context.Context, id string) (*models.GraphResponse, error)
	GetPoliticianGraph(ctx context.Context, id string) (*models.GraphResponse, error)
	ExpandNode(ctx context.Context, id string, hops int) (*models.GraphResponse, error)
	GetTimeline(ctx context.Context, from time.Time, to time.Time) (*models.GraphResponse, error)
	GetPolitician(ctx context.Context, id string) (*models.Politician, error)
	GetScandal(ctx context.Context, id string) (*models.Scandal, error)
	ListPoliticians(ctx context.Context, filter, party, uf, sort string, page, pageSize int) (*models.PoliticianListResponse, error)
	ListScandals(ctx context.Context, sort string, page, pageSize int) (*models.ScandalListResponse, error)
	ListProceedings(ctx context.Context, page, pageSize int) (*models.ProceedingListResponse, error)
	GetProceeding(ctx context.Context, id string) (*models.ProceedingDetailResponse, error)
}

type graphService struct {
	memgraph memgraph.Repository
}

func NewGraphService(memgraph memgraph.Repository) GraphService {
	return &graphService{memgraph: memgraph}
}

func (s *graphService) GetScandalGraph(ctx context.Context, id string) (*models.GraphResponse, error) {
	return s.memgraph.QueryScandalGraph(ctx, id)
}

func (s *graphService) GetPoliticianGraph(ctx context.Context, id string) (*models.GraphResponse, error) {
	return s.memgraph.QueryPoliticianGraph(ctx, id)
}

func (s *graphService) ExpandNode(ctx context.Context, id string, hops int) (*models.GraphResponse, error) {
	return s.memgraph.QueryExpandNode(ctx, id, hops)
}

func (s *graphService) GetTimeline(ctx context.Context, from time.Time, to time.Time) (*models.GraphResponse, error) {
	return s.memgraph.QueryTimeline(ctx, from, to)
}

func (s *graphService) GetPolitician(ctx context.Context, id string) (*models.Politician, error) {
	return s.memgraph.QueryPolitician(ctx, id)
}

func (s *graphService) GetScandal(ctx context.Context, id string) (*models.Scandal, error) {
	return s.memgraph.QueryScandal(ctx, id)
}

func (s *graphService) ListPoliticians(ctx context.Context, filter, party, uf, sort string, page, pageSize int) (*models.PoliticianListResponse, error) {
	return s.memgraph.QueryPoliticians(ctx, filter, party, uf, sort, page, pageSize)
}

func (s *graphService) ListScandals(ctx context.Context, sort string, page, pageSize int) (*models.ScandalListResponse, error) {
	return s.memgraph.QueryScandals(ctx, sort, page, pageSize)
}

func (s *graphService) ListProceedings(ctx context.Context, page, pageSize int) (*models.ProceedingListResponse, error) {
	return s.memgraph.QueryProceedings(ctx, page, pageSize)
}

func (s *graphService) GetProceeding(ctx context.Context, id string) (*models.ProceedingDetailResponse, error) {
	return s.memgraph.QueryProceeding(ctx, id)
}
