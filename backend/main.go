package main

import (
	"context"
	"corruption-center/api"
	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ctx := context.Background()

	// psql must start first — owns schema_migrations for both dbs
	pg, err := psql.New(ctx, mustEnv("DATABASE_URL"), log)
	if err != nil {
		log.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	mg, err := memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), mustEnv("MEMGRAPH_USER"), mustEnv("MEMGRAPH_PASS"), pg, log)
	if err != nil {
		log.Error("failed to connect to memgraph", "err", err)
		os.Exit(1)
	}
	defer mg.Close(ctx)

	server := api.NewApiServer(pg, mg)
	server.Start(getEnv("PORT", "8080"))
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required environment variable", "key", key)
		os.Exit(1)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
