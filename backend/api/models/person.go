package models

type Person struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	CPF                   string `json:"cpf"`
	ProvenanceSource      string `json:"provenance_source"`
	ProvenanceLink        string `json:"provenance_link"`
	ProvenanceTribunal    string `json:"provenance_tribunal"`
	ProvenanceComunicaoID string `json:"provenance_comunicacao_id"`
}
