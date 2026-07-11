-- 003_sanctions.sql
--
-- Sanctions Sync worker (CGU Portal da Transparência + TCU). Adds per-registry
-- import-state tracking and extends the pending_review type whitelist with the
-- masked-CPF politician-sanction review created by this worker.

-- ─── Extend pending_review type whitelist ─────────────────────────────────────
-- The Sanctions worker never auto-links a masked CPF to a Politician; every
-- possible hit is queued as 'possible_politician_sanction' for human review.

ALTER TABLE pending_review DROP CONSTRAINT IF EXISTS pending_review_type_check;

-- NOTE: this constraint is the UNION of all types known up to this migration: -- it must include the DJEN types added in 002_djen.sql, since each rebuild
-- replaces the whole allowlist.
ALTER TABLE pending_review ADD CONSTRAINT pending_review_type_check CHECK (type IN (
  'unknown_cpf',                   -- DataJud found a case party CPF not in DB
  'unknown_cnpj',                  -- DataJud found a case party CNPJ not in DB
  'cpf_partial_match',             -- masked CPF loosely matches a Politician
  'possible_politician_in_qsa',    -- masked CPF in QSA loosely matches a Politician
  'possible_politician_sanction',  -- masked CPF / name in a sanction registry loosely matches a Politician
  'scandal_cluster',               -- watcher detected potential new scandal
  'unlinked_spinoff',              -- new case with no processoRelacionado
  'djen_party_match',              -- DJEN party name exactly matches a Politician (002)
  'djen_case_candidate'            -- DJEN name-mode case candidate (002)
));

-- ─── Sanctions import state ───────────────────────────────────────────────────
-- One row per registry. Records where the last weekly sync left off so runs are
-- observable and resumable. registry is the normalized code, e.g. CEIS, CNEP,
-- CEAF, LENIENCIA, TCU_IRREGULAR, TCU_INABILITADO, TCU_INIDONEO.

CREATE TABLE IF NOT EXISTS sanctions_import_state (
  registry          TEXT PRIMARY KEY,
  last_page         INT NOT NULL DEFAULT 0,     -- last CGU page fetched (0 for CSV registries)
  records_seen      INT NOT NULL DEFAULT 0,     -- records processed in the last completed run
  last_synced_at    TIMESTAMPTZ,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
