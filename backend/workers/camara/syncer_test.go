package camara

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncCurrentDeputies_MapsAndFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/deputados":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": []map[string]any{
					{"id": 1, "nome": "N1", "siglaPartido": "P1", "siglaUf": "SP", "urlFoto": "f1"},
					{"id": 2, "nome": "N2", "siglaPartido": "P2", "siglaUf": "RJ", "urlFoto": "f2"},
					{"id": 3, "nome": "N3", "siglaPartido": "P3", "siglaUf": "MG", "urlFoto": "f3"},
				},
			})
		case "/api/v2/deputados/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": map[string]any{
					"id":        1,
					"nomeCivil": "Nome Civil 1",
					"cpf":       "11111111111",
					"siglaUf":   "SP",
					"ultimoStatus": map[string]any{
						"siglaPartido":    "PX",
						"siglaUf":         "SP",
						"urlFoto":         "fx",
						"descricaoStatus": "Em exercício",
					},
				},
			})
		case "/api/v2/deputados/2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": map[string]any{
					"id":        2,
					"nomeCivil": "Nome Civil 2",
					"cpf":       "",
					"siglaUf":   "RJ",
					"ultimoStatus": map[string]any{
						"siglaPartido":    "P2",
						"siglaUf":         "RJ",
						"urlFoto":         "f2d",
						"descricaoStatus": "",
					},
				},
			})
		case "/api/v2/deputados/3":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": map[string]any{
					"id":        3,
					"nomeCivil": "Nome Civil 3",
					"cpf":       "33333333333",
					"siglaUf":   "MG",
					"ultimoStatus": map[string]any{
						"siglaPartido":    "P3",
						"siglaUf":         "MG",
						"urlFoto":         "f3d",
						"descricaoStatus": "Licenciado",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	res, err := SyncCurrentDeputies(context.Background(), SyncOptions{
		BaseURL:  server.URL + "/api/v2",
		Items:    100,
		MaxPages: 1,
	})
	if err != nil {
		t.Fatalf("SyncCurrentDeputies error: %v", err)
	}

	if len(res.Records) != 1 {
		t.Fatalf("expected 1 active record with cpf, got %d", len(res.Records))
	}
	rec := res.Records[0]
	if rec.CPF != "11111111111" || rec.RoleCurrent != "Deputado Federal" || !rec.Active {
		t.Fatalf("unexpected mapped record: %#v", rec)
	}
	if rec.PartyCurrent != "PX" || rec.Name != "Nome Civil 1" {
		t.Fatalf("expected detail fields precedence, got %#v", rec)
	}

	if res.Stats.PagesFetched != 1 || res.Stats.ListedDeputies != 3 || res.Stats.DetailFetched != 3 {
		t.Fatalf("unexpected stats counters: %#v", res.Stats)
	}
	if res.Stats.SkippedNoCPF != 1 || res.Stats.SkippedNotActive != 1 || res.Stats.ActiveConfirmed != 1 {
		t.Fatalf("unexpected skip/active stats: %#v", res.Stats)
	}
}
