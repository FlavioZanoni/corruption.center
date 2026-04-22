// 001_init.cypher

// ─── Uniqueness constraints ───────────────────────────────────────────────────

CREATE CONSTRAINT ON (p:Politician)       ASSERT p.id IS UNIQUE;
CREATE CONSTRAINT ON (p:Person)           ASSERT p.id IS UNIQUE;
CREATE CONSTRAINT ON (s:Scandal)          ASSERT s.id IS UNIQUE;
CREATE CONSTRAINT ON (o:Organization)     ASSERT o.id IS UNIQUE;
CREATE CONSTRAINT ON (lp:LegalProceeding) ASSERT lp.id IS UNIQUE;
CREATE CONSTRAINT ON (src:Source)         ASSERT src.id IS UNIQUE;

// ─── Lookup indexes ───────────────────────────────────────────────────────────

CREATE INDEX ON :Politician(name);
CREATE INDEX ON :Politician(cpf);
CREATE INDEX ON :Politician(state);
CREATE INDEX ON :Politician(active);

CREATE INDEX ON :Person(name);
CREATE INDEX ON :Person(cpf);

CREATE INDEX ON :Scandal(status);
CREATE INDEX ON :Scandal(date_start);

CREATE INDEX ON :Organization(cnpj);
CREATE INDEX ON :Organization(uf);
CREATE INDEX ON :Organization(active);

CREATE INDEX ON :LegalProceeding(case_number);
CREATE INDEX ON :LegalProceeding(court);
CREATE INDEX ON :LegalProceeding(status);
CREATE INDEX ON :LegalProceeding(type);

// Full-text procedures vary across Memgraph versions and may be unavailable.
// Search endpoints use property-based fallback queries in application code.
