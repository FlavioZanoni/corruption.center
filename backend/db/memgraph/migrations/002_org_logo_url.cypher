// 002_org_logo_url.cypher
//
// Adds logo_url to the :Organization node schema.
//
// Memgraph is schema-free — no ALTER TABLE needed. This migration
// creates an index so scraper upserts and graph queries that filter
// or project logo_url can do so efficiently.

CREATE INDEX ON :Organization(logo_url);
