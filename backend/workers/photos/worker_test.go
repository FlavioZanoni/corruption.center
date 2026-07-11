package photos

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── CPF→SQ consulta parsing ──────────────────────────────────────────────────

func TestParseConsultaCPFtoSQ(t *testing.T) {
	// ISO-8859-1, ';'-separated. Header order is intentionally not the natural
	// one to prove the parser resolves columns by name. The name column carries
	// a latin1 'ã' (0xE3) to exercise the ISO-8859-1 decode path.
	var buf bytes.Buffer
	buf.WriteString("SQ_CANDIDATO;NM_CANDIDATO;NR_CPF_CANDIDATO\n")
	buf.WriteString("230002529954;JO")
	buf.WriteByte(0xC3) // Ã in ISO-8859-1 is 0xC3
	buf.WriteString("O;12345678901\n")
	// Leading-zero CPF that must normalize to 11 digits.
	buf.WriteString("230000000001;MARIA;987654321\n")
	// Duplicate CPF (later turn) must not overwrite the first SQ.
	buf.WriteString("230009999999;JOAO 2;12345678901\n")
	// Blank CPF row skipped.
	buf.WriteString("230000000002;SEM CPF;\n")

	m, err := parseConsultaCPFtoSQ(&buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m["12345678901"]; got != "230002529954" {
		t.Errorf("cpf 12345678901 -> %q, want 230002529954 (first SQ wins)", got)
	}
	if got := m["00987654321"]; got != "230000000001" {
		t.Errorf("leading-zero cpf -> %q, want 230000000001", got)
	}
	if len(m) != 2 {
		t.Errorf("map size = %d, want 2 (blank cpf skipped)", len(m))
	}
}

func TestParseConsultaMissingColumns(t *testing.T) {
	if _, err := parseConsultaCPFtoSQ(strings.NewReader("A;B\n1;2\n")); err == nil {
		t.Fatal("expected error for missing required columns")
	}
}

func TestNormalizeCPF(t *testing.T) {
	cases := map[string]string{
		"123.456.789-01":  "12345678901",
		"987654321":       "00987654321",
		"12345678901":     "12345678901",
		"":                "",
		"00000000000":     "",
		"123456789012345": "", // too long
	}
	for in, want := range cases {
		if got := normalizeCPF(in); got != want {
			t.Errorf("normalizeCPF(%q) = %q, want %q", in, got, want)
		}
	}
}

// ─── Photo filename / URL resolution ──────────────────────────────────────────

func TestParsePhotoFilename(t *testing.T) {
	cases := []struct {
		in     string
		uf, sq string
		ok     bool
	}{
		{"FRR230002529954_div.jpg", "RR", "230002529954", true},
		{"FRR230002529860_div.jpeg", "RR", "230002529860", true},
		{"FSP210000012345_div.JPG", "SP", "210000012345", true},
		{"leiame.pdf", "", "", false},
		{"garbage.txt", "", "", false},
	}
	for _, c := range cases {
		uf, sq, ok := parsePhotoFilename(c.in)
		if ok != c.ok || uf != c.uf || sq != c.sq {
			t.Errorf("parsePhotoFilename(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, uf, sq, ok, c.uf, c.sq, c.ok)
		}
	}
}

func TestBuildPhotoURL(t *testing.T) {
	got := buildPhotoURL("https://x/{year}/{uf}/{sq}/foto.jpg", 2022, "rr", " 230002529954 ")
	want := "https://x/2022/RR/230002529954/foto.jpg"
	if got != want {
		t.Errorf("buildPhotoURL = %q, want %q", got, want)
	}
}

func TestLooksLikeImage(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	html := []byte("<!DOCTYPE html>")
	if !looksLikeImage("image/jpeg", nil) {
		t.Error("content-type image/jpeg should pass")
	}
	if !looksLikeImage("", jpeg) {
		t.Error("jpeg magic should pass")
	}
	if !looksLikeImage("application/octet-stream", png) {
		t.Error("png magic should pass")
	}
	if looksLikeImage("text/html; charset=utf-8", html) {
		t.Error("html should NOT pass")
	}
}

// ─── "don't overwrite camara/senado photo_url" rule ───────────────────────────

func TestNeedsPhoto(t *testing.T) {
	if !needsPhoto("") {
		t.Error("empty photo_url should need a photo")
	}
	if !needsPhoto("   ") {
		t.Error("whitespace photo_url should need a photo")
	}
	// A photo_url set by the camara/senado syncer must never be overwritten.
	if needsPhoto("https://www.camara.leg.br/internet/deputado/bandep/12345.jpg") {
		t.Error("camara-set photo_url must be treated as already present (not overwritten)")
	}
}

// ─── Wikidata / Commons pure mapping ──────────────────────────────────────────

func TestFormatCNPJ(t *testing.T) {
	if got := formatCNPJ("33000167000101"); got != "33.000.167/0001-01" {
		t.Errorf("formatCNPJ = %q", got)
	}
	if got := formatCNPJ("123"); got != "123" {
		t.Errorf("formatCNPJ short passthrough = %q", got)
	}
}

func TestParseSPARQLImage(t *testing.T) {
	body := []byte(`{"head":{"vars":["image"]},"results":{"bindings":[
	  {"image":{"type":"uri","value":"http://commons.wikimedia.org/wiki/Special:FilePath/Petrobras%20logo.jpg"}}
	]}}`)
	v, ok := parseSPARQLImage(body)
	if !ok || v != "http://commons.wikimedia.org/wiki/Special:FilePath/Petrobras%20logo.jpg" {
		t.Fatalf("parseSPARQLImage = %q,%v", v, ok)
	}
	if _, ok := parseSPARQLImage([]byte(`{"results":{"bindings":[]}}`)); ok {
		t.Error("empty bindings should be ok=false")
	}
}

func TestCommonsFilenameFromP18(t *testing.T) {
	cases := map[string]string{
		"http://commons.wikimedia.org/wiki/Special:FilePath/Foo%20Bar.jpg": "Foo Bar.jpg",
		"Some_File.jpg": "Some File.jpg",
		"Plain.png":     "Plain.png",
	}
	for in, want := range cases {
		if got := commonsFilenameFromP18(in); got != want {
			t.Errorf("commonsFilenameFromP18(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildCommonsURLAndAttribution(t *testing.T) {
	url := buildCommonsThumbURL("Foo Bar.jpg")
	if url != "https://commons.wikimedia.org/wiki/Special:FilePath/Foo_Bar.jpg?width=512" {
		t.Errorf("thumb url = %q", url)
	}
	attr := buildCommonsAttribution("Foo Bar.jpg")
	if !strings.HasPrefix(attr, "Foo Bar.jpg: Wikimedia Commons") ||
		!strings.Contains(attr, "https://commons.wikimedia.org/wiki/File:Foo_Bar.jpg") {
		t.Errorf("attribution = %q", attr)
	}
}

func TestParseWikibaseItem(t *testing.T) {
	single := []byte(`{"query":{"pages":{"123":{"pageprops":{"wikibase_item":"Q42"}}}}}`)
	if qid, ok, _ := parseWikibaseItem(single); !ok || qid != "Q42" {
		t.Errorf("single = %q,%v", qid, ok)
	}
	missing := []byte(`{"query":{"pages":{"-1":{"missing":""}}}}`)
	if _, ok, _ := parseWikibaseItem(missing); ok {
		t.Error("missing page should be ok=false")
	}
	// Two existing pages → ambiguous → refuse.
	ambiguous := []byte(`{"query":{"pages":{"1":{"pageprops":{"wikibase_item":"Q1"}},"2":{"pageprops":{"wikibase_item":"Q2"}}}}}`)
	if _, ok, _ := parseWikibaseItem(ambiguous); ok {
		t.Error("ambiguous pages should be ok=false")
	}
}

func TestParseEntityDataP18(t *testing.T) {
	body := []byte(`{"entities":{"Q42":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Douglas adams.jpg"}}}]}}}}`)
	if f, ok, _ := parseEntityDataP18(body, "Q42"); !ok || f != "Douglas adams.jpg" {
		t.Errorf("p18 = %q,%v", f, ok)
	}
	noImg := []byte(`{"entities":{"Q42":{"claims":{}}}}`)
	if _, ok, _ := parseEntityDataP18(noImg, "Q42"); ok {
		t.Error("no P18 claim should be ok=false")
	}
}

// ─── Wikidata client end-to-end mapping (httptest, no live calls) ─────────────

func TestFindOrgImageByCNPJ(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(`{"results":{"bindings":[{"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Petrobras.jpg"}}]}}`))
	}))
	defer srv.Close()

	wc := newWikidataClient(srv.Client(), srv.URL, "test")
	wc.minInterval = 0
	file, ok, err := wc.FindOrgImageByCNPJ(context.Background(), "33000167000101")
	if err != nil || !ok || file != "Petrobras.jpg" {
		t.Fatalf("FindOrgImageByCNPJ = %q,%v,%v", file, ok, err)
	}
}

// TestFindPoliticianImageAmbiguous proves a name-only match resolving to two
// distinct Wikidata entities is refused (never auto-set).
func TestFindPoliticianImageAmbiguous(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case q.Get("action") == "query" && q.Get("titles") == "Alpha":
			_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"pageprops":{"wikibase_item":"Q1"}}}}}`))
		case q.Get("action") == "query" && q.Get("titles") == "Beta":
			_, _ = w.Write([]byte(`{"query":{"pages":{"2":{"pageprops":{"wikibase_item":"Q2"}}}}}`))
		case strings.Contains(r.URL.Path, "Q1"):
			_, _ = w.Write([]byte(`{"entities":{"Q1":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"A.jpg"}}}]}}}}`))
		case strings.Contains(r.URL.Path, "Q2"):
			_, _ = w.Write([]byte(`{"entities":{"Q2":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"B.jpg"}}}]}}}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	wc := newWikidataClient(srv.Client(), srv.URL, "test")
	wc.minInterval = 0
	// Point the Wikipedia + EntityData endpoints at the test server via overrides.
	oldWP, oldED := ptWikipediaAPIOverride, entityDataOverride
	ptWikipediaAPIOverride = srv.URL
	entityDataOverride = srv.URL + "/%s.json"
	defer func() { ptWikipediaAPIOverride, entityDataOverride = oldWP, oldED }()

	_, ok, err := wc.FindPoliticianImage(context.Background(), []string{"Alpha", "Beta"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("two distinct entities must be refused (ok=false)")
	}
}

func TestFindPoliticianImageSingle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case q.Get("action") == "query":
			_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"pageprops":{"wikibase_item":"Q1"}}}}}`))
		case strings.Contains(r.URL.Path, "Q1"):
			_, _ = w.Write([]byte(`{"entities":{"Q1":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Solo.jpg"}}}]}}}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	wc := newWikidataClient(srv.Client(), srv.URL, "test")
	wc.minInterval = 0
	oldWP, oldED := ptWikipediaAPIOverride, entityDataOverride
	ptWikipediaAPIOverride = srv.URL
	entityDataOverride = srv.URL + "/%s.json"
	defer func() { ptWikipediaAPIOverride, entityDataOverride = oldWP, oldED }()

	file, ok, err := wc.FindPoliticianImage(context.Background(), []string{"Solo", "Solo Alias"})
	if err != nil || !ok || file != "Solo.jpg" {
		t.Fatalf("FindPoliticianImage single = %q,%v,%v", file, ok, err)
	}
}

func TestSelectedModes(t *testing.T) {
	if got := selectedModes(nil); len(got) != 2 {
		t.Errorf("default modes = %v", got)
	}
	if got := selectedModes([]string{"tse", "tse"}); len(got) != 1 || got[0] != "tse" {
		t.Errorf("dedup modes = %v", got)
	}
}
