# Sanctions Sync Worker

Ingests official Brazilian punishment registries into the graph as `Sanction`
nodes with deterministic, CPF/CNPJ-keyed `SANCTIONED_IN` edges. Two sources,
one shared graph shape:

- **CGU Portal da Transparência** (JSON API, free key): CEIS, CNEP, CEAF,
  Acordos de Leniência.
- **TCU** (open CSV downloads, no auth): contas julgadas irregulares,
  inabilitados para função pública, licitantes inidôneos.

See `docs/workerDetails/SANCTIONS.md` for the authoritative spec. CNCIAI
(CNJ improbidade) is **deferred** and intentionally not implemented here.

## Run

```bash
cd backend

# All registries (default), writing to Memgraph + Postgres reviews
TRANSPARENCIA_API_KEY="..." \
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/sanctions/cmd --registries=ceis,cnep,ceaf,leniencia,tcu

# TCU only, no key needed
DATABASE_URL="postgres://..." MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/sanctions/cmd --registries=tcu

# Dry run: parse + classify, no writes (Memgraph not required)
DATABASE_URL="postgres://..." \
go run ./workers/sanctions/cmd --registries=tcu --dry-run
```

Flags: `--registries` (`ceis,cnep,ceaf,leniencia,tcu`, default all),
`--dry-run`, `--max-pages` (CGU page cap, 0 = until empty),
`--cgu-base-url` / `--tcu-base-url` (test overrides).

Env: `TRANSPARENCIA_API_KEY` (**required only when a CGU registry - `ceis`,
`cnep`, `ceaf`, or `leniencia` - is selected**; `--registries=tcu` needs no
key), `DATABASE_URL` (required), `MEMGRAPH_URI` (required unless `--dry-run`).

**Keyless degrade (no `TRANSPARENCIA_API_KEY`):** when the key is empty and the
selection includes CGU **and** TCU registries (the default `all` case), the CGU
registries are logged as skipped (counted in `stats.skipped_registries`) and the
keyless TCU lists still run - the run exits success if TCU succeeds. Selecting
**only** CGU registries with no key remains a hard error (nothing runnable).
`MEMGRAPH_USER` / `MEMGRAPH_PASS` are **optional** - leave them empty for the
auth-less dev Memgraph (the dev compose stack passes empty strings).

Schedule: **weekly**. CGU is rate-limited to 90 req/min daytime; the client
paces requests at ~0.7 s each and backs off on 429/5xx.

## Matching policy (hard rules)

The scoring rules, weights and auto-link threshold are defined once, in
`docs/identity_matching.md` (implementation: `backend/matching`). This worker
applies them; it does not define them.

| Source identity                         | Action                                                                 |
| --------------------------------------- | ---------------------------------------------------------------------- |
| Full **CNPJ**                           | Upsert `Sanction`; ensure `Organization` (bare node if new, so the CNPJ Enricher picks it up); `SANCTIONED_IN` edge. Document grade, no review. |
| Full **CPF** (11 digits, e.g. TCU rows) | Match existing `Politician`/`Person` by CPF, else create `Person` keyed by CPF; `SANCTIONED_IN` edge. Document grade, no review. |
| **Masked CPF** (`***.435.151-**`)       | Match against `Politician` CPFs on the 6 visible middle digits, then **score the match** (`matching.AutoLink`): the six digits alone identify nobody (many people share them), so the name is weighed with them. Masked CPF **+ exact name** reaches document grade (≥ `matching.AutoLinkThreshold`) and is **auto-linked** (counted in `stats.auto_linked`). A name that does not match, or evidence that fits more than one politician, → `pending_review` (`possible_politician_sanction`) carrying both ids, `confidence`, `confidence_signals` and `source_url`. |
| **Name only**                           | `pending_review` only, and only when the name matches a known `Politician`. A name can never reach the threshold on its own. |

Every `SANCTIONED_IN` edge is written with `confidence` (float) and
`confidence_signals` (e.g. `full_document`, `masked_cpf_middle6`, `exact_name`,
`long_name`, `ambiguous_match`), so any link can be explained after the fact and
the frontend can show how the subject was identified.

Every `Sanction` node carries a `source_url` deep link (legal requirement):
`UpsertSanction` rejects records without one. The source link vouches for the
sanction, not for the identification of its subject; that is what the confidence
properties are for.

**LGPD purge guard:** before auto-creating a `Person` (full-CPF path) or
`Organization` (full-CNPJ path), and before auto-linking a `Politician` on the
masked-CPF path, the worker consults `purge_tombstone` (migration 008) via
`IsSubjectPurged`. A purged subject is **skipped** (no node, no edge) and counted
in `stats.skipped_tombstoned`, so a purged subject is never resurrected by a
later sync.

**Review dedup:** masked-CPF and name-only matches file at most one
`possible_politician_sanction` review per `(sanction_id, politician_id)` pair.
`HasSanctionReview` checks for an existing review in **any** status (including
operator-rejected), so weekly re-runs do not pile up duplicates; suppressed
duplicates are counted in `stats.skipped_duplicate_review`.

**Stats:** `records_processed`, `sanctions_upserted`, `orgs_created`,
`persons_created`, `edges_created`, `masked_cpf_matches`, `auto_linked` (edges
written with no review because the evidence reached document grade),
`pending_reviews`, `name_only`, `skipped_tombstoned`, `skipped_duplicate_review`,
`skipped_registries`, `per_registry`.

## Graph shape

```txt
Sanction  (id = registry + ":" + entry id, unique)
  · registry       CEIS | CNEP | CEAF | LENIENCIA | TCU_IRREGULAR | TCU_INABILITADO | TCU_INIDONEO
  · sanction_type  registry-specific description
  · organ          sanctioning body
  · date_start, date_end (nullable, yyyy-mm-dd)
  · process_ref    official process / acórdão reference
  · source_url     deep link to the official record

(Politician|Person|Organization)-[:SANCTIONED_IN]->(Sanction)
```

Migrations: `db/memgraph/migrations/002_sanction.cypher` (unique constraint +
indexes), `db/psql/migrations/003_sanctions.sql` (import-state table + adds
`possible_politician_sanction` to the `pending_review` type whitelist).

## Source schemas (verified live before coding)

### CGU - `GET api.portaldatransparencia.gov.br/api-de-dados/{registry}?pagina=N`

Header `chave-api-dados: $TRANSPARENCIA_API_KEY`. Verified against the live
OpenAPI spec at `https://api.portaldatransparencia.gov.br/v3/api-docs`. The
public endpoint **requires the key** (401 without it), so the response structs
were coded from the OpenAPI schema, not a live sample - masking format for
individual CPFs (`***.XXX.XXX-**`) is the documented Portal convention.

| Registry     | Path                 | Item DTO             | Identity field(s) used                                |
| ------------ | -------------------- | -------------------- | ----------------------------------------------------- |
| CEIS         | `ceis`               | `CeisDTO`            | `pessoa.cnpjFormatado` / `pessoa.cpfFormatado` (masked) → fallback `sancionado.codigoFormatado` |
| CNEP         | `cnep`               | `CnepDTO`            | same as CEIS                                          |
| CEAF         | `ceaf`               | `CeafDTO`            | `pessoa.cpfFormatado` / `punicao.cpfPunidoFormatado` (masked - public servants) |
| Leniência    | `acordos-leniencia`  | `AcordosLenienciaDTO`| `sancoes[].cnpjFormatado` (one Sanction per company; see EntryID note) |

- Query params include `pagina` (required). Pagination runs until an empty page.
- CEIS/CNEP carry `linkPublicacao` → used directly as `source_url`. CEAF and
  Leniência have no publication link; a best-effort public portal URL is built
  (`portaldatransparencia.gov.br/sancoes/{ceis|cnep|ceaf}?id=` /
  `/acordos-leniencia?id=`). These fallbacks are noted as best-effort.
- Dates arrive as `dd/MM/yyyy` (also handles ISO); normalized to `yyyy-mm-dd`.
- Leniência EntryID = `<agreementId>-<discriminator>`. The discriminator is the
  company CNPJ; when a sanctioned company carries no document it falls back to a
  deterministic name slug (uppercase, accents folded, non-alphanumerics collapsed
  to `-`, e.g. `7-RAZAO-SOCIAL-LTDA`), and to the company's index within the
  agreement when the name is also empty. This keeps distinct document-less
  companies in one agreement from colliding on the same `Sanction` node.

### TCU - CSV downloads

Base: `https://sites.tcu.gov.br/dados-abertos/inidoneos-irregulares/arquivos/`.
The listing page is JS-rendered; the real filenames come from the manifest
`inidoneos-irregulares-arquivos.csv`. Files downloaded and inspected live:

| Registry code       | File                                        | Rows (2026-07) |
| ------------------- | ------------------------------------------- | -------------- |
| `TCU_IRREGULAR`     | `resp-contas-julgadas-irregulares.csv`      | ~34.9k         |
| `TCU_INABILITADO`   | `inabilitados-funcao-publica.csv`           | ~720           |
| `TCU_INIDONEO`      | `licitantes-inidoneos.csv`                  | ~98            |

Format: **pipe-delimited (`|`)**, every field double-quoted, **UTF-8**, one
header row. The column layout differs between files, so the parser is
**header-driven** (maps by column name, not position):

- `resp-contas-julgadas-irregulares.csv` (7 cols):
  `NOME | CPF_CNPJ | PROCESSO | DELIBERACAO | DATA TRANSITO JULGADO | UF | MUNICIPIO` - here `DELIBERACAO` is an **acórdão URL** (used directly as `source_url`).
- `inabilitados-funcao-publica.csv` (9 cols) and `licitantes-inidoneos.csv`
  (9 cols):
  `NOME | CPF(_CNPJ) | PROCESSO | DELIBERACAO | DATA TRANSITO JULGADO | DATA FINAL | DATA ACORDAO | UF | MUNICIPIO`
  - here `DELIBERACAO` is an **acórdão number** (e.g. `AC-000738/2022-PL`), so
  `source_url` is constructed from the process number
  (`contas.tcu.gov.br/pesquisaJurisprudencia/#/resultado/acordao-completo/<proc>.PROC`).

Field mapping: `process_ref` ← `PROCESSO`; `date_start` ← `DATA ACORDAO`
(fallback `DATA TRANSITO JULGADO`); `date_end` ← `DATA FINAL`; CPF/CNPJ from
`CPF_CNPJ`/`CPF`. The `organ` is always "Tribunal de Contas da União". The
entry id is `digits(PROCESSO) + "-" + digits(CPF_CNPJ)`.

The fourth manifest file (`resp-contas-julgadas-irreg-implicacao-eleitoral.csv`)
is a filtered view of the irregular-accounts list and is **not** ingested - the
spec defines only three TCU registries.

## Notes / caveats

- CGU response structs are coded from the OpenAPI spec (no key available in this
  environment). Field names verified against `/v3/api-docs`; masked-CPF handling
  should be re-validated against a real keyed response on first live run.
- CGU `source_url` fallbacks for CEAF and Leniência are best-effort public
  portal URLs; CEIS/CNEP use the exact `linkPublicacao` from the API.
- Import progress per registry is recorded in `sanctions_import_state`
  (Postgres) for observability.
```
