package models

import "time"

type ProceedingType string
type ProceedingStatus string

const (
	ProceedingTypeCriminal       ProceedingType = "criminal"
	ProceedingTypeAdministrative ProceedingType = "administrative"
	ProceedingTypeCPI            ProceedingType = "cpi"
)

const (
	ProceedingStatusOngoing   ProceedingStatus = "ongoing"
	ProceedingStatusConcluded ProceedingStatus = "concluded"
)

type LegalProceeding struct {
	ID            string           `json:"id"`
	CaseNumber    string           `json:"case_number"`
	Court         string           `json:"court"`
	Type          ProceedingType   `json:"type"`
	Status        ProceedingStatus `json:"status"`
	Assuntos      []string         `json:"assuntos"`
	DateFiled     time.Time        `json:"date_filed"`
	DateConcluded *time.Time       `json:"date_concluded"`
	URL           string           `json:"url"`
}

// ProceedingListItem is a lightweight row for the paginated browse endpoint,
// sized for sitemap enumeration and case cards.
type ProceedingListItem struct {
	ID            string           `json:"id"`
	CaseNumber    string           `json:"case_number"`
	Court         string           `json:"court"`
	Status        ProceedingStatus `json:"status"`
	Phase         string           `json:"phase"`
	HasConviction bool             `json:"has_conviction"`
	Type          ProceedingType   `json:"type"`
	Polled        bool             `json:"polled"` // true when has_conviction IS NOT NULL; false means we never polled DataJud
}

// ProceedingListResponse is the paginated envelope for GET /proceedings.
type ProceedingListResponse struct {
	Items    []ProceedingListItem `json:"items"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Total    int                  `json:"total"`
}

// ProceedingScandalRef is the scandal a proceeding INVESTIGATES, if any.
type ProceedingScandalRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProceedingDefendant is one party on the far end of a DEFENDANT_IN edge.
// Type is the node's primary graph label (Politician, Person or Organization):
// only Politician nodes are named public figures, so the frontend must treat
// Person defendants as anonymous. The edge's provenance (source, confidence and
// the signals behind it) rides along because a defendant link is never a bare
// fact: it is an assertion with a strength, per the review-queue contract.
// Confidence is a pointer so an unscored edge serializes as absent rather than
// as 0. A DJEN defendant edge carries no score at all (DJEN names the party
// outright, so no identity was inferred), and a literal "confidence": 0 reads as
// "we are 0% sure" to any consumer: the exact opposite of the truth.
type ProceedingDefendant struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	Outcome           string         `json:"outcome"`
	Source            string         `json:"source"`
	Confidence        *float64       `json:"confidence,omitempty"`
	ConfidenceSignals []string       `json:"confidence_signals,omitempty"`
	Properties        map[string]any `json:"properties"`
}

// ProceedingDetailResponse is the envelope for GET /proceeding/{id}.
type ProceedingDetailResponse struct {
	ProceedingListItem
	Assuntos   []string              `json:"assuntos"`
	URL        string                `json:"url"`
	Scandal    *ProceedingScandalRef `json:"scandal"`
	Defendants []ProceedingDefendant `json:"defendants"`
}
