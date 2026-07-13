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
		// Defaults from the environment, so a run started WITHOUT deploy/datajud-watcher.sh
		// behaves the same as one started with it. It did not: the flag defaulted to
		// false and nothing read ENABLE_WRITES, so `go run ./workers/datajud/cmd`
		// happily queried DataJud eight thousand times, wrote nothing, and reported
		// success. The env var existed, was set to "true" in compose, and was ignored.
		enableWrites = flag.Bool("enable-writes", envBool("ENABLE_WRITES", false),
			"Enable case-level graph writes (proceeding upsert + phase/has_conviction/status state machine). Defaults to $ENABLE_WRITES.")
	)
	flag.Parse()

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// A worker that reads an API for hours and writes nothing looks exactly like a
	// worker that is working. Say it out loud.
	if !*enableWrites {
		log.Warn("datajud: WRITES ARE DISABLED — this run will poll DataJud and change nothing. Pass -enable-writes or set ENABLE_WRITES=true.")
	}
	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	var mg *memgraph.DB
	if *enableWrites {
		mg, err = memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
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

// envBool reads a boolean flag default from the environment. Accepts the spellings
// a compose file or a shell actually produces.
func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
