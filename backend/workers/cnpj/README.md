# CNPJ Enricher Worker

Enriches `Organization` nodes that carry a CNPJ but are missing detailed data,
extracts the QSA (Quadro de Sócios e Administradores — board/ownership), links
individuals via `CONTROLS`, and follows shell ownership chains via `OWNED_BY`.
Spec: `docs/sources_workers.md` (CNPJ Enricher) and `docs/arch.md`.

## Provider

Primary provider is **minha receita** (`https://minhareceita.org`), which serves
official Receita Federal open data as JSON. The public `cnpj.ws` API is capped at
3 req/min, so it is **not** used as the primary source.

`GET {CNPJ_API_BASE}/{cnpj}` returns, among many fields:

| response field                | mapped to (`Organization`)      |
| ----------------------------- | ------------------------------- |
| `razao_social`                | `name`                          |
| `descricao_situacao_cadastral`| `active` (`ATIVA` → true)       |
| `natureza_juridica`           | `type` (free string)            |
| `uf`                          | `uf`                            |
| `capital_social`              | `share_capital_brl`             |
| `cnae_fiscal_descricao`       | `main_activity`                 |
| `qsa[]`                       | board members (see below)       |

Each `qsa[]` entry carries `nome_socio`, `cnpj_cpf_do_socio`,
`qualificacao_socio` / `codigo_qualificacao_socio`. `cnpj_cpf_do_socio` is either
a **masked CPF** (`***641988**`, an individual) or a full **14-digit CNPJ**
(another company).

## Behavior

For each `Organization` node needing enrichment (`enriched` flag missing/false):

1. Fetch the CNPJ, write the mapped fields, set `enriched = true` and `source_url`
   (the provider deep-link — required for provenance).
2. For each QSA entry, classify `cnpj_cpf_do_socio`:
   - **Masked CPF (individual)** → take the 6 visible middle digits and partial-
     match against `Politician` CPFs (`MatchPoliticiansByMaskedCPF`).
     - Match → `pending_review` type `possible_politician_in_qsa` carrying the
       organization id/cnpj, the candidate politician id(s), the socio name, the
       masked CPF, and the source URL. **Never** auto-creates an edge — a human
       confirms the identity (masked CPFs collide).
     - No match → name-keyed `Person` node + `CONTROLS` edge (person → org).
   - **Full CNPJ (company)** → partner `Organization` (merged by CNPJ) +
     `OWNED_BY` edge (enriched org → partner: the org is owned by its corporate
     shareholder, a shell chain). A newly created partner is enqueued for
     enrichment **within the same run**, bounded to **2 hops** (`maxHops`).
   - **Neither** → logged and skipped.

**LGPD purge guard:** before auto-creating a QSA `Person` (no-politician-match
path) or a partner `Organization`, the worker consults `purge_tombstone`
(migration 008) via `IsSubjectPurged` — keyed on the socio **name** for
individuals (QSA exposes only a masked CPF, no full digits) and on the partner
**CNPJ** for companies. A purged subject is skipped (no node, no edge, no
shell-chain expansion) and counted in `stats.skipped_tombstoned`.

### Depth ceiling

Shell chains are followed at most **2 hops per run** (`maxHops` in `worker.go`,
marked `// ponytail:`). Partner orgs created beyond the ceiling keep their
un-enriched flag and are picked up by a later scheduled run, so deep chains
resolve incrementally instead of via one unbounded recursive crawl.

## Politeness

- Configurable rate limiter, default **60 req/min** (`CNPJ_RATE_PER_MIN`).
- Exponential backoff on 429/5xx (1s → 60s).
- `User-Agent: corruption.center-cnpj/1.0 (contato@corruption.center)`.
- Provider `404` (unknown CNPJ) is skipped, not an error.

> **Pointing at the shared PUBLIC minha receita instance?** It is a courtesy
> service — set `CNPJ_RATE_PER_MIN` low (e.g. `10`). The 60/min default assumes a
> self-hosted instance.

## Configuration

| env / flag           | default                    | purpose                                  |
| -------------------- | -------------------------- | ---------------------------------------- |
| `CNPJ_API_BASE`      | `https://minhareceita.org` | provider base URL (`--base-url` overrides)|
| `CNPJ_RATE_PER_MIN`  | `60`                       | request rate; lower for the public instance |
| `DATABASE_URL`       | —  (required)              | Postgres (pending reviews)               |
| `MEMGRAPH_URI`       | —  (required)              | Memgraph (graph writes)                  |
| `MEMGRAPH_USER/PASS` | optional                   | dev Memgraph is auth-less                |

## Run

```bash
cd backend
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
CNPJ_API_BASE="https://minhareceita.org" \
CNPJ_RATE_PER_MIN="10" \
go run ./workers/cnpj/cmd --limit 100

# single-shot, no writes (testing):
go run ./workers/cnpj/cmd --cnpj 33683111000280 --dry-run
```

Flags: `--limit` (max root orgs, 0 = all needing enrichment), `--dry-run` (fetch
and classify, no writes; also skips shell-chain expansion), `--cnpj` (enrich one
CNPJ), `--base-url` (provider override).

## Storage

- `Organization` nodes: enriched fields + `enriched`/`source_url`.
- `Person` nodes + `CONTROLS` edges (QSA individuals with no politician match).
- `Organization` + `OWNED_BY` edges (QSA corporate partners — shell chains).
- `pending_review` type `possible_politician_in_qsa` (already in the CHECK
  constraint, migration `002_djen.sql`) — masked-CPF hits on a Politician.

No dedicated Postgres migration or state table: the "needs enrichment" queue is
derived from the graph (`enriched` flag), and the only review type used already
exists.
