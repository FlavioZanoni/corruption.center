package datajud

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	CasesConcluded    int
	ProbeFieldsOK     bool
	TPUVerificationOK bool
	VerificationNotes []string
	// FetchErrors counts cases abandoned after the client exhausted its retries.
	// There are ~90 tribunal endpoints and any one can be down; one of them being
	// unreachable is a fact about that case, not a reason to discard the run.
	FetchErrors int
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
			stats.ProbeFieldsOK = probeCapabilities(raw)
			if !stats.ProbeFieldsOK {
				if opts.StrictVerify {
					return nil, fmt.Errorf("datajud: probe response missing core fields (numeroProcesso or movimentos)")
				}
				stats.VerificationNotes = append(stats.VerificationNotes, "probe response missing core fields; continuing in non-strict mode")
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
			// One unreachable tribunal used to end the whole run. A pass over 701
			// cases died on a single timeout against api_publica_tjap and threw away
			// every case it had already polled — the same failure that cost us a CGU
			// run and two DJEN runs. There are ~90 tribunal endpoints and any one of
			// them can be down; that is a fact about one case, not about the run.
			//
			// A cancelled context is different: that is us stopping, and it should.
			if ctx.Err() != nil {
				return nil, err
			}
			stats.FetchErrors++
			slog.Warn("datajud: case lookup failed, skipping",
				"case", c.CaseNumber, "endpoint", c.TribunalEndpoint, "err", err)
			continue
		}
		stats.CasesPolled++
		if src == nil {
			stats.CasesNotFound++
			// Read-only runs never mutate watcher_tracking (not even
			// last_polled_at): Postgres tracking must stay in lockstep with the
			// graph, which is only written when EnableWrites is set.
			if opts.EnableWrites {
				_ = w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, c.LastMovementID, c.Status, time.Now().UTC())
			}
			continue
		}
		if src.NivelSigilo > 0 {
			stats.CasesRestricted++
			if opts.EnableWrites {
				_ = w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, c.LastMovementID, c.Status, time.Now().UTC())
			}
			continue
		}

		state := deriveCaseState(src.Movimentos)

		newStatus := c.Status
		if state.concluded {
			newStatus = "concluded"
			stats.CasesConcluded++
		}

		// Fully read-only when writes are disabled: derived state and stats are
		// reported (dry run), but neither the graph nor watcher_tracking is
		// mutated. Gating the status flip and last_movement_id advance here keeps
		// Postgres from desynchronizing from the skipped graph write.
		if !opts.EnableWrites {
			continue
		}

		if err := w.applyCaseWrites(ctx, c, src, state); err != nil {
			return nil, err
		}

		last := maxMovementID(src.Movimentos)
		lastStr := c.LastMovementID
		if last != "" {
			lastStr = &last
		}
		if err := w.pg.UpdateWatcherTrackingPoll(ctx, c.CaseNumber, lastStr, newStatus, time.Now().UTC()); err != nil {
			return nil, err
		}
		stats.CasesUpdated++
	}

	return stats, nil
}

func (w *Worker) applyCaseWrites(ctx context.Context, c psql.WatcherCase, src *CaseSource, state caseState) error {
	if w.mg == nil {
		return fmt.Errorf("datajud: memgraph writer is required when --enable-writes is true")
	}

	proceedingID, err := w.mg.UpsertLegalProceedingByCase(ctx, memgraph.DataJudProceedingUpsert{
		CaseNumber: src.NumeroProcesso,
		Court:      courtName(src.OrgaoJulgador),
		Type:       proceedingTypeFromClasse(src.Classe),
		ClassName:  proceedingClassName(src.Classe),
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

	if err := w.mg.UpdateProceedingCaseState(ctx, proceedingID, state.phase, state.hasConviction, state.concluded); err != nil {
		return err
	}

	return nil
}

// caseState is the case-level status derived from movements. All flags are
// case-level; per-defendant outcomes are set only via backoffice review using
// DJEN evidence (see docs/workerDetails/DATAJUD.md).
type caseState struct {
	phase         string // "" | "accepted" | "sentenced"
	hasConviction bool
	concluded     bool
}

// deriveCaseState implements the case-level movement state machine:
//
//	51  Recebimento de denúncia   -> phase = accepted
//	848 Sentença                  -> phase = sentenced (+ complement inference)
//	60  Condenação                -> has_conviction = true
//	61  Absolvição                -> explicitly not a conviction (timeline only)
//	901 Prescrição                -> concluded
//	132 Baixa definitiva          -> concluded
//	246 Arquivamento definitivo   -> concluded
//
// sentenced takes precedence over accepted regardless of movement order.
//
// Conviction is derived with precedence: an explicit disposition movement
// (code 60 Condenação / 61 Absolvição) always wins over any 848 inference, and
// the LAST explicit disposition in chronological order is authoritative; a
// conviction reversed on appeal (a later Absolvição) must clear the conviction
// rather than latch it (defamation-grade if it did not). Movements are
// evaluated in chronological order by dataHora, falling back to input order
// when timestamps are missing or equal. When no explicit 60/61 exists, the
// disposition of a Sentença (848) is inferred from its nome, complementos, and
// complementosTabelados: convictions frequently live there (e.g. "Sentença
// condenatória", "Procedente") rather than as a standalone code-60 movement, so
// scanning them avoids losing convictions.
func deriveCaseState(movs []map[string]any) caseState {
	var st caseState
	var inferredConviction, inferredAcquittal bool
	// lastExplicit tracks the most recent (chronological) explicit disposition:
	// "conviction" (code 60), "acquittal" (code 61), or "" when none was seen.
	lastExplicit := ""
	for _, m := range movementsChronological(movs) {
		switch movementCode(m) {
		case "51":
			if st.phase == "" {
				st.phase = "accepted"
			}
		case "848":
			st.phase = "sentenced"
			conv, acq := sentencaComplementSignals(m)
			if conv {
				inferredConviction = true
			}
			if acq {
				inferredAcquittal = true
			}
		case "60":
			// Condenação. Overwrites any earlier explicit disposition so the
			// latest one wins.
			lastExplicit = "conviction"
		case "61":
			// Absolvição (e.g. reversed on appeal). Overwrites any earlier
			// explicit disposition so a later acquittal clears an earlier
			// conviction rather than latching it.
			lastExplicit = "acquittal"
		case "901", "132", "246":
			st.concluded = true
		}
	}

	switch {
	case lastExplicit == "conviction":
		// Latest explicit code 60 wins regardless of any 848 complement text.
		st.hasConviction = true
	case lastExplicit == "acquittal":
		// Latest explicit code 61 wins over any 848 inference.
		st.hasConviction = false
	case inferredConviction && !inferredAcquittal:
		// 848 complement indicates a conviction with no conflicting acquittal.
		st.hasConviction = true
	}
	return st
}

// movementsChronological returns a copy of movs sorted by dataHora ascending.
// The sort is stable and only reorders movements with parseable timestamps:
// when either timestamp is missing/unparseable (or they are equal), the pair
// keeps its original input order. Sorting a copy leaves the caller's slice
// (used later by maxMovementID) untouched.
func movementsChronological(movs []map[string]any) []map[string]any {
	ordered := make([]map[string]any, len(movs))
	copy(ordered, movs)
	sort.SliceStable(ordered, func(i, j int) bool {
		ti, oki := movementTime(ordered[i])
		tj, okj := movementTime(ordered[j])
		if !oki || !okj {
			return false
		}
		return ti.Before(tj)
	})
	return ordered
}

// movementTime parses a movement's dataHora field into a time.Time. It reports
// ok=false when the field is absent or does not match a known layout, so the
// caller can fall back to input order.
func movementTime(m map[string]any) (time.Time, bool) {
	s := mapString(m, "dataHora")
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// sentencaComplementSignals inspects a Sentença (code 848) movement to infer
// its disposition when no explicit Condenação (60) / Absolvição (61) movement is
// present. It scans the movement's nome, its plain complementos string-list, and
// its complementosTabelados nome/descricao entries: a conviction may be encoded
// in any of them (e.g. nome "Sentença condenatória" with no tabulated
// complement). All text is normalized to lowercase and accent-folded before
// matching. Because "improcedente" contains "procedente", acquittal signals are
// checked first and win within a single text fragment.
func sentencaComplementSignals(m map[string]any) (conviction, acquittal bool) {
	texts := []string{foldText(mapString(m, "nome"))}
	for _, c := range stringList(m["complementos"]) {
		texts = append(texts, foldText(c))
	}
	for _, cm := range complementList(m["complementosTabelados"]) {
		texts = append(texts, foldText(fmt.Sprintf("%v %v", cm["nome"], cm["descricao"])))
	}
	for _, text := range texts {
		switch {
		case strings.Contains(text, "improcedente"), strings.Contains(text, "absolv"):
			acquittal = true
		case strings.Contains(text, "conden"), strings.Contains(text, "procedente"):
			conviction = true
		}
	}
	return conviction, acquittal
}

// stringList coerces a plain string-list field (e.g. movimentos[].complementos)
// into []string, tolerating the []any shape from json.Unmarshal, a directly
// constructed []string, or a single string value.
func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if e == nil {
				continue
			}
			out = append(out, fmt.Sprintf("%v", e))
		}
		return out
	case string:
		return []string{t}
	}
	return nil
}

// complementList coerces a complementosTabelados value into a slice of maps,
// tolerating both the []any shape produced by json.Unmarshal into map[string]any
// and a directly constructed []map[string]any.
func complementList(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, e := range t {
			if cm, ok := e.(map[string]any); ok {
				out = append(out, cm)
			}
		}
		return out
	}
	return nil
}

// accentFolder maps lowercase Portuguese accented characters to their ASCII
// base so keyword matching is accent-insensitive.
var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

func foldText(s string) string {
	return accentFolder.Replace(strings.ToLower(s))
}

func probeCapabilities(src *CaseSource) bool {
	if src == nil {
		return false
	}
	numero := strings.TrimSpace(src.NumeroProcesso)
	if numero == "" && src.Raw != nil {
		numero = strings.TrimSpace(fmt.Sprintf("%v", src.Raw["numeroProcesso"]))
	}

	movsFieldExists := len(src.Movimentos) > 0
	if !movsFieldExists && src.Raw != nil {
		_, movsFieldExists = src.Raw["movimentos"]
	}

	return numero != "" && movsFieldExists
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

// mapString returns the trimmed string form of m[key], or "" when the map is
// nil or the key is absent/nil (avoids fmt's "<nil>" rendering).
func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v := m[key]
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func movementCode(m map[string]any) string {
	return mapString(m, "codigo")
}

func proceedingTypeFromClasse(classe map[string]any) string {
	if code := mapString(classe, "codigo"); code != "" {
		return code
	}
	return "criminal"
}

// proceedingClassName is the human name of the class ("Ação Penal - Procedimento
// Ordinário"). DataJud sends it right beside the code and we used to throw it
// away, keeping only "283" — so a case could be labelled nothing better than a
// TPU number, and the graph showed the reader a code where it meant to show them
// what kind of case they were looking at.
func proceedingClassName(classe map[string]any) string {
	return mapString(classe, "nome")
}

func assuntosCodes(assuntos []map[string]any) []string {
	out := make([]string, 0, len(assuntos))
	for _, a := range assuntos {
		if code := mapString(a, "codigo"); code != "" {
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
	return mapString(orgao, "nome")
}
