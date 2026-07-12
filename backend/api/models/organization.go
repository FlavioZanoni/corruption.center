package models

type Organization struct {
	ID               string  `json:"id"`
	CNPJ             string  `json:"cnpj"`
	Name             string  `json:"name"`
	Active           bool    `json:"active"`
	Type             string  `json:"type"`
	UF               string  `json:"uf"`
	ShareCapitalBRL  float64 `json:"share_capital_brl"`
	MainActivity     string  `json:"main_activity"`
	SourceURL        string  `json:"source_url"`
	Enriched         bool    `json:"enriched"`
	PhotoURL         string  `json:"photo_url"`
	PhotoAttribution string  `json:"photo_attribution"`
}
