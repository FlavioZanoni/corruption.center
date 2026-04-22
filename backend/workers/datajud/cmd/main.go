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
	"corruption-center/workers/datajud"
)

func main() {
	var (
		apiBase       = flag.String("api-base", "", "DataJud API base URL override")
		probeCase     = flag.String("probe-case", "", "Known case number to verify response fields")
		probeTribunal = flag.String("probe-tribunal", "", "Tribunal endpoint for probe case, e.g. api_publica_trf4")
		verifyTPU     = flag.Bool("verify-tpu", true, "Verify movement codes against TPU public table")
		strictVerify  = flag.Bool("strict-verify", false, "Fail fast when verification/probe checks are incomplete")
		pollLimit     = flag.Int("poll-limit", 0, "Max watcher cases to poll (0 = all)")
		enableWrites  = flag.Bool("enable-writes", false, "Enable graph and pending_review writes from movement processing")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	var mg *memgraph.DB
	if *enableWrites {
		mg, err = memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), mustEnv("MEMGRAPH_USER"), mustEnv("MEMGRAPH_PASS"), pg, log)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
			os.Exit(1)
		}
		defer mg.Close(ctx)
	}

	w, err := datajud.NewWorker(ctx, pg, mg, datajud.Options{APIBase: *apiBase, APIKey: os.Getenv("DATAJUD_API_KEY")})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init datajud worker: %v\n", err)
		os.Exit(1)
	}

	stats, err := w.Run(ctx, datajud.Options{
		APIBase:         *apiBase,
		APIKey:          os.Getenv("DATAJUD_API_KEY"),
		ProbeCaseNumber: *probeCase,
		ProbeTribunal:   *probeTribunal,
		VerifyTPUCodes:  *verifyTPU,
		StrictVerify:    *strictVerify,
		PollLimit:       *pollLimit,
		EnableWrites:    *enableWrites,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "datajud run failed: %v\n", err)
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
