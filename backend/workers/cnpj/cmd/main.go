package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"corruption-center/workers/cnpj"
)

func main() {
	var (
		baseURL = flag.String("base-url", os.Getenv("CNPJ_API_BASE"), "CNPJ provider base URL (default env CNPJ_API_BASE or https://minhareceita.org)")
		limit   = flag.Int("limit", 0, "Max root Organization nodes to enrich (0 = all needing enrichment)")
		dryRun  = flag.Bool("dry-run", false, "Fetch and classify but perform no writes")
		single  = flag.String("cnpj", "", "Enrich a single CNPJ (14 digits) — for testing")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log) // so the worker's warnings surface

	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	// MEMGRAPH_USER/MEMGRAPH_PASS are optional: dev Memgraph is auth-less and the
	// dev compose passes empty strings, so they must not be required.
	mg, err := memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
		os.Exit(1)
	}
	defer mg.Close(ctx)

	opts := cnpj.Options{
		BaseURL:    *baseURL,
		RatePerMin: ratePerMin(),
		Limit:      *limit,
		DryRun:     *dryRun,
		SingleCNPJ: *single,
	}

	w := cnpj.NewWorker(pg, mg, opts)
	stats, err := w.Run(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cnpj run failed: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(stats)
}

// ratePerMin reads CNPJ_RATE_PER_MIN (requests/minute). 0 or unset lets the
// client apply its default. Point CNPJ_API_BASE at the shared PUBLIC instance?
// Set this low (see README).
func ratePerMin() int {
	v := strings.TrimSpace(os.Getenv("CNPJ_RATE_PER_MIN"))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid CNPJ_RATE_PER_MIN %q: %v\n", v, err)
		os.Exit(2)
	}
	return n
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(2)
	}
	return v
}
