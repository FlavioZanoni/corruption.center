# corruption.center

A graph database of Brazilian corruption scandals: who was convicted, who was
merely cited, and which company, court case or sanction connects them.

Every fact comes from an **official Brazilian government source**. The project
never scrapes court front-ends or third-party aggregators, and it never adds or
infers information beyond what the official records state. See
[docs/legal_compliance.md](docs/legal_compliance.md).

## How the data gets in

The graph is seeded **by scandal** and grown by workers:

1. A scandal and its landmark case numbers are registered (the baseline seed does
   this on every API start; an operator can add more in the backoffice).
2. The **DataJud watcher** polls each case for its status (accepted, sentenced,
   concluded, whether there was a conviction). DataJud does not publish case
   parties, by law (Portaria CNJ 160/2020).
3. **DJEN** supplies the parties, because court publications name them. Its
   coverage starts around 2023, which is a hard limit: a case closed in 2017 has
   no party list obtainable from any official source.
4. **Sanctions** (CGU: CEIS, CNEP, CEAF, leniency agreements; TCU) add the
   punishment layer. These are keyed by CPF/CNPJ and reach back decades.
5. **CNPJ** and **photos** enrichers fill in companies and portraits.
6. Anything that cannot be identified from a document lands in a **human review
   queue** before it is published.

## Who counts as a politician

The politician base comes from **TSE**, every winner of a general election since
2002: president, vice-president, senator, federal deputy, governor,
vice-governor, state and district deputy. The Câmara and Senado syncs only mark
who is currently in office. A person absent from this base can never be matched
to a court case or a sanction: they stay an anonymous `Person`.

## Identifying a person is an inference, and it is gated

Official sources are trustworthy about **what happened** and often silent about
**who it happened to**. DJEN publishes names with no document at all; CGU masks
CPFs (`***.435.151-**`).

**A document identifies. A name only leads.**

| Evidence | Outcome |
| --- | --- |
| Full CPF/CNPJ published by the source | link created automatically |
| CGU masked CPF + exact name | link created automatically |
| A name alone, however exact | human review, always |

The threshold is calibrated so no amount of name-only evidence can reach it. Every
automatic link stores the evidence behind it (`confidence`, `confidence_signals`)
and the UI shows it. Full policy: [docs/identity_matching.md](docs/identity_matching.md).

## Quickstart

```bash
cp .env.example .env      # then fill in the API keys (see the file's comments)
docker compose -f docker-compose.dev.yml up -d postgres memgraph api frontend
```

- Frontend: http://localhost:3000
- API: http://localhost:8080 (Swagger at `/swagger/index.html`)
- Backoffice: http://localhost:8080/backoffice (`BACKOFFICE_USER` / `BACKOFFICE_PASS`)
- Memgraph Bolt: `localhost:7687`
- Postgres: `localhost:${POSTGRES_HOST_PORT}` (default 5432; set it if that port is taken)

The API seeds the baseline scandals (Lava Jato, Calicute, Mensalão) on every
start and registers their cases with the watchers, so a fresh install has real
content immediately. Set `SEED_BASELINE=false` to skip.

All configuration lives in `.env`; compose reads it. Never pass secrets on the
command line.

## Workers

Run on demand with the `workers` profile:

```bash
docker compose -f docker-compose.dev.yml --profile workers run --rm <service>
```

| Service | Source | What it does |
| --- | --- | --- |
| `camara-sync` | Câmara API | Current federal deputies; marks who is in office |
| `senado-sync` | Senado API | Current senators (updates only: no CPF in the source) |
| `datajud-watcher` | CNJ DataJud | Case status and movements for every watched case |
| `djen-sync` | CNJ DJEN | Case parties; discovers and registers new cases by politician name |
| `sanctions-sync` | CGU + TCU | CEIS, CNEP, CEAF, leniency agreements, TCU lists |
| `cnpj-enricher` | Minha Receita | Company data from official Receita Federal records |
| `photos-enricher` | TSE, Wikimedia | Portrait URLs (hotlinked, never rehosted) |

TSE is a bulk import, run it directly (it downloads the official zips itself and
resumes interrupted transfers, which the TSE CDN causes routinely):

```bash
docker compose -f docker-compose.dev.yml --profile workers run --rm djen-sync \
  sh -lc 'go run ./workers/tse/cmd --all-years --persist-db --workdir /workspace/.tse-work'
```

Two DJEN modes worth knowing:

- `-name-mode` searches DJEN by each politician's name, and **registers** the
  criminal/improbidade cases it finds. Registering a case makes no claim about a
  person, so it needs no review.
- `-rematch-mode` re-tests the `Person` defendants already in the graph against
  the current politician base. Run it after a TSE import: parties are matched only
  once at discovery, so defendants found while the base was smaller would otherwise
  stay anonymous forever. For pre-2023 cases this is the only path to a politician.

Scheduling: see `deploy/cron/`. Each run takes a `flock` lock and logs to
`logs/workers/<service>.log`.

## Frontend

Next.js App Router. The interactive graph is a client component; everything a
crawler needs is server-rendered:

- `/` graph explorer, `/politicos` browse, `/metodologia` methodology and disclaimer
- `/politico/[id]` and `/escandalo/[id]`: server-rendered entity pages with
  per-entity metadata and JSON-LD, plus `sitemap.ts` and `robots.ts`
- Set `NEXT_PUBLIC_API_URL` to the Go API. Leave it empty to run against the mock
  API routes in `frontend/app/api/`.

## Before opening a PR

```bash
./verify-contracts.sh   # regenerates Swagger, runs Go tests, builds the frontend
```

## Docs

| Doc | What is in it |
| --- | --- |
| [docs/arch.md](docs/arch.md) | System architecture, services, worker pipeline |
| [docs/identity_matching.md](docs/identity_matching.md) | How a record is tied to a person, and when a human must decide |
| [docs/legal_compliance.md](docs/legal_compliance.md) | LGPD posture, removal requests, review discipline, risk table |
| [docs/sources_workers.md](docs/sources_workers.md) | Each source and worker: schedule, inputs, outputs |
| [docs/graph_nodes_edges.md](docs/graph_nodes_edges.md) | Node and edge types and their properties |
| [docs/workerDetails/](docs/workerDetails/) | Deep specs per worker |
