package models

import "time"

type StatusType string

const (
	StatusTypeOngoing    StatusType = "ongoing"
	StatusTypeConcluded  StatusType = "concluded"
	StatusTypePrescribed StatusType = "prescribed"
)

type Scandal struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Aliases        []string   `json:"aliases"`
	Description    string     `json:"description"`
	DateStart      time.Time  `json:"date_start"`
	DateEnd        *time.Time `json:"date_end"`
	TotalAmountBRL float64    `json:"total_amount_brl"`
	Status         StatusType `json:"status"` // ongoing|concluded|prescribed
	WikipediaURL   string     `json:"wikipedia_url"`
}

