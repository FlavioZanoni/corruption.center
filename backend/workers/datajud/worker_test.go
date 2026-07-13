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

// ─── deriveCaseState: fixtures use the REAL TPU codes and names verified in live
// DataJud responses (219 Procedência, 220 Improcedência, 221 em Parte, 22 Baixa
// Definitiva, 848 Trânsito em julgado, 60 Expedição de documento). The previous
// suite encoded a fictional table (60=Condenação) and passed while the graph
// marked 2,082 cases convicted in error. ─────────────────────────────────────────

const criminalClass = "Ação Penal - Procedimento Ordinário"

func mov(code int, nome, when string) map[string]any {
	m := map[string]any{"codigo": float64(code), "nome": nome}
	if when != "" {
		m["dataHora"] = when
	}
	return m
}

func TestDeriveCaseState_ClericalCode60NeverConvicts(t *testing.T) {
	// The bug this suite exists to prevent: code 60 is "Expedição de documento",
	// present in nearly every case. It must derive NOTHING.
	movs := []map[string]any{
		mov(60, "Expedição de documento", "2020-01-01T00:00:00"),
		mov(60, "Expedição de documento", "2021-01-01T00:00:00"),
		mov(51, "Conclusão", "2021-02-01T00:00:00"),
		mov(132, "Recebimento", "2021-03-01T00:00:00"),
	}
	st := deriveCaseState(criminalClass, movs)
	if st.disposition != "" {
		t.Fatalf("clerical movements produced disposition %q", st.disposition)
	}
	if st.concluded {
		t.Fatal("132 is 'Recebimento', not a conclusion")
	}
}

func TestDeriveCaseState_ProcedenciaConvicts(t *testing.T) {
	movs := []map[string]any{
		mov(391, "Denúncia", "2016-01-01T00:00:00"),
		mov(219, "Procedência", "2017-01-01T00:00:00"),
		mov(848, "Trânsito em julgado", "2018-01-01T00:00:00"),
		mov(22, "Baixa Definitiva", "2019-01-01T00:00:00"),
	}
	st := deriveCaseState(criminalClass, movs)
	if !st.hasConviction() {
		t.Fatal("Procedência on a criminal action is the conviction")
	}
	if !st.concluded || st.phase != "sentenced" {
		t.Fatalf("want concluded+sentenced, got %+v", st)
	}
}

func TestDeriveCaseState_ImprocedenteIsNotProcedente(t *testing.T) {
	// "julgo improcedente" CONTAINS "procedente": ordering of checks is the test.
	st := deriveCaseState(criminalClass, []map[string]any{
		mov(220, "Improcedência", "2020-01-01T00:00:00"),
	})
	if st.disposition != "acquittal" {
		t.Fatalf("improcedência must read as acquittal, got %q", st.disposition)
	}
}

func TestDeriveCaseState_LaterAcquittalClearsConviction(t *testing.T) {
	// Reversal on appeal: latching the conviction would be defamation.
	movs := []map[string]any{
		mov(219, "Procedência", "2017-01-01T00:00:00"),
		mov(0, "Absolvição", "2019-01-01T00:00:00"),
	}
	if st := deriveCaseState(criminalClass, movs); st.hasConviction() {
		t.Fatal("a later absolvição must clear the conviction")
	}
	// And chronological order must come from timestamps, not slice order.
	reversed := []map[string]any{movs[1], movs[0]}
	if st := deriveCaseState(criminalClass, reversed); st.hasConviction() {
		t.Fatal("timestamp order, not input order, decides which disposition is last")
	}
}

func TestDeriveCaseState_ExtinctionOfPunibilityMeansCannotSay(t *testing.T) {
	// Prescrição/morte may follow a conviction or preempt any judgment; either
	// affirmative claim would be wrong half the time.
	movs := []map[string]any{
		mov(219, "Procedência", "2017-01-01T00:00:00"),
		mov(1042, "Morte do Agente", "2020-01-01T00:00:00"),
	}
	if st := deriveCaseState(criminalClass, movs); st.disposition != "" {
		t.Fatalf("extinction must reset to undeterminable, got %q", st.disposition)
	}
}

func TestDeriveCaseState_CivilCaseNeverGetsADisposition(t *testing.T) {
	// A live Apelação Cível was marked convicted. Civil procedência is
	// liability, not crime.
	movs := []map[string]any{
		mov(219, "Procedência", "2020-01-01T00:00:00"),
	}
	if st := deriveCaseState("Apelação Cível", movs); st.disposition != "" {
		t.Fatalf("civil class must never carry has_conviction, got %q", st.disposition)
	}
}

func TestDeriveCaseState_AppellateProvimentoIsIgnored(t *testing.T) {
	// Provimento without knowing whose appeal is uninterpretable.
	movs := []map[string]any{
		mov(237, "Provimento", "2020-01-01T00:00:00"),
		mov(239, "Não-Provimento", "2021-01-01T00:00:00"),
	}
	if st := deriveCaseState("Apelação Criminal", movs); st.disposition != "" {
		t.Fatalf("appellate provimento must stay undeterminable, got %q", st.disposition)
	}
}

func TestFoldPT(t *testing.T) {
	if got := foldPT("Extinção da Punibilidade"); got != "extincao da punibilidade" {
		t.Fatalf("fold: %q", got)
	}
	if got := foldPT("Trânsito em Julgado"); got != "transito em julgado" {
		t.Fatalf("fold: %q", got)
	}
}
