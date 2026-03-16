package models

type Person struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	CPF    string `json:"cpf"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}
