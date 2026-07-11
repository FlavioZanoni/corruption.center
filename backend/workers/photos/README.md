# Photos Enrichment Worker

Gives graph entities a photo, hotlinking to official servers only — the backend
stores **no image bytes**. `photo_url` is always an absolute external URL.

- **Politicians** should all have a photo. Historical politicians (no photo set
  by the camara/senado syncers) get a **TSE** candidate photo hotlink; any still
  without one can be filled from **Wikidata/Wikimedia Commons**.
- **Organizations** with a Wikidata presence get their **Wikimedia Commons P18
  image** — never the P154 logo (legal reasons).

## Modes

| Mode       | Targets                                            | Source                                                                 |
| ---------- | -------------------------------------------------- | --------------------------------------------------------------------- |
| `tse`      | Politicians with empty `photo_url`                 | TSE candidate photo hotlink, CPF→SQ_CANDIDATO via `consulta_cand` CSVs |
| `wikidata` | Organizations (by CNPJ) + Politicians still unset  | Wikidata SPARQL (P6204→P18) and pt.wikipedia title→Wikidata→P18        |

## Run

```bash
cd backend

# Both modes, all targets
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/photos/cmd --mode=tse,wikidata --year=2022

# Wikidata only (organizations + politician fallback)
DATABASE_URL="postgres://..." MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/photos/cmd --mode=wikidata --limit=100

# Dry run: resolve + verify, no graph writes (Memgraph still required to read targets)
DATABASE_URL="postgres://..." MEMGRAPH_URI="bolt://localhost:7687" \
go run ./workers/photos/cmd --mode=wikidata --dry-run
```

### Flags

- `--mode` — `tse`, `wikidata`, or `tse,wikidata` (default both)
- `--year` — TSE election year for the photo lookup (default `2022`)
- `--uf` — optional UF filter on `Politician.state` (e.g. `SP`)
- `--limit` — per-mode cap on graph targets (0 = all)
- `--dry-run` — resolve + verify but perform no writes
- `--tse-url-template` — override the TSE candidate-photo URL template (placeholders `{year}`, `{uf}`, `{sq}`)
- `--sparql-endpoint` — override the Wikidata SPARQL endpoint
- `--workdir` — where the `consulta_cand` zip is downloaded (metadata only; default system temp)

### Env

- `DATABASE_URL` (**required** — psql is the Memgraph migration tracker)
- `MEMGRAPH_URI` (**required**)
- `MEMGRAPH_USER` / `MEMGRAPH_PASS` (**optional** — empty for the auth-less dev Memgraph)

No `PHOTOS_DIR`: nothing is stored on disk except a transient `consulta_cand`
metadata zip, which is deleted after the CPF→SQ map is built.

## Node property contract (for the API / frontend)

**Politician** (set only when `photo_url` was empty — a camara/senado photo is
never overwritten):

| Property            | Value                                                             |
| ------------------- | ---------------------------------------------------------------- |
| `photo_url`         | absolute hotlink (TSE divulgacandcontas, or Commons FilePath)    |
| `photo_source`      | `TSE Divulgação de Candidaturas {year}` or `Wikimedia Commons`   |
| `photo_attribution` | Commons: `{file} — Wikimedia Commons ({file page URL})`; TSE: "" |

**Organization** (set only when `image_url` was empty):

| Property            | Value                                                     |
| ------------------- | --------------------------------------------------------- |
| `image_url`         | Commons `Special:FilePath/{file}?width=512` hotlink        |
| `photo_url`         | same value (mirrored for the shared contract)             |
| `photo_source`      | `Wikimedia Commons`                                       |
| `photo_attribution` | `{file} — Wikimedia Commons ({file page URL})`            |

> The Organization API model currently exposes only `image_url` and has no
> attribution field — flagged for the API agent, since CC-BY-SA attribution is a
> legal requirement to render.

## Hard rules

- **Never overwrite** a non-empty `photo_url` (Politician) / `image_url`
  (Organization). Enforced in both the read query (targets filter out non-empty)
  and the write query (`WHERE ... IS NULL OR = ''`).
- **Organizations: P18 only, never P154** (logo). The SPARQL query only ever
  binds `wdt:P18`.
- **Politician name matching is fuzzy → never auto-set on ambiguity.** A photo is
  set only when the candidate names (primary + aliases) resolve to exactly one
  Wikidata entity that carries an image. Two distinct entities → skip + count.
- **Every hotlink is runtime-verified.** TSE URLs are `GET`-checked to return
  real image bytes (content-type / magic number) before being written, so a
  wrong URL can never produce bad data.
- **Politeness.** Wikidata/Wikipedia calls are ≤ 1 req/s, use a descriptive
  User-Agent with a contact email, and back off on 429/5xx.

## TSE hotlink status (2026-07-10)

**Verified** — the per-UF TSE candidate photo bundles
(`https://cdn.tse.jus.br/estatistica/sead/eleicoes/eleicoes{year}/fotos/foto_cand{year}_{UF}_div.zip`)
name each photo `F{UF}{SQ_CANDIDATO}_div.jpg` **or** `_div.jpeg` (both
extensions occur; confirmed by downloading `foto_cand2022_RR_div.zip`, e.g.
`FRR230002529954_div.jpg`). CPF→SQ_CANDIDATO comes from the `consulta_cand`
CSVs (`NR_CPF_CANDIDATO`, `SQ_CANDIDATO`).

**Not verified** — a stable, directly-linkable **per-candidate** photo URL. The
`divulgacandcontas` service that serves individual candidate photos was under
scheduled maintenance (its REST API and static paths returned an HTML
"Serviço temporariamente indisponível" page, not images) throughout
development, so no per-candidate hotlink pattern could be confirmed empirically.

Consequently, `--mode=tse` runs a **pre-flight probe**: it constructs a URL from
`--tse-url-template` for a known-existing sample candidate and only proceeds if
that returns real image bytes. Until a working pattern is confirmed, TSE mode
**fails fast** with a clear `not implemented: no stable TSE photo hotlink ...`
error instead of guessing or falling back to downloads. Supply a verified
`--tse-url-template` (with `{year}`/`{uf}`/`{sq}` placeholders) to enable it; the
runtime image verification then guarantees only valid hotlinks are written.

The **Wikidata mode is fully implemented and verified live** (organizations by
CNPJ and the politician fallback).

## Tests

```bash
go test ./workers/photos
```

Unit tests (offline; live calls are never made in tests) cover the CPF→SQ
consulta parse, photo-filename resolution, SPARQL/EntityData response mapping,
Commons URL + attribution building, and the "don't overwrite camara/senado
`photo_url`" rule. The Wikidata client's single-vs-ambiguous match logic is
tested against an `httptest` server.
