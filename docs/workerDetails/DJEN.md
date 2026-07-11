# DJEN Party Discovery Worker: Deep Dive

## Purpose

Fills the gap DataJud cannot: **who the parties of a case are**. Uses the
Diário de Justiça Eletrônico Nacional (DJEN) public API: the CNJ's official
national gazette of procedural communications. Three modes:

1. **Case mode** (default on): for every case in `watcher_tracking`, fetch its
   communications and extract the party roster.
2. **Name mode** (default on): for every `Politician` (name + aliases), search
   communications to discover case numbers not yet tracked, and register them
   for watching.
3. **Rematch mode** (`-rematch-mode`, default off): re-test the `Person`
   defendants already in the graph against the current politician index.

Source is official (CNJ), public, keyless. Verified empirically 2026-07.

## What may be automated, and what may not

DJEN publishes **names with no document, ever**. The identity policy that
follows from that is written once, in `docs/identity_matching.md`; this file
does not restate it. The distinction that governs this worker:

- **Registering a case asserts nothing about a person.** It creates a
  `LegalProceeding` node and a `watcher_tracking` row so the case gets polled.
  Name mode does this **automatically, with no review**.
- **"This politician is a defendant in this case" is a claim about a person.**
  DJEN gives only a name, so it can never be auto-created: it always goes to
  `pending_review` (type `djen_party_match`), in case mode and in rematch mode
  alike.

## Coverage limitation (important)

DJEN only carries communications published since tribunals migrated to it
(~2023-2025, mandatory per Resolução CNJ 455/2022). A concluded 2016 case
returns zero results. DJEN therefore discovers parties for **live/recent
cases only**. Historical scandal rosters are seeded manually in the
backoffice from decision texts (see `docs/legal_compliance.md`; every
manual entry must cite its official source).

This limitation is also why rematch mode exists (see below): for a pre-2023
case, the party list cannot be re-fetched from any official source (DJEN does
not carry it and DataJud publishes no parties at all), so the `Person` nodes
already stored are the only material left to work with.

## Endpoint

```txt
GET https://comunicaapi.pje.jus.br/api/v1/comunicacao
    ?pagina=1&itensPorPagina=100
    &numeroProcesso=<20 digits>        (case mode)
    &nomeParte=<name>                  (name mode)
```

No auth. JSON response:

```txt
count
items[]:
  id, siglaTribunal, tipoComunicacao, tipoDocumento
  numero_processo (20 digits), numeroprocessocommascara
  nomeClasse, codigoClasse
  data_disponibilizacao, nomeOrgao, link, texto (HTML)
  destinatarios[]: { nome, polo }      -- polo: "A" (ativo) | "P" (passivo)
  destinatarioadvogados[]: lawyer entries
```

`destinatarios` carries **names only** (no CPF/CNPJ). All person matching is
therefore name-based and must go through review (see below).

## Case mode logic

```txt
for each case in watcher_tracking (active first):
  GET ?numeroProcesso=<case_number>, paginate
  roster ← union of destinatarios across communications, dedup by (nome, polo)
  for each party with polo = "P":
    exact normalized match against Politician names+aliases
      → pending_review (type: djen_party_match) with evidence links
      → NEVER auto-create DEFENDANT_IN on a Politician
    no match → upsert Person node (name only, provenance = DJEN comunicação id)
               + DEFENDANT_IN edge with outcome "cited"
  store per-case roster snapshot in Postgres (djen_party_snapshot) so only
  deltas are processed on the next poll
```

## Name mode logic

```txt
for each Politician (active first) and each alias:
  GET ?nomeParte=<name>, paginate (cap: 300 items/name per run)
  group items by numero_processo
  skip cases already tracked in watcher_tracking (IsCaseTracked, digits-only)
  filter by nomeClasse/codigoClasse: criminal and improbidade classes only
  → register the case: upsert LegalProceeding + watcher_tracking row
    (added_by = "djen", tribunal_endpoint from the publication's siglaTribunal,
     NO scandal id)
```

Registration is `registerDiscoveredCase`. It runs with **no human in the loop**,
because it makes no claim about anybody: it only says "this case exists and is
worth watching". What DJEN found through a politician's name (the party link)
is a separate inference and stays gated, so nothing is asserted about that
politician by the act of registering.

Consequences of registering with **no scandal id**: DJEN found the case through
one name, which says nothing about which scandal (if any) the case belongs to.
The case stands alone until an operator attaches it in the backoffice. The next
DJEN case-mode run pulls its full party roster, and the DataJud watcher polls
its status, so the case starts producing evidence immediately.

Two cases are **not** registered, and are logged instead:

- a case number that is not 20 digits after normalization;
- a group whose publications carry no `siglaTribunal`, so no DataJud endpoint
  (`api_publica_<sigla>`) can be resolved and the case could not be polled.

Dedup is `IsCaseTracked` alone. It compares **digits-only** on both sides
(`regexp_replace(case_number, '\D', '', 'g')`), because a backoffice-seeded row
may hold a formatted CNJ number while DJEN returns the bare 20 digits.

The `djen_case_candidate` review type predates this: name mode used to file one
per discovered case and wait for approval. The worker no longer files them. The
type still exists in the `pending_review` whitelist and the backoffice still
knows how to approve a legacy row.

## Rematch mode (`-rematch-mode`)

A party is matched against the politician index exactly **once**, at discovery,
and is then written to `djen_party_snapshot`, so it never reappears in a roster
delta and is never re-tested. That is correct for the API budget and wrong for
identity: a defendant discovered while the politician base was small (before a
TSE import added the state-level offices, or an older election year) would stay
an anonymous `Person` forever, no matter how much the base grew afterwards.

For a **pre-2023 case**, this is not a delay, it is permanent: the party list
can never be re-fetched (DJEN does not cover it, DataJud publishes no parties),
so re-testing the `Person` nodes we already hold is the only path by which such
a case can ever gain a politician link.

```txt
for each Person with a DEFENDANT_IN edge (ListCitedPersons):
  exact normalized match against the current politician index
    → skip if a djen_party_match review already exists for
      (politician_id, proceeding_id)   [HasPartyMatchReview]
    → else pending_review (type: djen_party_match), payload carries
      person_id, politician_id, proceeding_id, case_number, scandal_id,
      origin: "rematch"
```

The claim is the same claim, so the gate is the same gate: rematch **never**
creates the `DEFENDANT_IN` edge on the `Politician`. It hands the operator a
`Person` node that already exists in the graph, alongside the politician it may
be, and the promotion happens only on approval.

Run it after every TSE import, and after any change that widens the politician
base.

## Homonym policy

See `docs/identity_matching.md` for the full policy. In short, name collisions
are common and verified real (e.g. "SERGIO CABRAL COELHO", a private citizen in
a labor case, is not the politician). Rules:

- A name match alone never creates an edge to a `Politician`.
- Full-name exact match (normalized: uppercase, accents stripped) is the
  minimum bar to even enter `pending_review`; substring matches are dropped.
- Review payload must include the communication `link` and `texto` snippet so
  the reviewer can judge context.

## Failure isolation

A run scans hundreds of names, and DJEN returns sporadic 500s. The client
already retries with exponential backoff (6 attempts, 1s to 60s); when it still
fails, the worker **skips that one lookup** and counts it in `fetch_errors`
instead of discarding the whole run. A cancelled context (not a DJEN fault) does
abort the run.

## Stats

| Stat | Meaning |
| --- | --- |
| `cases_registered` | cases discovered in name mode and registered for watching, with no review |
| `skipped_unregistrable` | discovered cases that could not be watched: case number not 20 digits, or the publication named no tribunal (no DataJud endpoint resolves) |
| `candidate_cases_flagged` | discovered cases that passed the tracked/class filters (counted in `--dry-run` too) |
| `persons_rematched` | existing `Person` defendants re-tested against the politician index (all of them, not just hits) |
| `politician_hits_flagged` | `djen_party_match` reviews filed (case mode + rematch mode) |
| `fetch_errors` | DJEN lookups abandoned after the client exhausted its retries; the item is skipped, the run continues |
| `persons_linked` / `orgs_linked` | name-only `Person` / `Organization` nodes with a `DEFENDANT_IN` "cited" edge |
| `skipped_already_tracked` | case already in `watcher_tracking` |
| `skipped_already_reviewed` | an equal `djen_party_match` review already exists (rematch) |
| `skipped_class_filter` | not a criminal / improbidade class |
| `skipped_tombstoned` | LGPD-purged name; the node is not resurrected |
| `pending_reviews` | reviews filed this run (all types) |

## Schedule and politeness

- Case mode: daily for active cases, weekly for concluded (piggyback on the
  DataJud watcher schedule).
- Name mode: weekly, politicians in batches.
- Rematch mode: on demand, after a TSE import or any widening of the politician
  base.
- No published rate limit; self-impose ≤ 60 req/min, exponential backoff on
  429/5xx, identify with a descriptive User-Agent including contact email.
