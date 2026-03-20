//go:build integration

package psql

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestJobAndTSEImportLogRoundtrip(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := startPostgresContainer(t, ctx)
	defer cleanup()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	db, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("create psql db: %v", err)
	}
	defer db.Close()

	jobID, err := db.CreateJob(ctx, "tse_csv_import")
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	last, err := db.GetLastJob(ctx, "tse_csv_import")
	if err != nil {
		t.Fatalf("get last job: %v", err)
	}
	if last == nil || last.ID != jobID {
		t.Fatalf("unexpected last job: %#v", last)
	}
	if last.Status != string(JobStatusRunning) {
		t.Fatalf("expected running status, got %s", last.Status)
	}

	if err := db.UpdateJob(ctx, jobID, JobStatusSuccess, 42, nil); err != nil {
		t.Fatalf("update job: %v", err)
	}

	last, err = db.GetLastJob(ctx, "tse_csv_import")
	if err != nil {
		t.Fatalf("get updated job: %v", err)
	}
	if last.RecordsUpserted != 42 {
		t.Fatalf("expected records_upserted=42, got %d", last.RecordsUpserted)
	}
	if last.Status != string(JobStatusSuccess) {
		t.Fatalf("expected success status, got %s", last.Status)
	}

	year := 2022
	ok, err := db.IsTSEYearSuccessful(ctx, year)
	if err != nil {
		t.Fatalf("check successful year before upsert: %v", err)
	}
	if ok {
		t.Fatalf("expected year %d to not be successful yet", year)
	}

	if err := db.UpsertTSEImportLog(ctx, year, JobStatusRunning, 0, nil); err != nil {
		t.Fatalf("upsert running tse import log: %v", err)
	}
	ok, err = db.IsTSEYearSuccessful(ctx, year)
	if err != nil {
		t.Fatalf("check successful year while running: %v", err)
	}
	if ok {
		t.Fatalf("expected year %d to still be non-success while running", year)
	}

	if err := db.UpsertTSEImportLog(ctx, year, JobStatusSuccess, 50, nil); err != nil {
		t.Fatalf("upsert success tse import log: %v", err)
	}
	ok, err = db.IsTSEYearSuccessful(ctx, year)
	if err != nil {
		t.Fatalf("check successful year after success: %v", err)
	}
	if !ok {
		t.Fatalf("expected year %d to be successful", year)
	}
}

func startPostgresContainer(t *testing.T, ctx context.Context) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "corruption_center_test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("get postgres host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("get postgres port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/corruption_center_test?sslmode=disable", host, port.Port())
	cleanup := func() {
		_ = container.Terminate(context.Background())
	}

	return dsn, cleanup
}
