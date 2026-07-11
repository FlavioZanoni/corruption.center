-- 008_purge_tombstones.sql
--
-- LGPD purge tombstones. When a Person/Organization node is purged via the
-- backoffice removal flow, a tombstone row is written here. Every worker that
-- auto-creates person/org nodes (sanctions, cnpj, djen) MUST consult this
-- table before creating a node, otherwise the weekly syncs silently resurrect
-- personal data that was legally removed (LGPD art. 18).
--
-- subject_key formats (normalized, one row per key: a purge may write several):
--   cpf:<11 digits>            full CPF known
--   cnpj:<14 digits>           organization CNPJ
--   name:<NORMALIZED NAME>     uppercase, accents stripped, single spaces

CREATE TABLE IF NOT EXISTS purge_tombstone (
  subject_key  TEXT PRIMARY KEY,
  node_id      TEXT NOT NULL,            -- graph node id that was purged
  removal_id   TEXT,                     -- removal_request.id (UUID) when applicable
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
