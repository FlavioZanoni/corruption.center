# Senado Worker

Syncs current senators from Senado Open Data list endpoint.

## Behavior

- Fetches `GET /senador/lista/atual.json` in a single request.
- Keeps only active entries (`Mandato.DescricaoParticipacao = Titular` and with Exercicios).
- Builds normalized senator records with:
  - `name`
  - `party_current`
  - `role_current = Senador`
  - `state`
  - `photo_url`
  - `active = true`

CPF is not available in Senado feed. Upsert strategy in graph uses name+state match.

## CLI

From `backend/`:

```bash
go run ./workers/senado/cmd
```

Persist to Postgres + Memgraph:

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/senado/cmd --persist-db
```

## Tests

```bash
go test ./workers/senado
```
