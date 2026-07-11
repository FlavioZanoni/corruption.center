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
	"corruption-center/workers/camara"
)

type output struct {
	Stats   camara.SyncStats      `json:"stats"`
	Records []camara.DeputyRecord `json:"records"`
}

func main() {
	var (
		baseURL  = flag.String("base-url", "", "Camara API base URL override")
		maxPages = flag.Int("max-pages", 0, "Max pages to fetch (0 = all)")
		persist  = flag.Bool("persist-db", false, "Persist sync output into Postgres and Memgraph")
		batch    = flag.Int("batch-size", 500, "Memgraph upsert batch size")
	)
	flag.Parse()

	ctx := context.Background()

	res, err := camara.SyncCurrentDeputies(ctx, camara.SyncOptions{
		BaseURL:  *baseURL,
		Items:    100,
		MaxPages: *maxPages,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "camara worker failed: %v\n", err)
		os.Exit(1)
	}

	if *persist {
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
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

		run, err := pg.CreateCamaraSyncRun(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create camara run: %v\n", err)
			os.Exit(1)
		}

		upserted, err := mg.UpsertPoliticiansFromCamara(ctx, res.Records, *batch)
		if err != nil {
			errMsg := err.Error()
			_ = pg.FinalizeCamaraSyncRun(ctx, run.ID, psql.JobStatusFailed, res.Stats, 0, &errMsg)
			fmt.Fprintf(os.Stderr, "persist camara sync: %v\n", err)
			os.Exit(1)
		}

		if err := pg.FinalizeCamaraSyncRun(ctx, run.ID, psql.JobStatusSuccess, res.Stats, upserted, nil); err != nil {
			fmt.Fprintf(os.Stderr, "finalize camara run: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[camara] persisted records=%d listed=%d details=%d\n", upserted, res.Stats.ListedDeputies, res.Stats.DetailFetched)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output{Stats: res.Stats, Records: res.Records}); err != nil {
		fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
		os.Exit(1)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(2)
	}
	return v
}
