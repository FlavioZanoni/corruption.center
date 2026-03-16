package models

type Organization struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	CNPJ            string  `json:"cnpj"`
	UF              string  `json:"uf"`
	ShareCapitalBRL float64 `json:"share_capital_brl"`
	MainActivity    string  `json:"main_activity"`
	Type            string  `json:"type"`
	ImageURL        string  `json:"image_url,omitempty"`
	Active          bool    `json:"active"`
}
