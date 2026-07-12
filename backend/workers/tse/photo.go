package tse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TSE serves candidate photos straight off divulgacandcontas, so we hotlink them
// and never rehost.
//
//	.../divulga/rest/arquivo/img/{idEleicao}/{SQ_CANDIDATO}/{SG_UF}
//
// The first segment is TSE's internal election id, NOT the year and NOT the
// CD_ELEICAO column of the CSV. That distinction matters more than it looks:
// with a wrong id the endpoint still answers 200 image/jpeg, but every candidate
// comes back as the same grey "sem foto" placeholder. A silently uniform photo
// for the whole base is a worse failure than no photo at all, so the id has to
// come from TSE itself (see ResolveElectionID).
const (
	photoBaseURL     = "https://divulgacandcontas.tse.jus.br/divulga/rest/arquivo/img"
	electionEndpoint = "https://divulgacandcontas.tse.jus.br/divulga/rest/v1/eleicao/ordinaria"

	// PhotoSource labels photos we set, so a later import can tell its own photo
	// apart from a better one (camara/senado portraits, Wikimedia) and leave those
	// alone.
	PhotoSource = "tse"

	// PhotoAttribution credits the source. TSE candidate photos are official
	// electoral records published by the Tribunal Superior Eleitoral.
	PhotoAttribution = "Tribunal Superior Eleitoral — divulgacandcontas"
)

// electionEndpointFor builds the ordinary-election URL for a year. A var so tests
// can point it at a stub instead of reaching for TSE.
var electionEndpointFor = func(year int) string {
	return fmt.Sprintf("%s/%d", electionEndpoint, year)
}

func itoa(i int) string { return strconv.Itoa(i) }

// buildPhotoURL returns the hotlink for a candidate's TSE photo, or "" when any
// part is missing. An empty electionID means TSE has no divulgacandcontas
// election for that year (2002 and earlier), so there is no photo to point at.
func buildPhotoURL(electionID, sq, uf string) string {
	electionID = strings.TrimSpace(electionID)
	sq = strings.TrimSpace(sq)
	uf = strings.ToUpper(strings.TrimSpace(uf))
	if electionID == "" || sq == "" || uf == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/%s", photoBaseURL, electionID, sq, uf)
}

// ResolveElectionID asks TSE for the ordinary election held in a year and
// returns its internal id, which is what the photo endpoint keys on.
//
// This is looked up rather than hardcoded on purpose. The ids follow no pattern
// we can derive (2006 is 14423, 2014 is 680, 2022 is 2040602022), and a table
// frozen in the source would leave the next election with no photos at all until
// someone noticed and edited it.
//
// A year TSE does not publish (2002, or an election that has not happened yet)
// returns "" with no error: that is a fact about the year, not a failure.
func ResolveElectionID(ctx context.Context, client *http.Client, year int) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, electionEndpointFor(year), nil)
	if err != nil {
		return "", fmt.Errorf("tse: build election request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tse: resolve election id for %d: %w", year, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tse: resolve election id for %d: status %d", year, res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("tse: read election response for %d: %w", year, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return "", nil
	}

	// The endpoint answers with a single election object, but has been seen to
	// wrap it in an array; accept either rather than depend on which.
	var one election
	if err := json.Unmarshal(body, &one); err == nil && one.ID != "" {
		return one.ID, nil
	}
	var many []election
	if err := json.Unmarshal(body, &many); err != nil {
		return "", fmt.Errorf("tse: decode election for %d: %w", year, err)
	}
	if len(many) == 0 {
		return "", nil
	}
	return many[0].ID, nil
}

// election is one entry from the ordinary-election endpoint. TSE sends the id as
// a bare number, so decode it loosely and render it back as a string.
type election struct {
	ID string
}

func (e *election) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	id := strings.Trim(strings.TrimSpace(string(raw.ID)), `"`)
	if id == "" || id == "null" {
		return nil
	}
	// Guard against a float landing here (2040602022 would survive float64, but
	// an id that did not would quietly corrupt every photo URL for the year).
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return fmt.Errorf("tse: election id %q is not an integer", id)
	}
	e.ID = id
	return nil
}
