package api

import (
	"encoding/json"
	"testing"
)

func TestResolveTribunalEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		in      djenCaseCandidate
		want    string
		wantErr bool
	}{
		{
			name: "explicit endpoint wins",
			in:   djenCaseCandidate{TribunalEndpoint: "api_publica_trf4", TribunalSigla: "TRF4"},
			want: "api_publica_trf4",
		},
		{
			name: "derived from sigla when endpoint missing (older payload)",
			in:   djenCaseCandidate{TribunalSigla: "TRF4"},
			want: "api_publica_trf4",
		},
		{
			name: "sigla lowercased",
			in:   djenCaseCandidate{TribunalSigla: "STJ"},
			want: "api_publica_stj",
		},
		{
			name:    "both missing is an error",
			in:      djenCaseCandidate{},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveTribunalEndpoint(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (endpoint=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveTribunalEndpoint = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDJENCandidatePayloadParse(t *testing.T) {
	// Payload carries display-only fields alongside the registration keys; only
	// the registration subset must parse.
	payload := `{
	  "case_number": "50465129420168040001",
	  "tribunal_sigla": "TRF4",
	  "tribunal_endpoint": "api_publica_trf4",
	  "politician": "Fulano de Tal",
	  "nomeClasse": "Ação Penal",
	  "link": "https://example.test/comunicacao/1"
	}`
	var cand djenCaseCandidate
	if err := json.Unmarshal([]byte(payload), &cand); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if cand.CaseNumber != "50465129420168040001" {
		t.Fatalf("case_number = %q", cand.CaseNumber)
	}
	ep, err := resolveTribunalEndpoint(cand)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ep != "api_publica_trf4" {
		t.Fatalf("endpoint = %q", ep)
	}
}
