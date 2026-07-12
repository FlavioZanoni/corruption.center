package models

// PersonProfileResponse is the envelope for GET /person/{id}.
type PersonProfileResponse struct {
	Person      *Person        `json:"person"`
	Connections *GraphResponse `json:"connections"`
}

// OrganizationProfileResponse is the envelope for GET /organization/{id}.
type OrganizationProfileResponse struct {
	Organization *Organization  `json:"organization"`
	Connections  *GraphResponse `json:"connections"`
}
