// Package photos enriches the graph with photos: every Politician should have a
// photo, and Organizations with a Wikidata/Wikimedia Commons presence should
// carry their Commons P18 image (never the P154 logo, for legal reasons).
//
// The worker stores NO image bytes locally: photo_url always hotlinks to an
// official server (TSE divulgacandcontas or Wikimedia Commons Special:FilePath).
//
// Two modes:
//   - tse:      historical Politicians without a photo → TSE candidate photo
//     hotlink, resolved via CPF→SQ_CANDIDATO (consulta_cand CSVs) and verified
//     at runtime (must return image bytes) before writing.
//   - wikidata: Organizations by CNPJ (P6204 → P18 → Commons), and Politicians
//     still without a photo via an exact pt.wikipedia title → Wikidata → P18.
package photos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"corruption-center/db/memgraph"
)

const (
	workerName = "photos"
	// userAgent identifies the worker to public endpoints, with a contact.
	userAgent = "corruption.center-photos/1.0 (flaviozgallon@gmail.com)"

	photoSourceTSEFmt = "TSE Divulgação de Candidaturas %d"
)

// Modes.
const (
	ModeTSE      = "tse"
	ModeWikidata = "wikidata"
)

// Options configures a photos run.
type Options struct {
	Modes  []string // "tse", "wikidata" (empty = both)
	Year   int      // TSE election year for photo lookup
	UF     string   // optional UF filter (Politician.state)
	Limit  int      // per-mode cap on graph targets (0 = all)
	DryRun bool     // resolve + verify but perform no graph writes

	// Injection / overrides (tests, and operator overrides).
	HTTPClient       *http.Client
	TSEURLTemplate   string // override default candidate-photo URL template
	SPARQLEndpoint   string // override Wikidata SPARQL endpoint
	WorkDir          string // where the consulta zip is downloaded (metadata only)
	ProbeUF, ProbeSQ string // TSE pre-flight probe sample
}

// Stats summarizes a run.
type Stats struct {
	PoliticiansScanned    int `json:"politicians_scanned"`
	PoliticianPhotosTSE   int `json:"politician_photos_tse"`
	PoliticianSkipNoSQ    int `json:"politician_skipped_no_sq"`
	PoliticianSkipUnverif int `json:"politician_skipped_unverified"`

	OrgsScanned        int `json:"orgs_scanned"`
	OrgPhotosWikidata  int `json:"org_photos_wikidata"`
	OrgsSkippedNoImage int `json:"orgs_skipped_no_image"`

	PoliticianWikidataScanned int `json:"politician_wikidata_scanned"`
	PoliticianWikidataSet     int `json:"politician_wikidata_set"`
	PoliticianWikidataSkipped int `json:"politician_wikidata_skipped"`
}

// Worker orchestrates a photos run.
type Worker struct {
	mg   *memgraph.DB
	opts Options
	log  *slog.Logger
}

// NewWorker builds a worker. mg may be nil only for --dry-run.
func NewWorker(mg *memgraph.DB, opts Options) *Worker {
	return &Worker{mg: mg, opts: opts, log: slog.Default()}
}

func selectedModes(requested []string) []string {
	if len(requested) == 0 {
		return []string{ModeTSE, ModeWikidata}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, m := range requested {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// Run executes the selected modes.
func (w *Worker) Run(ctx context.Context) (*Stats, error) {
	stats := &Stats{}
	if !w.opts.DryRun && w.mg == nil {
		return nil, fmt.Errorf("photos: memgraph writer is required unless --dry-run")
	}
	for _, mode := range selectedModes(w.opts.Modes) {
		switch mode {
		case ModeTSE:
			if err := w.runTSE(ctx, stats); err != nil {
				return stats, fmt.Errorf("photos: tse: %w", err)
			}
		case ModeWikidata:
			if err := w.runWikidata(ctx, stats); err != nil {
				return stats, fmt.Errorf("photos: wikidata: %w", err)
			}
		default:
			return stats, fmt.Errorf("photos: unknown mode %q", mode)
		}
	}
	return stats, nil
}

// needsPhoto reports whether a current photo_url is empty, i.e. whether the
// photos worker is allowed to set one. A non-empty value (set by the
// camara/senado syncers) is never overwritten. Pure and unit-tested.
func needsPhoto(currentPhotoURL string) bool {
	return strings.TrimSpace(currentPhotoURL) == ""
}

// ─── TSE mode ─────────────────────────────────────────────────────────────────

func (w *Worker) runTSE(ctx context.Context, stats *Stats) error {
	year := w.opts.Year
	if year == 0 {
		year = defaultProbeYear
	}
	tc := newTSEClient(w.opts.HTTPClient, w.opts.TSEURLTemplate, userAgent)

	// Pre-flight: verify the hotlink template actually serves image bytes for a
	// known-existing candidate BEFORE downloading the (large) consulta metadata.
	// This is the safety valve required by the spec: if no stable hotlink can be
	// verified, TSE mode fails fast with a clear error rather than guessing.
	probeUF := firstNonEmptyStr(w.opts.ProbeUF, defaultProbeUF)
	probeSQ := firstNonEmptyStr(w.opts.ProbeSQ, defaultProbeSQ)
	probeYear := year
	if probeSQ == defaultProbeSQ && year != defaultProbeYear {
		// The default sample only exists for 2022; probe that year's sample.
		probeYear = defaultProbeYear
	}
	probeURL := buildPhotoURL(tc.urlTemplate, probeYear, probeUF, probeSQ)
	okImg, err := tc.verifyImageURL(ctx, probeURL)
	if err != nil {
		return fmt.Errorf("not implemented: no stable TSE photo hotlink could be verified (probe %s failed: %v). "+
			"divulgacandcontas served no image (it was under scheduled maintenance as of 2026-07-10). "+
			"Re-run when available, or pass --tse-url-template with a verified pattern", probeURL, err)
	}
	if !okImg {
		return fmt.Errorf("not implemented: no stable TSE photo hotlink found (probe %s did not return an image). "+
			"The default template is unverified and divulgacandcontas was under maintenance as of 2026-07-10. "+
			"Pass --tse-url-template with a verified {year}/{uf}/{sq} pattern once one is confirmed", probeURL)
	}

	// Hotlink verified → build CPF→SQ from the official consulta metadata.
	workDir := firstNonEmptyStr(w.opts.WorkDir, os.TempDir())
	zipPath, err := tc.downloadConsultaZip(ctx, year, workDir)
	if err != nil {
		return err
	}
	cpfToSQ, err := buildCPFtoSQFromZip(zipPath, w.opts.UF)
	if err != nil {
		return err
	}
	_ = os.Remove(zipPath) // metadata only; not retained

	targets, err := w.mg.ListPoliticiansNeedingPhoto(ctx, w.opts.UF, w.opts.Limit)
	if err != nil {
		return err
	}
	for _, t := range targets {
		stats.PoliticiansScanned++
		if !needsPhoto(t.PhotoURL) {
			continue
		}
		sq, ok := cpfToSQ[normalizeCPF(t.CPF)]
		if !ok {
			stats.PoliticianSkipNoSQ++
			continue
		}
		uf := firstNonEmptyStr(w.opts.UF, t.State)
		photoURL := buildPhotoURL(tc.urlTemplate, year, uf, sq)
		verified, err := tc.verifyImageURL(ctx, photoURL)
		if err != nil {
			return err
		}
		if !verified {
			stats.PoliticianSkipUnverif++
			continue
		}
		if w.opts.DryRun {
			stats.PoliticianPhotosTSE++
			continue
		}
		set, err := w.mg.SetPoliticianPhotoByCPF(ctx, t.CPF, photoURL, fmt.Sprintf(photoSourceTSEFmt, year), "")
		if err != nil {
			return err
		}
		if set {
			stats.PoliticianPhotosTSE++
		}
	}
	return nil
}

// ─── Wikidata mode ────────────────────────────────────────────────────────────

func (w *Worker) runWikidata(ctx context.Context, stats *Stats) error {
	wc := newWikidataClient(w.opts.HTTPClient, w.opts.SPARQLEndpoint, userAgent)

	// Organizations by CNPJ.
	orgs, err := w.mg.ListOrganizationsNeedingPhoto(ctx, w.opts.Limit)
	if err != nil {
		return err
	}
	for _, o := range orgs {
		stats.OrgsScanned++
		file, ok, err := wc.FindOrgImageByCNPJ(ctx, o.CNPJ)
		if err != nil {
			return err
		}
		if !ok {
			stats.OrgsSkippedNoImage++
			continue
		}
		imageURL := buildCommonsThumbURL(file)
		attribution := buildCommonsAttribution(file)
		if w.opts.DryRun {
			stats.OrgPhotosWikidata++
			continue
		}
		set, err := w.mg.SetOrganizationPhotoByCNPJ(ctx, o.CNPJ, imageURL, photoSourceCommons, attribution)
		if err != nil {
			return err
		}
		if set {
			stats.OrgPhotosWikidata++
		}
	}

	// Politicians still without a photo → exact-title Wikipedia fallback.
	targets, err := w.mg.ListPoliticiansNeedingPhoto(ctx, w.opts.UF, w.opts.Limit)
	if err != nil {
		return err
	}
	for _, t := range targets {
		stats.PoliticianWikidataScanned++
		if !needsPhoto(t.PhotoURL) {
			continue
		}
		names := append([]string{t.Name}, t.Aliases...)
		file, ok, err := wc.FindPoliticianImage(ctx, names)
		if err != nil {
			return err
		}
		if !ok {
			stats.PoliticianWikidataSkipped++
			continue
		}
		photoURL := buildCommonsThumbURL(file)
		attribution := buildCommonsAttribution(file)
		if w.opts.DryRun {
			stats.PoliticianWikidataSet++
			continue
		}
		set, err := w.mg.SetPoliticianPhotoByCPF(ctx, t.CPF, photoURL, photoSourceCommons, attribution)
		if err != nil {
			return err
		}
		if set {
			stats.PoliticianWikidataSet++
		} else {
			stats.PoliticianWikidataSkipped++
		}
	}
	return nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
