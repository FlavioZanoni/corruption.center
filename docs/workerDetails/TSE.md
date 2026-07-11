# TSE CSV Import: Deep Dive

## Purpose

TSE CSV import is responsible for **historical coverage only**: it creates `Politician`
nodes for every person who won a federal or state-level election since 2002. It does not
determine whether they are currently in office. That is the responsibility of Câmara Sync
and Senado Sync, which set `active: true` on nodes they find in the current mandate lists.

All nodes created by TSE import default to `active: false`.

The base this import builds is the ceiling on everything downstream: a name that is
**not** in it can never be matched to a court party or to a sanction, so the person
stays an anonymous `Person` node forever (see `docs/identity_matching.md`). That is why
the office filter is deliberately wide (below).

---

## Files

One file per election year: `consulta_cand_{year}.zip` (about 4MB zipped). Inside
it are 28 CSVs: `CONSULTA_CAND_{year}_{UF}.csv` for each of the 27 states, plus
`CONSULTA_CAND_{year}_BR.csv`, the national file that carries the **Presidente**
and **Vice-Presidente** rows (`SG_UF = BR`).

```txt
https://cdn.tse.jus.br/estatistica/sead/odsele/consulta_cand/consulta_cand_{year}.zip
```

There is no second file and no join. `consulta_cand` already carries everything
the import needs: `DS_CARGO` (office), `DS_SIT_TOT_TURNO` (result), `SG_UF`,
`SG_PARTIDO`, `NM_CANDIDATO`, `NM_URNA_CANDIDATO`, `NM_SOCIAL_CANDIDATO`,
`NR_CPF_CANDIDATO`, `SQ_CANDIDATO`, `NR_TURNO` and `ANO_ELEICAO`.

The importer used to also require `votacao_candidato_munzona_{year}.zip`, joined
on `SQ_CANDIDATO`, but that file only adds vote tallies, which this project does
not use. It cost 552MB zipped per year (several GB unzipped) against 4MB for
`consulta_cand`, and unzipping it exhausted the disk and made the 2022 import
fail outright. A consulta-only import reproduces the same winner counts as the
old two-file pipeline (2006: 1626, 2022: 1655), so the `votacao` file was dropped
entirely: it is no longer downloaded, unzipped or referenced anywhere in the code.

A separate, much larger aggregate named `CONSULTA_CAND_{year}_BRASIL.CSV`
(distinct from the per-state `_BR.csv` above) is pruned before extraction
(`unzipAndPrune`) if present: it is never written to disk and never processed.

> **`SQ_CANDIDATO` is not unique on its own.** It is only unique per state: in
> 2006, SQ `10204` is Cláudio Cajado (BA), Marcos Ramos da Hora (PE) and Givaldo
> Carimbão (AL), three different elected people. Deduplicating winners by SQ
> alone would silently drop two of the three and, far worse, could attach one
> person's CPF to another, which would link their sanctions and court cases to
> the wrong human. Candidates are keyed by `(SG_UF, SQ_CANDIDATO)` within a year
> (`candidateKey`), and when the same key appears in more than one round, the
> row from the latest `NR_TURNO` is kept (`keepLatestTurn`).

---

## Schema

All files: **ISO-8859-1 (Latin 1)**, **CRLF line terminators**, **semicolon separator**,
fields **double-quoted**. Confirmed on 2006 and 2022. Read by column name, never by
position; 2022 has extra columns that 2006 does not.

---

## `consulta_cand` columns

The `consulta_cand` file has far more columns than these (party number, coalition,
candidacy status, ballot number, and so on), but the importer reads by column
name and only requires the following (`consultaHeaders` in `importer.go`); any
other column is ignored:

| Column | Description | Used |
| --- | --- | --- |
| `ANO_ELEICAO` | Election year | Yes: logging + profile URL |
| `NR_TURNO` | Round (1 or 2) | **Yes**: keep the row from the latest round per candidate |
| `DS_CARGO` | Office description | **Yes**: filter |
| `SQ_CANDIDATO` | Candidate sequence number | **Yes**: dedup key, combined with `SG_UF` |
| `NR_CPF_CANDIDATO` | Candidate CPF | Yes → `Politician.cpf` (unique key) |
| `DS_SIT_TOT_TURNO` | Final result description | **Yes**: filter |
| `SG_UF` | State | Yes → `Politician.state`, and part of the dedup key |
| `SG_PARTIDO` | Party acronym | Yes → `Politician.party_current` (TSE fallback only) |
| `NM_CANDIDATO` | Full legal name | Yes → `Politician.name` |
| `NM_URNA_CANDIDATO` | Ballot/popular name | Yes → `Politician.name_aliases` |
| `NM_SOCIAL_CANDIDATO` | Social name if declared | Yes → `Politician.name_aliases` if non-null and not `#NE` |

If any of these columns is missing from the header row, the import fails fast
(`ensureHeaders`) instead of silently reading the wrong column.

---

## Filters

**`DS_CARGO`: keep only** (`allowedCargos`, `importer.go`):

- `PRESIDENTE`
- `VICE-PRESIDENTE`
- `SENADOR`
- `DEPUTADO FEDERAL`
- `GOVERNADOR`
- `VICE-GOVERNADOR`
- `DEPUTADO ESTADUAL`
- `DEPUTADO DISTRITAL` (Distrito Federal)

Why the state-level offices are in: governors are heavily represented in corruption
prosecutions, and the political base is the ceiling on every later match. A name absent
from it can never be tied to a court party or a sanction: it stays an anonymous `Person`
node, and the case or sanction it appears in never reaches the politician's page. Adding
an office costs one filter entry; leaving it out costs every link that office would ever
have produced.

Municipal offices (`PREFEITO`, `VICE-PREFEITO`, `VEREADOR`) are **not** covered: they are
elected in the other even-year cycle (2004, 2008, 2012, …) and do not appear in the
general-election files at all.

The cargo filter is enforced on every file, including `_BR`. `_BR` carries Presidente
rows exclusively today, so filtering it costs nothing; not filtering it would mean that
if TSE ever ships a `_BR` file for a municipal year, every elected mayor and councillor
in it would enter the politician base unfiltered.

**`DS_SIT_TOT_TURNO`: keep only:**

- `ELEITO`
- `ELEITO POR QP` (proportional seat by party quota)
- `ELEITO POR MÉDIA` (proportional seat by average)
- `MÉDIA` / `MEDIA` / `QP` / `ELEITO POR MEDIA` (legacy spellings)

> **The label set is not stable across years.** 2002 and 2022 say `ELEITO POR MÉDIA`;
> 2006 says bare `MÉDIA`. Accepting only the modern spellings dropped 79 of the 513
> federal deputies elected in 2006 (`434 ELEITO + 79 MÉDIA = 513`, the exact size of
> the Câmara). Any new variant TSE invents must be added here, or those winners
> vanish silently.

Note that the files are ISO-8859-1 encoded, so accented labels only match after the
Latin-1 decode. A UTF-8 test fixture will silently fail to match `MÉDIA`.

---

## Processing logic

There is a single pass over a single file shape: no join, no enrichment pass.

```txt
// One pass over every CONSULTA_CAND_{year}_*.csv (27 UF + BR)
winners = {}
for each consulta_cand file:
  for each row:
    if DS_SIT_TOT_TURNO not in allowed_status: skip (SkippedByStatus)
    if DS_CARGO not in allowed_cargos: skip (SkippedByCargo)
    if SQ_CANDIDATO empty or NR_TURNO not numeric: skip (SkippedByInvalidRow)

    key = (SG_UF, SQ_CANDIDATO)
    if key not in winners or NR_TURNO > winners[key].NR_TURNO:
      winners[key] = row  // keepLatestTurn: the runoff row wins over round 1

// Build Politician records from the deduplicated winners
for each (key, row) in winners:
  cpf = normalize_null(NR_CPF_CANDIDATO)
  if cpf is empty: skip (MissingCPF), do not emit a record

  emit PoliticianRecord:
    cpf               ← cpf
    name              ← NM_CANDIDATO
    name_aliases      ← append NM_URNA_CANDIDATO if different from name
    name_aliases      ← append NM_SOCIAL_CANDIDATO if non-null and not "#NE"
    party_current     ← SG_PARTIDO (TSE fallback, Câmara/Senado sync takes precedence)
    state             ← SG_UF
    active            ← false (Câmara/Senado sync sets true for current mandate holders)
    election_year     ← ANO_ELEICAO
    candidate_sq      ← SQ_CANDIDATO
    tse_profile_urls  ← https://divulgacandcontas.tse.jus.br/divulga/#/candidato/{ANO_ELEICAO}/{SQ_CANDIDATO}
```

Cross-year alias merging (a CPF that already exists in the graph under a
different `NM_CANDIDATO` gets the new name appended to `name_aliases`) happens
at upsert time in Memgraph, not in this file-parsing step: each run of
`ImportYearFromZipFiles` only sees one year's file and returns that year's
records.

---

## `active` field

TSE data is historical. A politician who won in 2006 may have died, resigned, been
impeached, or simply not run again. TSE has no way to tell.

- TSE import always sets `active: false`
- Câmara Sync sets `active: true` for current deputies
- Senado Sync sets `active: true` for current senators
- If neither sync activates a node, the politician won an election historically
  but does not hold a federal seat today

---

## Two-round handling

Presidente and Governador are the offices with a potential second round (`NR_TURNO=2`).
Always keep the row with the **highest `NR_TURNO`** per `(SG_UF, SQ_CANDIDATO)`
(`keepLatestTurn`). Senador, Deputado Federal, Deputado Estadual and Deputado
Distrital are always `NR_TURNO=1`.

---

## Null sentinel values

| Sentinel | Numeric equivalent | Meaning |
|---|---|---|
| `#NULO` | `-1` | Field is blank in the source database |
| `#NE` | `-3` | Field did not exist in electoral systems that year |

Treat both as null. `NM_SOCIAL_CANDIDATO` is `#NE` in most pre-2018 rows.

---

## Profile URL

```txt
https://divulgacandcontas.tse.jus.br/divulga/#/candidato/{ANO_ELEICAO}/{SQ_CANDIDATO}
```

`SQ_CANDIDATO` is not stable across years. Store as `tse_profile_urls: [string]`,
appending one entry per election year the politician won.

---

## Election years and cargo availability

Brazil alternates two even-year cycles. Only the **general** cycle carries the offices
this import keeps.

| Year | Cycle | Offices in the files | Kept by `allowedCargos` | Files |
|---|---|---|---|---|
| 2002 | general | Presidente, Governador, Senador (1/3), Dep. Federal/Estadual/Distrital | all | 27 UF + BR |
| 2004 | municipal | Prefeito, Vereador | none | 27 UF only |
| 2006 | general | as 2002, Senador (2/3) | all | 27 UF + BR |
| 2008 | municipal | Prefeito, Vereador | none | 27 UF only |
| 2010 | general | as 2002, Senador (1/3) | all | 27 UF + BR |
| 2012 | municipal | Prefeito, Vereador | none | 27 UF only |
| 2014 | general | as 2002, Senador (2/3) | all | 27 UF + BR |
| 2016 | municipal | Prefeito, Vereador | none | 27 UF only |
| 2018 | general | as 2002, Senador (1/3) | all | 27 UF + BR |
| 2020 | municipal | Prefeito, Vereador | none | 27 UF only |
| 2022 | general | as 2002, Senador (2/3) | all | 27 UF + BR |
| 2024 | municipal | Prefeito, Vereador | none | 27 UF only |

`--all-years` walks every even year from 2002 to the latest election year, municipal ones
included. A municipal year is downloaded and parsed, every row is dropped by the cargo
filter (counted in `SkippedByCargo`) and the year yields no records. That is
expected, not a failure.

The worker must not fail when `_BR` is absent (municipal years) or when Senador rows are
absent from the UF files.

---

## Downloading the zips

When no local zip is supplied, the CLI fetches the single zip from the TSE CDN:

```txt
https://cdn.tse.jus.br/estatistica/sead/odsele/consulta_cand/consulta_cand_{year}.zip
```

At about 4MB zipped, this file is small enough that a plain `GET` usually succeeds in
one shot. The downloader still retries with **HTTP Range resume** (`downloadRetries` = 6,
`resumeDownload`): each attempt appends to the partial `.tmp` from the byte it reached,
sending `Range: bytes=<have>-`, and only a server that answers `200` instead of `206`
forces a restart from zero. Backoff between attempts grows linearly (2s, 4s, 6s, …). The
`.tmp` is renamed to the final path only after the body copied cleanly, so a completed
file in the cache is always a whole file.

Completed downloads are cached under `<workdir>/tse-downloads` and reused
(`downloadFileIfMissing`); a year run deletes them afterwards.
