// 001_init.cypher

// --- Uniqueness constraints ---

CREATE CONSTRAINT ON (p:Politician)       ASSERT p.id IS UNIQUE;
CREATE CONSTRAINT ON (s:Scandal)          ASSERT s.id IS UNIQUE;
CREATE CONSTRAINT ON (o:Organization)     ASSERT o.id IS UNIQUE;
CREATE CONSTRAINT ON (lp:LegalProceeding) ASSERT lp.id IS UNIQUE;
CREATE CONSTRAINT ON (src:Source)         ASSERT src.id IS UNIQUE;

// --- Lookup indexes ---

CREATE INDEX ON :Politician(name);
CREATE INDEX ON :Politician(state);
CREATE INDEX ON :Scandal(status);
CREATE INDEX ON :Scandal(date_start);
CREATE INDEX ON :Organization(cnpj);
CREATE INDEX ON :LegalProceeding(case_number);
CREATE INDEX ON :LegalProceeding(court);

// --- Full-text indexes ---

CALL db.index.fulltext.createNodeIndex(
  "politician_fulltext",
  ["Politician"],
  ["name", "name_aliases"]
);

CALL db.index.fulltext.createNodeIndex(
  "scandal_fulltext",
  ["Scandal"],
  ["name", "aliases", "description"]
);

CALL db.index.fulltext.createNodeIndex(
  "organization_fulltext",
  ["Organization"],
  ["name"]
);

CALL db.index.fulltext.createNodeIndex(
  "global_fulltext",
  ["Politician", "Scandal", "Organization"],
  ["name", "name_aliases", "aliases"]
);
