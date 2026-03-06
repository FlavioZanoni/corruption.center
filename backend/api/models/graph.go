package models

type NodeType string

const (
	NodeTypePolitician      NodeType = "politician"
	NodeTypeScandal         NodeType = "scandal"
	NodeTypeOrganization    NodeType = "organization"
	NodeTypeLegalProceeding NodeType = "legal_proceeding"
)

type EdgeType string

const (
	EdgeTypeInvolvedIn   EdgeType = "INVOLVED_IN"
	EdgeTypeDefendantIn  EdgeType = "DEFENDANT_IN"
	EdgeTypeMemberOf     EdgeType = "MEMBER_OF"
	EdgeTypeImplicatedIn EdgeType = "IMPLICATED_IN"
	EdgeTypeInvestigates EdgeType = "INVESTIGATES"
	EdgeTypeRelatedTo    EdgeType = "RELATED_TO"
	EdgeTypeSupports     EdgeType = "SUPPORTS"
)

// Node is the generic graph node returned to the frontend.
// Properties holds the full domain model (Politician, Scandal, etc.)
// serialized as a map so Sigma.js can consume it directly.
type Node struct {
	ID         string         `json:"id"`
	Type       NodeType       `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
}

// Edge is the generic graph edge returned to the frontend.
type Edge struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Type       EdgeType       `json:"type"`
	Properties map[string]any `json:"properties"`
}

// GraphResponse is the envelope for all graph endpoints.
type GraphResponse struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
