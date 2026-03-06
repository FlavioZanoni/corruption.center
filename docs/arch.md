# Architecture — Rede de Corrupção Brasil

## Overview

A system for mapping, visualizing and searching corruption scandals in Brazil
and their connections to politicians, organizations and legal proceedings.
Data is sourced from public government APIs, court records, official gazettes
and journalism, processed via NLP pipelines, stored in a graph database, and
served through an interactive node-based visualization.

---

## System Diagram

```txt
┌─────────────────────────────────────────────────────────────────┐
│                        DATA SOURCES                             │
│  TSE · DataJud · Portal Transparência · Câmara · Senado · DOU   │
│  STF · STJ · TCU · Brasil.IO · Agência Pública · News Outlets   │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌────────────────────────────────────────────────────────────────┐
│                     PYTHON PIPELINE                            │
│                                                                │
│  ┌─────────────┐   ┌─────────────┐   ┌──────────────────────┐  │
│  │   Scrapers  │──▶│  NLP / NER │─▶│   Entity Resolution  │  │
│  │  (workers)  │   │  (spaCy     │   │   (deduplication,    │  │
│  │  one/source │   │  pt_core_   │   │   alias matching,    │  │
│  │             │   │  news_lg)   │   │   CPF/TSE linking)   │  │
│  └─────────────┘   └─────────────┘   └───────────┬──────────┘  │
│                                                  │             │
└──────────────────────────────────────────────────┼─────────────┘
                                                   │
                    ┌──────────────────────────────┘
                    │
          ┌─────────▼──────────┐
          │                    │
          ▼                    ▼
┌──────────────────┐  ┌─────────────────────────────────────────┐
│   PostgreSQL     │  │               Memgraph                  │
│                  │  │                                         │
│  · sources       │  │  Nodes:                                 │
│  · scraper_jobs  │  │  · Politician                           │
│  · audit_log     │  │  · Scandal                              │
│  · migrations    │  │  · Organization                         │
│                  │  │  · LegalProceeding                      │
│                  │  │  · Source (ref only)                    │
│                  │  │                                         │
│                  │  │  Edges:                                 │
│                  │  │  · INVOLVED_IN                          │
│                  │  │  · DEFENDANT_IN                         │
│                  │  │  · MEMBER_OF                            │
│                  │  │  · IMPLICATED_IN                        │
│                  │  │  · INVESTIGATES                         │
│                  │  │  · RELATED_TO                           │
│                  │  │  · SUPPORTS                             │
└────────┬─────────┘  └──────────────────┬──────────────────────┘
         │                               │
         └──────────────┬────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                       GO API                                    │
│                                                                 │
│  · Graph query endpoints (scandal-centric, politician-centric)  │
│  · Full-text search (Memgraph FTS)                              │
│  · Network traversal (N-hop queries)                            │
│  · Auth (future backoffice?)                                    │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         FRONTEND                                │
│                                                                 │
│  · Canvas graph (Sigma.js)                                      │
│  · Search bar (politician or scandal entry point)               │
│  · Timeline scrubber                                            │
│  · Node detail sidebar (dossier view)                           │
│  · LOD zoom behavior                                            │
└─────────────────────────────────────────────────────────────────┘
```

---

## Services

| Service | Language | Role |
| --- | --- | --- |
| scraper workers | Python | One process per source, independent schedules |
| nlp pipeline | Python | NER, entity dedup, graph construction |
| api | Go | Serves graph data and search to frontend |
| frontend | Svelte + Sigma.js | Interactive graph visualization |
| graph db | Memgraph | Stores nodes and relationships |
| relational db | PostgreSQL | Operational data, sources, job logs |

---

## Python Pipeline — Detail

### Workers

Each worker is an independent Python process responsible for one data source.
Workers are isolated — a failure in one does not affect others.

```txt
worker responsibilities:
  1. fetch data from source (API call or Playwright scrape)
  2. write raw content to sources table in Postgres
  3. pass content to NLP pipeline
  4. upsert resulting nodes and edges into Memgraph
  5. log job result to scraper_jobs table
```

Workers are scheduled via cron. Frequency varies by source:

| Frequency | Workers |
| --- | --- |
| Daily | DOU, STJ DJe feed, active LegalProceedings |
| Weekly | Portal Transparência, TCU, Câmara, Senado, STF, news outlets |
| Monthly | TSE bulk, Brasil.IO, Base dos Dados, dados.gov.br catalog |

> [!NOTE]
> Not sure about frequencies yet, might change

### NLP Pipeline

Runs on raw text after scraping. Uses `spaCy pt_core_news_lg`.

```txt
steps:
  1. Named Entity Recognition — extract PER, ORG, LOC, DATE
  2. Entity classification — is this PER a politician? cross-ref TSE dataset
  3. Alias resolution — "Lula", "Luiz Inácio", "Lula da Silva" → same node
  4. Relation extraction — what is this person's role in this context?
  5. Confidence scoring — how certain are we about each extracted relation?
  6. Graph write — upsert nodes and edges with source_id and confidence
```

### Entity Resolution

The hardest problem in the pipeline. Strategy:

- **Politicians:** match against TSE canonical dataset by name similarity + U
F + party. CPF hash used as primary key where available.
- **Organizations:** match by CNPJ where present, otherwise name fuzzy match.
- **Scandals:** manually seeded initial dataset, NLP only adds new references to
existing scandals.
- **Aliases:** stored in `name_aliases[]` on the node, used for all future
matching and search.

---

## Go API — Endpoints

```txt
GET  /graph/scandal/:id          scandal-centric subgraph
GET  /graph/politician/:id       politician-centric subgraph
GET  /graph/expand/:id           expand one node N hops
GET  /search?q=&type=            full-text search across all node types
GET  /politician/:id             full politician profile + all connections
GET  /scandal/:id                full scandal profile + all connections
GET  /timeline?from=&to=         nodes/edges active within date range
```

All graph endpoints return a consistent shape:

```json
{
  "nodes": [{ "id": "", "type": "", "label": "", "properties": {} }],
  "edges": [{ "id": "", "from": "", "to": "", "type": "", "properties": {} }]
}
```

The frontend maps this directly to Sigma.js graph data.
