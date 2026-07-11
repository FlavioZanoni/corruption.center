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

	// browse
	QueryPoliticians(ctx context.Context, filter, party, uf, sort string, page, pageSize int) (*models.PoliticianListResponse, error)

	// backoffice writes
	UpsertLegalProceedingByCase(ctx context.Context, p DataJudProceedingUpsert) (string, error)
	EnsureInvestigatesEdge(ctx context.Context, proceedingID, scandalID string) error

	// backoffice: scandal selector, provenance display and Person purge
	ListScandals(ctx context.Context) ([]ScandalOption, error)
	UpsertScandal(ctx context.Context, id, name, dateStart string) error
	GetNodeProvenance(ctx context.Context, id string) (*NodeProvenance, error)
	PurgePersonNode(ctx context.Context, id string) (*NodeProvenance, error)

	// backoffice: human-confirmed edges written when a review is approved
	EnsurePoliticianDefendantEdge(ctx context.Context, politicianID, proceedingID string) error
	EnsurePoliticianControlsOrganization(ctx context.Context, politicianID, organizationID string) error
	EnsurePoliticianSanctionedInEdge(ctx context.Context, politicianID, sanctionID string) error
}
