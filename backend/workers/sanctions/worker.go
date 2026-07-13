// Package sanctions ingests official Brazilian punishment registries: CGU
// Portal da Transparência (CEIS, CNEP, CEAF, leniency agreements) and TCU
// (irregular accounts, inabilitados, inidôneos) into the graph as Sanction
// nodes with deterministic CPF/CNPJ-keyed SANCTIONED_IN edges.
package sanctions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"corruption-center/matching"
)

const workerName = "sanctions_sync"

// Registry codes stored on Sanction nodes (registry + ":" + entry id = node id).
const (
	RegistryCEIS           = "CEIS"
	RegistryCNEP           = "CNEP"
	RegistryCEAF           = "CEAF"
	RegistryLeniencia      = "LENIENCIA"
	RegistryTCUIrregular   = "TCU_IRREGULAR"
	RegistryTCUInabilitado = "TCU_INABILITADO"
	RegistryTCUInidoneo    = "TCU_INIDONEO"
)

// SanctionRecord is the normalized, source-agnostic record produced by both the
// CGU and TCU clients. Exactly one identity field (CNPJ / CPF / MaskedCPF) is
// populated for deterministic rows; Name is always kept for review fallback.
type SanctionRecord struct {
	Registry     string
	EntryID      string // registry-unique entry id
	SanctionType string
	Organ        string
	DateStart    string // yyyy-mm-dd or ""
	DateEnd      string // yyyy-mm-dd or ""
	ProcessRef   string
	SourceURL    string // deep link to the official record (required)

	Name      string
	CNPJ      string // 14 digits, or ""
	CPF       string // 11 digits (full), or ""
	MaskedCPF string // 6 visible middle digits of a masked CPF, or ""
}

// Options configures a Sanctions Sync run.
type Options struct {
	APIKey     string   // CGU chave-api-dados
	Registries []string // ceis,cnep,ceaf,leniencia,tcu (empty = all)
	DryRun     bool     // parse + match, no graph/review writes
	CGUBaseURL string   // override for tests
	TCUBaseURL string   // override for tests
	MaxPages   int      // per-registry CGU page cap (0 = until empty)

	// Sweep enables the retraction sweep after each registry completes cleanly:
	// anything the run did not see gets unpublished. OFF by default, and it must
	// stay that way — a partial run (MaxPages set, a registry filter, a dry run)
	// sees only part of the source, and its silence must never be mistaken for
	// the state having withdrawn a sanction. Only a FULL sync may sweep.
	Sweep bool
}

// Stats summarizes a run.
type Stats struct {
	// RunID stamps every record this run touches, so the retraction sweep can
	// tell "the source still says this" from "the source stopped saying it".
	RunID             string                 `json:"run_id"`
	Sweeps            []memgraph.SweepResult `json:"sweeps,omitempty"`
	RecordsProcessed  int                    `json:"records_processed"`
	SanctionsUpserted int                    `json:"sanctions_upserted"`
	OrgsCreated       int                    `json:"orgs_created"`
	PersonsCreated    int                    `json:"persons_created"`
	EdgesCreated      int                    `json:"edges_created"`
	PendingReviews    int                    `json:"pending_reviews"`
	MaskedCPFMatches  int                    `json:"masked_cpf_matches"`
	// AutoLinked counts edges written with no human review because the evidence
	// reached document grade (matching.AutoLinkThreshold).
	AutoLinked int `json:"auto_linked"`
	NameOnly   int `json:"name_only"`
	// SkippedTombstoned counts records whose subject was LGPD-purged: the node
	// is NOT re-created (resurrection guard, see purge_tombstone / migration 008).
	SkippedTombstoned int `json:"skipped_tombstoned"`
	// SkippedDuplicateReview counts possible_politician_sanction reviews suppressed
	// because an equal (sanction_id, politician_id) review already exists.
	SkippedDuplicateReview int `json:"skipped_duplicate_review"`
	// SkippedRegistries lists CGU registries skipped this run because no
	// TRANSPARENCIA_API_KEY was set (TCU still runs, being keyless).
	SkippedRegistries []string `json:"skipped_registries,omitempty"`
	// FailedRegistries lists registries that stopped early because the API kept
	// failing on a page ("ceis@p15"). What they ingested before that is kept; the
	// rest of the run continues. Re-run to pick up the remainder.
	FailedRegistries []string       `json:"failed_registries,omitempty"`
	PerRegistry      map[string]int `json:"per_registry"`
}

// sanctionsStore is the subset of *psql.DB the worker consults. Declared as an
// interface so the tombstone/dedup skip paths can be unit-tested with a fake.
type sanctionsStore interface {
	UpsertSanctionImportState(ctx context.Context, registry string, lastPage, recordsSeen int, syncedAt time.Time) error
	CreatePendingReview(ctx context.Context, reviewType string, payload []byte, worker string) error
	IsSubjectPurged(ctx context.Context, keys ...string) (bool, error)
	HasSanctionReview(ctx context.Context, sanctionID, politicianID string) (bool, error)
}

// Worker orchestrates a Sanctions Sync run.
type Worker struct {
	pg   sanctionsStore
	mg   *memgraph.DB
	opts Options
}

// NewWorker builds a worker. mg may be nil only for dry runs.
func NewWorker(pg *psql.DB, mg *memgraph.DB, opts Options) *Worker {
	w := &Worker{mg: mg, opts: opts}
	// Preserve nil-interface semantics for the w.pg != nil guards on the
	// import-state writes: a nil *psql.DB must stay a nil store.
	if pg != nil {
		w.pg = pg
	}
	return w
}

// selectedRegistries resolves the --registries flag into concrete registry
// groups. "tcu" expands to the three TCU lists.
func selectedRegistries(requested []string) []string {
	all := []string{"ceis", "cnep", "ceaf", "leniencia", "tcu"}
	if len(requested) == 0 {
		return all
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range requested {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// Run executes the selected registries and returns aggregate stats.
func (w *Worker) Run(ctx context.Context) (*Stats, error) {
	stats := &Stats{PerRegistry: map[string]int{}, RunID: newRunID()}

	if !w.opts.DryRun && w.mg == nil {
		return nil, fmt.Errorf("sanctions: memgraph writer is required unless --dry-run")
	}

	// TCU needs no auth, CGU requires a key. Without a key we must not let the
	// CGU registries abort the run before the keyless TCU lists ever run.
	toRun, skipped, err := planRegistries(selectedRegistries(w.opts.Registries), strings.TrimSpace(w.opts.APIKey) != "")
	if err != nil {
		return stats, err
	}
	if len(skipped) > 0 {
		stats.SkippedRegistries = skipped
		slog.Warn("sanctions: skipping CGU registries: TRANSPARENCIA_API_KEY is empty; running keyless TCU only",
			"skipped", skipped, "running", toRun)
	}

	for _, group := range toRun {
		switch group {
		case "ceis", "cnep", "ceaf", "leniencia":
			if err := w.runCGURegistry(ctx, group, stats); err != nil {
				return stats, fmt.Errorf("sanctions: %s: %w", group, err)
			}
			w.sweep(ctx, strings.ToUpper(group), stats)
		case "tcu":
			if err := w.runTCU(ctx, stats); err != nil {
				return stats, fmt.Errorf("sanctions: tcu: %w", err)
			}
			// TCU writes several registries (TCU_IRREGULAR, TCU_INABILITADO,
			// TCU_INIDONEO); each is swept on its own so one short list cannot
			// retract another.
			for _, r := range tcuRegistries {
				w.sweep(ctx, r, stats)
			}
		default:
			return stats, fmt.Errorf("sanctions: unknown registry %q", group)
		}
	}

	return stats, nil
}

// tcuRegistries are the registry names the TCU importer writes.
var tcuRegistries = []string{"TCU_IRREGULAR", "TCU_INABILITADO", "TCU_INIDONEO"}

// newRunID is a monotonic, human-readable stamp. Uniqueness per run is all that
// is required — it is compared for equality, never parsed.
func newRunID() string {
	return "run_" + time.Now().UTC().Format("20060102T150405.000000000")
}

// sweep unpublishes the records of one registry that this run did not see.
//
// It refuses in every circumstance where an absence might be OUR fault rather
// than the state's: a dry run, a partial fetch (MaxPages), or the sweep not
// being explicitly asked for. The graph library adds its own guards on top (a
// ceiling on how much one sweep may unpublish). Retraction is the only bulk
// unpublish in this system, and it fires on missing data — the same shape as a
// truncated response.
func (w *Worker) sweep(ctx context.Context, registry string, stats *Stats) {
	if !w.opts.Sweep || w.opts.DryRun || w.mg == nil {
		return
	}
	if w.opts.MaxPages > 0 {
		slog.Warn("sanctions: not sweeping, this was a partial fetch",
			"registry", registry, "max_pages", w.opts.MaxPages)
		return
	}
	res, err := w.mg.SweepRetractedSanctions(ctx, registry, stats.RunID, memgraph.DefaultRetractionGuard)
	if err != nil {
		// A failed sweep leaves everything published. That is the safe direction:
		// publishing a stale record is bad, but silently unpublishing thousands of
		// real ones on a broken sweep is worse and much harder to notice.
		slog.Error("sanctions: retraction sweep failed, leaving records published",
			"registry", registry, "err", err)
		return
	}
	stats.Sweeps = append(stats.Sweeps, res)
}

// planRegistries decides which of the selected registries to run given whether a
// CGU API key is present, and which CGU registries to skip.
//
//   - key present, or no CGU registry selected  → run everything as selected.
//   - no key, CGU + TCU both selected            → skip CGU, still run TCU.
//   - no key, ONLY CGU registries selected       → hard error (nothing runnable).
//
// "tcu" is the only keyless group; every other group is a CGU registry.
func planRegistries(selected []string, hasKey bool) (toRun, skipped []string, err error) {
	hasCGU, hasTCU := false, false
	for _, g := range selected {
		if g == "tcu" {
			hasTCU = true
		} else {
			hasCGU = true
		}
	}

	if hasKey || !hasCGU {
		return selected, nil, nil
	}

	// No key and at least one CGU registry was selected.
	if !hasTCU {
		return nil, nil, fmt.Errorf("sanctions: TRANSPARENCIA_API_KEY is required for the selected CGU registries %v", selected)
	}

	for _, g := range selected {
		if g == "tcu" {
			toRun = append(toRun, g)
		} else {
			skipped = append(skipped, g)
		}
	}
	return toRun, skipped, nil
}

// apply enforces the matching policy for a single normalized record.
func (w *Worker) apply(ctx context.Context, rec SanctionRecord, stats *Stats) error {
	stats.RecordsProcessed++
	stats.PerRegistry[rec.Registry]++

	if w.opts.DryRun {
		// Classify for reporting without writing.
		switch {
		case rec.MaskedCPF != "":
			stats.MaskedCPFMatches++ // counted as candidates in dry-run
		case rec.CNPJ == "" && rec.CPF == "":
			stats.NameOnly++
		}
		return nil
	}

	sanctionID, err := w.mg.UpsertSanction(ctx, memgraph.SanctionUpsert{
		Registry:     rec.Registry,
		EntryID:      rec.EntryID,
		SanctionType: rec.SanctionType,
		Organ:        rec.Organ,
		DateStart:    rec.DateStart,
		DateEnd:      rec.DateEnd,
		ProcessRef:   rec.ProcessRef,
		SourceURL:    rec.SourceURL,
		RunID:        stats.RunID,
	})
	if err != nil {
		return err
	}
	stats.SanctionsUpserted++

	switch {
	case rec.CNPJ != "":
		return w.linkCNPJ(ctx, rec, sanctionID, stats)
	case rec.CPF != "":
		return w.linkFullCPF(ctx, rec, sanctionID, stats)
	case rec.MaskedCPF != "":
		return w.linkMaskedCPF(ctx, rec, sanctionID, stats)
	default:
		return w.linkNameOnly(ctx, rec, sanctionID, stats)
	}
}

// linkCNPJ: deterministic - ensure Organization (create bare for the enricher)
// and link. No review.
func (w *Worker) linkCNPJ(ctx context.Context, rec SanctionRecord, sanctionID string, stats *Stats) error {
	// LGPD resurrection guard: if this CNPJ was purged, do not re-create the
	// Organization node (EnsureOrganizationByCNPJ would otherwise auto-create it).
	purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyCNPJ(rec.CNPJ))
	if err != nil {
		return err
	}
	if purged {
		stats.SkippedTombstoned++
		return nil
	}

	orgID, created, err := w.mg.EnsureOrganizationByCNPJ(ctx, rec.CNPJ)
	if err != nil {
		return err
	}
	if created {
		stats.OrgsCreated++
	}
	_, docScore, docSignals := matching.AutoLink(matching.Evidence{FullDocument: true})
	if err := w.mg.EnsureSanctionedInEdge(ctx, "Organization", orgID, sanctionID, docScore, docSignals); err != nil {
		return err
	}
	stats.EdgesCreated++
	return nil
}

// linkFullCPF: deterministic - match existing Politician/Person, else create a
// Person keyed by the full CPF. No review.
func (w *Worker) linkFullCPF(ctx context.Context, rec SanctionRecord, sanctionID string, stats *Stats) error {
	// LGPD resurrection guard: if this CPF (or its purged name) was tombstoned,
	// skip entirely - neither link to a possibly-recreated node nor auto-create a
	// new Person via UpsertPersonByCPF.
	purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyCPF(rec.CPF), psql.TombstoneKeyName(rec.Name))
	if err != nil {
		return err
	}
	if purged {
		stats.SkippedTombstoned++
		return nil
	}

	nodeType, nodeID, err := w.mg.FindSubjectByCPF(ctx, rec.CPF)
	if err != nil {
		return err
	}
	if nodeType == "" {
		nodeID, created, err := w.mg.UpsertPersonByCPF(ctx, rec.Name, rec.CPF)
		if err != nil {
			return err
		}
		if created {
			stats.PersonsCreated++
		}
		_, docScore, docSignals := matching.AutoLink(matching.Evidence{FullDocument: true})
		if err := w.mg.EnsureSanctionedInEdge(ctx, "Person", nodeID, sanctionID, docScore, docSignals); err != nil {
			return err
		}
		stats.EdgesCreated++
		return nil
	}
	_, docScore, docSignals := matching.AutoLink(matching.Evidence{FullDocument: true})
	if err := w.mg.EnsureSanctionedInEdge(ctx, nodeType, nodeID, sanctionID, docScore, docSignals); err != nil {
		return err
	}
	stats.EdgesCreated++
	return nil
}

// linkMaskedCPF: CGU masks CPFs on the person registries (***.435.151-**), so the
// six visible middle digits are compared against the full CPF we hold from TSE.
// That alone is not an identification - several people share any six middle
// digits: so the link is scored (see package matching): six digits AND an exact
// name reach document grade and link automatically; anything less, or evidence
// that fits more than one politician, goes to a human.
func (w *Worker) linkMaskedCPF(ctx context.Context, rec SanctionRecord, sanctionID string, stats *Stats) error {
	matches, err := w.mg.MatchPoliticiansByMaskedCPF(ctx, rec.MaskedCPF)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return nil
	}
	for _, m := range matches {
		stats.MaskedCPFMatches++

		auto, score, signals := matching.AutoLink(matching.Evidence{
			MaskedCPF:   true,
			SourceName:  rec.Name,
			SubjectName: m.Name,
			Candidates:  len(matches),
		})
		if !auto {
			created, err := w.queueReview(ctx, rec, sanctionID, m.ID, m.Name, score, signals)
			if err != nil {
				return err
			}
			if created {
				stats.PendingReviews++
			} else {
				stats.SkippedDuplicateReview++
			}
			continue
		}

		// LGPD resurrection guard, same as the full-CPF path.
		purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyName(m.Name))
		if err != nil {
			return err
		}
		if purged {
			stats.SkippedTombstoned++
			continue
		}
		if err := w.mg.EnsureSanctionedInEdge(ctx, "Politician", m.ID, sanctionID, score, signals); err != nil {
			return err
		}
		stats.EdgesCreated++
		stats.AutoLinked++
	}
	return nil
}

// linkNameOnly: no document at all. Only queued for review when the name matches
// a known Politician; otherwise the Sanction node stays unlinked.
func (w *Worker) linkNameOnly(ctx context.Context, rec SanctionRecord, sanctionID string, stats *Stats) error {
	stats.NameOnly++
	name := strings.TrimSpace(rec.Name)
	if name == "" {
		return nil
	}
	polID, err := w.mg.MatchPoliticianByName(ctx, name)
	if err != nil {
		return err
	}
	if polID == "" {
		return nil
	}
	score, signals := matching.Score(matching.Evidence{SourceName: rec.Name, SubjectName: name})
	created, err := w.queueReview(ctx, rec, sanctionID, polID, name, score, signals)
	if err != nil {
		return err
	}
	if created {
		stats.PendingReviews++
	} else {
		stats.SkippedDuplicateReview++
	}
	return nil
}

// queueReview files a possible_politician_sanction review, deduplicated on the
// (sanction_id, politician_id) pair: if an equal review already exists in ANY
// status (including operator-rejected), it is NOT re-filed. Returns whether a new
// review row was created.
func (w *Worker) queueReview(ctx context.Context, rec SanctionRecord, sanctionID, politicianID, politicianName string, confidence float64, signals []string) (bool, error) {
	exists, err := w.pg.HasSanctionReview(ctx, sanctionID, politicianID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	payload, _ := json.Marshal(map[string]any{
		"sanction_id":        sanctionID,
		"politician_id":      politicianID,
		"politician_name":    politicianName,
		"confidence":         confidence,
		"confidence_signals": signals,
		"registry":           rec.Registry,
		"sanctioned_name":    rec.Name,
		"masked_cpf":         rec.MaskedCPF,
		"source_url":         rec.SourceURL,
	})
	if err := w.pg.CreatePendingReview(ctx, "possible_politician_sanction", payload, workerName); err != nil {
		return false, err
	}
	return true, nil
}

// digitsOnly strips everything but ASCII digits.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// classifyDocument normalizes a source CPF/CNPJ string (formatted, masked, or
// raw digits) into exactly one of the identity buckets.
//
//	CNPJ:   14 digits              -> cnpj
//	CPF:    11 digits              -> cpf (full)
//	masked: contains '*' + 6 digits -> masked (the visible middle block)
func classifyDocument(raw string) (cpf, cnpj, masked string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	digits := digitsOnly(raw)
	switch {
	case len(digits) == 14:
		return "", digits, ""
	case len(digits) == 11:
		return digits, "", ""
	case strings.Contains(raw, "*") && len(digits) == 6:
		return "", "", digits
	default:
		return "", "", ""
	}
}
