// 003_id_indexes.cypher
//
// Index every label on `id`.
//
// 001 declared `ASSERT <label>.id IS UNIQUE` and stopped there. In Memgraph a
// uniqueness CONSTRAINT is not a lookup INDEX: it enforces the invariant on write
// and does nothing for reads. SHOW INDEX INFO listed name, cpf, cnpj, case_number
// — and not one id.
//
// But `id` is how virtually every query in this codebase finds a node:
// MATCH (lp:LegalProceeding {id: $id}), MERGE (o:Organization {id: $id}),
// MATCH (s:Sanction {id: $id}). Each of those was a full label scan — 8k
// proceedings, 16k persons, 65k sanctions — on every single lookup, and the
// workers do one per record they touch. It is why aggregate queries against this
// graph time out, and it is a tax on every page of the site.
//
// The unlabelled lookups (MATCH (x {id: $id}), where the party may be a Politician,
// a Person or an Organization) still scan the whole graph, because a label-property
// index cannot help a query with no label. They are backoffice-write paths, not hot
// reads, so they stay as they are — but that is why they are slow, and this comment
// is here so the next person does not go looking for an index that cannot exist.

CREATE INDEX ON :Politician(id);
CREATE INDEX ON :Person(id);
CREATE INDEX ON :Scandal(id);
CREATE INDEX ON :Organization(id);
CREATE INDEX ON :LegalProceeding(id);
CREATE INDEX ON :Sanction(id);
CREATE INDEX ON :Source(id);
