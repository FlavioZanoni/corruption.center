# Architecture: Rede de Corrupção Brasil

## Overview

A system for mapping, visualizing, and exploring corruption scandals in Brazil
and their connections to politicians, organizations, and legal proceedings.

Data is ingested from official public datasets (TSE, Câmara, Senado, CNJ
DataJud, CNJ DJEN, CGU Portal da Transparência, TCU), normalized via
deterministic pipelines (CPF/CNPJ-based identity), stored in a graph database,
and served through an interactive graph visualization ([d3-force](https://d3js.org/d3-force)).

---

## System Diagram

```txt
┌─────────────────────────────────────────────────────────────────┐
│                        DATA SOURCES                             │
│  TSE · Câmara · Senado · DataJud · DJEN · CGU · TCU · CNPJ WS   │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│                     WORKER PIPELINE                            │
│                                                                │
│  ┌────────────────────┐   ┌────────────────────┐               │
│  │  Source Workers    │──▶│  Graph Upserts     │               │
│  │                    │   │  (CPF/CNPJ keyed)  │               │
│  └────────────────────┘   └──────────┬─────────┘               │
│                                      │                         │
│                         ┌────────────▼────────────┐            │
│                         │  Watchers / Enrichers   │            │
│                         │  (DataJud, CNPJ)        │            │
│                         └────────────┬────────────┘            │
│                                      │                         │
└──────────────────────────────────────┼─────────────────────────┘
                                       │
                 ┌─────────────────────┴─────────────────────┐
                 │                                           │
                 ▼                                           ▼
┌──────────────────────────────┐     ┌──────────────────────────────────┐
│        PostgreSQL            │     │            Memgraph              │
│                              │     │                                  │
│  · scraper_jobs              │     │  Nodes:                          │
│  · watcher_tracking          │     │  · Politician (CPF)              │
│  · pending_review            │     │  · Person                        │
│  · pending_aliases           │     │  · Organization (CNPJ)           │
│  · audit_log                 │     │  · LegalProceeding               │
│  · tse_import_log            │     │  · Scandal                       │
│  · schema_migrations         │     │  · Sanction                      │
│                              │     │  · Source                        │
│                              │     │                                  │
│                              │     │  Edges:                          │
│                              │     │  · DEFENDANT_IN                  │
│                              │     │  · IMPLICATED_IN                 │
│                              │     │  · INVOLVED_IN                   │
│                              │     │  · INVESTIGATES                  │
│                              │     │  · CONTROLS                      │
│                              │     │  · OWNED_BY                      │
│                              │     │  · RELATED_TO                    │
│                              │     │  · SANCTIONED_IN                 │
│                              │     │  · SUPPORTS                      │
└──────────────┬───────────────┘     └──────────────┬───────────────────┘
               │                                    │
               └──────────────┬─────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                          GO API                                 │
│                                                                 │
│  · Graph traversal (scandal / politician centric)               │
│  · Full-text search (aliases + names)                           │
│  · Timeline queries                                             │
│  · Profile + paginated browse endpoints                         │
│    (politicians, scandals, proceedings)                         │
│  · Baseline scandal seed on start (idempotent)                  │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                          FRONTEND                               │
│                                                                 │
│  · Graph visualization (d3.js, client component)                │
│  · Server-rendered entity pages: /politico/[id],                │
│    /escandalo/[id] (metadata + JSON-LD, sitemap, robots)        │
│  · Search (alias-aware)                                         │
│  · Timeline                                                     │
│  · Node dossier views                                           │
└─────────────────────────────────────────────────────────────────┘
```

---

## Services

| Service       | Language          | Role                               |
| ------------- | ----------------- | ---------------------------------- |
| workers       | Go                | Deterministic ingestion per source |
| watchers      | Go                | Incremental updates (DataJud)      |
| enrichers     | Go                | External enrichment (CNPJ WS)      |
| api           | Go                | Graph querying + aggregation       |
| frontend      | Next.js + d3.js   | SSR entity pages + graph viz       |
| graph db      | Memgraph          | Core relationship storage          |
| relational db | PostgreSQL        | State, tracking, review queues     |

---

## Public API

Read-only, no auth, CORS-enabled. Base path `/api/v1` (Swagger at `/swagger`).

| Endpoint                    | Returns                                                             |
| --------------------------- | ------------------------------------------------------------------- |
| `GET /graph/scandal/:id`    | scandal-centric subgraph                                            |
| `GET /graph/politician/:id` | politician-centric subgraph                                         |
| `GET /graph/expand/:id`     | one more hop from a node                                            |
| `GET /search`               | alias-aware full-text search                                        |
| `GET /politicians`          | paginated politician list                                           |
| `GET /politician/:id`       | politician profile + connections                                    |
| `GET /scandals`             | paginated scandal list                                              |
| `GET /scandal/:id`          | scandal profile + connections                                       |
| `GET /proceedings`          | paginated legal proceeding list                                     |
| `GET /proceeding/:id`       | proceeding, its scandal, and every defendant with the provenance of its `DEFENDANT_IN` edge |
| `GET /timeline`             | dated events                                                        |

The paginated list endpoints (`/politicians`, `/scandals`, `/proceedings`) share
one page shape: walking every page enumerates the corpus, which is what the
frontend sitemap does.

### Baseline seed

On every start the API seeds the landmark scandals hardcoded in
`backend/api/seed.go`, idempotently and through exactly the backoffice
registration path (`Scandal` node via `UpsertScandalSeed` + `LegalProceeding` +
`watcher_tracking` row), so a fresh install boots with real content and the
watchers immediately poll those cases. Today: Operação Lava Jato (3 TRF4 cases),
Operação Calicute (1 TRF2 case), Escândalo do Mensalão (no case: AP 470 was tried
at the STF, which has no public DataJud endpoint, so it cannot be auto-updated).
Every case number was verified against the live DataJud API. Set
`SEED_BASELINE=false` to skip. A seed failure is logged and never blocks startup.

---

## Frontend

Next.js App Router. Two rendering modes, on purpose:

* **Server-rendered, crawlable entity routes**: `/politico/[id]` and
  `/escandalo/[id]` are plain HTML (no client component, no canvas) with
  per-entity metadata and JSON-LD, revalidated hourly, plus `app/sitemap.ts` and
  `app/robots.ts` (which disallow `/backoffice` and `/api`). A reader with no
  JavaScript, and a crawler, get the full dossier including the provenance of
  every link.
* **Interactive graph**: a client component (d3-force), for exploration.

---

## Worker Pipeline

### Design Philosophy

* **All core identity resolution via CPF/CNPJ**
* **Graph is built deterministically from structured sources**
* Where a source gives no document (DJEN names, CGU masked CPFs), the identity is
  **scored**, and a link is written automatically only at document grade:
  `docs/identity_matching.md`
* NLP is optional and only used for:
  * alias suggestions (Wikipedia)

---

## Workers

Each worker is a **single-responsibility ingestion unit**.

### 1. TSE CSV Import (Batch, Historical)

* Creates **baseline Politician dataset**
* Key: `cpf`
* Sets `active: false`
* Offices: federal (presidente, vice, senador, deputado federal) **and state**
  (governador, vice-governador, deputado estadual, deputado distrital): a person
  outside this base can never be matched to a court party or a sanction
* Handles:
  * cross-year deduplication
  * name alias accumulation
  * resumable downloads (the TSE CDN drops these multi-hundred-MB zips mid-transfer)

Output:

* `Politician` nodes (historical winners since 2002)

---

### 2. Câmara Sync (Weekly)

* Source of truth for **current deputies**
* Upserts by CPF
* Overrides:
  * `active = true`
  * `party_current`
  * `role_current`

---

### 3. Senado Sync (Weekly)

* Same role as Câmara for senators

---

### 4. DataJud Watcher (Case Status Engine)

Keeps tracked cases current at the **case level** (the public API exposes no
party data; Portaria CNJ 160/2020):

* Tracks new movements
* Updates case status/phase (accepted / sentenced / concluded)
* Per-defendant outcomes are set only via backoffice review

---

### 5. DJEN Party Discovery (Party Engine)

Official CNJ national gazette API (public, keyless):

* Case mode: party rosters (`destinatarios`, name + polo) for tracked cases
* Name mode: candidate cases for tracked politicians + aliases. Registering a case
  asserts nothing about any person, so it is automatic (`LegalProceeding` +
  `watcher_tracking`, no scandal attached)
* Rematch mode: re-tests known name-only `Person` defendants against a grown
  politician index (run after a TSE import)
* Names only, never a document: a Politician **link** can therefore never be
  automatic and always goes to review (see Human-in-the-Loop below)
* Coverage: communications since ~2023; historical rosters are manually seeded

---

### 6. Sanctions Sync (Punishment Layer)

Official CPF/CNPJ-keyed registries:

* CGU Portal da Transparência: CEIS, CNEP, CEAF, leniency agreements
* TCU: irregular accounts, inabilitados, inidôneos
* Creates `Sanction` nodes + `SANCTIONED_IN` edges, each stamped with the
  `confidence` and `confidence_signals` that identified the subject
* Links automatically at document grade only (full CPF/CNPJ, or CGU's masked CPF
  plus an exact name); weaker or ambiguous evidence goes to review
* CNJ CNCIAI deferred pending institutional API access (Portaria 94)

---

### 7. CNPJ Enricher

* Resolves organizations from CNPJ

* Builds:
  * `Organization` nodes
  * `CONTROLS` edges (people → org)
  * `OWNED_BY` edges (org → org)

* Detects:
  * shell company chains
  * possible politician ownership (requires human review)

---

### 8. Alias Extractor

Hybrid system:

* deterministic (cross-source CPF grouping)
* NLP (Wikipedia bold extraction)

Writes to:

* `pending_aliases` (human-reviewed)

---

## Watcher System

The system is **event-driven over time**, not batch-based.

### Watcher Table (Postgres)

Tracks:

```txt
case_number
tribunal_endpoint            (e.g. api_publica_trf4)
scandal_id / proceeding_id
last_movement_id
last_polled_at
status (active | concluded | paused)
added_by (baseline_seed | backoffice | worker name)
```

### Behavior

* Rows are created by the baseline seed, the backoffice, and DJEN name mode
* Daily polling for active cases
* Weekly for concluded
* The graph expands through DJEN, not DataJud: the public DataJud API exposes no
  related-case references (`processoRelacionado`, `desmembramento`) and no
  parties, so new cases are found by searching politician names in DJEN

This builds **complete investigation trees over time**

---

## Human-in-the-Loop Layer

### The gate

A human is required for **claims about a named person**, not for records. Official
sources are reliable about *what* happened and often silent about *who* it
happened to: DJEN publishes party names with no document at all, CGU masks CPFs to
their six middle digits. Tying such a record to one of our politicians is an
inference we make, and a wrong one publishes a false accusation.

So every such link is scored (`backend/matching`) and only document-grade evidence
is written without a human:

* a full CPF/CNPJ **identifies**: auto-link
* a masked CPF **plus an exact name** reaches document grade: auto-link
* a name alone, however exact, **never** can: review

The score and its signals travel on the edge, so any automatic link can be
explained after the fact. Weights, thresholds and the full policy live in
**`docs/identity_matching.md`**, which is the single source of truth: this section
does not restate them.

Registering a **case** for watching asserts nothing about anybody, so it is fully
automatic (DJEN name mode).

### Required Reviews

| Type                          | Trigger                                          |
| ----------------------------- | ------------------------------------------------ |
| djen_party_match              | politician name in a case party roster           |
| possible_politician_sanction  | masked CPF below document grade, or ambiguous    |
| possible_politician_in_qsa    | politician-looking partner in a company's QSA    |
| unknown_cnpj                  | organization with no CNPJ to enrich              |
| alias suggestions             | NLP/Wikipedia                                    |

---

## Graph Model

### Nodes

* `Politician` (CPF unique)
* `Person` (non-politician individuals)
* `Organization` (CNPJ unique)
* `LegalProceeding` (case_number unique)
* `Scandal` (seeded: from the hardcoded baseline on API start, or in the backoffice)
* `Sanction` (registry + entry id unique: CEIS/CNEP/CEAF/TCU records)
* `Source` (external reference / provenance)

---

### Edges

* DEFENDANT_IN → person/org → legal proceeding
* INVESTIGATES → legal proceeding → scandal
* INVOLVED_IN → person → scandal
* IMPLICATED_IN → organization → scandal
* MEMBER_OF → politician → organization (party / public role)
  * Used for:
    * party membership
    * legislative roles
  * Not used for corporate ownership (use CONTROLS instead)
* CONTROLS → person/politician → organization
* OWNED_BY → organization → organization
* RELATED_TO → same-type relationships only:
  * scandal ↔ scandal
* SANCTIONED_IN → person/politician/org → sanction (document-grade match, or
  human-confirmed; carries `confidence` + `confidence_signals`)
* SUPPORTS → source → node/edge

Edges written by a worker or a reviewer carry a `source` (`backoffice_review`, or
the worker: `djen`, `cnpj`, `sanctions`). An edge that involved no identity
inference carries **no** `confidence` property: absent means "nothing was
inferred", not "0%". Property-level detail: `docs/graph_nodes_edges.md`.

---

## Scandal Model

* Created in the backoffice, or from the hardcoded baseline (`backend/api/seed.go`)
  that the API applies idempotently on every start, through the same registration
  path: Scandal node + LegalProceeding + `watcher_tracking` row
* Seeded with root case number(s) + historical defendant roster (cited to
  official decision texts)
* A scandal whose case has no public DataJud endpoint (the STF is not in the
  public API: Mensalão/AP 470) is seeded as a node with no watched case, and gets
  no automatic updates
* DJEN name mode registers additional cases it finds by a politician's name, with
  no scandal attached until an operator links them

```txt
Scandal
  └── root LegalProceeding(s)   (baseline seed or backoffice)
         ├── status/timeline    → DataJud Watcher (case-level)
         ├── party roster       → DJEN case mode (recent cases)
         └── politician name hit → 👤 review (never automatic)
```

### Identity Rules

* `Politician` is authoritative when CPF is known and confirmed
* `Person` is used when:
  * CPF is missing or masked
  * identity is uncertain
* A `Person` may later be merged into a `Politician` via human review

### Merge / Promotion Flow

When a pending review confirms identity:

* `Person` → merged into `Politician` OR linked via CPF
* edges are reattached to canonical node
* audit_log records the merge

### Uniqueness Constraints

* Politician.cpf → unique
* Organization.cnpj → unique
* LegalProceeding.case_number → unique
* Scandal.id → unique

### Relationship Semantics

* `DEFENDANT_IN` → strictly legal relationship (comes from DataJud)
* `INVOLVED_IN` → broader contextual involvement (manual or derived)

### Edge Direction Convention

All relationships are directed from **entity → context**.

Examples:

* Person → LegalProceeding (`DEFENDANT_IN`)
* LegalProceeding → Scandal (`INVESTIGATES`)
* Person → Organization (`CONTROLS`)
* Politician → Scandal (`INVOLVED_IN`)
