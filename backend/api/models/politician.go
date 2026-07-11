package models

type Politician struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	CPF              string   `json:"cpf"`
	NameAliases      []string `json:"name_aliases"`
	PartyCurrent     string   `json:"party_current"`
	RoleCurrent      string   `json:"role_current"`
	State            string   `json:"state"`
	TSEProfileURLs   []string `json:"tse_profile_urls"`
	PhotoURL         string   `json:"photo_url"`
	PhotoSource      string   `json:"photo_source,omitempty"`
	PhotoAttribution string   `json:"photo_attribution,omitempty"`
	Active           bool     `json:"active"`
}

// PoliticianListItem is a lightweight row for the paginated browse endpoint.
// It intentionally omits heavy graph connections so a fresh install (only
// politicians, no scandals) still has content to explore.
type PoliticianListItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	PartyCurrent     string `json:"party_current"`
	RoleCurrent      string `json:"role_current"`
	State            string `json:"state"`
	PhotoURL         string `json:"photo_url,omitempty"`
	PhotoAttribution string `json:"photo_attribution,omitempty"`
	SanctionCount    int    `json:"sanction_count"`
	ProceedingCount  int    `json:"proceeding_count"`
}

// PoliticianListResponse is the paginated envelope for GET /politicians.
type PoliticianListResponse struct {
	Items    []PoliticianListItem `json:"items"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Total    int                  `json:"total"`
}
