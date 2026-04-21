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

Stop dev stack:

```bash
docker compose -f docker-compose.dev.yml down
```
