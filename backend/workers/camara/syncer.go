package camara

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://dadosabertos.camara.leg.br/api/v2"

type DeputyRecord struct {
	CamaraID     int
	CPF          string
	Name         string
	PartyCurrent string
	RoleCurrent  string
	State        string
	PhotoURL     string
	Active       bool
}

type SyncStats struct {
	PagesFetched     int
	ListedDeputies   int
	DetailFetched    int
	ActiveConfirmed  int
	SkippedNoCPF     int
	SkippedNotActive int
}

type SyncResult struct {
	Records []DeputyRecord
	Stats   SyncStats
}

type SyncOptions struct {
	BaseURL    string
	Items      int
	MaxPages   int
	HTTPClient *http.Client
}

type listResponse struct {
	Dados []struct {
		ID           int    `json:"id"`
		Nome         string `json:"nome"`
		SiglaPartido string `json:"siglaPartido"`
		SiglaUF      string `json:"siglaUf"`
		URLFoto      string `json:"urlFoto"`
	} `json:"dados"`
}

type detailResponse struct {
	Dados struct {
		ID           int    `json:"id"`
		NomeCivil    string `json:"nomeCivil"`
		CPF          string `json:"cpf"`
		SiglaUF      string `json:"siglaUf"`
		UltimoStatus struct {
			SiglaPartido    string `json:"siglaPartido"`
			SiglaUF         string `json:"siglaUf"`
			URLFoto         string `json:"urlFoto"`
			DescricaoStatus string `json:"descricaoStatus"`
		} `json:"ultimoStatus"`
	} `json:"dados"`
}

func SyncCurrentDeputies(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	items := opts.Items
	if items <= 0 || items > 100 {
		items = 100
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	result := &SyncResult{Records: make([]DeputyRecord, 0, items)}

	for page := 1; ; page++ {
		if opts.MaxPages > 0 && page > opts.MaxPages {
			break
		}

		listed, err := fetchDeputiesPage(ctx, client, baseURL, page, items)
		if err != nil {
			return nil, err
		}
		if len(listed) == 0 {
			break
		}

		result.Stats.PagesFetched++
		result.Stats.ListedDeputies += len(listed)

		for _, dep := range listed {
			detail, err := fetchDeputyDetail(ctx, client, baseURL, dep.ID)
			if err != nil {
				return nil, err
			}
			result.Stats.DetailFetched++

			if !isEmExercicio(detail.Dados.UltimoStatus.DescricaoStatus) {
				result.Stats.SkippedNotActive++
				continue
			}

			cpf := strings.TrimSpace(detail.Dados.CPF)
			if cpf == "" {
				result.Stats.SkippedNoCPF++
				continue
			}

			name := strings.TrimSpace(detail.Dados.NomeCivil)
			if name == "" {
				name = strings.TrimSpace(dep.Nome)
			}

			party := strings.TrimSpace(detail.Dados.UltimoStatus.SiglaPartido)
			if party == "" {
				party = strings.TrimSpace(dep.SiglaPartido)
			}

			state := strings.TrimSpace(detail.Dados.UltimoStatus.SiglaUF)
			if state == "" {
				state = strings.TrimSpace(detail.Dados.SiglaUF)
			}
			if state == "" {
				state = strings.TrimSpace(dep.SiglaUF)
			}

			photo := strings.TrimSpace(detail.Dados.UltimoStatus.URLFoto)
			if photo == "" {
				photo = strings.TrimSpace(dep.URLFoto)
			}

			result.Records = append(result.Records, DeputyRecord{
				CamaraID:     dep.ID,
				CPF:          cpf,
				Name:         name,
				PartyCurrent: party,
				RoleCurrent:  "Deputado Federal",
				State:        state,
				PhotoURL:     photo,
				Active:       true,
			})
			result.Stats.ActiveConfirmed++
		}

		if len(listed) < items {
			break
		}
	}

	return result, nil
}

func fetchDeputiesPage(ctx context.Context, client *http.Client, baseURL string, page int, items int) ([]struct {
	ID           int    `json:"id"`
	Nome         string `json:"nome"`
	SiglaPartido string `json:"siglaPartido"`
	SiglaUF      string `json:"siglaUf"`
	URLFoto      string `json:"urlFoto"`
}, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/deputados")
	if err != nil {
		return nil, fmt.Errorf("camara: parse list url: %w", err)
	}
	q := u.Query()
	q.Set("itens", strconv.Itoa(items))
	q.Set("pagina", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("camara: build list request: %w", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("camara: list request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("camara: list status %d", res.StatusCode)
	}

	var payload listResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("camara: decode list response: %w", err)
	}
	return payload.Dados, nil
}

func fetchDeputyDetail(ctx context.Context, client *http.Client, baseURL string, deputyID int) (*detailResponse, error) {
	u := strings.TrimRight(baseURL, "/") + "/deputados/" + strconv.Itoa(deputyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("camara: build detail request: %w", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("camara: detail request for %d: %w", deputyID, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("camara: detail status for %d: %d", deputyID, res.StatusCode)
	}

	var payload detailResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("camara: decode detail response for %d: %w", deputyID, err)
	}
	return &payload, nil
}

func isEmExercicio(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "Em exercício")
}
