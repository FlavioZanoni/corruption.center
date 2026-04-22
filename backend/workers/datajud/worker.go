package datajud

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
)

type Worker struct {
	pg     *psql.DB
	mg     *memgraph.DB
	client *Client
}

type Options struct {
	APIBase         string
	APIKey          string
	ProbeCaseNumber string
	ProbeTribunal   string
	VerifyTPUCodes  bool
	StrictVerify    bool
	PollLimit       int
	EnableWrites    bool
}

type RunStats struct {
	CasesLoaded       int
	CasesPolled       int
	CasesRestricted   int
	CasesNotFound     int
	CasesUpdated      int
	ProbeFieldsOK     bool
	ProbeHasPartes    bool
	ProbeHasRelated   bool
	TPUVerificationOK bool
	VerificationNotes []string
	DefendantsLinked  int
	PendingReviews    int
	RelatedCasesAdded int
}

func NewWorker(ctx context.Context, pg *psql.DB, mg *memgraph.DB, opts Options) (*Worker, error) {
	client, err := NewClient(ctx, opts.APIBase, opts.APIKey)
	if err != nil {
		return nil, err
	}
	return &Worker{pg: pg, mg: mg, client: client}, nil
}

func (w *Worker) Run(ctx context.Context, opts Options) (*RunStats, error) {
	stats := &RunStats{}
	if opts.VerifyTPUCodes {
		ok, err := VerifyMovementCodes(ctx)
		if err != nil {
			if opts.StrictVerify {
				return nil, err
			}
			stats.VerificationNotes = append(stats.VerificationNotes, fmt.Sprintf("TPU verification skipped after lookup error: %v", err))
		}
		stats.TPUVerificationOK = ok
		if !ok && opts.StrictVerify {
			return nil, fmt.Errorf("datajud: TPU code verification failed for required movements")
		}
		if !ok && !opts.StrictVerify {
			stats.VerificationNotes = append(stats.VerificationNotes, "TPU verification did not confirm all required movement labels; continuing in non-strict mode")
		}
	}

	if strings.TrimSpace(opts.ProbeCaseNumber) != "" && strings.TrimSpace(opts.ProbeTribunal) != "" {
		raw, err := w.client.SearchByCaseNumber(ctx, opts.ProbeTribunal, opts.ProbeCaseNumber)
		if err != nil {
			if opts.StrictVerify {
				return nil, fmt.Errorf("datajud: probe call failed: %w", err)
			}
			stats.VerificationNotes = append(stats.VerificationNotes, fmt.Sprintf("probe call failed in non-strict mode: %v", err))
		}
		if raw == nil {
			if opts.StrictVerify {
				return nil, fmt.Errorf("datajud: probe case not found")
			}
			stats.VerificationNotes = append(stats.VerificationNotes, "probe case not found in non-strict mode")
		} else {
			stats.ProbeFieldsOK, stats.ProbeHasPartes, stats.ProbeHasRelated = probeCapabilities(raw)
			if !stats.ProbeFieldsOK {
				if opts.StrictVerify {
					return nil, fmt.Errorf("datajud: probe response missing core fields (numeroProcesso or movimentos)")
				}
				stats.VerificationNotes = append(stats.VerificationNotes, "probe response missing core fields; continuing in non-strict mode")
			}
			if !stats.ProbeHasPartes {
				stats.VerificationNotes = append(stats.VerificationNotes, "probe response did not include partes[].documento (tribunal/dataset variability)")
			}
			if !stats.ProbeHasRelated {
				stats.VerificationNotes = append(stats.VerificationNotes, "probe response did not include processoRelacionado entries (tribunal/dataset variability)")
			}
		}
	}

	cases, err := w.pg.ListWatcherCasesForPoll(ctx)
	if err != nil {
		return nil, err
	}
	stats.CasesLoaded = len(cases)

	limit := opts.PollLimit
	if limit <= 0 || limit > len(cases) {
		limit = len(cases)
	}

	for i := 0; i < limit; i++ {
		c := cases[i]
		src, err := w.client.SearchByCaseNumber(ctx, c.TribunalEndpoint, c.CaseNumber)
		if err != nil {
			return nil, err
		}
		stats.CasesPolled++
		if src == nil {
			stats.CasesNotFound++
			_ = w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, c.LastMovementID, c.Status, time.Now().UTC())
			continue
		}
		if src.NivelSigilo > 0 {
			stats.CasesRestricted++
			_ = w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, c.LastMovementID, c.Status, time.Now().UTC())
			continue
		}

		if opts.EnableWrites {
			if err := w.applyCaseWrites(ctx, c, src, stats); err != nil {
				return nil, err
			}
		}

		last := maxMovementID(src.Movimentos)
		lastStr := c.LastMovementID
		if last != "" {
			lastStr = &last
		}
		if err := w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, lastStr, c.Status, time.Now().UTC()); err != nil {
			return nil, err
		}
		stats.CasesUpdated++
	}

	return stats, nil
}

func (w *Worker) applyCaseWrites(ctx context.Context, c psql.WatcherCase, src *CaseSource, stats *RunStats) error {
	if w.mg == nil {
		return fmt.Errorf("datajud: memgraph writer is required when --enable-writes is true")
	}

	proceedingID, err := w.mg.UpsertLegalProceedingByCase(ctx, memgraph.DataJudProceedingUpsert{
		CaseNumber: src.NumeroProcesso,
		Court:      courtName(src.OrgaoJulgador),
		Type:       proceedingTypeFromClasse(src.Classe),
		Status:     "ongoing",
		Assuntos:   assuntosCodes(src.Assuntos),
		DateFiled:  parseDate(src.DataAjuizamento),
	})
	if err != nil {
		return err
	}

	if c.ScandalID != "" {
		if err := w.mg.EnsureInvestigatesEdge(ctx, proceedingID, c.ScandalID); err != nil {
			return err
		}
	}

	outcome := deriveOutcome(src.Movimentos)
	if outcome == "prescribed" || movementExists(src.Movimentos, "132") || movementExists(src.Movimentos, "246") {
		if err := w.mg.UpdateProceedingStatusByID(ctx, proceedingID, "concluded"); err != nil {
			return err
		}
	}

	if err := w.handleParties(ctx, c, proceedingID, outcome, src.Partes, stats); err != nil {
		return err
	}

	if err := w.handleRelatedCases(ctx, c, src, stats); err != nil {
		return err
	}

	return nil
}

func (w *Worker) handleParties(ctx context.Context, c psql.WatcherCase, proceedingID, outcome string, partes []map[string]any, stats *RunStats) error {
	for _, p := range partes {
		doc := strings.TrimSpace(fmt.Sprintf("%v", p["documento"]))
		name := strings.TrimSpace(fmt.Sprintf("%v", p["nome"]))
		docDigits := digitsOnly(doc)

		if len(docDigits) == 11 || len(docDigits) == 14 {
			match, err := w.mg.FindDefendantByDocument(ctx, docDigits)
			if err != nil {
				return err
			}
			if match != nil {
				if err := w.mg.EnsureDefendantInEdge(ctx, match.NodeType, match.NodeID, proceedingID, outcome); err != nil {
					return err
				}
				stats.DefendantsLinked++
				continue
			}

			if len(docDigits) == 11 {
				personID, err := w.mg.UpsertUnknownPerson(ctx, name, maskCPF(docDigits))
				if err != nil {
					return err
				}
				if err := w.mg.EnsureDefendantInEdge(ctx, "Person", personID, proceedingID, outcome); err != nil {
					return err
				}
				stats.DefendantsLinked++
				payload, _ := json.Marshal(map[string]any{"case_number": c.CaseNumber, "documento": maskCPF(docDigits), "name": name})
				if err := w.pg.CreatePendingReview(ctx, "unknown_cpf", payload, "datajud_watcher"); err != nil {
					return err
				}
				stats.PendingReviews++
				continue
			}

			if len(docDigits) == 14 {
				orgID, err := w.mg.UpsertUnknownOrganization(ctx, docDigits)
				if err != nil {
					return err
				}
				if err := w.mg.EnsureDefendantInEdge(ctx, "Organization", orgID, proceedingID, outcome); err != nil {
					return err
				}
				stats.DefendantsLinked++
				payload, _ := json.Marshal(map[string]any{"case_number": c.CaseNumber, "documento": docDigits, "name": name, "trigger": "cnpj_enricher"})
				if err := w.pg.CreatePendingReview(ctx, "unknown_cnpj", payload, "datajud_watcher"); err != nil {
					return err
				}
				stats.PendingReviews++
				continue
			}
		}

		if name != "" {
			polID, err := w.mg.SearchPoliticianByNameState(ctx, name)
			if err != nil {
				return err
			}
			if polID != "" {
				payload, _ := json.Marshal(map[string]any{"case_number": c.CaseNumber, "name": name, "politician_id": polID})
				if err := w.pg.CreatePendingReview(ctx, "cpf_partial_match", payload, "datajud_watcher"); err != nil {
					return err
				}
				stats.PendingReviews++
				continue
			}

			personID, err := w.mg.UpsertUnknownPerson(ctx, name, "")
			if err != nil {
				return err
			}
			if err := w.mg.EnsureDefendantInEdge(ctx, "Person", personID, proceedingID, outcome); err != nil {
				return err
			}
			stats.DefendantsLinked++
			payload, _ := json.Marshal(map[string]any{"case_number": c.CaseNumber, "name": name})
			if err := w.pg.CreatePendingReview(ctx, "unknown_cpf", payload, "datajud_watcher"); err != nil {
				return err
			}
			stats.PendingReviews++
		}
	}
	return nil
}

func (w *Worker) handleRelatedCases(ctx context.Context, c psql.WatcherCase, src *CaseSource, stats *RunStats) error {
	related := parseRelatedCases(src)
	if movementExists(src.Movimentos, "981") {
		related = append(related, parseDesmembramentoFromMovements(src.Movimentos)...)
	}

	for _, rel := range related {
		caseNum := strings.TrimSpace(rel.CaseNumber)
		tribunal := strings.TrimSpace(rel.TribunalEndpoint)
		if caseNum == "" {
			continue
		}
		if tribunal == "" {
			tribunal = c.TribunalEndpoint
		}

		tracked, err := w.pg.IsWatcherCaseTracked(ctx, caseNum)
		if err != nil {
			return err
		}
		if tracked {
			continue
		}

		relatedSrc, err := w.client.SearchByCaseNumber(ctx, tribunal, caseNum)
		if err != nil {
			return err
		}
		if relatedSrc == nil {
			continue
		}

		lpID, err := w.mg.UpsertLegalProceedingByCase(ctx, memgraph.DataJudProceedingUpsert{
			CaseNumber: relatedSrc.NumeroProcesso,
			Court:      courtName(relatedSrc.OrgaoJulgador),
			Type:       proceedingTypeFromClasse(relatedSrc.Classe),
			Status:     "ongoing",
			Assuntos:   assuntosCodes(relatedSrc.Assuntos),
			DateFiled:  parseDate(relatedSrc.DataAjuizamento),
		})
		if err != nil {
			return err
		}

		scandalID := c.ScandalID
		if scandalID == "" {
			scandalID, _ = w.pg.GetProceedingScandalID(ctx, c.CaseNumber)
		}
		if scandalID != "" {
			if err := w.mg.EnsureInvestigatesEdge(ctx, lpID, scandalID); err != nil {
				return err
			}
		}
		if err := w.pg.UpsertWatcherCase(ctx, relatedSrc.NumeroProcesso, tribunal, scandalID, lpID, "watcher"); err != nil {
			return err
		}
		stats.RelatedCasesAdded++
	}

	return nil
}

func hasRequiredFields(src *CaseSource) bool {
	coreOK, _, _ := probeCapabilities(src)
	return coreOK
}

func probeCapabilities(src *CaseSource) (coreOK, hasPartesDocument, hasRelated bool) {
	if src == nil {
		return false, false, false
	}
	numero := strings.TrimSpace(src.NumeroProcesso)
	if numero == "" && src.Raw != nil {
		numero = strings.TrimSpace(fmt.Sprintf("%v", src.Raw["numeroProcesso"]))
	}

	partes := src.Partes
	if len(partes) == 0 && src.Raw != nil {
		if v, ok := src.Raw["partes"].([]any); ok {
			partes = make([]map[string]any, 0, len(v))
			for _, it := range v {
				if m, ok := it.(map[string]any); ok {
					partes = append(partes, m)
				}
			}
		}
	}
	for _, p := range partes {
		if _, ok := p["documento"]; ok {
			hasPartesDocument = true
			break
		}
	}

	movsFieldExists := len(src.Movimentos) > 0
	if !movsFieldExists && src.Raw != nil {
		_, movsFieldExists = src.Raw["movimentos"]
	}

	if len(src.ProcessoRelacionado) > 0 {
		hasRelated = true
	} else if src.Raw != nil {
		if v, ok := src.Raw["processoRelacionado"].([]any); ok && len(v) > 0 {
			hasRelated = true
		}
	}

	coreOK = numero != "" && movsFieldExists
	return coreOK, hasPartesDocument, hasRelated
}

func maxMovementID(movs []map[string]any) string {
	var max int64
	for _, m := range movs {
		id := strings.TrimSpace(fmt.Sprintf("%v", m["id"]))
		if id == "" || id == "<nil>" {
			continue
		}
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	if max == 0 {
		return ""
	}
	return strconv.FormatInt(max, 10)
}

func movementExists(movs []map[string]any, code string) bool {
	for _, m := range movs {
		if movementCode(m) == code {
			return true
		}
	}
	return false
}

func movementCode(m map[string]any) string {
	v := m["codigo"]
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func deriveOutcome(movs []map[string]any) string {
	outcome := "pending"
	for _, m := range movs {
		switch movementCode(m) {
		case "60":
			outcome = "convicted"
		case "61":
			outcome = "acquitted"
		case "901":
			outcome = "prescribed"
		case "848":
			if outcome == "pending" {
				text := movementText(m)
				if strings.Contains(text, "conden") || strings.Contains(text, "procedente") {
					outcome = "convicted"
				} else if strings.Contains(text, "absolv") || strings.Contains(text, "improcedente") {
					outcome = "acquitted"
				}
			}
		}
	}
	return outcome
}

func movementText(m map[string]any) string {
	parts := []string{strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", m["nome"])))}
	if comps, ok := m["complementos"].([]any); ok {
		for _, c := range comps {
			parts = append(parts, strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", c))))
		}
	}
	if comps, ok := m["complementosTabelados"].([]any); ok {
		for _, c := range comps {
			parts = append(parts, strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", c))))
		}
	}
	return strings.Join(parts, " ")
}

func proceedingTypeFromClasse(classe map[string]any) string {
	if classe == nil {
		return "criminal"
	}
	code := strings.TrimSpace(fmt.Sprintf("%v", classe["codigo"]))
	if code == "" {
		return "criminal"
	}
	return code
}

func assuntosCodes(assuntos []map[string]any) []string {
	out := make([]string, 0, len(assuntos))
	for _, a := range assuntos {
		code := strings.TrimSpace(fmt.Sprintf("%v", a["codigo"]))
		if code != "" && code != "<nil>" {
			out = append(out, code)
		}
	}
	return out
}

func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

func courtName(orgao map[string]any) string {
	if orgao == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", orgao["nome"]))
}

type relatedCase struct {
	CaseNumber       string
	TribunalEndpoint string
}

func parseRelatedCases(src *CaseSource) []relatedCase {
	if src == nil || src.Raw == nil {
		return nil
	}
	arr, ok := src.Raw["processoRelacionado"].([]any)
	if !ok {
		return nil
	}
	out := make([]relatedCase, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		caseNum := strings.TrimSpace(fmt.Sprintf("%v", m["numeroProcesso"]))
		tribunal := strings.TrimSpace(fmt.Sprintf("%v", m["tribunal"]))
		if strings.HasPrefix(tribunal, "api_publica_") {
			out = append(out, relatedCase{CaseNumber: caseNum, TribunalEndpoint: tribunal})
		}
	}
	return out
}

func parseDesmembramentoFromMovements(movs []map[string]any) []relatedCase {
	out := make([]relatedCase, 0)
	for _, m := range movs {
		if movementCode(m) != "981" {
			continue
		}
		caseNum := extractCaseNumber(movementText(m))
		if caseNum == "" {
			continue
		}
		out = append(out, relatedCase{CaseNumber: caseNum})
	}
	return out
}

func extractCaseNumber(text string) string {
	for _, tok := range strings.Fields(text) {
		cand := strings.Trim(tok, ",.;()[]{}")
		if isLikelyCaseNumber(cand) {
			return cand
		}
	}
	return ""
}

func isLikelyCaseNumber(v string) bool {
	if len(v) < 20 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && r != '.' && r != '-' {
			return false
		}
	}
	return strings.Count(v, "-") >= 1
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func maskCPF(cpf string) string {
	if len(cpf) != 11 {
		return cpf
	}
	return cpf[:3] + "***" + cpf[6:9] + "**"
}
