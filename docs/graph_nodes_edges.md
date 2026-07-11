# Graph nodes and edges definitions

## Nodes

```cypher
(:Politician {
  id: string,
  name: string,
  cpf: string,                // dedup key across TSE/Câmara/Senado, used by DataJud searcher
  name_aliases: [string],
  party_current: string,
  role_current: string,       // current role today
  state: string,
  tse_profile_urls: [string],  // one entry per election year
  photo_url: string,
  active: boolean              // false from TSE import; set true by Câmara/Senado sync
})

(:Person {
  id: string,
  name: string,
  cpf: string,                // partially masked e.g. "***456789**"
  role: string,               // "sócio-administrador", "diretor-presidente", etc.
  active: boolean,
  provenance_source: string,        // worker that discovered the person: "djen", "cnpj"
  provenance_comunicacao_id: string, // DJEN: id of the communication the name was read from
  provenance_link: string,           // DJEN: link to the official publication
  provenance_tribunal: string        // DJEN: siglaTribunal, e.g. "TRF4"
})

(:Scandal {
  id: string,
  name: string,               // "Operação Lava Jato"
  aliases: [string],
  description: string,
  date_start: date,
  date_end: date,             // null if ongoing
  total_amount_brl: float,
  status: "ongoing|concluded|prescribed",
  wikipedia_url: string,
  source: string              // "baseline_seed" on the landmark scandals hardcoded in
                              // backend/api/seed.go (name, description, date_start,
                              // date_end, status, wikipedia_url are rewritten from code
                              // on every API start); absent on scandals created in the
                              // backoffice
})

(:Sanction {
  id: string,                 // registry + ":" + entry id, e.g. "CEIS:12345"
  registry: string,           // "CEIS|CNEP|CEAF|ACORDOS_LENIENCIA|TCU_*"
  sanction_type: string,      // registry's own sanction label
  organ: string,              // sanctioning body
  date_start: date,
  date_end: date,
  process_ref: string,        // administrative/judicial process reference, when published
  source_url: string          // required: deep link to the official record
})

(:Organization {
  id: string,
  name: string,
  cnpj: string,
  uf: string,
  share_capital_brl: float,
  main_activity: string,      // CNJ primary activity description
  type: string,               // raw natureza_jurídica from CNPJ API e.g. "2054 - Sociedade Anônima Fechada"
  image_url: string,          // optional, future only
  active: boolean
})

(:LegalProceeding {
  id: string,
  case_number: string,        // número do processo
  court: string,              // "STF", "TRF4", "STJ"
  type: "criminal|administrative|cpi",
  status: "ongoing|concluded",
  assuntos: [string],         // CNJ subject codes; used for scandal cluster detection
  date_filed: date,
  date_concluded: date,
  url: string
})

(:Source {
  id: string,
  url: string,
  title: string,
  publisher: string,          // "DataJud", "STF", "Folha": free text, set by worker or human
  type: "government_agency|court_document|parliamentary|news_outlet",
  date_published: date
})
```

## Edges

```cypher
// Politician → Scandal
(p:Politician)-[:INVOLVED_IN {
  role_at_time: string,
  party_at_time: string,
  status: "convicted|acquitted|under_investigation|cited",
  date_from: date,
  date_to: date,
  source_id: string
}]->(s:Scandal)

// Politician → LegalProceeding
// Never automatic: DJEN publishes party names with no document, so this edge only
// exists once a reviewer confirmed the name is this politician (outcome
// "confirmed", source "backoffice_review").
(p:Politician)-[:DEFENDANT_IN {
  outcome: "convicted|acquitted|pending|prescribed|confirmed",
  sentence: string,
  date: date,
  source: string,             // "backoffice_review"
  source_id: string
}]->(lp:LegalProceeding)

// Politician → Organization (party membership, public role)
(p:Politician)-[:MEMBER_OF {
  role_at_time: string,
  date_from: date,
  date_to: date,
  source_id: string
}]->(o:Organization)

// Politician → Organization (appears in QSA)
(p:Politician)-[:CONTROLS {
  role: string,               // "sócio-administrador", "diretor-presidente"
  date_from: date,
  date_to: date,
  source_id: string
}]->(o:Organization)

// Person → Organization (QSA board membership)
(p:Person)-[:CONTROLS {
  role: string,
  date_from: date,
  date_to: date,              // null if current
  source_id: string
}]->(o:Organization)

// Organization → Organization (QSA entry with CNPJ: shell ownership chains)
(o1:Organization)-[:OWNED_BY {
  share_percent: float,       // ownership percentage if available
  date_from: date,
  date_to: date,
  source_id: string
}]->(o2:Organization)

// Person → LegalProceeding
// Written by DJEN case mode with outcome "cited": the party appears in an official
// publication for this case, and nothing more is asserted.
(p:Person)-[:DEFENDANT_IN {
  outcome: "convicted|acquitted|pending|prescribed|cited",
  sentence: string,
  date: date,
  source: string,             // "djen"
  source_id: string
}]->(lp:LegalProceeding)

// Person → Scandal
(p:Person)-[:INVOLVED_IN {
  role_at_time: string,
  status: "convicted|acquitted|under_investigation|cited",
  date_from: date,
  date_to: date,
  source_id: string
}]->(s:Scandal)

// Organization → LegalProceeding
(o:Organization)-[:DEFENDANT_IN {
  outcome: "convicted|acquitted|pending|prescribed",
  sentence: string,
  date: date,
  source_id: string
}]->(lp:LegalProceeding)

// Organization → Scandal
(o:Organization)-[:IMPLICATED_IN {
  role: "contractor|funder|intermediary|target",
  amount_brl: float,
  date_from: date,
  date_to: date,
  source_id: string
}]->(s:Scandal)

// LegalProceeding → Scandal
(lp:LegalProceeding)-[:INVESTIGATES {
  source_id: string
}]->(s:Scandal)

// Scandal → Scandal
(s1:Scandal)-[:RELATED_TO {
  relationship: "spawned|parallel|same_network",
  source_id: string
}]->(s2:Scandal)

// Politician/Person/Organization → Sanction
// Written by the sanctions worker (CGU/TCU) or by an approved backoffice review.
(n)-[:SANCTIONED_IN {
  confidence: float,            // 0..1, how strongly the record identifies this subject
  confidence_signals: [string], // "full_document" | "masked_cpf_middle6" | "exact_name"
                                //   | "long_name" | "ambiguous_match"
  source: string                // "sanctions" worker, or "backoffice_review"
}]->(s:Sanction)

// Source → anything (reverse attribution)
(src:Source)-[:SUPPORTS {
  date_added: date
}]->(n)
```

## Edge provenance and confidence

Two properties travel on the edges a worker or a reviewer creates, so any link in
the graph can be explained after the fact.

### `source`

Present on every edge written by the code paths above:

* `backoffice_review`: a human confirmed this link in the backoffice.
* otherwise the worker that created it: `djen` (`DEFENDANT_IN`), `cnpj`
  (`CONTROLS`, `OWNED_BY`), `sanctions` (`SANCTIONED_IN`).

### `confidence` / `confidence_signals`

Written on `SANCTIONED_IN` by the sanctions worker. `confidence` is the score
from `backend/matching` (`Score`), and `confidence_signals` lists the evidence
that produced it. A link is written without a human only at document grade
(`matching.AutoLinkThreshold` = 0.85): a full CPF/CNPJ scores 1.00, and a masked
CPF plus an exact name reaches 0.90. A name alone can never reach it. Full policy
and the weight table: `docs/identity_matching.md`.

**An unscored edge carries no `confidence` property at all**, and this is
deliberate: absent means "no identity was inferred", not "0% confident". A
`DEFENDANT_IN` edge from DJEN names the party the way the court's own publication
names it, and a `SANCTIONED_IN` edge confirmed in the backoffice rests on a human
decision: neither is an inference to be scored. The API therefore returns
`confidence: null` for those edges (`float64PtrProp` in
`backend/db/memgraph/proceedings.go`), and the frontend renders them as
"confirmed" or as plain source attribution, never as 0%.
