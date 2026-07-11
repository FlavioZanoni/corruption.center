# DJEN Party Discovery Worker

Discovers **who the parties of a case are** and **new cases for tracked
people** from the Diário de Justiça Eletrônico Nacional (DJEN), the CNJ's
official national gazette API (public, keyless). Fills the gap DataJud cannot:
party rosters. Spec: `docs/workerDetails/DJEN.md`. Identity policy:
`docs/identity_matching.md`.

DJEN carries only names (no CPF/CNPJ), so every link between a case party and a
`Politician` goes through `pending_review`. A name match alone never creates an
edge to a Politician: homonyms are common and real.

Registering a **case**, on the other hand, asserts nothing about any person, so
name mode registers discovered cases automatically (see below).

## Modes

Case mode and name mode run by default; disable one with `--case-mode=false` /
`--name-mode=false`. Rematch mode is off by default (`--rematch-mode`).

### Case mode

Case selection uses DJEN's **own** poll cursor,
`watcher_tracking.djen_last_polled_at` (migration `004_djen_poll.sql`), not the
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
   purged name is skipped (no node, no edge, no `unknown_cnpj` review) and
   counted in `stats.skipped_tombstoned`.

### Name mode

For each Politician (name + each alias):

1. `GET /comunicacao?nomeParte=<name>`, paginate, **cap 300 items/name/run**.
2. Group items by `numero_processo`.
3. Skip cases already in `watcher_tracking`. `IsCaseTracked` is the **sole**
   dedup gate: it compares on **digits-only** case numbers, normalizing both the
   stored value (`regexp_replace(..., '\D', '', 'g')`) and the passed param,
   because a backoffice-seeded row may be in formatted CNJ form while the API
   returns the bare 20 digits. Without this, a formatted stored row would be
   missed and the case re-registered.
4. Filter to criminal / improbidade classes only (allowlist in `worker.go`,
   `isCriminalOrImprobidadeClass`). The class-name keyword fallback matches
   *júri* courts by whole word only, so `JURISDICAO`/`JURIDICA` do not
   false-positive.
5. **Register the case** (`registerDiscoveredCase`), with no human in the loop:
   - `UpsertLegalProceedingByCase` creates the `LegalProceeding` node;
   - `UpsertWatcherCase(case_number, tribunal_endpoint, scandal_id="",
     proceeding_id, added_by="djen")` starts the watch.

   `tribunal_endpoint` is derived from the publication's `siglaTribunal`
   (`"TRF4"` → `"api_publica_trf4"`, `endpointForGroup`). A group with no
   `siglaTribunal` cannot be mapped to a DataJud index and is **not** registered
   (logged); nor is a case number that is not 20 digits.

   The case is registered with **no scandal id**: DJEN surfaced it through one
   politician's name, which says nothing about which scandal it belongs to. An
   operator can attach it later.

Why no review here: registering a case asserts nothing about a person. It
creates the proceeding and starts polling it, and the case's provenance (the
DJEN publication) is what is recorded. The claim "this politician is a defendant"
is a separate, name-only inference, and it remains gated as `djen_party_match`
(filed by case mode once the roster is pulled, or by rematch mode). See
`docs/identity_matching.md`.

The older `djen_case_candidate` review is no longer filed by this worker (and
`HasDjenCaseCandidate` is no longer consulted by it); the type remains in the
`pending_review` whitelist and the backoffice can still approve legacy rows.

### Rematch mode (`--rematch-mode`)

A party is matched against the politician index **once**, at discovery, and is
then snapshotted, so it never reappears in a roster delta. Defendants found
while the politician base was small (before a TSE import added the state-level
offices, say) would therefore stay anonymous `Person` nodes forever. For a
**pre-2023 case** this is permanent, not merely slow: DJEN does not carry its
publications and DataJud publishes no parties, so its party list can never be
re-fetched, and re-testing the `Person` nodes we already hold is the only path
by which it can ever gain a politician link.

`runRematchMode` walks every `Person` with a `DEFENDANT_IN` edge
(`ListCitedPersons`), re-tests the name against the current politician index and
files a `djen_party_match` review per hit, carrying `person_id`, `politician_id`,
`proceeding_id`, `case_number`, `scandal_id` and `origin: "rematch"`. Dedup is
`HasPartyMatchReview(politician_id, proceeding_id)`, so re-runs do not pile up
duplicates (counted in `stats.skipped_already_reviewed`).

Like discovery, it **never** auto-creates the edge: the operator approves the
promotion in the backoffice. Run it after every TSE import.

## Failure isolation

The client retries with exponential backoff (6 attempts, 1s → 60s). When a
lookup still fails, the worker skips **that item** and counts it in
`stats.fetch_errors`, rather than aborting a run that scans hundreds of names
over an API that returns sporadic 500s. Context cancellation still aborts.

## Stats

`cases_registered` (cases now watched, no review), `skipped_unregistrable`
(discovered but unwatchable: case number not 20 digits, or no tribunal on the
publication), `candidate_cases_flagged` (discovered cases passing the filters,
also counted under `--dry-run`), `persons_rematched` (Person defendants
re-tested, hits included), `politician_hits_flagged` (`djen_party_match`
reviews), `fetch_errors`, `persons_linked`, `orgs_linked`, `pending_reviews`,
`skipped_already_tracked`, `skipped_already_reviewed`, `skipped_class_filter`,
`skipped_tombstoned`.

## Politeness

- Self-imposed ≤ 60 req/min limiter.
- Exponential backoff on 429/5xx (1s → 60s).
- `User-Agent: corruption.center-djen/1.0 (contato@corruption.center)`.
- Old concluded (pre-2023) cases return `count: 0`, handled gracefully.

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

# After a TSE import: re-test existing Person defendants only.
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
go run ./workers/djen/cmd \
  --case-mode=false --name-mode=false --rematch-mode
```

Flags: `--case-mode` / `--name-mode` (both default true), `--rematch-mode`
(default false), `--dry-run` (fetch and compute but no writes), `--name-cap`
(default 300), `--poll-limit` (case mode, 0 = all), `--base-url` (API override).

## Storage

- `djen_party_snapshot` (Postgres, migration `002_djen.sql`): per-case roster
  snapshot for delta detection.
- `watcher_tracking` (Postgres): rows written by name-mode registration
  (`added_by = "djen"`), plus `djen_last_polled_at` (migration `004_djen_poll.sql`),
  DJEN's independent case-mode poll cursor.
- `pending_review` types `djen_party_match` and `unknown_cnpj` (both added by
  `002_djen.sql`, which also declares the now-unused `djen_case_candidate`).

`MEMGRAPH_USER` / `MEMGRAPH_PASS` are optional (dev Memgraph is auth-less);
`DATABASE_URL` and `MEMGRAPH_URI` are required.

## Coverage limitation

DJEN only carries communications published since tribunals migrated (~2023+).
Concluded historical cases return zero results; those rosters are seeded
manually in the backoffice with cited sources, and rematch mode is the only way
they later gain politician links.
