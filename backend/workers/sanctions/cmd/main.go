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
	"corruption-center/workers/sanctions"
)

func main() {
	var (
		registries = flag.String("registries", "ceis,cnep,ceaf,leniencia,tcu", "Comma-separated registries to sync")
		dryRun     = flag.Bool("dry-run", false, "Parse and match without writing to Memgraph/Postgres reviews")
		cguBase    = flag.String("cgu-base-url", "", "Override CGU API base URL")
		tcuBase    = flag.String("tcu-base-url", "", "Override TCU CSV base URL")
		maxPages   = flag.Int("max-pages", 0, "Per-registry CGU page cap (0 = until empty)")
		sweep      = flag.Bool("sweep", false, "After a FULL sync, unpublish records the source no longer lists (retraction). Refused on any partial run.")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	reg := splitCSV(*registries)
	opts := sanctions.Options{
		APIKey:     os.Getenv("TRANSPARENCIA_API_KEY"),
		Registries: reg,
		DryRun:     *dryRun,
		CGUBaseURL: *cguBase,
		TCUBaseURL: *tcuBase,
		MaxPages:   *maxPages,
		Sweep:      *sweep,
	}

	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	var mg *memgraph.DB
	if !opts.DryRun {
		// MEMGRAPH_USER/PASS are optional: the dev Memgraph runs auth-less and the
		// compose file passes empty strings. Only the URI is required here.
		mg, err = memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
			os.Exit(1)
		}
		defer mg.Close(ctx)
	}

	w := sanctions.NewWorker(pg, mg, opts)
	stats, err := w.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sanctions run failed: %v\n", err)
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
