# TSE Worker

Imports historical election winners (federal and state-level) from TSE bulk ZIP files.
Spec: `docs/workerDetails/TSE.md`.

## What It Does

- Processes a single yearly `consulta_cand_{year}.zip` (about 4MB zipped): 28 CSVs,
  one per state plus a national `_BR` file carrying Presidente/Vice-Presidente.
- There is no second file and no join: `consulta_cand` already carries the office
  (`DS_CARGO`), the result (`DS_SIT_TOT_TURNO`), the state, the party, the names
  and the CPF. The `votacao_candidato_munzona` files this importer used to also
  require added nothing but vote tallies, which this project does not use, and
  cost 552MB zipped per year against 4MB here; unzipping them exhausted the disk
  and made the 2022 import fail outright, so they were dropped entirely.
- Keeps only winners by rule:
  - `DS_CARGO` in `PRESIDENTE`, `VICE-PRESIDENTE`, `SENADOR`, `DEPUTADO FEDERAL`,
    `GOVERNADOR`, `VICE-GOVERNADOR`, `DEPUTADO ESTADUAL`, `DEPUTADO DISTRITAL`
    (`allowedCargos` in `importer.go`; applied to every file, including `_BR`)
  - `DS_SIT_TOT_TURNO` in `ELEITO`, `ELEITO POR QP`, `ELEITO POR MÉDIA` (plus the
    legacy spellings `MÉDIA`, `MEDIA`, `QP`, `ELEITO POR MEDIA`)
  - highest `NR_TURNO` per `(SG_UF, SQ_CANDIDATO)` (`candidateKey`, `keepLatestTurn`):
    `SQ_CANDIDATO` alone is not unique across states in the older files, so winners
    are deduplicated by state and SQ together, not by SQ alone
- Produces records with:
  - `active: false`
  - `tse_profile_urls: []string`
  - aliases from `NM_URNA_CANDIDATO` and `NM_SOCIAL_CANDIDATO`

### Why the office list is wide

This base is the ceiling on every later match. A person who is not a `Politician` node
here can never be tied to a court party or a sanction: they stay an anonymous `Person`
node and the case never reaches a politician page (see `docs/identity_matching.md`).
State governors in particular are heavily represented in corruption prosecutions, so the
filter covers the state executives and legislators as well as the federal offices.

Municipal offices (prefeito, vereador) are elected in the other even-year cycle (2004,
2008, 2012, …) and are not present in these files. Running `--all-years` over a municipal
year is harmless: every row is dropped by the cargo filter and the year yields no records.

## CLI Usage

From `backend/` directory:

```bash
go run ./workers/tse/cmd \
  --year 2022 \
  --workdir "$HOME/tmp/tse"
```

If you are at repo root, run `cd backend` first.

By default, the `consulta_cand_{year}.zip` file is downloaded from the official TSE URL
and cached under `<workdir>/tse-downloads`.

### Downloads: retry with Range resume

At about 4MB zipped, the file is small enough that a plain `GET` usually succeeds in one
shot, but `downloadFileIfMissing` still retries up to `downloadRetries` (6) times, and
each attempt **resumes** the partial `.tmp` with an HTTP `Range: bytes=<have>-` request
(`resumeDownload`) instead of starting over; a server answering `200` instead of `206`
restarts from zero. Backoff between attempts is 2s, 4s, 6s, … The `.tmp` is renamed to
its final name only after the body was copied cleanly, so a cached file is always a
complete file. Progress and each interruption are logged to stderr.

Range mode (directory with all yearly zip files):

```bash
go run ./workers/tse/cmd \
  --from-year 2006 \
  --to-year 2022 \
  --zip-dir "/path/to/tse-zips"
```

All-years mode (every even year from 2002 to the latest election year). `--zip-dir` is
optional: without it, each year's zip is downloaded (with Range resume) and cached in
`<workdir>/tse-downloads`.

```bash
go run ./workers/tse/cmd \
  --all-years \
  --zip-dir "/path/to/tse-zips"

# Full base, downloading everything, persisted:
DATABASE_URL="postgres://..." MEMGRAPH_URI="bolt://localhost:7687" \
go run ./workers/tse/cmd \
  --all-years \
  --workdir "$HOME/tmp/tse" \
  --persist-db
```

Municipal years (2004, 2008, …) are walked too and produce zero records: their offices
(prefeito, vereador) are not in `allowedCargos`. Exactly one mode may be given:
`--year`, or `--from-year`/`--to-year`, or `--all-years`.

After a full `--all-years` run the politician base is much larger than it was, so the
`Person` defendants DJEN discovered earlier may now match a politician. Run
`go run ./workers/djen/cmd --case-mode=false --name-mode=false --rematch-mode` to re-test
them (`docs/workerDetails/DJEN.md`).

Optional flags:

- `--workdir` temp processing directory (default system temp)
- `--min-disk-mb` minimum free disk required (default `512`)
- `--min-mem-mb` minimum available memory required (default `256`)
- `--zip-dir` directory mode for range/all
- `--consulta-zip` optional override for single-year mode
- `--from-year` / `--to-year` range mode (inclusive)
- `--all-years` runs every even year from 2002 to the latest election year
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

All years from 2002 to the latest election year (manual full refresh):

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

Trigger Camara/Senado after TSE completes:

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/corruption_center?sslmode=disable" \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="memgraph" \
MEMGRAPH_PASS="memgraph" \
go run ./workers/tse/cmd \
  --year 2022 \
  --persist-db \
  --trigger-camara \
  --trigger-senado
```

## Persistence Mode

When `--persist-db` is enabled, the CLI will:

1. Create/update a `scraper_jobs` run entry.
2. Mark year status in `tse_import_log` (`running` → `success`/`failed`).
3. Upsert `Politician` nodes into Memgraph in batches.

Required environment variables in persistence mode:

- `DATABASE_URL`
- `MEMGRAPH_URI`

`MEMGRAPH_USER` / `MEMGRAPH_PASS` are optional: the dev Memgraph is auth-less and the dev
compose stack passes empty strings.

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
