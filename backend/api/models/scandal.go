package models

import "time"

type StatusType string

const (
	StatusTypeOngoing    StatusType = "ongoing"
	StatusTypeConcluded  StatusType = "concluded"
	StatusTypePrescribed StatusType = "prescribed"
)

type Scandal struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Aliases        []string   `json:"aliases"`
	Description    string     `json:"description"`
	DateStart      time.Time  `json:"date_start"`
	DateEnd        *time.Time `json:"date_end"`
	TotalAmountBRL float64    `json:"total_amount_brl"`
	Status         StatusType `json:"status"` // ongoing|concluded|prescribed
	WikipediaURL   string     `json:"wikipedia_url"`
}

// ScandalListItem is a lightweight row for the paginated browse endpoint. It
// carries just enough to render a card and an SEO sitemap entry; the full
// profile (and its graph) still comes from GET /scandal/{id}.
type ScandalListItem struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	DateStart       time.Time  `json:"date_start"`
	DateEnd         *time.Time `json:"date_end"`
	Status          StatusType `json:"status"`
	PoliticianCount int        `json:"politician_count"`
	ProceedingCount int        `json:"proceeding_count"`
}

// ScandalListResponse is the paginated envelope for GET /scandals.
type ScandalListResponse struct {
	Items    []ScandalListItem `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int               `json:"total"`
}
