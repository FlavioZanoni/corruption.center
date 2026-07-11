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
	"corruption-center/workers/djen"
)

func main() {
	var (
		baseURL   = flag.String("base-url", "", "DJEN API base URL override")
		caseMode  = flag.Bool("case-mode", true, "Run case mode (party roster discovery for tracked cases)")
		nameMode  = flag.Bool("name-mode", true, "Run name mode (case discovery for tracked politicians)")
		dryRun    = flag.Bool("dry-run", false, "Fetch and compute but perform no writes")
		nameCap   = flag.Int("name-cap", 300, "Max items pulled per politician name per run")
		pollLimit = flag.Int("poll-limit", 0, "Max watcher cases to poll in case mode (0 = all)")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log) // so the worker's case-number warnings surface

	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	// Both modes need to read Politician names from Memgraph; case mode also
	// writes Person/Organization nodes and DEFENDANT_IN edges (unless --dry-run).
	// MEMGRAPH_USER/MEMGRAPH_PASS are optional: dev Memgraph is auth-less and the
	// dev compose passes empty strings, so they must not be required.
	mg, err := memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
		os.Exit(1)
	}
	defer mg.Close(ctx)

	opts := djen.Options{
		BaseURL:   *baseURL,
		CaseMode:  *caseMode,
		NameMode:  *nameMode,
		DryRun:    *dryRun,
		NameCap:   *nameCap,
		PollLimit: *pollLimit,
	}

	w := djen.NewWorker(pg, mg, opts)
	stats, err := w.Run(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "djen run failed: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(stats)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(2)
	}
	return v
}
