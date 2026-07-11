package senado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseParlamentar_Shapes covers the three payload shapes the Senado API
// returns for the Parlamentar node: array, single object, and null/empty.
func TestParseParlamentar_Shapes(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		raw := json.RawMessage(`[{"IdentificacaoParlamentar":{"CodigoParlamentar":"1"}},{"IdentificacaoParlamentar":{"CodigoParlamentar":"2"}}]`)
		got, err := parseParlamentar(raw)
		if err != nil || len(got) != 2 {
			t.Fatalf("array parse: got %d err %v", len(got), err)
		}
	})
	t.Run("single object", func(t *testing.T) {
		raw := json.RawMessage(`{"IdentificacaoParlamentar":{"CodigoParlamentar":"1"}}`)
		got, err := parseParlamentar(raw)
		if err != nil || len(got) != 1 {
			t.Fatalf("single parse: got %d err %v", len(got), err)
		}
		if got[0].IdentificacaoParlamentar.CodigoParlamentar != "1" {
			t.Fatalf("unexpected single payload: %+v", got[0])
		}
	})
	t.Run("null", func(t *testing.T) {
		got, err := parseParlamentar(json.RawMessage(`null`))
		if err != nil || len(got) != 0 {
			t.Fatalf("null parse: got %d err %v", len(got), err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		got, err := parseParlamentar(json.RawMessage(``))
		if err != nil || len(got) != 0 {
			t.Fatalf("empty parse: got %d err %v", len(got), err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, err := parseParlamentar(json.RawMessage(`123`)); err == nil {
			t.Fatalf("expected error for invalid Parlamentar payload")
		}
	})
}

func TestIsActive(t *testing.T) {
	mk := func(participacao, exercicio string) senatorPayload {
		var s senatorPayload
		s.Mandato.DescricaoParticipacao = participacao
		s.Mandato.Exercicios.Exercicio = json.RawMessage(exercicio)
		return s
	}
	cases := []struct {
		name string
		s    senatorPayload
		want bool
	}{
		{"titular with object exercicio", mk("Titular", `{"CodigoExercicio":"x"}`), true},
		{"titular with array exercicio", mk("Titular", `[{"CodigoExercicio":"x"}]`), true},
		{"titular with empty array", mk("Titular", `[]`), false},
		{"titular with null exercicio", mk("Titular", `null`), false},
		{"titular with empty exercicio", mk("Titular", ``), false},
		{"suplente is inactive", mk("Suplente", `{"CodigoExercicio":"x"}`), false},
		{"case-insensitive titular", mk("titular", `{"CodigoExercicio":"x"}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isActive(tc.s); got != tc.want {
				t.Fatalf("isActive = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSyncCurrentSenators_SingleObject verifies the whole sync path when the API
// returns a single Parlamentar object (not an array).
func TestSyncCurrentSenators_SingleObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ListaParlamentarEmExercicio": map[string]any{
				"Parlamentares": map[string]any{
					"Parlamentar": map[string]any{
						"IdentificacaoParlamentar": map[string]any{
							"CodigoParlamentar":       "42",
							"NomeParlamentar":         "Curto",
							"NomeCompletoParlamentar": "Nome Completo",
							"SiglaPartidoParlamentar": "AAA",
							"UfParlamentar":           "SP",
							"UrlFotoParlamentar":      "foto",
						},
						"Mandato": map[string]any{
							"DescricaoParticipacao": "Titular",
							"Exercicios":            map[string]any{"Exercicio": map[string]any{"CodigoExercicio": "1"}},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentSenators(context.Background(), SyncOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("SyncCurrentSenators: %v", err)
	}
	if len(res.Records) != 1 || res.Records[0].SenadoID != "42" {
		t.Fatalf("expected 1 senator id 42, got %+v", res.Records)
	}
}

// TestSyncCurrentSenators_SkippedInvalid: a Titular in exercise but missing a
// required identity field (party) is counted as invalid, not mapped.
func TestSyncCurrentSenators_SkippedInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ListaParlamentarEmExercicio": map[string]any{
				"Parlamentares": map[string]any{
					"Parlamentar": []map[string]any{
						{
							"IdentificacaoParlamentar": map[string]any{
								"CodigoParlamentar":       "1",
								"NomeParlamentar":         "Sem Partido",
								"NomeCompletoParlamentar": "Sem Partido Completo",
								"SiglaPartidoParlamentar": "", // missing → invalid
								"UfParlamentar":           "SP",
							},
							"Mandato": map[string]any{
								"DescricaoParticipacao": "Titular",
								"Exercicios":            map[string]any{"Exercicio": []map[string]any{{"CodigoExercicio": "x"}}},
							},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentSenators(context.Background(), SyncOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("SyncCurrentSenators: %v", err)
	}
	if res.Stats.SkippedInvalid != 1 || res.Stats.ActiveConfirmed != 0 {
		t.Fatalf("expected 1 invalid / 0 active, got %#v", res.Stats)
	}
	if len(res.Records) != 0 {
		t.Fatalf("expected no records, got %d", len(res.Records))
	}
}

func TestSyncCurrentSenators_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	if _, err := SyncCurrentSenators(context.Background(), SyncOptions{URL: server.URL}); err == nil {
		t.Fatalf("expected non-200 status error")
	}
}

func TestSyncCurrentSenators_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	if _, err := SyncCurrentSenators(context.Background(), SyncOptions{URL: server.URL}); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestSyncCurrentSenators_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ListaParlamentarEmExercicio": map[string]any{
				"Parlamentares": map[string]any{"Parlamentar": nil},
			},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentSenators(context.Background(), SyncOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("SyncCurrentSenators: %v", err)
	}
	if res.Stats.ListedSenators != 0 || len(res.Records) != 0 {
		t.Fatalf("expected empty result, got %#v", res.Stats)
	}
}
