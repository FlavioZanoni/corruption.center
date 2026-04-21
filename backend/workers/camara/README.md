# Camara Worker

Syncs currently active federal deputies from Camara Open Data.

## Behavior

- Lists current deputies using `GET /deputados` (default response is current/active).
- Fetches detail per deputy via `GET /deputados/{id}`.
- Builds normalized records to upsert by `cpf`.
- Marks all imported records as `active: true`.
- Sets `role_current` to `Deputado Federal`.

## Mapping

- `cpf` from detail endpoint (required)
- `name` from `nomeCivil` (fallback: list `nome`)
- `party_current` from detail `ultimoStatus.siglaPartido` (fallback list)
- `state` from detail `ultimoStatus.siglaUf` (fallback detail/list)
- `photo_url` from detail `ultimoStatus.urlFoto` (fallback list)

## Run tests

```bash
go test ./workers/camara
```

## CLI

From `backend/`:

```bash
go run ./workers/camara/cmd
```

Persist to Postgres + Memgraph:

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/camara/cmd --persist-db
```

## Weekly entrypoint

Use `deploy/camara-sync.sh` in cron/systemd. It:

- validates required env vars
- waits for Postgres readiness
- waits for Memgraph readiness
- runs `go run ./workers/camara/cmd --persist-db`
