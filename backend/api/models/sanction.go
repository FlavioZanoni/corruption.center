package models

import "time"

// Sanction represents a sanction node from the graph.
type Sanction struct {
	ID         string     `json:"id"`
	Registry   string     `json:"registry"`
	Type       string     `json:"sanction_type"`
	Organ      string     `json:"organ"`
	DateStart  *time.Time `json:"date_start"`
	DateEnd    *time.Time `json:"date_end"`
	ProcessRef string     `json:"process_ref"`
	SourceURL  string     `json:"source_url"`
}

// SanctionListItem is a lightweight row for the paginated browse endpoint.
// It includes the sanctioned party information.
type SanctionListItem struct {
	ID             string     `json:"id"`
	Registry       string     `json:"registry"`
	Type           string     `json:"sanction_type"`
	Organ          string     `json:"organ"`
	DateStart      *time.Time `json:"date_start"`
	DateEnd        *time.Time `json:"date_end"`
	ProcessRef     string     `json:"process_ref"`
	SourceURL      string     `json:"source_url"`
	SanctionedID   string     `json:"sanctioned_id"`
	SanctionedName string     `json:"sanctioned_name"`
	// The CNPJ/CPF. For a sanctioned COMPANY this is very often the ONLY identity we
	// hold: CGU and TCU publish the document, not the razão social, and until the
	// CNPJ enricher runs, the name is empty. A row rendered from the name alone would
	// be a sanction against nobody.
	SanctionedDocument string `json:"sanctioned_document"`
	SanctionedType     string `json:"sanctioned_type"` // "Person", "Organization", "Politician"
}

// SanctionListResponse is the paginated envelope for GET /sanctions.
type SanctionListResponse struct {
	Items    []SanctionListItem `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
}

// SanctionDetailResponse is the envelope for GET /sanction/{id}.
// It includes the sanction itself and all parties sanctioned in it.
type SanctionDetailResponse struct {
	Sanction *Sanction         `json:"sanction"`
	Parties  []SanctionedParty `json:"parties"`
}

// SanctionedParty represents a party (Person, Organization, or Politician)
// that was sanctioned in a given sanction.
type SanctionedParty struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "Person", "Organization", "Politician"
}

// SanctionRegistryItem represents a registry name and its count.
type SanctionRegistryItem struct {
	Registry string `json:"registry"`
	Count    int    `json:"count"`
}

// SanctionRegistriesResponse is the envelope for GET /sanction-registries.
type SanctionRegistriesResponse struct {
	Registries []SanctionRegistryItem `json:"registries"`
}
