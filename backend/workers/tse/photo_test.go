package tse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildPhotoURL(t *testing.T) {
	got := buildPhotoURL("2040602022", "280001607829", "BR")
	want := "https://divulgacandcontas.tse.jus.br/divulga/rest/arquivo/img/2040602022/280001607829/BR"
	if got != want {
		t.Fatalf("buildPhotoURL = %q, want %q", got, want)
	}
	if got := buildPhotoURL("546", "123", "pe"); !strings.HasSuffix(got, "/PE") {
		t.Fatalf("UF should be upper-cased, got %q", got)
	}
}

// Every part is required. A URL missing one still resolves at TSE, but answers
// with a grey "sem foto" placeholder under a 200, so an incomplete URL would give
// the whole base the same fake portrait. Better to have no photo.
func TestBuildPhotoURL_MissingPartYieldsNoURL(t *testing.T) {
	cases := []struct{ electionID, sq, uf, why string }{
		{"", "280001607829", "BR", "no election id (2002 and earlier predate divulgacandcontas)"},
		{"2040602022", "", "BR", "no candidate sequence"},
		{"2040602022", "280001607829", "", "no UF"},
	}
	for _, c := range cases {
		if got := buildPhotoURL(c.electionID, c.sq, c.uf); got != "" {
			t.Errorf("%s: expected no URL, got %q", c.why, got)
		}
	}
}

func TestResolveElectionID(t *testing.T) {
	// The shapes TSE actually answers with: a bare object, and (defensively) an
	// array. The id arrives as a bare number, not a string.
	for _, body := range []string{
		`{"id":2040602022,"nomeEleicao":"Eleição Geral Federal 2022"}`,
		`[{"id":2040602022,"nomeEleicao":"Eleição Geral Federal 2022"}]`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/2022") {
				t.Errorf("expected the year in the path, got %q", r.URL.Path)
			}
			_, _ = w.Write([]byte(body))
		}))

		id, err := resolveElectionIDAt(t, server.URL, 2022)
		server.Close()
		if err != nil {
			t.Fatalf("ResolveElectionID(%s): %v", body, err)
		}
		if id != "2040602022" {
			t.Fatalf("id = %q, want 2040602022 (from %s)", id, body)
		}
	}
}

// A year with no election (2002, or one that has not happened yet) is a fact
// about the year, not an error: import the year, just without photos.
func TestResolveElectionID_NoElectionIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TSE answers 200 with an empty body.
	}))
	defer server.Close()

	id, err := resolveElectionIDAt(t, server.URL, 2002)
	if err != nil {
		t.Fatalf("expected no error for a year without an election, got %v", err)
	}
	if id != "" {
		t.Fatalf("expected no election id, got %q", id)
	}
}

// resolveElectionIDAt points ResolveElectionID at a test server by swapping the
// package endpoint for the duration of the test.
func resolveElectionIDAt(t *testing.T, baseURL string, year int) (string, error) {
	t.Helper()
	original := electionEndpointFor
	t.Cleanup(func() { electionEndpointFor = original })
	electionEndpointFor = func(y int) string {
		return baseURL + "/" + itoa(y)
	}
	return ResolveElectionID(context.Background(), nil, year)
}
