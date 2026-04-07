# Architecture — Rede de Corrupção Brasil

## Overview

A system for mapping, visualizing, and exploring corruption scandals in Brazil
and their connections to politicians, organizations, and legal proceedings.

Data is ingested from official public datasets (TSE, Câmara, Senado, CNJ DataJud),
normalized via deterministic pipelines (CPF/CNPJ-based identity), stored in a
graph database, and served through an interactive graph visualization ([d3-force](https://d3js.org/d3-force)).

---

## System Diagram

```txt
┌─────────────────────────────────────────────────────────────────┐
│                        DATA SOURCES                             │
│      TSE · Câmara · Senado · DataJud · CNPJ WS API              │
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
│  · schema_migrations         │     │  · Source                        │
│                              │     │                                  │
│                              │     │  Edges:                          │
│                              │     │  · DEFENDANT_IN                  │
│                              │     │  · IMPLICATED_IN                 │
│                              │     │  · INVOLVED_IN                   │
│                              │     │  · INVESTIGATES                  │
│                              │     │  · CONTROLS                      │
│                              │     │  · OWNED_BY                      │
│                              │     │  · RELATED_TO                    │
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

### 4. DataJud Searcher (Batch Seeder)

* First pass across all people
* Creates:
  * `LegalProceeding` nodes
  * `DEFENDANT_IN` edges
* Registers cases in **watcher tracking table**

---

### 5. DataJud Watcher (Core Engine)

Continuously updates legal graph:

* Tracks new movements
* Updates outcomes:
  * pending / convicted / acquitted / prescribed
* Discovers:
  * new defendants (CPF/CNPJ)
  * new cases (desmembramento)
  * related cases (processoRelacionado)

Automatically expands the **case tree of a scandal**

---

### 6. CNPJ Enricher

* Resolves organizations from CNPJ

* Builds:
  * `Organization` nodes
  * `CONTROLS` edges (people → org)
  * `OWNED_BY` edges (org → org)

* Detects:
  * shell company chains
  * possible politician ownership (requires human review)

---

### 7. Alias Extractor

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
* SUPPORTS → source → node/edge

---

## Scandal Model

* Created manually
* Seeded with **one root case**
* Watcher expands automatically

```txt
Scandal
  └── root LegalProceeding
         ├── related cases → via `processoRelacionado` on DataJud
         ├── split cases → via `desmembramento` on DataJud
         └── defendants
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
