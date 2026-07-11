# DJEN Party Discovery Worker - Deep Dive

## Purpose

Fills the gap DataJud cannot: **who the parties of a case are**. Uses the
Diário de Justiça Eletrônico Nacional (DJEN) public API — the CNJ's official
national gazette of procedural communications. Two modes:

1. **Case mode** — for every case in `watcher_tracking`, fetch its
   communications and extract the party roster.
2. **Name mode** — for every `Politician` (name + aliases), search
   communications to discover case numbers not yet tracked.

Source is official (CNJ), public, keyless. Verified empirically 2026-07.

## Coverage limitation (important)

DJEN only carries communications published since tribunals migrated to it
(~2023–2025, mandatory per Resolução CNJ 455/2022). A concluded 2016 case
returns zero results. DJEN therefore discovers parties for **live/recent
cases only**. Historical scandal rosters are seeded manually in the
backoffice from decision texts (see `docs/legal_compliance.md` — every
manual entry must cite its official source).

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

`destinatarios` carries **names only — no CPF/CNPJ**. All person matching is
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
  skip cases already in watcher_tracking or already rejected in review
  filter by nomeClasse/codigoClasse: criminal and improbidade classes only
  → pending_review (type: djen_case_candidate) with tribunal, class,
    sample communications, and which polo the name appears in
```

Approval in the backoffice registers the case in `watcher_tracking`
(`added_by = "djen"`) and links it to a scandal (or creates one).

## Homonym policy

Name collisions are common and verified real (e.g. "SERGIO CABRAL COELHO",
a private citizen in a labor case, ≠ the politician). Rules:

- A name match alone never creates an edge to a `Politician`.
- Full-name exact match (normalized: uppercase, accents stripped) is the
  minimum bar to even enter `pending_review`; substring matches are dropped.
- Review payload must include the communication `link` and `texto` snippet so
  the reviewer can judge context.

## Schedule and politeness

- Case mode: daily for active cases, weekly for concluded (piggyback on the
  DataJud watcher schedule).
- Name mode: weekly, politicians in batches.
- No published rate limit — self-impose ≤ 60 req/min, exponential backoff on
  429/5xx, identify with a descriptive User-Agent including contact email.
