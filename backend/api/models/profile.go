package models

type PoliticianProfileResponse struct {
	Politician  *Politician    `json:"politician"`
	Connections *GraphResponse `json:"connections"`
}

type ScandalProfileResponse struct {
	Scandal     *Scandal       `json:"scandal"`
	Connections *GraphResponse `json:"connections"`
}
