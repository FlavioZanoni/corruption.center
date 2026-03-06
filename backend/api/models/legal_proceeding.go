package models

import "time"

type ProceedingType string
type ProceedingStatus string

const (
	ProceedingTypeCriminal       ProceedingType = "criminal"
	ProceedingTypeAdministrative ProceedingType = "administrative"
	ProceedingTypeCPI            ProceedingType = "cpi"
)

const (
	ProceedingStatusOngoing   ProceedingStatus = "ongoing"
	ProceedingStatusConcluded ProceedingStatus = "concluded"
)

type LegalProceeding struct {
	ID            string           `json:"id"`
	CaseNumber    string           `json:"case_number"`
	Court         string           `json:"court"`
	Type          ProceedingType   `json:"type"`
	Status        ProceedingStatus `json:"status"`
	DateFiled     time.Time        `json:"date_filed"`
	DateConcluded *time.Time       `json:"date_concluded"`
	URL           string           `json:"url"`
}
