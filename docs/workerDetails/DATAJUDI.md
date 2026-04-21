# DataJud Watcher - Deep Dive

## Purpose

Primary engine for keeping the legal graph live. Polls all tracked
`LegalProceeding` nodes for new movements, updates outcomes, discovers new
defendants, and walks the full case tree outward from a human-seeded root.

Works exclusively by `numeroProcesso` lookup - always reliable, no CPF/name
search ambiguity.

---

## How cases enter the watcher

Only one entry point: a human seeds a root case number in the backoffice,
linking it to a `Scandal` node. The watcher then expands automatically from
there via `processoRelacionado` and `desmembramento`.

The `watcher_tracking` Postgres table is the source of truth:

```txt
case_number        TEXT NOT NULL UNIQUE
tribunal_endpoint  TEXT NOT NULL        -- e.g. "api_publica_trf4"
scandal_id         TEXT NOT NULL        -- Memgraph Scandal node id
proceeding_id      TEXT NOT NULL        -- Memgraph LegalProceeding node id
last_movement_id   TEXT                 -- last DataJud movimento id seen
last_polled_at     TIMESTAMPTZ
status             TEXT                 -- active | concluded | paused
added_by           TEXT                 -- "backoffice" or "watcher"
```

---

## Endpoint

```txt
POST https://api-publica.datajud.cnj.jus.br/{tribunal_endpoint}/_search
Authorization: APIKey cDZHYzlZa0JadVREZDJCendFbzVlQTU2S3phNTYwdjAy
Content-Type: application/json
```

Query by case number:

```json
{
  "size": 1,
  "query": {
    "match": {
      "numeroProcesso": "5046512-94.2016.4.04.7000"
    }
  }
}
```

---

## Response fields used

```txt
_source.numeroProcesso          → dedup / confirmation
_source.nivelSigilo             → skip if > 0 (restricted)
_source.classe.codigo           → LegalProceeding.type
_source.assuntos[].codigo       → LegalProceeding.assuntos
_source.dataAjuizamento         → LegalProceeding.date_filed
_source.orgaoJulgador.nome      → LegalProceeding.court (human-readable)
_source.partes[]                → defendant discovery
_source.movimentos[]            → state machine inputs
_source.processoRelacionado[]   → case tree expansion
```

---

## Movement state machine

Movement codes that trigger graph writes. Evaluated in order per movement:

| Code | Nome | Action |
| --- | --- | --- |
| `51` | Recebimento de denúncia | `DEFENDANT_IN.outcome → pending` |
| `60` | Condenação | `DEFENDANT_IN.outcome → convicted` |
| `61` | Absolvição | `DEFENDANT_IN.outcome → acquitted` |
| `901` | Prescrição | `DEFENDANT_IN.outcome → prescribed` + `LegalProceeding.status → concluded` |
| `848` | Sentença | Read `complemento` - if no `60`/`61` present, infer outcome from text |
| `132` | Baixa definitiva | `LegalProceeding.status → concluded` |
| `246` | Arquivamento definitivo | `LegalProceeding.status → concluded` |
| `981` | Desmembramento | Extract new case number from `complemento`, create new `LegalProceeding`, register with watcher |
| any new | Inclusão de parte | Extract `partes[]` delta, run defendant discovery (see below) |

**Important:** Not all tribunals emit `60`/`61` explicitly. When `848` (Sentença)
appears without them, read `complemento` for keywords (`condenad`, `absolv`,
`procedente`, `improcedente`) to infer outcome. Flag ambiguous cases for human
review rather than guessing.

---

## Defendant discovery (Inclusão de parte)

When a new party appears in `partes[]` that wasn't in the previous poll:

```txt
if documento is CPF:
  exact match against Politician.cpf
    → found: create DEFENDANT_IN edge on Politician
    → not found: create Person node (cpf masked), flag pending_review (unknown_cpf)

if documento is CNPJ:
  exact match against Organization.cnpj
    → found: create DEFENDANT_IN edge on Organization
    → not found: create Organization node (cnpj only), trigger CNPJ Enricher,
                 flag pending_review (unknown_cnpj)

if documento absent or redacted:
  search Politician by name (exact then fuzzy)
    → confident match: flag pending_review (possible_politician_match) - never auto-create
    → no match: create Person node (name only), flag pending_review (unknown_cpf)
```

**Never auto-create a `DEFENDANT_IN` edge on a `Politician` from name matching
alone.** Always require human confirmation. `Person` nodes can be created
automatically since they carry no electoral implication.

---

## Case tree expansion

On every poll, check `processoRelacionado[]`:

```txt
for each related case:
  if not in watcher_tracking:
    POST /{related.tribunal}/_search by numeroProcesso
    create LegalProceeding node
    create INVESTIGATES edge → same Scandal as parent
    insert into watcher_tracking (added_by = "watcher")
```

Also check `movimentos[]` for code `981` (Desmembramento):

- Extract new case number from `complemento` field
- Same treatment as processoRelacionado

This walks the full case tree without human intervention. The only gap is a
genuinely new investigation opened later with no back-reference to the root -
those must be manually added via the backoffice.

---

## Processing logic

```txt
// Load cases to poll
active_cases   ← watcher_tracking WHERE status = 'active'
concluded_cases ← watcher_tracking WHERE status = 'concluded'
                   AND last_polled_at < now() - 7 days

cases_to_poll ← active_cases + concluded_cases

// Rate limit: 120 req/min across all workers
// Target: consume ≤ 60 req/min leaving headroom for Searcher and retries

for each case in cases_to_poll:
  response ← POST /{tribunal_endpoint}/_search { numeroProcesso }

  if nivelSigilo > 0:
    skip  // restricted case, do not process

  // Diff movimentos
  new_movements ← [m for m in movimentos if m.id > last_movement_id]

  for each movement in new_movements (chronological order):
    apply state machine (see above)

  // Expand case tree
  for each entry in processoRelacionado:
    if not tracked: ingest + register

  // Update tracking
  UPDATE watcher_tracking SET
    last_movement_id = max(movimento.id),
    last_polled_at = now(),
    status = derived from LegalProceeding.status
  WHERE case_number = ?
```

---

## Schedule

| Cases | Frequency | Reason |
| --- | --- | --- |
| `status: active` | Daily | Active proceedings move frequently |
| `status: concluded` | Weekly | Late movements (appeals, corrections) still appear |
| `status: paused` | Never | Human-paused, skip until manually reactivated |

---

## Rate limiting

- Hard cap: 120 req/min (ToU clause 3.13)
- Watcher target: ≤ 60 req/min (leave headroom)
- On 429: exponential backoff starting at 1s, max 60s
- Spread daily polls across the day - do not batch all cases at midnight

---

## Tribunals

| Endpoint | Tribunal |
| --- | --- |
| `api_publica_stf` | STF - foro privilegiado for congresspeople |
| `api_publica_stj` | STJ - ministers, governors |
| `api_publica_trf1` | TRF1 - DF + 13 states |
| `api_publica_trf2` | TRF2 - RJ + ES |
| `api_publica_trf3` | TRF3 - SP + MS |
| `api_publica_trf4` | TRF4 - PR + SC + RS (Lava Jato) |
| `api_publica_trf5` | TRF5 - AL + CE + PB + PE + RN + SE |
| `api_publica_trf6` | TRF6 - MG |

The `tribunal_endpoint` column in `watcher_tracking` stores which of these to
hit. It is set when the case is first registered (either by backoffice seed or
by watcher discovery).

---

## DataJud Searcher - footnote

A future Searcher worker would proactively search all Politician nodes by name
across all 8 endpoints to find cases not yet connected to any seeded scandal.

Deferred because:

- CPF/CNPJ search in DataJud is inconsistent, name is the only reliable field
- Name matching produces false positives requiring human review at scale
- The watcher already discovers defendants automatically from seeded roots
- The coverage gap (politicians with no known cases) is surfaced as a backoffice
  report, not automated discovery

When implemented: search by legal name + all aliases, filter by
`classe.codigo` (corruption-relevant only), score by CPF match confidence,
route low-confidence hits to `pending_review`.
