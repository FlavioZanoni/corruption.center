# Graph nodes and edges definitions

## Nodes

```cypher
(:Politician {
  id: string,
  name: string,
  name_aliases: [string],
  party_current: string,
  role_current: string,    // current role today
  state: string,
  tse_profile_url: string,
  photo_url: string,
  active: boolean
})

(:Scandal {
  id: string,
  name: string,        // "Operação Lava Jato"
  aliases: [string],
  description: string,
  date_start: date,
  date_end: date,      // null if ongoing
  total_amount_brl: float,
  status: "ongoing|concluded|prescribed",
  wikipedia_url: string
})

(:Organization {
  id: string,
  name: string,
  cnpj: string,
  type: "party|company|shell|ngo|public_agency",
  active: boolean
})

(:LegalProceeding {
  id: string,
  case_number: string, // número do processo
  court: string,       // "STF", "TRF4", "STJ"
  type: "criminal|administrative|cpi",
  status: "ongoing|concluded",
  date_filed: date,
  date_concluded: date,
  url: string
})

(:Source {
  id: string,
  url: string,
  title: string,
  publisher: string,   // "Folha", "TCU", "STF"
  type: "news|court_doc|official|wikipedia",
  date_published: date,
  reliability: "high|medium|low"
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

// Politician → Organization
(p:Politician)-[:MEMBER_OF {
  role_at_time: string,
  date_from: date,
  date_to: date,
  source_id: string
}]->(o:Organization)

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
