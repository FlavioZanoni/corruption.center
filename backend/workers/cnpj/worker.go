package cnpj

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
)

const workerName = "cnpj"

// maxHops bounds how deep an OWNED_BY shell chain is followed within a single
// run. Root orgs are depth 0; a corporate partner is depth 1; its partner depth
// 2. Beyond this the chain is not expanded this run; newly created partner orgs
// keep their un-enriched flag and are picked up by a later pass.
//
// ponytail: hard ceiling of 2 hops per run. Deep shell chains resolve across
// multiple scheduled runs instead of one unbounded recursive crawl; good enough
// and self-throttling. Raise only if a real case demonstrably needs it.
const maxHops = 2

// cnpjStore is the subset of *psql.DB the worker consults. Declared as an
// interface so the tombstone skip paths can be unit-tested with a fake.
type cnpjStore interface {
	CreatePendingReview(ctx context.Context, reviewType string, payload []byte, worker string) error
	IsSubjectPurged(ctx context.Context, keys ...string) (bool, error)
}

type Worker struct {
	pg     cnpjStore
	mg     *memgraph.DB
	client *Client
	log    *slog.Logger
}

type Options struct {
	BaseURL      string
	RatePerMin   int
	Limit        int    // max root orgs to enrich (0 = all needing enrichment)
	DryRun       bool   // fetch + classify but write nothing
	SingleCNPJ   string // enrich just this CNPJ (testing); bypasses the queue query
	ReenrichDays int    // staleness threshold in days (0 or negative disables re-enrichment)
}

type RunStats struct {
	OrgsEnriched   int `json:"orgs_enriched"`
	OrgsNotFound   int `json:"orgs_not_found"`    // provider 404
	QSAIndividuals int `json:"qsa_individuals"`   // masked-CPF rows seen
	QSACompanies   int `json:"qsa_companies"`     // CNPJ rows seen
	QSAUnknown     int `json:"qsa_unknown"`       // unclassifiable socio docs
	PersonsLinked  int `json:"persons_linked"`    // Person + CONTROLS
	OrgsChained    int `json:"orgs_chained"`      // partner Organization + OWNED_BY
	OrgsQueued     int `json:"orgs_queued_chain"` // partners enqueued for same-run enrichment
	PoliticianHits int `json:"politician_hits"`   // masked-CPF matched a Politician
	PendingReviews int `json:"pending_reviews"`   // possible_politician_in_qsa
	// SkippedTombstoned counts QSA members whose subject was LGPD-purged: the
	// Person/Organization node is NOT re-created (resurrection guard, migration 008).
	SkippedTombstoned int `json:"skipped_tombstoned"`
	// FetchErrors counts CNPJs the provider would not enrich (a malformed document in
	// the source data, a transient failure). They are skipped, not fatal — the run
	// used to die on the first one.
	FetchErrors int `json:"fetch_errors"`
}

func NewWorker(pg *psql.DB, mg *memgraph.DB, opts Options) *Worker {
	w := &Worker{
		mg:     mg,
		client: NewClient(opts.BaseURL, opts.RatePerMin),
		log:    slog.Default(),
	}
	if pg != nil {
		w.pg = pg
	}
	return w
}

// job is one org to enrich, tagged with its shell-chain depth.
type job struct {
	orgID string
	cnpj  string
	depth int
}

func (w *Worker) Run(ctx context.Context, opts Options) (*RunStats, error) {
	stats := &RunStats{}

	queue, err := w.rootJobs(ctx, opts)
	if err != nil {
		return nil, err
	}

	visited := map[string]bool{}
	for len(queue) > 0 {
		j := queue[0]
		queue = queue[1:]
		key := digitsOnly(j.cnpj)
		if visited[key] {
			continue
		}
		visited[key] = true

		children, err := w.enrichOne(ctx, opts, j, stats)
		if err != nil {
			// One unenrichable CNPJ must not discard the run. A live pass died on
			// "CNPJ 01.006.599/0001-02 inválido" — a bad check digit in the CGU data,
			// which we do not get to fix and which will still be bad tomorrow — and
			// took the other 14,000 companies with it after naming 390.
			//
			// A cancelled context is different: that is us stopping, not the record
			// being bad, and it must still end the run.
			if ctx.Err() != nil {
				return nil, err
			}
			stats.FetchErrors++
			w.log.Warn("cnpj: enrichment failed, skipping", "cnpj", j.cnpj, "err", err)
			continue
		}
		// Bounded-depth expansion only happens on real writes; dry-run reports the
		// root(s) only (see enrichOne).
		for _, child := range children {
			if visited[digitsOnly(child.cnpj)] {
				continue
			}
			if child.depth <= maxHops {
				queue = append(queue, child)
				stats.OrgsQueued++
			}
		}
	}
	return stats, nil
}

// rootJobs seeds the queue: a single CNPJ (ensuring its node exists) or the set
// of Organization nodes needing enrichment.
func (w *Worker) rootJobs(ctx context.Context, opts Options) ([]job, error) {
	if strings.TrimSpace(opts.SingleCNPJ) != "" {
		digits := digitsOnly(opts.SingleCNPJ)
		if len(digits) != 14 {
			return nil, fmt.Errorf("cnpj: invalid --cnpj %q", opts.SingleCNPJ)
		}
		orgID := "org_" + digits
		if !opts.DryRun {
			// Ensure the node exists so QSA edges have an anchor. Reuses the
			// shared by-cnpj upsert (does not mutate other workers' code).
			id, _, err := w.mg.EnsureOrganizationByCNPJ(ctx, digits)
			if err != nil {
				return nil, err
			}
			orgID = id
		}
		return []job{{orgID: orgID, cnpj: digits, depth: 0}}, nil
	}

	// Staleness cutoff: an org whose enriched_at is older than this is re-fetched.
	// ReenrichDays <= 0 disables the time-based pass — an empty cutoff is smaller
	// than every RFC3339 timestamp, so `enriched_at < cutoff` never matches and
	// only never-enriched or never-stamped orgs are picked up.
	cutoff := ""
	if opts.ReenrichDays > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -opts.ReenrichDays).Format(time.RFC3339)
	}
	orgs, err := w.mg.ListOrganizationsNeedingEnrichment(ctx, opts.Limit, cutoff)
	if err != nil {
		return nil, err
	}
	jobs := make([]job, 0, len(orgs))
	for _, o := range orgs {
		jobs = append(jobs, job{orgID: o.ID, cnpj: o.CNPJ, depth: 0})
	}
	return jobs, nil
}

// enrichOne fetches one CNPJ, writes the enrichment (unless dry-run) and processes
// its QSA. It returns the corporate-partner jobs discovered (empty on dry-run).
func (w *Worker) enrichOne(ctx context.Context, opts Options, j job, stats *RunStats) ([]job, error) {
	resp, err := w.client.FetchCNPJ(ctx, j.cnpj)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		stats.OrgsNotFound++
		w.log.Warn("cnpj: provider returned no record", "cnpj", j.cnpj)
		return nil, nil
	}

	sourceURL := w.client.baseURL + "/" + digitsOnly(j.cnpj)
	enrichment := mapEnrichment(resp, j.cnpj, sourceURL)

	orgID := j.orgID
	if !opts.DryRun {
		enrichedAt := time.Now().UTC().Format(time.RFC3339)
		id, err := w.mg.UpdateOrganizationEnrichment(ctx, enrichment, enrichedAt)
		if err != nil {
			return nil, err
		}
		orgID = id
	}
	stats.OrgsEnriched++

	var children []job
	for _, entry := range resp.QSA {
		switch classifyDoc(entry.CNPJCPFDoSocio) {
		case docIndividual:
			stats.QSAIndividuals++
			if err := w.handleIndividual(ctx, opts, orgID, enrichment.CNPJ, sourceURL, entry, stats); err != nil {
				return nil, err
			}
		case docCompany:
			stats.QSACompanies++
			child, err := w.handleCompany(ctx, opts, j, orgID, sourceURL, entry, stats)
			if err != nil {
				return nil, err
			}
			if child != nil {
				children = append(children, *child)
			}
		default:
			stats.QSAUnknown++
			w.log.Warn("cnpj: unclassifiable socio document",
				"cnpj", j.cnpj, "nome_socio", entry.NomeSocio, "doc", entry.CNPJCPFDoSocio)
		}
	}
	return children, nil
}

// handleIndividual routes a masked-CPF QSA member: a Politician match becomes a
// possible_politician_in_qsa review (never an auto-link); no match becomes a
// Person + CONTROLS edge.
func (w *Worker) handleIndividual(ctx context.Context, opts Options, orgID, orgCNPJ, sourceURL string, entry QSAEntry, stats *RunStats) error {
	var candidates []memgraph.PoliticianMatch
	if middle, ok := maskedCPFMiddleSix(entry.CNPJCPFDoSocio); ok {
		var err error
		candidates, err = w.mg.MatchPoliticiansByMaskedCPF(ctx, middle)
		if err != nil {
			return err
		}
	}

	if len(candidates) > 0 {
		stats.PoliticianHits++
		if !opts.DryRun {
			payload := politicianReviewPayload(orgID, orgCNPJ, sourceURL, entry, candidates)
			raw, _ := json.Marshal(payload)
			if err := w.pg.CreatePendingReview(ctx, "possible_politician_in_qsa", raw, workerName); err != nil {
				return err
			}
			stats.PendingReviews++
		}
		return nil
	}

	// No politician match → name-only Person + CONTROLS edge.
	if !opts.DryRun {
		// LGPD resurrection guard: QSA individuals expose only a masked CPF (no
		// full digits), so key on the purged name.
		purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyName(entry.NomeSocio))
		if err != nil {
			return err
		}
		if purged {
			stats.SkippedTombstoned++
			return nil
		}
		if _, err := w.mg.UpsertQSAPerson(ctx, orgID, memgraph.QSAPersonUpsert{
			Name:          entry.NomeSocio,
			MaskedCPF:     entry.CNPJCPFDoSocio,
			Qualification: entry.QualificacaoSocio,
			SourceCNPJ:    orgCNPJ,
			SourceURL:     sourceURL,
		}); err != nil {
			return err
		}
	}
	stats.PersonsLinked++
	return nil
}

// handleCompany routes a corporate QSA member to an Organization + OWNED_BY edge
// and returns a job to enrich the partner within this run if newly created and
// within the depth ceiling. Returns nil on dry-run.
func (w *Worker) handleCompany(ctx context.Context, opts Options, parent job, orgID, sourceURL string, entry QSAEntry, stats *RunStats) (*job, error) {
	if opts.DryRun {
		return nil, nil
	}
	partnerCNPJ := digitsOnly(entry.CNPJCPFDoSocio)
	// LGPD resurrection guard: a purged partner CNPJ must not be re-created as an
	// Organization node, and its shell chain must not be followed.
	purged, err := w.pg.IsSubjectPurged(ctx, psql.TombstoneKeyCNPJ(partnerCNPJ))
	if err != nil {
		return nil, err
	}
	if purged {
		stats.SkippedTombstoned++
		return nil, nil
	}
	partnerID, created, err := w.mg.UpsertQSAOrganization(ctx, orgID, partnerCNPJ, entry.QualificacaoSocio, sourceURL)
	if err != nil {
		return nil, err
	}
	stats.OrgsChained++
	if !created {
		return nil, nil
	}
	return &job{orgID: partnerID, cnpj: partnerCNPJ, depth: parent.depth + 1}, nil
}

// ─── Pure logic (unit-tested) ─────────────────────────────────────────────────

type docKind int

const (
	docUnknown docKind = iota
	docIndividual
	docCompany
)

// classifyDoc classifies a QSA cnpj_cpf_do_socio value. A masked CPF
// ("***641988**") is an individual; a full 14-digit CNPJ is a company; a full
// 11-digit CPF is treated as an individual; anything else is unknown.
func classifyDoc(doc string) docKind {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return docUnknown
	}
	if strings.Contains(doc, "*") {
		// Masked form: individuals only. Must expose exactly 6 middle digits.
		if len(digitsOnly(doc)) == 6 {
			return docIndividual
		}
		return docUnknown
	}
	switch len(digitsOnly(doc)) {
	case 14:
		return docCompany
	case 11:
		return docIndividual
	default:
		return docUnknown
	}
}

// maskedCPFMiddleSix extracts the 6 visible middle digits of a masked CPF
// (***XXXXXX**), the projection MatchPoliticiansByMaskedCPF expects. A full CPF
// yields its own middle six. Returns false when the digit count is not 6 or 11.
func maskedCPFMiddleSix(doc string) (string, bool) {
	digits := digitsOnly(doc)
	switch len(digits) {
	case 6:
		return digits, true
	case 11:
		// Full CPF: the masked middle is positions 4-9 (0-indexed 3..8).
		return digits[3:9], true
	default:
		return "", false
	}
}

// mapEnrichment maps a provider response onto the graph enrichment struct.
func mapEnrichment(resp *CNPJResponse, cnpj, sourceURL string) memgraph.OrganizationEnrichment {
	return memgraph.OrganizationEnrichment{
		CNPJ:            cnpj,
		Name:            strings.TrimSpace(resp.RazaoSocial),
		Active:          isActive(resp.DescricaoSituacaoCadastral),
		Type:            strings.TrimSpace(resp.NaturezaJuridica),
		UF:              strings.TrimSpace(resp.UF),
		ShareCapitalBRL: resp.CapitalSocial,
		MainActivity:    strings.TrimSpace(resp.CNAEFiscalDescricao),
		SourceURL:       sourceURL,
	}
}

// isActive maps descricao_situacao_cadastral to a boolean. Only "ATIVA" is
// active; Baixada/Suspensa/Nula/Inapta are not.
func isActive(descricao string) bool {
	return strings.EqualFold(strings.TrimSpace(descricao), "ativa")
}

func politicianReviewPayload(orgID, orgCNPJ, sourceURL string, entry QSAEntry, candidates []memgraph.PoliticianMatch) map[string]any {
	cands := make([]map[string]string, 0, len(candidates))
	for _, c := range candidates {
		cands = append(cands, map[string]string{"id": c.ID, "name": c.Name})
	}
	payload := map[string]any{
		"organization_id":   orgID,
		"organization_cnpj": orgCNPJ,
		"socio_name":        entry.NomeSocio,
		"masked_cpf":        entry.CNPJCPFDoSocio,
		"qualification":     entry.QualificacaoSocio,
		"source":            "cnpj",
		"source_url":        sourceURL,
		"candidates":        cands,
	}
	if len(candidates) == 1 {
		payload["politician_id"] = candidates[0].ID
	}
	return payload
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
