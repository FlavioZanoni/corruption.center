# DataJud Watcher

A **case-level status engine**. It keeps tracked `LegalProceeding` nodes up to
date by polling the DataJud public API by `numeroProcesso` and applying a
movement-driven state machine. It does **not** discover parties or expand case
trees - the public API does not expose that data (Portaria CNJ 160/2020). Party
discovery and per-defendant outcomes belong to the DJEN worker and human review.

See the authoritative spec: [`docs/workerDetails/DATAJUD.md`](../../../docs/workerDetails/DATAJUD.md).

## What DataJud gives us (verified empirically 2026-07)

The public API returns only case-level fields: `numeroProcesso`, `classe`,
`assuntos`, `movimentos[]`, `orgaoJulgador`, `nivelSigilo`, `grau`, `formato`,
`sistema`, `tribunal`, `dataAjuizamento`, `dataHoraUltimaAtualizacao`.

It never returns `partes[]`, `processoRelacionado[]`, or free-text movement
`complemento`. There is therefore no way to attribute movements to a specific
defendant from DataJud alone.

## Movement state machine (case-level)

| Code | Nome | Action |
| --- | --- | --- |
| `51` | Recebimento de denúncia | `LegalProceeding.phase → accepted` |
| `848` | Sentença | `LegalProceeding.phase → sentenced` (wins over accepted) |
| `60` | Condenação | explicit conviction disposition |
| `61` | Absolvição | explicit acquittal disposition (also clears an earlier conviction) |
| `848` complements | Sentença disposition | infers conviction when the movement's `nome`, plain `complementos`, or a `complementosTabelados` entry reads *condenatória/procedente* (not *improcedente*); *absolvição/improcedente* rules it out. Any explicit code `60`/`61` always wins. |
| `901` | Prescrição | `LegalProceeding.status → concluded` |
| `132` | Baixa definitiva | `LegalProceeding.status → concluded` |
| `246` | Arquivamento definitivo | `LegalProceeding.status → concluded` |

`has_conviction` is **order-sensitive, not latched**. Movements are evaluated in
chronological order (by `dataHora`, falling back to input order when timestamps
are missing or equal) and the **last** explicit `60`/`61` disposition wins - so a
Condenação later reversed on appeal (a subsequent Absolvição) *clears*
`has_conviction` instead of leaving a stale, defamation-grade `true`. An explicit
`60`/`61` always outranks an `848` complement inference regardless of order. The
writer stores the freshly computed value verbatim (it is clearable), not an
OR-latch.

A concluded case also sets `watcher_tracking.status = concluded` (polled weekly
thereafter). `nivelSigilo > 0` → the case is skipped entirely.

## Security and key handling

- `DATAJUD_API_KEY` is read from the environment and is the **only** source of
  the key. The worker fails fast if it is unset.
- The key is never hardcoded in code or docs. The public key rotates and is
  published at <https://datajud-wiki.cnj.jus.br/api-publica/acesso/>.

## Verification modes

Before enabling writes, the worker can verify:

1. Movement codes `51`, `60`, `61`, `848`, `901`, `132`, `246` against the TPU
   public pages (`--verify-tpu`).
2. That a real probe response carries the required core fields
   (`_source.numeroProcesso` and `_source.movimentos`) via `--probe-case` /
   `--probe-tribunal`.

Defaults are non-strict to tolerate tribunal payload variability. Use
`--strict-verify=true` for fail-fast startup.

```bash
cd backend
DATABASE_URL="postgres://..." DATAJUD_API_KEY="..." \
go run ./workers/datajud/cmd \
  --verify-tpu=true \
  --strict-verify=false \
  --probe-case "5046512-94.2016.4.04.7000" \
  --probe-tribunal "api_publica_trf4" \
  --poll-limit 10
```

Enable the case-level write path (proceeding upsert + state machine):

```bash
cd backend
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
DATAJUD_API_KEY="..." \
go run ./workers/datajud/cmd \
  --verify-tpu=true \
  --strict-verify=false \
  --probe-case "50001655420174047004" \
  --probe-tribunal "api_publica_trf4" \
  --enable-writes \
  --poll-limit 50
```

## Scope

- Loads cases from `watcher_tracking` (active daily, concluded weekly).
- Polls DataJud by `numeroProcesso`; skips restricted (`nivelSigilo > 0`).
- Applies the case-level movement state machine to `LegalProceeding`.
- Updates `last_movement_id`, `last_polled_at`, and `status` in Postgres.

Writes are all-or-nothing per run. **Without `--enable-writes` the run is fully
read-only**: it polls, derives case state, and reports stats/logs, but performs
no graph writes and no `watcher_tracking` mutations (not even `last_polled_at`).
This keeps Postgres tracking from desynchronizing from the graph on dry runs.

## Live verification notes

- TRF/STJ endpoints respond; `api_publica_stf` returned 404 in live probing: prefer STJ/TRF for probes.
