package datajud

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// caseFetchers is how many DataJud lookups are in flight at once. The API answers
// in 7-32 seconds, so a serial poller idles on the network almost all the time;
// six in flight brings a full pass over 8,000 cases from ~89 hours to a few. The
// client's rate limiter is global and still caps the run at 60 req/min.
const caseFetchers = 6

// progressEvery: a run this long must say where it is. See the DJEN worker, which
// was a black box for ten hours for exactly this reason.
const progressEvery = 100

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

	// Fetch concurrently, write serially.
	//
	// DataJud is SLOW: a single lookup measured 7s on api_publica_trf4, 8s on
	// api_publica_tjes and 32s on api_publica_tjba. Polled one at a time, the run
	// managed about 1.5 cases a minute — 8,000 cases would have taken 89 hours, and
	// the 60 req/min self-limit never even came into play, because the API's own
	// latency was the ceiling, not our politeness.
	//
	// So the fetches overlap and the WRITES stay on this goroutine: no locks, no
	// racing MERGEs, and stats that add up. The client's limiter is global, so the
	// self-imposed 60 req/min cap still holds no matter how many fetchers there are.
	type poll struct {
		c   psql.WatcherCase
		src *CaseSource
		err error
	}

	work := make(chan psql.WatcherCase)
	polls := make(chan poll, caseFetchers)

	go func() {
		defer close(work)
		for i := 0; i < limit; i++ {
			select {
			case work <- cases[i]:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer close(polls)
		var wg sync.WaitGroup
		for i := 0; i < caseFetchers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for c := range work {
					src, err := w.client.SearchByCaseNumber(ctx, c.TribunalEndpoint, c.CaseNumber)
					select {
					case polls <- poll{c: c, src: src, err: err}:
					case <-ctx.Done():
						return
					}
				}
			}()
		}
		wg.Wait()
	}()

	for p := range polls {
		c, src := p.c, p.src

		if p.err != nil {
			// One unreachable tribunal used to end the whole run. A pass over 701
			// cases died on a single timeout against api_publica_tjap and threw away
			// every case it had already polled — the same failure that cost us a CGU
			// run and two DJEN runs. There are ~90 tribunal endpoints and any one of
			// them can be down; that is a fact about one case, not about the run.
			//
			// A cancelled context is different: that is us stopping, and it should.
			if ctx.Err() != nil {
				return nil, p.err
			}
			stats.FetchErrors++
			slog.Warn("datajud: case lookup failed, skipping",
				"case", c.CaseNumber, "endpoint", c.TribunalEndpoint, "err", p.err)
			continue
		}
		stats.CasesPolled++

		if stats.CasesPolled%progressEvery == 0 {
			slog.Info("datajud: progress",
				"polled", stats.CasesPolled, "of", limit,
				"updated", stats.CasesUpdated, "errors", stats.FetchErrors)
		}

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

		state := deriveCaseState(proceedingClassName(src.Classe), src.Movimentos)

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

	if err := w.mg.UpdateProceedingCaseState(ctx, proceedingID, state.phase, state.disposition, state.concluded); err != nil {
		return err
	}

	return nil
}

// caseState is what one poll of a case's movement history supports claiming.
// disposition is deliberately three-valued: "conviction", "acquittal", or "" —
// and "" means WE CANNOT SAY, which is not the same claim as an acquittal. A
// boolean here was one of the two bugs that marked 2,082 cases convicted.
type caseState struct {
	phase       string // "" | "sentenced"
	disposition string // "conviction" | "acquittal" | "" (undeterminable)
	concluded   bool
}

// hasConviction/hasAcquittal are conveniences for tests and callers.
func (c caseState) hasConviction() bool { return c.disposition == "conviction" }

// deriveCaseState derives the case-level disposition from DataJud movements.
//
// HISTORY, because this went badly once: the first implementation trusted a
// hardcoded TPU table claiming 60=Condenação, 61=Absolvição, 848=Sentença,
// 51=Recebimento da denúncia, 132=Baixa definitiva. Verified against the live
// API across three tribunals, every one of those was wrong: 60 is "Expedição de
// documento" — a clerical act present in nearly every case — 61 never occurs,
// 848 is "Trânsito em julgado", 51 is "Conclusão", 132 is "Recebimento". Result:
// 2,082 of 2,344 polled cases marked convicted, an 89% "conviction rate" that
// was 100% artifact. The safety net (VerifyMovementCodes against the CNJ SGT
// page) silently no-ops because that page ignores its query parameter.
//
// The rule now:
//   - Signals come from each movement's own nome (folded), corroborated by the
//     codes we VERIFIED in live data: 219 Procedência, 220 Improcedência,
//     221 Procedência em Parte, 1042 Morte do agente.
//   - "improcedên"/"absolvi"/220 → acquittal. Checked BEFORE the conviction
//     patterns because "julgo improcedente" contains "procedente".
//   - "condena"/"procedên"/"procedente"/219/221 → conviction (in a criminal
//     action, procedência of the denúncia IS the conviction).
//   - "extinção da punibilidade"/1042 → clears to acquittal-equivalent "not
//     convicted"? NO — it clears to "" (cannot say): extinction may follow a
//     conviction (prescrição da pena) or precede any judgment. Claiming either
//     way would be wrong half the time, so it resets the disposition.
//   - The LAST dispositive movement in chronological order wins, so an appeal
//     that reverses a conviction clears it (defamation-grade if it latched).
//   - Appellate provimento/não-provimento movements are IGNORED: without
//     knowing whose appeal was granted they are uninterpretable, so appeal-class
//     cases generally stay "" — honest, not timid.
//   - The whole derivation is gated on the case class being criminal (folded
//     class contains "penal" or "criminal" or "crime"): a live Apelação Cível
//     was found marked convicted, and civil "procedência" is liability, not a
//     crime. Non-criminal cases never get a disposition.
//   - concluded: 22 Baixa Definitiva (verified) or 848 Trânsito em julgado.
//     phase "sentenced" only when a dispositive signal was actually seen.
func deriveCaseState(className string, movs []map[string]any) caseState {
	var st caseState

	criminal := false
	fc := foldPT(className)
	for _, marker := range []string{"penal", "criminal", "crime"} {
		if strings.Contains(fc, marker) {
			criminal = true
			break
		}
	}

	for _, m := range movementsChronological(movs) {
		code := movementCode(m)
		nome := foldPT(movementNome(m))

		if code == "22" || strings.Contains(nome, "transito em julgado") || code == "848" {
			st.concluded = true
		}
		if !criminal {
			continue
		}

		switch {
		// Order matters: "julgo improcedente" contains "procedente".
		case strings.Contains(nome, "improceden") || code == "220":
			st.disposition = "acquittal"
			st.phase = "sentenced"
		case strings.Contains(nome, "absolvi"):
			st.disposition = "acquittal"
			st.phase = "sentenced"
		case strings.Contains(nome, "extincao da punibilidade") ||
			strings.Contains(nome, "extinta a punibilidade") || code == "1042":
			// Punibility extinguished: may follow a conviction (prescrição) or
			// preempt any judgment. Either claim would be wrong half the time.
			st.disposition = ""
		case strings.Contains(nome, "condena"):
			st.disposition = "conviction"
			st.phase = "sentenced"
		case strings.Contains(nome, "proceden") || code == "219" || code == "221":
			st.disposition = "conviction"
			st.phase = "sentenced"
		}
	}
	return st
}

// foldPT lowercases and strips the Portuguese accents that appear in TPU
// movement names, so signal matching is spelling-insensitive.
func foldPT(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer(
		"á", "a", "â", "a", "ã", "a", "à", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
	)
	return r.Replace(s)
}

// movementNome extracts the movement's own display name as sent by the tribunal.
func movementNome(m map[string]any) string {
	if v, ok := m["nome"].(string); ok {
		return v
	}
	return ""
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
