package senado

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultURL = "https://legis.senado.leg.br/dadosabertos/senador/lista/atual.json"

type SenatorRecord struct {
	SenadoID     string
	Name         string
	PartyCurrent string
	RoleCurrent  string
	State        string
	PhotoURL     string
	Active       bool
}

type SyncStats struct {
	ListedSenators   int
	ActiveConfirmed  int
	SkippedNotActive int
	SkippedInvalid   int
}

type SyncResult struct {
	Records []SenatorRecord
	Stats   SyncStats
}

type SyncOptions struct {
	URL        string
	HTTPClient *http.Client
}

type responseEnvelope struct {
	ListaParlamentarEmExercicio struct {
		Parlamentares struct {
			Parlamentar json.RawMessage `json:"Parlamentar"`
		} `json:"Parlamentares"`
	} `json:"ListaParlamentarEmExercicio"`
}

type senatorPayload struct {
	IdentificacaoParlamentar struct {
		CodigoParlamentar       string `json:"CodigoParlamentar"`
		NomeParlamentar         string `json:"NomeParlamentar"`
		NomeCompletoParlamentar string `json:"NomeCompletoParlamentar"`
		SiglaPartidoParlamentar string `json:"SiglaPartidoParlamentar"`
		UfParlamentar           string `json:"UfParlamentar"`
		UrlFotoParlamentar      string `json:"UrlFotoParlamentar"`
	} `json:"IdentificacaoParlamentar"`
	Mandato struct {
		DescricaoParticipacao string `json:"DescricaoParticipacao"`
		Exercicios            struct {
			Exercicio json.RawMessage `json:"Exercicio"`
		} `json:"Exercicios"`
	} `json:"Mandato"`
}

func SyncCurrentSenators(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	u := strings.TrimSpace(opts.URL)
	if u == "" {
		u = defaultURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("senado: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("senado: request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("senado: status %d", res.StatusCode)
	}

	var env responseEnvelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("senado: decode response: %w", err)
	}

	entries, err := parseParlamentar(env.ListaParlamentarEmExercicio.Parlamentares.Parlamentar)
	if err != nil {
		return nil, err
	}

	out := &SyncResult{Records: make([]SenatorRecord, 0, len(entries))}
	out.Stats.ListedSenators = len(entries)

	for _, s := range entries {
		if !isActive(s) {
			out.Stats.SkippedNotActive++
			continue
		}
		name := strings.TrimSpace(s.IdentificacaoParlamentar.NomeCompletoParlamentar)
		if name == "" {
			name = strings.TrimSpace(s.IdentificacaoParlamentar.NomeParlamentar)
		}
		state := strings.TrimSpace(s.IdentificacaoParlamentar.UfParlamentar)
		party := strings.TrimSpace(s.IdentificacaoParlamentar.SiglaPartidoParlamentar)
		sid := strings.TrimSpace(s.IdentificacaoParlamentar.CodigoParlamentar)
		if sid == "" || name == "" || state == "" || party == "" {
			out.Stats.SkippedInvalid++
			continue
		}

		out.Records = append(out.Records, SenatorRecord{
			SenadoID:     sid,
			Name:         name,
			PartyCurrent: party,
			RoleCurrent:  "Senador",
			State:        state,
			PhotoURL:     strings.TrimSpace(s.IdentificacaoParlamentar.UrlFotoParlamentar),
			Active:       true,
		})
		out.Stats.ActiveConfirmed++
	}

	return out, nil
}

func parseParlamentar(raw json.RawMessage) ([]senatorPayload, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return []senatorPayload{}, nil
	}
	var arr []senatorPayload
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var single senatorPayload
	if err := json.Unmarshal(raw, &single); err == nil {
		return []senatorPayload{single}, nil
	}
	return nil, fmt.Errorf("senado: invalid Parlamentar payload")
}

func isActive(s senatorPayload) bool {
	if !strings.EqualFold(strings.TrimSpace(s.Mandato.DescricaoParticipacao), "Titular") {
		return false
	}
	raw := strings.TrimSpace(string(s.Mandato.Exercicios.Exercicio))
	if raw == "" || raw == "null" {
		return false
	}
	if strings.HasPrefix(raw, "[") {
		var arr []json.RawMessage
		_ = json.Unmarshal(s.Mandato.Exercicios.Exercicio, &arr)
		return len(arr) > 0
	}
	return true
}
