# DJEN Party Discovery Worker

Discovers **who the parties of a case are** and **new cases for tracked
people** from the Diário de Justiça Eletrônico Nacional (DJEN) — the CNJ's
official national gazette API (public, keyless). Fills the gap DataJud cannot:
party rosters. Spec: `docs/workerDetails/DJEN.md`.

DJEN carries only names (no CPF/CNPJ), so every link to a `Politician` goes
through `pending_review`. A name match alone never creates an edge to a
Politician — homonyms are common and real.

## Modes

Both modes run by default; disable one with `--case-mode=false` /
`--name-mode=false`.

### Case mode

Case selection uses DJEN's **own** poll cursor,
`watcher_tracking.djen_last_polled_at` (migration `004_djen_poll.sql`) — not the
DataJud watcher's `last_polled_at`. The watcher cron runs before DJEN and
refreshes `last_polled_at`, which previously starved concluded cases out of
DJEN's window entirely. Cadence (`ListDjenCasesForPoll`):

- **active** cases: never polled by DJEN, or polled > 1 day ago;
- **concluded** cases: never polled by DJEN, or polled > 7 days ago;
- **paused** cases: skipped.

`djen_last_polled_at` is advanced after each case is polled (`UpdateDjenPolledAt`).

For each selected case:

1. Normalize the stored case number to bare **20 digits** (backoffice-seeded
   numbers may be in formatted CNJ form, e.g. `5046512-94.2016.4.04.7000`); a
   warning is logged if the result is not 20 digits. `GET
   /comunicacao?numeroProcesso=<20 digits>`, paginate.
2. Build the roster: union of `destinatarios[]`, deduped by
   `(normalized nome, polo)`.
3. Process only the **delta** vs the `djen_party_snapshot` table so a stable
   roster does no repeated work.
4. For each new party on the passivo (`P`) side, strip a trailing
   `E OUTROS (N)` co-party marker, then classify:
   - **Company / public body** (heuristic `isCompanyName`: LTDA, S/A, EIRELI,
     ME, MEI, EPP, CIA, COMPANHIA, CONSORCIO, CONSTRUTORA, INCORPORADORA,
     ASSOCIACAO, FUNDACAO, INSTITUTO, MUNICIPIO, PREFEITURA, ESTADO DE, UNIAO,
     MINISTERIO PUBLICO, BANCO, …) → name-only `Organization` node (provenance =
     DJEN comunicação id + link) with a `DEFENDANT_IN` edge, outcome `"cited"`,
     **plus** a `pending_review` type `unknown_cnpj`
     (`{name, case_number, source:"djen", link}`) so a human can attach the CNPJ.
     DJEN destinatarios carry no document, so companies never become Person nodes.
   - **Exact** normalized (uppercase, accents stripped) full-name match against
     Politician names + aliases → `pending_review` type `djen_party_match` with
     the communication `link` and a `texto` snippet (≤500 chars) as evidence.
     **Never** auto-creates a `DEFENDANT_IN` edge on a Politician.
   - Otherwise → name-only `Person` node (provenance = DJEN comunicação id +
     link) with a `DEFENDANT_IN` edge, outcome `"cited"`.

   **LGPD purge guard:** before auto-creating a name-only `Person` or
   `Organization`, the worker consults `purge_tombstone` (migration 008) via
   `IsSubjectPurged`, keyed on the purged **name** (DJEN carries no document). A
   purged name is skipped — no node, no edge, no `unknown_cnpj` review — and
   counted in `stats.skipped_tombstoned`.

### Name mode

For each Politician (name + each alias):

1. `GET /comunicacao?nomeParte=<name>`, paginate, **cap 300 items/name/run**.
2. Group items by `numero_processo`.
3. Skip cases already in `watcher_tracking` or already flagged (any state) as a
   `djen_case_candidate`. Both checks compare on **digits-only** case numbers:
   `watcher_tracking.case_number` and the candidate payload may be stored in
   formatted CNJ form (backoffice-seeded), so `IsCaseTracked` /
   `HasDjenCaseCandidate` normalize both the stored value
   (`regexp_replace(..., '\D', '', 'g')`) and the passed param before comparing.
   Without this, the API's bare 20-digit number would silently miss a formatted
   stored row and re-flag an already-tracked case.
4. Filter to criminal / improbidade classes only (allowlist in `worker.go`,
   `isCriminalOrImprobidadeClass`). The class-name keyword fallback matches
   *júri* courts by whole word only, so `JURISDICAO`/`JURIDICA` no longer false-
   positive.
5. `pending_review` type `djen_case_candidate`. The payload carries exactly what
   the backoffice needs to register the case on approval:

   | key | value |
   | --- | --- |
   | `case_number` | 20-digit digits-only string |
   | `tribunal_sigla` | e.g. `"TRF4"` (first siglaTribunal in the group) |
   | `tribunal_endpoint` | `"api_publica_" + lowercase sigla`, e.g. `"api_publica_trf4"` |
   | `politician_id` | matched Politician node id |
   | `matched_name` | the name searched |
   | `classes` | distinct `nomeClasse` values |
   | `tribunals` | distinct siglaTribunal values |
   | `polos` | polo(s) the name appears in |
   | `sample_links` | up to 5 communication links |
   | `item_count` | communications in the group |

Approval in the backoffice registers the case in `watcher_tracking`
(`added_by = "djen"`).

## Politeness

- Self-imposed ≤ 60 req/min limiter.
- Exponential backoff on 429/5xx (1s → 60s).
- `User-Agent: corruption.center-djen/1.0 (contato@corruption.center)`.
- Old concluded (pre-2023) cases return `count: 0` — handled gracefully.

## Run

```bash
cd backend
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/djen/cmd \
  --case-mode --name-mode \
  --name-cap 300 \
  --poll-limit 50
```

Flags: `--case-mode` / `--name-mode` (both default true), `--dry-run` (fetch and
compute but no writes), `--name-cap` (default 300), `--poll-limit` (case mode,
0 = all), `--base-url` (API override).

## Storage

- `djen_party_snapshot` (Postgres, migration `002_djen.sql`) — per-case roster
  snapshot for delta detection.
- `watcher_tracking.djen_last_polled_at` (Postgres, migration `004_djen_poll.sql`)
  — DJEN's independent case-mode poll cursor.
- `pending_review` types `djen_party_match`, `djen_case_candidate`, and
  `unknown_cnpj` (types added by `002_djen.sql`).

`MEMGRAPH_USER` / `MEMGRAPH_PASS` are optional (dev Memgraph is auth-less);
`DATABASE_URL` and `MEMGRAPH_URI` are required.

## Coverage limitation

DJEN only carries communications published since tribunals migrated (~2023+).
Concluded historical cases return zero results; those rosters are seeded
manually in the backoffice with cited sources.
