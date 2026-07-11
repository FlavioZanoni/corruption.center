package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"corruption-center/workers/photos"
)

func main() {
	var (
		modes       = flag.String("mode", "tse,wikidata", "Comma-separated modes: tse,wikidata")
		year        = flag.Int("year", 2022, "TSE election year for candidate photo lookup")
		uf          = flag.String("uf", "", "Optional UF filter (Politician.state), e.g. SP")
		limit       = flag.Int("limit", 0, "Per-mode cap on graph targets (0 = all)")
		dryRun      = flag.Bool("dry-run", false, "Resolve + verify photos but perform no graph writes")
		tseTemplate = flag.String("tse-url-template", "", "Override TSE candidate-photo URL template ({year}/{uf}/{sq})")
		sparql      = flag.String("sparql-endpoint", "", "Override Wikidata SPARQL endpoint")
		workDir     = flag.String("workdir", "", "Directory for the consulta_cand zip (metadata only; default system temp)")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log)

	opts := photos.Options{
		Modes:          splitCSV(*modes),
		Year:           *year,
		UF:             *uf,
		Limit:          *limit,
		DryRun:         *dryRun,
		TSEURLTemplate: *tseTemplate,
		SPARQLEndpoint: *sparql,
		WorkDir:        *workDir,
	}

	// DATABASE_URL / MEMGRAPH_URI are mustEnv (the psql migration tracker is
	// needed by memgraph.New). MEMGRAPH_USER/PASS are optional: the dev Memgraph
	// runs auth-less and the compose stack passes empty strings.
	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	mg, err := memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
		os.Exit(1)
	}
	defer mg.Close(ctx)

	w := photos.NewWorker(mg, opts)
	stats, err := w.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "photos run failed: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(stats)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(2)
	}
	return v
}
