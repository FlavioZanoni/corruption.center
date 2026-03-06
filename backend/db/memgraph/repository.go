package memgraph

import (
	"context"
	"time"

	"corruption-center/api/models"
)

type Repository interface {
	// graph traversal
	QueryScandalGraph(ctx context.Context, id string) (*models.GraphResponse, error)
	QueryPoliticianGraph(ctx context.Context, id string) (*models.GraphResponse, error)
	QueryExpandNode(ctx context.Context, id string, hops int) (*models.GraphResponse, error)
	QueryTimeline(ctx context.Context, from time.Time, to time.Time) (*models.GraphResponse, error)

	// profile lookups
	QueryPolitician(ctx context.Context, id string) (*models.Politician, error)
	QueryScandal(ctx context.Context, id string) (*models.Scandal, error)

	// search
	QuerySearch(ctx context.Context, q string, nodeType string) (*models.GraphResponse, error)
}
