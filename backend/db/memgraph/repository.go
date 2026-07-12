package memgraph

import (
	"context"
	"time"

	"corruption-center/api/models"
)

type Repository interface {
	// per-defendant judicial outcomes (backoffice, human-entered only)
	ListProceedingsForOutcome(ctx context.Context) ([]ProceedingSummary, error)
	ListDefendants(ctx context.Context, proceedingID string) ([]Defendant, error)
	SetDefendantOutcome(ctx context.Context, proceedingID, partyID, outcome, evidenceURL, actor string) error

	// AttachOrganizationCNPJ resolves an unknown_cnpj review: it gives a name-only
	// DJEN company the document it never carried, merging it into the sanctioned
	// node if one already holds that CNPJ.
	AttachOrganizationCNPJ(ctx context.Context, srcID, cnpj, actor string) (string, error)

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
	QueryScandals(ctx context.Context, sort string, page, pageSize int) (*models.ScandalListResponse, error)
	QueryProceedings(ctx context.Context, page, pageSize int) (*models.ProceedingListResponse, error)
	QueryProceeding(ctx context.Context, id string) (*models.ProceedingDetailResponse, error)

	// backoffice writes
	UpsertLegalProceedingByCase(ctx context.Context, p DataJudProceedingUpsert) (string, error)
	EnsureInvestigatesEdge(ctx context.Context, proceedingID, scandalID string) error

	// backoffice: scandal selector, provenance display and Person purge
	ListScandals(ctx context.Context) ([]ScandalOption, error)
	UpsertScandal(ctx context.Context, id, name, dateStart string) error
	UpsertScandalSeed(ctx context.Context, s ScandalSeed) error
	GetNodeProvenance(ctx context.Context, id string) (*NodeProvenance, error)
	PurgePersonNode(ctx context.Context, id string) (*NodeProvenance, error)

	// backoffice: human-confirmed edges written when a review is approved
	EnsurePoliticianDefendantEdge(ctx context.Context, politicianID, proceedingID string) error
	EnsurePoliticianControlsOrganization(ctx context.Context, politicianID, organizationID string) error
	EnsurePoliticianSanctionedInEdge(ctx context.Context, politicianID, sanctionID string) error
}
