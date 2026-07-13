package models

type Person struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	CPF                   string `json:"cpf"`
	ProvenanceSource      string `json:"provenance_source"`
	ProvenanceLink        string `json:"provenance_link"`
	ProvenanceTribunal    string `json:"provenance_tribunal"`
	ProvenanceComunicaoID string `json:"provenance_comunicacao_id"`

	// Ambiguous marks a name-only node: a Person with no full CPF is keyed by
	// its normalized name (DJEN defendants, QSA board members carry names but
	// never a document), so two different real people with the same name collapse
	// into ONE node accumulating both their records. The public profile must not
	// present those records as one person's history — see the /pessoa banner.
	Ambiguous bool `json:"ambiguous"`
}
