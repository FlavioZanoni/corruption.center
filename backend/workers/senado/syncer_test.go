package senado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncCurrentSenators_MapsAndFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ListaParlamentarEmExercicio": map[string]any{
				"Parlamentares": map[string]any{
					"Parlamentar": []map[string]any{
						{
							"IdentificacaoParlamentar": map[string]any{
								"CodigoParlamentar":       "1",
								"NomeParlamentar":         "Nome Curto",
								"NomeCompletoParlamentar": "Nome Completo",
								"SiglaPartidoParlamentar": "AAA",
								"UfParlamentar":           "SP",
								"UrlFotoParlamentar":      "foto1",
							},
							"Mandato": map[string]any{
								"DescricaoParticipacao": "Titular",
								"Exercicios":            map[string]any{"Exercicio": []map[string]any{{"CodigoExercicio": "x"}}},
							},
						},
						{
							"IdentificacaoParlamentar": map[string]any{
								"CodigoParlamentar":       "2",
								"NomeParlamentar":         "Inativo",
								"NomeCompletoParlamentar": "",
								"SiglaPartidoParlamentar": "BBB",
								"UfParlamentar":           "RJ",
							},
							"Mandato": map[string]any{
								"DescricaoParticipacao": "Suplente",
								"Exercicios":            map[string]any{"Exercicio": []map[string]any{{"CodigoExercicio": "y"}}},
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
		t.Fatalf("SyncCurrentSenators error: %v", err)
	}
	if res.Stats.ListedSenators != 2 || res.Stats.ActiveConfirmed != 1 || res.Stats.SkippedNotActive != 1 {
		t.Fatalf("unexpected stats: %#v", res.Stats)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 mapped senator, got %d", len(res.Records))
	}
	rec := res.Records[0]
	if rec.SenadoID != "1" || rec.Name != "Nome Completo" || rec.RoleCurrent != "Senador" || !rec.Active {
		t.Fatalf("unexpected mapped record: %#v", rec)
	}
}
