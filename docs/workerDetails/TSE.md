# TSE CSV Import — Deep Dive

## Purpose

TSE CSV import is responsible for **historical coverage only** — it creates `Politician`
nodes for every person who ever won a federal election since 2002. It does not determine
whether they are currently in office. That is the responsibility of Câmara Sync and
Senado Sync, which set `active: true` on nodes they find in the current mandate lists.

All nodes created by TSE import default to `active: false`.

---

## Files

Two types of files per election year, both needed. Join on `SQ_CANDIDATO`.

### 1. `votacao_candidato_munzona_{year}_BR.csv`

Federal scope only — contains **Presidente** rows exclusively.

```txt
https://cdn.tse.jus.br/estatistica/sead/odsele/votacao_candidato_munzona/votacao_candidato_munzona_{year}.zip
```

### 2. `votacao_candidato_munzona_{year}_{UF}.csv` (27 files)

Per-state files — contain **Deputado Federal**, **Deputado Estadual**, **Governador**,
and **Senador** mixed together. Must filter by `DS_CARGO`. One file per UF:
`AC`, `AL`, `AM`, `AP`, `BA`, `CE`, `DF`, `ES`, `GO`, `MA`, `MG`, `MS`, `MT`,
`PA`, `PB`, `PE`, `PI`, `PR`, `RJ`, `RN`, `RO`, `RR`, `RS`, `SC`, `SE`, `SP`, `TO`

### 3. `consulta_cand_{year}_{UF}.csv` (27 files + `_BR`)

Contains `NR_CPF_CANDIDATO` and `SQ_CANDIDATO`. CPF does not exist in the votacao
files — this join is mandatory. Same per-UF structure as votacao: winners from a
UF file will have their CPF in the matching UF consulta_cand file.

```txt
https://cdn.tse.jus.br/estatistica/sead/odsele/consulta_cand/consulta_cand_{year}.zip
```

**Total downloads per year: 56 files** (28 votacao + 28 consulta_cand)

---

## Schema

All files: **ISO-8859-1 (Latin 1)**, **CRLF line terminators**, **semicolon separator**,
fields **double-quoted**. Confirmed on 2006 and 2022. Read by column name, never by
position — 2022 has extra columns that 2006 does not.

---

## `votacao_candidato_munzona` columns

| Column | Description | Used |
| --- | --- | --- |
| `DT_GERACAO` | File generation date | No |
| `HH_GERACAO` | File generation time | No |
| `ANO_ELEICAO` | Election year | Yes — logging + profile URL |
| `CD_TIPO_ELEICAO` | Election type code | No |
| `NM_TIPO_ELEICAO` | Election type name | No |
| `NR_TURNO` | Round (1 or 2) | **Yes — keep highest per candidate** |
| `CD_ELEICAO` | Election code | No |
| `DS_ELEICAO` | Election description | No |
| `DT_ELEICAO` | Election date | No |
| `TP_ABRANGENCIA` | Scope — `F` federal, `E` estadual | No — handled by file selection |
| `SG_UF` | State | Yes → `Politician.state` |
| `SG_UE` | Electoral unit code | No |
| `NM_UE` | Electoral unit name | No |
| `CD_MUNICIPIO` | Municipality code | No |
| `NM_MUNICIPIO` | Municipality name | No |
| `NR_ZONA` | Electoral zone | No |
| `CD_CARGO` | Office code | No |
| `DS_CARGO` | Office description | **Yes — filter** |
| `SQ_CANDIDATO` | Candidate sequence number | **Yes — join key** |
| `NR_CANDIDATO` | Ballot number | No |
| `NM_CANDIDATO` | Full legal name | Yes → `Politician.name` |
| `NM_URNA_CANDIDATO` | Ballot/popular name | Yes → `Politician.name_aliases` |
| `NM_SOCIAL_CANDIDATO` | Social name if declared | Yes → `Politician.name_aliases` if non-null and not `#NE` |
| `CD_SITUACAO_CANDIDATURA` | Candidacy status code | No |
| `DS_SITUACAO_CANDIDATURA` | Candidacy status | No |
| `CD_DETALHE_SITUACAO_CAND` | Detail status code | No |
| `DS_DETALHE_SITUACAO_CAND` | Detail status | No |
| `TP_AGREMIACAO` | Party grouping type | No |
| `NR_PARTIDO` | Party number | No |
| `SG_PARTIDO` | Party acronym | Yes → `Politician.party_current` (TSE fallback only) |
| `NM_PARTIDO` | Party full name | No |
| `SQ_COLIGACAO` | Coalition sequence | No |
| `NM_COLIGACAO` | Coalition name | No |
| `DS_COMPOSICAO_COLIGACAO` | Coalition composition | No |
| `CD_SIT_TOT_TURNO` | Result status code | No |
| `DS_SIT_TOT_TURNO` | Final result description | **Yes — filter** |
| `ST_VOTO_EM_TRANSITO` | Transit vote flag | No |
| `QT_VOTOS_NOMINAIS` | Vote count for this row | No |
| `NM_TIPO_DESTINACAO_VOTOS` | Vote destination type (2022+) | No |
| `QT_VOTOS_NOMINAIS_VALIDOS` | Valid vote count (2022+) | No |

---

## `consulta_cand` columns used

| Column | Position | Maps to |
| --- | --- | --- |
| `SQ_CANDIDATO` | 16 | join key |
| `NR_CPF_CANDIDATO` | 21 | `Politician.cpf` (dedup key) |

Read by name, not position. Position noted only for reference.

---

## Filters

**`DS_CARGO` — keep only:**

- `DEPUTADO FEDERAL`
- `SENADOR`
- `PRESIDENTE`
- `VICE-PRESIDENTE`

**`DS_SIT_TOT_TURNO` — keep only:**

- `ELEITO`
- `ELEITO POR QP` (proportional seat by party quota)
- `ELEITO POR MÉDIA` (proportional seat by average)

---

## Processing logic

```txt
// Pass 1 — collect winning SQ_CANDIDATO from _BR (Presidente)
winners = {}
for each row in votacao_candidato_munzona_{year}_BR.csv:
  if DS_SIT_TOT_TURNO not in allowed_status: skip
  keep highest NR_TURNO per SQ_CANDIDATO

// Pass 2 — collect winning SQ_CANDIDATO from per-UF files
for each UF in all 27 states:
  for each row in votacao_candidato_munzona_{year}_{UF}.csv:
    if DS_CARGO not in allowed_cargos: skip
    if DS_SIT_TOT_TURNO not in allowed_status: skip
    keep highest NR_TURNO per SQ_CANDIDATO

// Pass 3 — enrich winners with CPF from all consulta_cand per-UF files
cpf_map = {}
for each UF file in consulta_cand_{year}_*.csv:
  for each row:
    if SQ_CANDIDATO in winners:
      cpf_map[SQ_CANDIDATO] = NR_CPF_CANDIDATO

// Pass 4 — upsert Politician nodes
for each SQ_CANDIDATO in winners:
  cpf = cpf_map[SQ_CANDIDATO]  // skip + log warning if missing

  upsert Politician by cpf:
    name              ← NM_CANDIDATO
    name_aliases      ← append NM_URNA_CANDIDATO if different from name
    name_aliases      ← append NM_SOCIAL_CANDIDATO if non-null and not "#NE"
    party_current     ← SG_PARTIDO (TSE fallback — Câmara/Senado sync takes precedence)
    state             ← SG_UF
    active            ← false  // Câmara/Senado sync sets true for current mandate holders
    tse_profile_urls  ← append https://divulgacandcontas.tse.jus.br/divulga/#/candidato/{ANO_ELEICAO}/{SQ_CANDIDATO}

  cross-year alias:
    if CPF already exists with different NM_CANDIDATO → append to name_aliases
```

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

Presidente is the only federal cargo with a potential second round (`NR_TURNO=2`).
Always keep the row with the **highest `NR_TURNO`** per `SQ_CANDIDATO`.
Deputado Federal and Senador are always `NR_TURNO=1`.

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

| Year | Deputado Federal | Senador | Presidente | Files |
|---|---|--- |---|---|
| 2002 | ✅ | ✅ (1/3) | ✅ | 27 UF + BR |
| 2004 | ✅ | ❌ municipal | ❌ | 27 UF only |
| 2006 | ✅ | ✅ (2/3) | ✅ | 27 UF + BR |
| 2008 | ✅ | ❌ municipal | ❌ | 27 UF only |
| 2010 | ✅ | ✅ (1/3) | ✅ | 27 UF + BR |
| 2012 | ✅ | ❌ municipal | ❌ | 27 UF only |
| 2014 | ✅ | ✅ (2/3) | ✅ | 27 UF + BR |
| 2016 | ✅ | ❌ municipal | ❌ | 27 UF only |
| 2018 | ✅ | ✅ (1/3) | ✅ | 27 UF + BR |
| 2020 | ✅ | ❌ municipal | ❌ | 27 UF only |
| 2022 | ✅ | ✅ (2/3) | ✅ | 27 UF + BR |
| 2024 | ✅ | ❌ municipal | ❌ | 27 UF only |

The worker must not fail when `_BR` is absent (even years) or when Senador rows
are absent from UF files (even years).
