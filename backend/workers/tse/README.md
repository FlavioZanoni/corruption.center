# TSE Worker

Imports historical federal election winners from TSE bulk ZIP files.

## What It Does

- Processes yearly `votacao_candidato_munzona_{year}.zip` and `consulta_cand_{year}.zip`.
- Reads `_BR` and per-UF files, joining on `SQ_CANDIDATO`.
- Keeps only winners by rule:
  - `DS_CARGO` in `DEPUTADO FEDERAL`, `SENADOR`, `PRESIDENTE`, `VICE-PRESIDENTE`
  - `DS_SIT_TOT_TURNO` in `ELEITO`, `ELEITO POR QP`, `ELEITO POR MÉDIA`
  - highest `NR_TURNO` per `SQ_CANDIDATO`
- Produces records with:
  - `active: false`
  - `tse_profile_urls: []string`
  - aliases from `NM_URNA_CANDIDATO` and `NM_SOCIAL_CANDIDATO`

## CLI Usage

From `backend/` directory:

```bash
go run ./workers/tse/cmd \
  --year 2022 \
  --workdir "$HOME/tmp/tse"
```

If you are at repo root, run `cd backend` first.

By default, ZIP files are downloaded from official TSE URLs and cached under
`<workdir>/tse-downloads`.

Range mode (directory with all yearly zip files):

```bash
go run ./workers/tse/cmd \
  --from-year 2006 \
  --to-year 2022 \
  --zip-dir "/path/to/tse-zips"
```

All-years mode:

```bash
go run ./workers/tse/cmd \
  --all-years \
  --zip-dir "/path/to/tse-zips"
```

Optional flags:

- `--workdir` temp processing directory (default system temp)
- `--min-disk-mb` minimum free disk required (default `512`)
- `--min-mem-mb` minimum available memory required (default `256`)
- `--zip-dir` directory mode for range/all
- `--votacao-zip` and `--consulta-zip` optional overrides for single-year mode
- `--from-year` / `--to-year` range mode (inclusive)
- `--all-years` runs every even year from 2002 to 2024
- `--persist-db` writes imported records to Postgres/Memgraph
- `--skip-processed` skips years already marked `success` in `tse_import_log` (default `true`)
- `--batch-size` Memgraph write batch size (default `500`)
- `--trigger-camara` runs Camara sync after TSE finishes (requires `--persist-db`)
- `--trigger-senado` runs Senado sync after TSE finishes (requires `--persist-db`)

The command writes JSON output to stdout containing one `runs[]` entry per processed year.

## Run Examples

Single year, parse only (no DB writes):

```bash
go run ./workers/tse/cmd \
  --year 2022 \
  --workdir "$HOME/tmp/tse"
```

Single year with persistence to Postgres + Memgraph:

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/tse/cmd \
  --year 2022 \
  --persist-db \
  --batch-size 500
```

Range mode with persistence (skips already successful years by default):

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/tse/cmd \
  --from-year 2006 \
  --to-year 2022 \
  --zip-dir "$HOME/data/tse" \
  --persist-db
```

All years from 2002 to 2024 (manual full refresh):

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/tse/cmd \
  --all-years \
  --zip-dir "$HOME/data/tse" \
  --persist-db \
  --skip-processed=false
```

## Persistence Mode

When `--persist-db` is enabled, the CLI will:

1. Create/update a `scraper_jobs` run entry.
2. Mark year status in `tse_import_log` (`running` → `success`/`failed`).
3. Upsert `Politician` nodes into Memgraph in batches.

Required environment variables in persistence mode:

- `DATABASE_URL`
- `MEMGRAPH_URI`
- `MEMGRAPH_USER`
- `MEMGRAPH_PASS`

## Tests

Run worker tests:

```bash
go test ./workers/tse
```

Run backend integration tests (requires Docker):

```bash
cd backend
go test -tags integration ./db/psql ./db/memgraph
```

## CLI Status Output

The CLI writes progress/status logs to stderr, including:

- year start/finish
- download/cache usage
- per-year record/file summary

Final JSON result remains on stdout.
