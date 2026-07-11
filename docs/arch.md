# Architecture — Rede de Corrupção Brasil

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
│  · worker_jobs               │     │  Nodes:                          │
│  · watcher_tracking          │     │  · Politician (CPF)              │
│  · pending_reviews           │     │  · Person                        │
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
│  · Profile aggregation endpoints                                │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                          FRONTEND                               │
│                                                                 │
│  · Graph visualization (d3.js)                                  │
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
| frontend      | React + d3.js     | Visualization                      |
| graph db      | Memgraph          | Core relationship storage          |
| relational db | PostgreSQL        | State, tracking, review queues     |

---

## Worker Pipeline

### Design Philosophy

* **All core identity resolution via CPF/CNPJ**
* **Graph is built deterministically from structured sources**
* NLP is optional and only used for:
  * alias suggestions (Wikipedia)

---

## Workers

Each worker is a **single-responsibility ingestion unit**.

### 1. TSE CSV Import (Batch, Historical)

* Creates **baseline Politician dataset**
* Key: `cpf`
* Sets `active: false`
* Handles:
  * cross-year deduplication
  * name alias accumulation

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
party data — Portaria CNJ 160/2020):

* Tracks new movements
* Updates case status/phase (accepted / sentenced / concluded)
* Per-defendant outcomes are set only via backoffice review

---

### 5. DJEN Party Discovery (Party Engine)

Official CNJ national gazette API (public, keyless):

* Case mode: party rosters (`destinatarios`, name + polo) for tracked cases
* Name mode: candidate case numbers for tracked politicians + aliases
* Names only — every Politician link requires human review
* Coverage: communications since ~2023; historical rosters are manually seeded

---

### 6. Sanctions Sync (Punishment Layer)

Official CPF/CNPJ-keyed registries — deterministic matching:

* CGU Portal da Transparência: CEIS, CNEP, CEAF, leniency agreements
* TCU: irregular accounts, inabilitados, inidôneos
* Creates `Sanction` nodes + `SANCTIONED_IN` edges
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
tribunal
last_movement_id
status (active | concluded | paused)
```

### Behavior

* Daily polling for active cases
* Weekly for concluded
* Automatically expands graph via:
  * `processoRelacionado`
  * `desmembramento`

This builds **complete investigation trees over time**

---

## Human-in-the-Loop Layer

### Required Reviews

| Type                       | Trigger            |
| -------------------------- | ------------------ |
| possible_politician_in_qsa | masked CPF match   |
| unknown CPF person         | new defendant      |
| new organization           | missing enrichment |
| alias suggestions          | NLP/Wikipedia      |
| unlinked case              | watcher gap        |

---

## Graph Model

### Nodes

* `Politician` (CPF unique)
* `Person` (non-politician individuals)
* `Organization` (CNPJ unique)
* `LegalProceeding` (case_number unique)
* `Scandal` (manually seeded)
* `Sanction` (registry + entry id unique — CEIS/CNEP/CEAF/TCU records)
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
* SANCTIONED_IN → person/politician/org → sanction (deterministic CPF/CNPJ match, or human-confirmed)
* SUPPORTS → source → node/edge

---

## Scandal Model

* Created manually
* Seeded with root case number(s) + historical defendant roster (cited to
  official decision texts)
* DJEN name mode surfaces additional case candidates for review

```txt
Scandal
  └── root LegalProceeding(s)   (manually seeded)
         ├── status/timeline    → DataJud Watcher (case-level)
         ├── party roster       → DJEN case mode (recent cases)
         └── new case candidates → DJEN name mode → 👤 review
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
