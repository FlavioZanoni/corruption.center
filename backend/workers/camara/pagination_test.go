package camara

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestSyncCurrentDeputies_Pagination verifies the syncer follows pages until a
// short/empty page and aggregates records across pages.
func TestSyncCurrentDeputies_Pagination(t *testing.T) {
	// items=2 per page. Page 1 returns 2 full deputies, page 2 returns 1 (short
	// page → loop stops after processing it).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/deputados" {
			page := r.URL.Query().Get("pagina")
			switch page {
			case "1":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"dados": []map[string]any{
						{"id": 1, "nome": "N1"},
						{"id": 2, "nome": "N2"},
					},
				})
			case "2":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"dados": []map[string]any{
						{"id": 3, "nome": "N3"},
					},
				})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"dados": []map[string]any{}})
			}
			return
		}
		// Detail: every deputy is active with a CPF.
		var id string
		fmt.Sscanf(r.URL.Path, "/api/v2/deputados/%s", &id)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dados": map[string]any{
				"id":        idToInt(id),
				"nomeCivil": "Civil " + id,
				"cpf":       id + "0000000000",
				"siglaUf":   "SP",
				"ultimoStatus": map[string]any{
					"siglaPartido":    "PX",
					"descricaoStatus": "Exercício",
				},
			},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentDeputies(context.Background(), SyncOptions{
		BaseURL: server.URL + "/api/v2",
		Items:   2,
	})
	if err != nil {
		t.Fatalf("SyncCurrentDeputies: %v", err)
	}
	if res.Stats.PagesFetched != 2 {
		t.Fatalf("PagesFetched = %d, want 2", res.Stats.PagesFetched)
	}
	if res.Stats.ListedDeputies != 3 || len(res.Records) != 3 {
		t.Fatalf("expected 3 listed/records, got listed=%d records=%d", res.Stats.ListedDeputies, len(res.Records))
	}
}

// TestSyncCurrentDeputies_MaxPages caps page fetching even when full pages keep
// coming.
func TestSyncCurrentDeputies_MaxPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/deputados" {
			// Always return exactly `items` full entries so the short-page stop
			// never triggers; only MaxPages bounds the loop.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": []map[string]any{
					{"id": 1, "nome": "N1"},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dados": map[string]any{"id": 1, "cpf": "11111111111", "ultimoStatus": map[string]any{"descricaoStatus": "Exercício"}},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentDeputies(context.Background(), SyncOptions{
		BaseURL:  server.URL + "/api/v2",
		Items:    1,
		MaxPages: 3,
	})
	if err != nil {
		t.Fatalf("SyncCurrentDeputies: %v", err)
	}
	if res.Stats.PagesFetched != 3 {
		t.Fatalf("PagesFetched = %d, want 3 (MaxPages cap)", res.Stats.PagesFetched)
	}
}

func TestSyncCurrentDeputies_ListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := SyncCurrentDeputies(context.Background(), SyncOptions{BaseURL: server.URL + "/api/v2", MaxPages: 1})
	if err == nil {
		t.Fatalf("expected list status error")
	}
}

func TestSyncCurrentDeputies_DetailError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/deputados" {
			_ = json.NewEncoder(w).Encode(map[string]any{"dados": []map[string]any{{"id": 1, "nome": "N1"}}})
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := SyncCurrentDeputies(context.Background(), SyncOptions{BaseURL: server.URL + "/api/v2", MaxPages: 1})
	if err == nil {
		t.Fatalf("expected detail status error")
	}
}

// TestSyncCurrentDeputies_FieldFallback exercises the list-value fallbacks when
// the detail payload leaves party/state/photo empty.
func TestSyncCurrentDeputies_FieldFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/deputados" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dados": []map[string]any{
					{"id": 1, "nome": "List Name", "siglaPartido": "LP", "siglaUf": "BA", "urlFoto": "listfoto"},
				},
			})
			return
		}
		// Detail leaves nomeCivil/party/state/photo empty → fall back to list values.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dados": map[string]any{
				"id":           1,
				"nomeCivil":    "",
				"cpf":          "99999999999",
				"ultimoStatus": map[string]any{"descricaoStatus": "Exercício"},
			},
		})
	}))
	defer server.Close()

	res, err := SyncCurrentDeputies(context.Background(), SyncOptions{BaseURL: server.URL + "/api/v2", MaxPages: 1})
	if err != nil {
		t.Fatalf("SyncCurrentDeputies: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(res.Records))
	}
	rec := res.Records[0]
	if rec.Name != "List Name" || rec.PartyCurrent != "LP" || rec.State != "BA" || rec.PhotoURL != "listfoto" {
		t.Fatalf("expected list-value fallbacks, got %#v", rec)
	}
}

func TestIsActiveDetail(t *testing.T) {
	if isActiveDetail(nil) {
		t.Fatalf("nil detail must be inactive")
	}
	// Empty status is treated as active (list endpoint already scopes to current).
	empty := &detailResponse{}
	if !isActiveDetail(empty) {
		t.Fatalf("empty status should be active")
	}
	active := &detailResponse{}
	active.Dados.UltimoStatus.DescricaoStatus = "Em Exercício"
	if !isActiveDetail(active) {
		t.Fatalf("exercício status should be active")
	}
	inactive := &detailResponse{}
	inactive.Dados.UltimoStatus.DescricaoStatus = "Licenciado"
	if isActiveDetail(inactive) {
		t.Fatalf("licenciado status should be inactive")
	}
}

func idToInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
