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
  active: boolean
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
  wikipedia_url: string
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
  assuntos: [string],         // CNJ subject codes — used for scandal cluster detection
  date_filed: date,
  date_concluded: date,
  url: string
})

(:Source {
  id: string,
  url: string,
  title: string,
  publisher: string,          // "DataJud", "STF", "Folha" — free text, set by worker or human
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
(p:Politician)-[:DEFENDANT_IN {
  outcome: "convicted|acquitted|pending|prescribed",
  sentence: string,
  date: date,
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

// Organization → Organization (QSA entry with CNPJ — shell ownership chains)
(o1:Organization)-[:OWNED_BY {
  share_percent: float,       // ownership percentage if available
  date_from: date,
  date_to: date,
  source_id: string
}]->(o2:Organization)

// Person → LegalProceeding
(p:Person)-[:DEFENDANT_IN {
  outcome: "convicted|acquitted|pending|prescribed",
  sentence: string,
  date: date,
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

// Source → anything (reverse attribution)
(src:Source)-[:SUPPORTS {
  date_added: date
}]->(n)
```
