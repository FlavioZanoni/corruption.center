package djen

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
)

const workerName = "djen"

// defaultNameCap bounds items pulled per politician name per run (spec).
const defaultNameCap = 300

// djenStore is the subset of *psql.DB the worker consults. Declared as an
// interface so the tombstone skip paths can be unit-tested with a fake.
type djenStore interface {
	ListDjenCasesForPoll(ctx context.Context) ([]psql.WatcherCase, error)
	ListDjenSnapshotKeys(ctx context.Context, caseNumber string) (map[string]bool, error)
	InsertDjenSnapshotParty(ctx context.Context, caseNumber, partyKey, nome, polo string) error
	UpdateDjenPolledAt(ctx context.Context, caseNumber string, polledAt time.Time) error
	CreatePendingReview(ctx context.Context, reviewType string, payload []byte, worker string) error
	IsCaseTracked(ctx context.Context, caseNumber string) (bool, error)
	HasPartyMatchReview(ctx context.Context, politicianID, proceedingID string) (bool, error)
	UpsertWatcherCase(ctx context.Context, caseNumber, tribunalEndpoint, scandalID, proceedingID, addedBy string) error
	IsSubjectPurged(ctx context.Context, keys ...string) (bool, error)
}

type Worker struct {
	pg     djenStore
	mg     *memgraph.DB
	client *Client
	log    *slog.Logger
}

type Options struct {
	BaseURL     string
	CaseMode    bool
	NameMode    bool
	RematchMode bool
	DryRun      bool
	NameCap     int // items per name per run; 0 → defaultNameCap
	// PollLimit bounds how many watcher cases are polled in case mode (0 = all).
	PollLimit int
}

type RunStats struct {
	// case mode
	CasesLoaded    int `json:"cases_loaded"`
	CasesPolled    int `json:"cases_polled"`
	PartiesSeen    int `json:"parties_seen"`
	PartiesNew     int `json:"parties_new"`
	PoliticianHits int `json:"politician_hits_flagged"` // djen_party_match reviews
	PersonsLinked  int `json:"persons_linked"`          // Person + DEFENDANT_IN cited
	OrgsLinked     int `json:"orgs_linked"`             // Organization + DEFENDANT_IN cited + unknown_cnpj
	// name mode
	PoliticiansScanned int `json:"politicians_scanned"`
	NamesSearched      int `json:"names_searched"`
	CandidatesFlagged  int `json:"candidate_cases_flagged"`
	// CasesRegistered counts cases DJEN discovered and started watching without
	// human review: registering a case makes no claim about any person.
	CasesRegistered int `json:"cases_registered"`
	// SkippedUnregistrable counts discovered cases that could not be watched: a
	// case number that is not 20 digits, or a publication naming no tribunal (so
	// no DataJud endpoint resolves).
	SkippedUnregistrable int `json:"skipped_unregistrable"`
	SkippedTracked       int `json:"skipped_already_tracked"`
	SkippedExisting      int `json:"skipped_already_reviewed"`
	SkippedClass         int `json:"skipped_class_filter"`
	// SkippedTombstoned counts parties whose name was LGPD-purged: the Person or
	// Organization node is NOT re-created (resurrection guard, migration 008).
	SkippedTombstoned int `json:"skipped_tombstoned"`
	// shared
	PendingReviews int `json:"pending_reviews"`
	// PersonsRematched counts existing Person defendants re-tested against the
	// politician index (rematch mode).
	PersonsRematched int `json:"persons_rematched"`
	// FetchErrors counts DJEN lookups abandoned after the client exhausted its
	// retries. DJEN returns sporadic 500s, and a run scans hundreds of names; one bad lookup skips its item instead of discarding the whole run.
	FetchErrors int `json:"fetch_errors"`
}

func NewWorker(pg *psql.DB, mg *memgraph.DB, opts Options) *Worker {
	w := &Worker{mg: mg, client: NewClient(opts.BaseURL), log: slog.Default()}
	if pg != nil {
		w.pg = pg
	}
	return w
}

func (w *Worker) Run(ctx context.Context, opts Options) (*RunStats, error) {
	stats := &RunStats{}

	// The politician name index is needed by both modes (case mode for matching,
	// name mode for iterating names). Loaded once per run.
	pols, err := w.mg.ListPoliticianNames(ctx)
	if err != nil {
		return nil, err
	}
	index := buildPoliticianIndex(pols)

	if opts.CaseMode {
		if err := w.runCaseMode(ctx, opts, index, stats); err != nil {
			return nil, err
		}
	}
	if opts.NameMode {
		if err := w.runNameMode(ctx, opts, pols, stats); err != nil {
			return nil, err
		}
	}
	if opts.RematchMode {
		if err := w.runRematchMode(ctx, opts, index, stats); err != nil {
			return nil, err
		}
	}
	return stats, nil
}

// ─── Rematch mode ─────────────────────────────────────────────────────────────

// runRematchMode re-tests Person defendants we already discovered against the
// current politician index. A party is matched only once, at discovery, and then
// snapshotted out of future roster deltas; so defendants found while the
// politician base was small (or before a TSE import added the state-level
// offices) would stay anonymous forever. This is the only path by which a
// pre-2023 case, whose party list can never be re-fetched, gains a politician.
//
// Like discovery, a match never auto-creates the edge: it files a djen_party_match
// review carrying the Person node so the operator sees exactly what is being
// promoted.
func (w *Worker) runRematchMode(ctx context.Context, opts Options, index map[string]string, stats *RunStats) error {
	persons, err := w.mg.ListCitedPersons(ctx)
	if err != nil {
		return err
	}
	stats.PersonsRematched = len(persons)

	for _, p := range persons {
		polID, ok := matchPolitician(p.Name, index)
		if !ok {
			continue
		}
		exists, err := w.pg.HasPartyMatchReview(ctx, polID, p.ProceedingID)
		if err != nil {
			return err
		}
		if exists {
			stats.SkippedExisting++
			continue
		}
		stats.PoliticianHits++
		if opts.DryRun {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"case_number":   p.CaseNumber,
			"scandal_id":    p.ScandalID,
			"proceeding_id": p.ProceedingID,
			"politician_id": polID,
			"person_id":     p.PersonID,
			"name":          p.Name,
			"polo":          "P",
			"origin":        "rematch",
		})
		if err := w.pg.CreatePendingReview(ctx, "djen_party_match", payload, workerName); err != nil {
			return err
		}
		stats.PendingReviews++
	}
	return nil
}

// ─── Case mode ────────────────────────────────────────────────────────────────

func (w *Worker) runCaseMode(ctx context.Context, opts Options, index map[string]string, stats *RunStats) error {
	// DJEN uses its own poll cursor (djen_last_polled_at), independent of the
	// DataJud watcher's last_polled_at, so concluded cases are not starved.
	cases, err := w.pg.ListDjenCasesForPoll(ctx)
	if err != nil {
		return err
	}
	stats.CasesLoaded = len(cases)

	limit := opts.PollLimit
	if limit <= 0 || limit > len(cases) {
		limit = len(cases)
	}

	for i := 0; i < limit; i++ {
		c := cases[i]

		// Case numbers seeded via the backoffice may be in formatted CNJ form
		// ("5046512-94.2016.4.04.7000"); the DJEN API needs 20 bare digits.
		caseNumber := normalizeCaseNumber(c.CaseNumber)
		if len(caseNumber) != 20 {
			w.log.Warn("djen: case number is not 20 digits after normalization",
				"raw", c.CaseNumber, "normalized", caseNumber, "digits", len(caseNumber))
		}

		items, err := w.client.SearchByCaseNumber(ctx, caseNumber)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			stats.FetchErrors++
			w.log.Warn("djen: case lookup failed, skipping", "case", caseNumber, "err", err)
			continue
		}
		stats.CasesPolled++

		roster := rosterFromItems(items)
		stats.PartiesSeen += len(roster)

		existing, err := w.pg.ListDjenSnapshotKeys(ctx, c.CaseNumber)
		if err != nil {
			return err
		}
		delta := rosterDelta(roster, existing)
		stats.PartiesNew += len(delta)

		for _, party := range delta {
			// Only the passivo ("P") side are potential defendants.
			if party.Polo == "P" {
				// A trailing " E OUTROS (N)" marker appears in real data; strip
				// it before classification and matching.
				name := stripEOutros(party.Nome)
				switch {
				case isCompanyName(name):
					if err := w.handleCompanyParty(ctx, opts, c, caseNumber, name, party, stats); err != nil {
						return err
					}
				default:
					if polID, ok := matchPolitician(name, index); ok {
						// A name match on a Politician NEVER auto-creates an edge:
						// it goes to review with the communication as evidence.
						if !opts.DryRun {
							payload, _ := json.Marshal(map[string]any{
								"case_number":    caseNumber,
								"scandal_id":     c.ScandalID,
								"proceeding_id":  c.ProceedingID,
								"politician_id":  polID,
								"name":           name,
								"polo":           party.Polo,
								"tribunal":       party.Tribunal,
								"comunicacao_id": strconv.FormatInt(party.ComunicacaoID, 10),
								"link":           party.Link,
								"texto_snippet":  snippet(party.Texto, 500),
							})
							if err := w.pg.CreatePendingReview(ctx, "djen_party_match", payload, workerName); err != nil {
								return err
							}
							stats.PendingReviews++
						}
						stats.PoliticianHits++
					} else if err := w.handlePersonParty(ctx, opts, c, name, party, stats); err != nil {
						return err
					}
				}
			}

			// Snapshot every new party (any polo) so the next poll sees only
			// truly new entries.
			if !opts.DryRun {
				if err := w.pg.InsertDjenSnapshotParty(ctx, c.CaseNumber, partyKey(party.Nome, party.Polo), party.Nome, party.Polo); err != nil {
					return err
				}
			}
		}

		// Advance the DJEN poll cursor so the active/concluded cadence applies
		// on the next run.
		if !opts.DryRun {
			if err := w.pg.UpdateDjenPolledAt(ctx, c.CaseNumber, time.Now()); err != nil {
				return err
			}
		}
	}
	return nil
}

// handlePersonParty creates a name-only Person + DEFENDANT_IN "cited" edge for a
// passive party that did not match a Politician. DJEN carries no document, so the
// LGPD resurrection guard keys on the (purged) name: a tombstoned name is skipped
// rather than re-created.
func (w *Worker) handlePersonParty(ctx context.Context, opts Options, c psql.WatcherCase, name string, party Party, stats *RunStats) error {
	if opts.DryRun {
		stats.PersonsLinked++
		return nil
	}
	purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyName(name))
	if err != nil {
		return err
	}
	if purged {
		stats.SkippedTombstoned++
		return nil
	}
	personID, err := w.mg.UpsertDjenPerson(ctx, memgraph.DjenPersonUpsert{
		Name:          name,
		ComunicacaoID: strconv.FormatInt(party.ComunicacaoID, 10),
		Link:          party.Link,
		Tribunal:      party.Tribunal,
	})
	if err != nil {
		return err
	}
	if err := w.mg.EnsureDjenDefendantEdge(ctx, "Person", personID, c.ProceedingID, "cited"); err != nil {
		return err
	}
	stats.PersonsLinked++
	return nil
}

// handleCompanyParty routes a company-like passive party to an Organization node
// (name-only, DJEN provenance) with a DEFENDANT_IN "cited" edge, plus an
// "unknown_cnpj" review so a human can attach the CNPJ later. DJEN destinatarios
// carry no document, so companies must not become Person nodes.
func (w *Worker) handleCompanyParty(ctx context.Context, opts Options, c psql.WatcherCase, caseNumber, name string, party Party, stats *RunStats) error {
	if opts.DryRun {
		stats.OrgsLinked++
		return nil
	}
	// LGPD resurrection guard: DJEN has no document, so key on the purged name.
	purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyName(name))
	if err != nil {
		return err
	}
	if purged {
		stats.SkippedTombstoned++
		return nil
	}
	orgID, err := w.mg.UpsertDjenOrganization(ctx, memgraph.DjenOrganizationUpsert{
		Name:          name,
		ComunicacaoID: strconv.FormatInt(party.ComunicacaoID, 10),
		Link:          party.Link,
		Tribunal:      party.Tribunal,
	})
	if err != nil {
		return err
	}
	if err := w.mg.EnsureDjenDefendantEdge(ctx, "Organization", orgID, c.ProceedingID, "cited"); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"name":        name,
		"case_number": caseNumber,
		"source":      "djen",
		"link":        party.Link,
	})
	if err := w.pg.CreatePendingReview(ctx, "unknown_cnpj", payload, workerName); err != nil {
		return err
	}
	stats.PendingReviews++
	stats.OrgsLinked++
	return nil
}

// ─── Name mode ────────────────────────────────────────────────────────────────

func (w *Worker) runNameMode(ctx context.Context, opts Options, pols []memgraph.PoliticianNames, stats *RunStats) error {
	cap := opts.NameCap
	if cap <= 0 {
		cap = defaultNameCap
	}

	for _, pol := range pols {
		stats.PoliticiansScanned++
		for _, name := range namesFor(pol) {
			stats.NamesSearched++
			items, err := w.client.SearchByPartyName(ctx, name, cap)
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				stats.FetchErrors++
				w.log.Warn("djen: name lookup failed, skipping", "name", name, "err", err)
				continue
			}

			for caseNumber, group := range groupByProcesso(items) {
				// watcher_tracking.case_number and candidate payloads may be stored
				// in formatted CNJ form; both the dedup queries and this param are
				// normalized to digits-only so the comparison actually matches.
				caseDigits := normalizeCaseNumber(caseNumber)
				tracked, err := w.pg.IsCaseTracked(ctx, caseDigits)
				if err != nil {
					return err
				}
				if tracked {
					stats.SkippedTracked++
					continue
				}
				if !groupHasAllowedClass(group) {
					stats.SkippedClass++
					continue
				}

				if !opts.DryRun {
					// Registering a case asserts nothing about any person: it
					// creates the LegalProceeding and starts polling it. The claim
					// "this politician is a defendant" is a separate, name-only
					// inference and still goes to review (case mode / rematch).
					// So no human is needed here - the discovery is the case, and
					// its provenance (DJEN publication) is recorded on the case.
					registered, err := w.registerDiscoveredCase(ctx, caseDigits, group)
					if err != nil {
						return err
					}
					if registered {
						stats.CasesRegistered++
					} else {
						stats.SkippedUnregistrable++
					}
				}
				stats.CandidatesFlagged++
			}
		}
	}
	return nil
}

// registerDiscoveredCase starts watching a case DJEN surfaced, with no human in
// the loop. It creates the LegalProceeding and the watcher_tracking row so the
// DataJud watcher polls its status and DJEN case mode pulls its full party
// roster on the next run.
//
// The case is registered with no scandal: DJEN found it through one politician's
// name, which says nothing about which scandal (if any) it belongs to. An
// operator can attach it to a scandal later; until then it stands alone.
// It reports whether the case was actually registered: a malformed case number
// or a publication with no tribunal cannot be watched, and is logged and skipped
// rather than counted as registered.
func (w *Worker) registerDiscoveredCase(ctx context.Context, caseNumber string, group []Item) (bool, error) {
	if len(caseNumber) != 20 {
		w.log.Warn("djen: refusing to register a case whose number is not 20 digits", "case", caseNumber)
		return false, nil
	}
	endpoint := endpointForGroup(group)
	if endpoint == "" {
		w.log.Warn("djen: no tribunal on the publication; cannot resolve a DataJud endpoint", "case", caseNumber)
		return false, nil
	}

	lpID, err := w.mg.UpsertLegalProceedingByCase(ctx, memgraph.DataJudProceedingUpsert{CaseNumber: caseNumber})
	if err != nil {
		return false, err
	}
	if err := w.pg.UpsertWatcherCase(ctx, caseNumber, endpoint, "", lpID, workerName); err != nil {
		return false, err
	}
	return true, nil
}

func namesFor(pol memgraph.PoliticianNames) []string {
	out := make([]string, 0, 1+len(pol.Aliases))
	if strings.TrimSpace(pol.Name) != "" {
		out = append(out, pol.Name)
	}
	for _, a := range pol.Aliases {
		if strings.TrimSpace(a) != "" {
			out = append(out, a)
		}
	}
	return out
}

// tribunalEndpoint maps a tribunal sigla (e.g. "TRF4") to the DataJud public
// endpoint name ("api_publica_trf4"). Empty sigla yields empty string.
func tribunalEndpoint(sigla string) string {
	s := strings.ToLower(strings.TrimSpace(sigla))
	if s == "" {
		return ""
	}
	return "api_publica_" + s
}

// ─── Pure logic (unit-tested) ─────────────────────────────────────────────────

// Party is a deduplicated roster entry carrying the evidence from the first
// communication it was observed in.
type Party struct {
	Nome          string
	Polo          string
	ComunicacaoID int64
	Link          string
	Texto         string
	Tribunal      string
}

// rosterFromItems unions destinatarios across communications, deduplicating by
// (normalized nome, polo) and keeping the first-seen communication as evidence.
func rosterFromItems(items []Item) []Party {
	seen := map[string]int{}
	out := make([]Party, 0)
	for _, it := range items {
		for _, d := range it.Destinatarios {
			nome := strings.TrimSpace(d.Nome)
			polo := strings.TrimSpace(d.Polo)
			if nome == "" || polo == "" {
				continue
			}
			key := partyKey(nome, polo)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = len(out)
			out = append(out, Party{
				Nome:          nome,
				Polo:          polo,
				ComunicacaoID: d.ComunicacaoID,
				Link:          it.Link,
				Texto:         it.Texto,
				Tribunal:      it.SiglaTribunal,
			})
		}
	}
	return out
}

// rosterDelta returns roster parties whose key is not already in the snapshot.
func rosterDelta(roster []Party, existing map[string]bool) []Party {
	out := make([]Party, 0)
	for _, p := range roster {
		if existing[partyKey(p.Nome, p.Polo)] {
			continue
		}
		out = append(out, p)
	}
	return out
}

func partyKey(nome, polo string) string {
	return normalizeName(nome) + "|" + strings.ToUpper(strings.TrimSpace(polo))
}

// buildPoliticianIndex maps each normalized full name and alias to its
// politician id. Only full-name exact (normalized) matches are supported;
// substrings are intentionally not indexed.
func buildPoliticianIndex(pols []memgraph.PoliticianNames) map[string]string {
	index := map[string]string{}
	for _, p := range pols {
		for _, n := range namesFor(p) {
			key := normalizeName(n)
			if key == "" {
				continue
			}
			if _, ok := index[key]; !ok {
				index[key] = p.ID
			}
		}
	}
	return index
}

func matchPolitician(name string, index map[string]string) (string, bool) {
	id, ok := index[normalizeName(name)]
	return id, ok
}

func groupByProcesso(items []Item) map[string][]Item {
	out := map[string][]Item{}
	for _, it := range items {
		num := strings.TrimSpace(it.NumeroProcesso)
		if num == "" {
			continue
		}
		out[num] = append(out[num], it)
	}
	return out
}

func groupHasAllowedClass(group []Item) bool {
	for _, it := range group {
		if isCriminalOrImprobidadeClass(it.NomeClasse, it.CodigoClasse) {
			return true
		}
	}
	return false
}

// classAllowlist gates name-mode candidates to criminal and improbidade
// proceedings only. Single source of truth. TPU class codes (codigoClasse) are
// the precise signal; nomeClasse keyword matching is the resilient fallback
// since code coverage varies across tribunals.
var (
	djenClassCodeAllowlist = map[string]bool{
		"283": true, // Ação Penal - Procedimento Ordinário
		"282": true, // Ação Penal de Competência do Júri
		"284": true, // Ação Penal - Procedimento Sumário
		"285": true, // Ação Penal - Procedimento Sumaríssimo
		"280": true, // Inquérito Policial
		"64":  true, // Ação Civil de Improbidade Administrativa
	}
	djenClassNameKeywords = []string{
		"penal", "crime", "criminal", "improbidade", "inquerito",
		"tribunal do juri",
	}
	// juriWordRe matches "juri" (júri, accents already stripped) only as a whole
	// word - so tribunal-do-júri classes pass, but JURISDICAO and JURIDICA (which
	// merely start with "juri") do NOT. This fixes a real false positive.
	juriWordRe = regexp.MustCompile(`\bjuri\b`)
)

func isCriminalOrImprobidadeClass(nomeClasse, codigoClasse string) bool {
	if djenClassCodeAllowlist[strings.TrimSpace(codigoClasse)] {
		return true
	}
	norm := normalizeName(nomeClasse) // uppercase + accent-strip
	lower := strings.ToLower(norm)
	for _, kw := range djenClassNameKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return juriWordRe.MatchString(lower)
}

// ─── Case-number normalization ────────────────────────────────────────────────

var nonDigitRe = regexp.MustCompile(`\D`)

// normalizeCaseNumber strips every non-digit character, turning a formatted CNJ
// number ("5046512-94.2016.4.04.7000") into the bare 20-digit form the DJEN API
// requires. The caller warns if the result is not 20 digits.
func normalizeCaseNumber(s string) string {
	return nonDigitRe.ReplaceAllString(s, "")
}

// ─── Party-name cleanup ───────────────────────────────────────────────────────

// eOutrosRe matches a trailing " E OUTROS (N)" / " E OUTRO" marker that appears
// in real DJEN party names when a case has several co-parties.
var eOutrosRe = regexp.MustCompile(`(?i)\s+E\s+OUTROS?(\s*\([0-9]+\))?\s*$`)

// stripEOutros removes the trailing co-party marker before classification and
// matching so "FULANO DE TAL E OUTROS (3)" is treated as "FULANO DE TAL".
func stripEOutros(name string) string {
	return strings.TrimSpace(eOutrosRe.ReplaceAllString(name, ""))
}

// ─── Company-name heuristic ───────────────────────────────────────────────────

var (
	// companyFinalTokenMarkers must appear as the last whitespace-delimited token
	// to count. These are short/ambiguous corporate suffixes (SA is the classic
	// case). Punctuated forms "S/A" and "S.A." fold to "SA" before matching.
	companyFinalTokenMarkers = map[string]bool{
		"SA":     true,
		"LTDA":   true,
		"EIRELI": true,
		"MEI":    true,
		"ME":     true,
		"EPP":    true,
		"CIA":    true,
	}
	// companyTokenMarkers count when they appear as a whole token anywhere in the
	// name (exact token equality is a word-boundary match).
	companyTokenMarkers = map[string]bool{
		"COMPANHIA":     true,
		"CONSORCIO":     true,
		"CONSTRUTORA":   true,
		"INCORPORADORA": true,
		"ASSOCIACAO":    true,
		"FUNDACAO":      true,
		"INSTITUTO":     true,
		"MUNICIPIO":     true,
		"PREFEITURA":    true,
		"UNIAO":         true,
		"BANCO":         true,
	}
	// companyPhraseMarkers are multi-word public/corporate markers matched as a
	// (word-bounded) substring of the normalized name.
	companyPhraseMarkers = []string{
		"ESTADO DE",
		"MINISTERIO PUBLICO",
	}
)

// isCompanyName reports whether a passive party name looks like a company or a
// public body rather than a natural person. DJEN destinatarios carry no
// document, so this heuristic keeps corporate/institutional parties from
// becoming Person nodes. Table-driven and unit-tested.
func isCompanyName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return false
	}

	// Short suffixes (SA, LTDA, ...) are checked on an accent-PRESERVING form so
	// the corporate "S.A."/"S/A"/"SA" is distinguished from the common surname
	// "Sá" (which accent-strips to "SA"). Corporate punctuation is folded away.
	foldedUpper := strings.NewReplacer("/", "", ".", "").Replace(upper)
	if uf := strings.Fields(foldedUpper); len(uf) > 0 && companyFinalTokenMarkers[uf[len(uf)-1]] {
		return true
	}

	// Full-word tokens/phrases are accent-insensitive, so use the normalized form.
	folded := strings.NewReplacer("/", "", ".", "").Replace(normalizeName(name))
	folded = strings.Join(strings.Fields(folded), " ")
	for _, f := range strings.Fields(folded) {
		if companyTokenMarkers[f] {
			return true
		}
	}
	for _, p := range companyPhraseMarkers {
		if strings.Contains(folded, p) {
			return true
		}
	}
	return false
}

// normalizeName uppercases, strips Portuguese accents and collapses whitespace,
// for exact (not substring) name comparison.
func normalizeName(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = stripAccents(s)
	return strings.Join(strings.Fields(s), " ")
}

func stripAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := accentMap[r]; ok {
			b.WriteRune(repl)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// accentMap folds the accented characters found in Brazilian names to ASCII.
var accentMap = map[rune]rune{
	'Á': 'A', 'À': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A',
	'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E',
	'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I',
	'Ó': 'O', 'Ò': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O',
	'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U',
	'Ç': 'C', 'Ñ': 'N',
}

// snippet strips HTML tags and returns at most max runes of readable text.
func snippet(html string, max int) string {
	text := stripTags(html)
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > max {
		return string(runes[:max])
	}
	return text
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// endpointForGroup resolves the DataJud index to poll from the tribunal that
// published the case ("TRF1" → "api_publica_trf1"). Returns "" when no
// publication in the group names a tribunal, in which case the case cannot be
// watched and is not registered.
func endpointForGroup(group []Item) string {
	for _, it := range group {
		if sigla := strings.TrimSpace(it.SiglaTribunal); sigla != "" {
			return tribunalEndpoint(sigla)
		}
	}
	return ""
}
