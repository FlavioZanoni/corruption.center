package datajud

import (
	"encoding/json"
	"testing"
)

// movementsFromJSON decodes a DataJud-style movimentos[] fixture into the
// []map[string]any shape the worker consumes.
func movementsFromJSON(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var movs []map[string]any
	if err := json.Unmarshal([]byte(raw), &movs); err != nil {
		t.Fatalf("decode movements fixture: %v", err)
	}
	return movs
}

func TestMaxMovementID(t *testing.T) {
	movs := []map[string]any{{"id": "10"}, {"id": "7"}, {"id": "42"}}
	got := maxMovementID(movs)
	if got != "42" {
		t.Fatalf("expected 42, got %s", got)
	}
}

func TestProbeCapabilities(t *testing.T) {
	ok := &CaseSource{
		NumeroProcesso: "5046512-94.2016.4.04.7000",
		Movimentos:     []map[string]any{{"id": "1", "codigo": "51"}},
	}
	if !probeCapabilities(ok) {
		t.Fatalf("expected coreOK true for numeroProcesso + movimentos")
	}

	if probeCapabilities(&CaseSource{Movimentos: []map[string]any{{"id": "1"}}}) {
		t.Fatalf("expected false when numeroProcesso missing")
	}
	if probeCapabilities(&CaseSource{NumeroProcesso: "x"}) {
		t.Fatalf("expected false when movimentos missing")
	}
	if probeCapabilities(nil) {
		t.Fatalf("expected false for nil source")
	}
}

func TestDeriveCaseState_Accepted(t *testing.T) {
	movs := movementsFromJSON(t, `[{"id":"1","codigo":"51","nome":"Recebimento de denúncia"}]`)
	st := deriveCaseState(movs)
	if st.phase != "accepted" {
		t.Fatalf("code 51: expected phase accepted, got %q", st.phase)
	}
	if st.hasConviction || st.concluded {
		t.Fatalf("code 51: expected no conviction/conclusion, got %+v", st)
	}
}

func TestDeriveCaseState_Sentenced(t *testing.T) {
	movs := movementsFromJSON(t, `[{"id":"9","codigo":"848","nome":"Sentença"}]`)
	st := deriveCaseState(movs)
	if st.phase != "sentenced" {
		t.Fatalf("code 848: expected phase sentenced, got %q", st.phase)
	}
}

func TestDeriveCaseState_SentencedOverridesAccepted(t *testing.T) {
	// 848 must win over 51 regardless of movement order.
	forward := movementsFromJSON(t, `[{"id":"1","codigo":"51"},{"id":"9","codigo":"848"}]`)
	reverse := movementsFromJSON(t, `[{"id":"9","codigo":"848"},{"id":"1","codigo":"51"}]`)
	if st := deriveCaseState(forward); st.phase != "sentenced" {
		t.Fatalf("forward order: expected sentenced, got %q", st.phase)
	}
	if st := deriveCaseState(reverse); st.phase != "sentenced" {
		t.Fatalf("reverse order: expected sentenced, got %q", st.phase)
	}
}

func TestDeriveCaseState_Conviction(t *testing.T) {
	movs := movementsFromJSON(t, `[{"id":"5","codigo":"60","nome":"Condenação"}]`)
	st := deriveCaseState(movs)
	if !st.hasConviction {
		t.Fatalf("code 60: expected has_conviction true, got %+v", st)
	}
	if st.concluded {
		t.Fatalf("code 60: conviction alone must not conclude the case")
	}
}

func TestDeriveCaseState_AcquittalTimelineOnly(t *testing.T) {
	movs := movementsFromJSON(t, `[{"id":"6","codigo":"61","nome":"Absolvição"}]`)
	st := deriveCaseState(movs)
	if st.phase != "" || st.hasConviction || st.concluded {
		t.Fatalf("code 61: expected timeline-only (no flags), got %+v", st)
	}
}

func TestDeriveCaseState_Concluded(t *testing.T) {
	for _, code := range []string{"901", "132", "246"} {
		movs := movementsFromJSON(t, `[{"id":"7","codigo":"`+code+`"}]`)
		st := deriveCaseState(movs)
		if !st.concluded {
			t.Fatalf("code %s: expected concluded true, got %+v", code, st)
		}
	}
}

func TestDeriveCaseState_FullConvictedLifecycle(t *testing.T) {
	// A conviction case-tree: denúncia accepted, sentença, condenação, then
	// baixa definitiva. Case-level flags only — no per-defendant attribution.
	movs := movementsFromJSON(t, `[
		{"id":"1","codigo":"51","nome":"Recebimento de denúncia"},
		{"id":"2","codigo":"848","nome":"Sentença"},
		{"id":"3","codigo":"60","nome":"Condenação"},
		{"id":"4","codigo":"132","nome":"Baixa definitiva"}
	]`)
	st := deriveCaseState(movs)
	if st.phase != "sentenced" {
		t.Fatalf("lifecycle: expected phase sentenced, got %q", st.phase)
	}
	if !st.hasConviction {
		t.Fatalf("lifecycle: expected has_conviction true")
	}
	if !st.concluded {
		t.Fatalf("lifecycle: expected concluded true")
	}
}

func TestDeriveCaseState_SentencaComplementConviction(t *testing.T) {
	cases := []struct {
		name          string
		movs          string
		wantPhase     string
		wantConvicted bool
	}{
		{
			name:          "848 with condenatória complement infers conviction",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Sentença condenatória","descricao":"tipo de decisão"}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: true,
		},
		{
			name:          "848 with procedente complement infers conviction",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Com resolução do mérito","descricao":"Procedente"}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: true,
		},
		{
			name:          "848 with improcedente complement is not a conviction",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Com resolução do mérito","descricao":"Improcedente"}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
		{
			name:          "848 with absolvição complement is not a conviction",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Sentença absolutória","descricao":"absolvição do réu"}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
		{
			name:          "848 with no complements is sentenced only",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença"}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
		{
			name:          "848 with unrelated complement is sentenced only",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Homologação de acordo","descricao":"outros"}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
		{
			name:          "explicit code 60 wins over improcedente 848 complement",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Improcedente","descricao":""}]},{"id":"10","codigo":"60","nome":"Condenação"}]`,
			wantPhase:     "sentenced",
			wantConvicted: true,
		},
		{
			name:          "explicit code 61 wins over condenatória 848 complement",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Sentença condenatória","descricao":""}]},{"id":"10","codigo":"61","nome":"Absolvição"}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
		{
			name:          "conflicting 848 complements do not assert conviction",
			movs:          `[{"id":"9","codigo":"848","nome":"Sentença","complementosTabelados":[{"nome":"Procedente em parte","descricao":""},{"nome":"Improcedente quanto a um réu","descricao":""}]}]`,
			wantPhase:     "sentenced",
			wantConvicted: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := deriveCaseState(movementsFromJSON(t, tc.movs))
			if st.phase != tc.wantPhase {
				t.Fatalf("phase: got %q, want %q", st.phase, tc.wantPhase)
			}
			if st.hasConviction != tc.wantConvicted {
				t.Fatalf("hasConviction: got %v, want %v (state %+v)", st.hasConviction, tc.wantConvicted, st)
			}
		})
	}
}

func TestDeriveCaseState_ExplicitDispositionOrderWins(t *testing.T) {
	// A conviction (60) followed by a later explicit acquittal (61) — e.g.
	// reversed on appeal — must NOT latch: the last explicit disposition wins,
	// so the case ends acquitted (has_conviction=false). This is the
	// defamation-grade regression the fix targets.
	convictionThenAcquittal := movementsFromJSON(t, `[
		{"id":"1","codigo":"51","nome":"Recebimento de denúncia","dataHora":"2016-01-01T10:00:00Z"},
		{"id":"2","codigo":"848","nome":"Sentença","dataHora":"2016-02-01T10:00:00Z"},
		{"id":"3","codigo":"60","nome":"Condenação","dataHora":"2016-03-01T10:00:00Z"},
		{"id":"4","codigo":"61","nome":"Absolvição","dataHora":"2017-06-01T10:00:00Z"}
	]`)
	if st := deriveCaseState(convictionThenAcquittal); st.hasConviction {
		t.Fatalf("conviction→acquittal: expected has_conviction=false (cleared), got %+v", st)
	}

	// The reverse order — acquittal first, later conviction — ends convicted.
	acquittalThenConviction := movementsFromJSON(t, `[
		{"id":"1","codigo":"61","nome":"Absolvição","dataHora":"2016-03-01T10:00:00Z"},
		{"id":"2","codigo":"60","nome":"Condenação","dataHora":"2017-06-01T10:00:00Z"}
	]`)
	if st := deriveCaseState(acquittalThenConviction); !st.hasConviction {
		t.Fatalf("acquittal→conviction: expected has_conviction=true, got %+v", st)
	}
}

func TestDeriveCaseState_ResolvesByTimestampNotInputOrder(t *testing.T) {
	// Input order lists the acquittal LAST but its dataHora is EARLIER than the
	// conviction: the chronological sort must make the conviction win despite
	// the input order.
	unsorted := movementsFromJSON(t, `[
		{"id":"2","codigo":"60","nome":"Condenação","dataHora":"2018-01-01T10:00:00Z"},
		{"id":"1","codigo":"61","nome":"Absolvição","dataHora":"2016-01-01T10:00:00Z"}
	]`)
	if st := deriveCaseState(unsorted); !st.hasConviction {
		t.Fatalf("unsorted-by-time: conviction is chronologically last, expected has_conviction=true, got %+v", st)
	}

	// Symmetric case: acquittal is chronologically last despite appearing first
	// in the input slice.
	unsortedAcquittalLast := movementsFromJSON(t, `[
		{"id":"1","codigo":"61","nome":"Absolvição","dataHora":"2018-01-01T10:00:00Z"},
		{"id":"2","codigo":"60","nome":"Condenação","dataHora":"2016-01-01T10:00:00Z"}
	]`)
	if st := deriveCaseState(unsortedAcquittalLast); st.hasConviction {
		t.Fatalf("unsorted-by-time: acquittal is chronologically last, expected has_conviction=false, got %+v", st)
	}
}

func TestDeriveCaseState_MissingTimestampsFallBackToInputOrder(t *testing.T) {
	// With no dataHora fields, the last explicit disposition in input order wins.
	acquittalLast := movementsFromJSON(t, `[
		{"id":"1","codigo":"60","nome":"Condenação"},
		{"id":"2","codigo":"61","nome":"Absolvição"}
	]`)
	if st := deriveCaseState(acquittalLast); st.hasConviction {
		t.Fatalf("no timestamps, acquittal last: expected has_conviction=false, got %+v", st)
	}
	convictionLast := movementsFromJSON(t, `[
		{"id":"1","codigo":"61","nome":"Absolvição"},
		{"id":"2","codigo":"60","nome":"Condenação"}
	]`)
	if st := deriveCaseState(convictionLast); !st.hasConviction {
		t.Fatalf("no timestamps, conviction last: expected has_conviction=true, got %+v", st)
	}
}

func TestDeriveCaseState_SentencaNomeAndPlainComplementos(t *testing.T) {
	// 848 whose disposition is only in the movement nome (no complements) →
	// convicted.
	nomeOnly := movementsFromJSON(t, `[{"id":"9","codigo":"848","nome":"Sentença condenatória"}]`)
	if st := deriveCaseState(nomeOnly); !st.hasConviction || st.phase != "sentenced" {
		t.Fatalf("848 nome condenatória: expected sentenced+convicted, got %+v", st)
	}

	// 848 whose disposition is only in the plain complementos string-list →
	// convicted.
	plainList := movementsFromJSON(t, `[{"id":"9","codigo":"848","nome":"Sentença","complementos":["Julgado procedente o pedido"]}]`)
	if st := deriveCaseState(plainList); !st.hasConviction || st.phase != "sentenced" {
		t.Fatalf("848 plain complementos procedente: expected sentenced+convicted, got %+v", st)
	}

	// Acquittal disposition in the plain complementos list is detected and rules
	// out a conviction.
	plainAcquittal := movementsFromJSON(t, `[{"id":"9","codigo":"848","nome":"Sentença","complementos":["Pedido julgado improcedente"]}]`)
	if st := deriveCaseState(plainAcquittal); st.hasConviction {
		t.Fatalf("848 plain complementos improcedente: expected not convicted, got %+v", st)
	}
}

func TestDeriveCaseState_NumericCodesTolerated(t *testing.T) {
	// DataJud may serialize codigo as a JSON number; movementCode must coerce it.
	movs := []map[string]any{{"codigo": float64(51)}, {"codigo": 848}}
	st := deriveCaseState(movs)
	if st.phase != "sentenced" {
		t.Fatalf("numeric codes: expected sentenced, got %q", st.phase)
	}
}
