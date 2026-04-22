# corruption.center

## Contract Verification

Run this before opening a PR:

```bash
./verify-contracts.sh
```

The script does three checks:

1. Regenerates backend Swagger docs from handler annotations.
2. Runs backend Go tests.
3. Builds frontend (includes TypeScript checks).

## Frontend API Modes

The frontend supports two API modes:

- Mock Next.js API routes (default): leave `NEXT_PUBLIC_API_URL` empty.
- Real Go API: set `NEXT_PUBLIC_API_URL` to the backend base URL (example: `http://localhost:8080`).

Current compatibility notes:

- Search: frontend adapter accepts backend `GraphResponse` and maps it to UI `SearchResponse`.
- Timeline: frontend sends `from`/`to` as `YYYY-MM-DD`, matching Go API expectations.

## Dev Stack (API, Frontend, DBs, Workers)

Use `docker-compose.dev.yml` to run everything needed for local end-to-end testing.

Start core services:

```bash
docker compose -f docker-compose.dev.yml up -d postgres memgraph api frontend
```

This exposes:

- Postgres on `localhost:5432`
- Memgraph Bolt on `localhost:7687` and HTTP on `localhost:7444`
- Go API on `localhost:8080`
- Frontend on `localhost:3000`

Run worker containers on demand (profile `workers`):

```bash
docker compose -f docker-compose.dev.yml --profile workers run --rm camara-sync
docker compose -f docker-compose.dev.yml --profile workers run --rm senado-sync
docker compose -f docker-compose.dev.yml --profile workers run --rm datajud-watcher
```

Run TSE manually from host while DBs are up:

```bash
cd backend
go run ./workers/tse/cmd --year 2022 --persist-db
```

Optional post-TSE sync triggers:

```bash
go run ./workers/tse/cmd --year 2022 --persist-db --trigger-camara --trigger-senado
```

Custom range in dev (test mode):

```bash
cd backend
go run ./workers/tse/cmd --from-year 2018 --to-year 2022 --persist-db
```

Stop dev stack:

```bash
docker compose -f docker-compose.dev.yml down
```

## Worker Deployment Notes

- `camara` and `senado` are intended to be scheduled weekly jobs (cron/systemd/K8s CronJob).
- `datajud-watcher` is configured as an on-demand/cron worker profile in compose (not auto-started).
- `tse` is manual. In production run from host/server:

```bash
cd backend
DATABASE_URL="..." MEMGRAPH_URI="..." MEMGRAPH_USER="..." MEMGRAPH_PASS="..." DATAJUD_API_KEY="..." \
go run ./workers/tse/cmd --year 2022 --persist-db --trigger-camara --trigger-senado
```

If you do not want the automatic post-run syncs, omit `--trigger-camara` and
`--trigger-senado` and run weekly jobs normally.

## Cron With Docker Compose

Use compose worker profile jobs with cron via:

- `deploy/cron/run-compose-worker.sh`
- `deploy/cron/crontab.dev.example`
- `deploy/cron/crontab.prod.example`

Install (after editing absolute paths):

```bash
crontab deploy/cron/crontab.dev.example
```

or

```bash
crontab deploy/cron/crontab.prod.example
```

Notes:

- Uses `flock` lock files to avoid overlapping runs per worker.
- Logs each run to `logs/workers/<service>.log`.
- Runs workers as `docker compose --profile workers run --rm <service>`.
