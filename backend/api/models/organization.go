package models

type OrgType string

const (
	OrgTypeParty        OrgType = "party"
	OrgTypeCompany      OrgType = "company"
	OrgTypeShell        OrgType = "shell"
	OrgTypeNGO          OrgType = "ngo"
	OrgTypePublicAgency OrgType = "public_agency"
)

type Organization struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	CNPJ    string  `json:"cnpj"`
	Type    OrgType `json:"type"`
	LogoURL string  `json:"logo_url,omitempty"`
	Active  bool    `json:"active"`
}
