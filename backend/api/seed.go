package api

import (
	"context"
	"log/slog"
	"os"

	"corruption-center/db/memgraph"
)

// Baseline scandals: the landmark Brazilian corruption cases, hardcoded so a
// fresh install boots with real content instead of an empty graph. Seeding runs
// on every API start and is idempotent — it is exactly the backoffice seed flow
// (Scandal node + LegalProceeding + watcher_tracking row), so from the first
// boot the DataJud watcher polls these cases and DJEN discovers their parties.
//
// Every case number below was verified to exist in the DataJud public API.
// Cases tried at the STF (Mensalão/AP 470) have no public DataJud endpoint, so
// those scandals are seeded as nodes with no watched case.
//
// Descriptions are neutral and factual and name no individuals: people only
// enter the graph through official sources (DataJud, DJEN, CGU, TCU).
type baselineCase struct {
	Number   string // CNJ number, formatted or bare
	Endpoint string // DataJud index, e.g. api_publica_trf4
}

type baselineScandal struct {
	memgraph.ScandalSeed
	Cases []baselineCase
}

var baselineScandals = []baselineScandal{
	{
		ScandalSeed: memgraph.ScandalSeed{
			ID:          "lava-jato",
			Name:        "Operação Lava Jato",
			Description: "Investigação da Polícia Federal e do Ministério Público Federal, deflagrada em março de 2014, sobre um esquema de corrupção e lavagem de dinheiro envolvendo contratos da Petrobras, empreiteiras e agentes públicos. A força-tarefa de Curitiba foi encerrada em fevereiro de 2021; recursos e revisões de condenações seguem em andamento.",
			DateStart:   "2014-03-17",
			Status:       "ongoing",
			WikipediaURL: "https://pt.wikipedia.org/wiki/Opera%C3%A7%C3%A3o_Lava_Jato",
		},
		Cases: []baselineCase{
			{Number: "5083376-05.2014.4.04.7000", Endpoint: "api_publica_trf4"}, // primeira ação penal, 13ª Vara Federal de Curitiba
			{Number: "5046512-94.2016.4.04.7000", Endpoint: "api_publica_trf4"}, // caso "triplex do Guarujá"
			{Number: "5021365-32.2017.4.04.7000", Endpoint: "api_publica_trf4"}, // caso "sítio de Atibaia"
		},
	},
	{
		ScandalSeed: memgraph.ScandalSeed{
			ID:           "calicute",
			Name:         "Operação Calicute",
			Description:  "Desdobramento da Lava Jato no Rio de Janeiro, deflagrado em novembro de 2016, sobre desvios em contratos de obras públicas do governo estadual fluminense.",
			DateStart:    "2016-11-17",
			Status:       "ongoing",
			WikipediaURL: "https://pt.wikipedia.org/wiki/Opera%C3%A7%C3%A3o_Calicute",
		},
		Cases: []baselineCase{
			{Number: "0509503-57.2016.4.02.5101", Endpoint: "api_publica_trf2"}, // 7ª Vara Federal Criminal do Rio de Janeiro
		},
	},
	{
		ScandalSeed: memgraph.ScandalSeed{
			ID:           "mensalao",
			Name:         "Escândalo do Mensalão",
			Description:  "Esquema de compra de apoio parlamentar revelado em 2005, julgado pelo Supremo Tribunal Federal na Ação Penal 470, com condenações a partir de 2012. O STF não integra a API pública do DataJud, portanto este caso não recebe atualizações automáticas de andamento processual.",
			DateStart:    "2005-06-06",
			DateEnd:      "2014-03-15",
			Status:       "concluded",
			WikipediaURL: "https://pt.wikipedia.org/wiki/Esc%C3%A2ndalo_do_mensal%C3%A3o",
		},
	},
}

// SeedBaseline writes the baseline scandals and registers their cases with the
// watchers. Best-effort: a failure on one scandal is logged and never blocks API
// startup. Set SEED_BASELINE=false to skip.
func (s *ApiServer) SeedBaseline(ctx context.Context, log *slog.Logger) {
	if os.Getenv("SEED_BASELINE") == "false" {
		return
	}
	h := &backofficeHandler{server: s}
	for _, sc := range baselineScandals {
		if err := s.memgraph.UpsertScandalSeed(ctx, sc.ScandalSeed); err != nil {
			log.Warn("baseline seed: scandal failed", "scandal", sc.ID, "err", err)
			continue
		}
		for _, c := range sc.Cases {
			if err := h.registerWatcherCase(ctx, c.Number, c.Endpoint, sc.ID, sc.Name, "baseline_seed"); err != nil {
				log.Warn("baseline seed: case failed", "scandal", sc.ID, "case", c.Number, "err", err)
			}
		}
	}
	log.Info("baseline seed applied", "scandals", len(baselineScandals))
}
