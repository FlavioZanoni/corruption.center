# DataJud Watcher: Deep Dive

## Purpose

Keeps tracked `LegalProceeding` nodes up to date at the **case level**: status,
movement timeline, court, class, subjects. Works exclusively by
`numeroProcesso` lookup.

## What DataJud can and cannot do (verified empirically 2026-07)

The public API (`api-publica.datajud.cnj.jus.br`) returns only:

```txt
numeroProcesso, classe, assuntos, movimentos[], orgaoJulgador,
nivelSigilo, grau, formato, sistema, tribunal, dataAjuizamento,
dataHoraUltimaAtualizacao
```

Per Portaria CNJ 160/2020 the public API **never returns**:

- `partes[]` - no party names, no CPF/CNPJ. Defendant discovery via DataJud
  is impossible.
- `processoRelacionado[]` - no automatic case-tree expansion.
- Free-text movement `complemento` - movements carry coded
  `complementosTabelados` only, so extracting spinoff case numbers from
  Desmembramento movements does not work.

Consequence: a case with many defendants may show dozens of `Condenação`
movements with no way to attribute them to a person. **DataJud gives
case-level status only.** Per-defendant outcomes come from the DJEN worker
(see `DJEN.md`) and human review.

## How cases enter the watcher

1. A human seeds a root case number in the backoffice, linked to a `Scandal`.
2. The DJEN worker discovers case numbers for tracked people and files them
   as `pending_review` (type `djen_case_candidate`); approval registers them
   here.

The `watcher_tracking` Postgres table is the source of truth:

```txt
case_number        TEXT NOT NULL UNIQUE
tribunal_endpoint  TEXT NOT NULL        -- e.g. "api_publica_trf4"
scandal_id         TEXT NOT NULL
proceeding_id      TEXT NOT NULL
last_movement_id   TEXT
last_polled_at     TIMESTAMPTZ
status             TEXT                 -- active | concluded | paused
added_by           TEXT                 -- "backoffice" | "djen" | "watcher"
```

## Endpoint

```txt
POST https://api-publica.datajud.cnj.jus.br/{tribunal_endpoint}/_search
Authorization: APIKey $DATAJUD_API_KEY
Content-Type: application/json
```

`DATAJUD_API_KEY` comes from the environment. The public key is published at
`https://datajud-wiki.cnj.jus.br/api-publica/acesso/` and rotates; never
hardcode it in code or docs.

Query by case number:

```json
{"size": 1, "query": {"match": {"numeroProcesso": "50465129420164047000"}}}
```

## Movement state machine (case-level)

| Code | Nome | Action |
| --- | --- | --- |
| `51` | Recebimento de denúncia | `LegalProceeding.phase → accepted` |
| `60` | Condenação | `LegalProceeding.has_conviction → true` |
| `61` | Absolvição | record on timeline |
| `848` | Sentença | `LegalProceeding.phase → sentenced` |
| `901` | Prescrição | `LegalProceeding.status → concluded` |
| `132` | Baixa definitiva | `LegalProceeding.status → concluded` |
| `246` | Arquivamento definitivo | `LegalProceeding.status → concluded` |

All outcome flags are **case-level**. `DEFENDANT_IN.outcome` per person is set
only via backoffice review, using DJEN publications and decision texts as
evidence.

`nivelSigilo > 0` → skip case entirely.

## Schedule and rate limiting

| Cases | Frequency |
| --- | --- |
| `status: active` | Daily |
| `status: concluded` | Weekly |
| `status: paused` | Never |

- Hard cap 120 req/min (ToU 3.13); watcher targets ≤ 60 req/min.
- On 429: exponential backoff 1s → 60s.
- Spread polls across the day.

## Tribunals

All 90+ tribunals are exposed as `api_publica_<sigla>`: federal (STF*, STJ,
TRF1-6) and state (`api_publica_tjpr`, `api_publica_tjsp`, ...). State-court
cases (mayors, state deputies) use TJ endpoints; there is no need to touch
Projudi/e-SAJ/eproc front-ends directly.

*`api_publica_stf` has returned 404 in live probing; STF availability is not
guaranteed; prefer STJ/TRF probes for verification.
