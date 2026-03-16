package models

type Politician struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CPF           string   `json:"cpf"`
	NameAliases   []string `json:"name_aliases"`
	PartyCurrent  string   `json:"party_current"`
	RoleCurrent   string   `json:"role_current"`
	State         string   `json:"state"`
	TSEProfileURL string   `json:"tse_profile_url"`
	PhotoURL      string   `json:"photo_url"`
	Active        bool     `json:"active"`
}
