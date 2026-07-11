-- 002_djen.sql
-- DJEN Party Discovery worker: extend the review queue with DJEN review types
-- and add a per-case party roster snapshot for delta detection.

-- ─── Extend pending_review type allowlist ─────────────────────────────────────
-- The original CHECK constraint (001_init.sql) only knew about the DataJud
-- review types. DJEN adds two more:
--   * djen_party_match   — a case party name exactly matches a Politician; a
--                          human must confirm the identity before any edge is
--                          created (names carry no CPF, homonyms are common).
--   * djen_case_candidate — name-mode discovered a case number not yet tracked
--                          that looks criminal/improbidade; approval registers
--                          it in watcher_tracking.

ALTER TABLE pending_review DROP CONSTRAINT IF EXISTS pending_review_type_check;
ALTER TABLE pending_review ADD CONSTRAINT pending_review_type_check CHECK (type IN (
  'unknown_cpf',
  'unknown_cnpj',
  'cpf_partial_match',
  'possible_politician_in_qsa',
  'scandal_cluster',
  'unlinked_spinoff',
  'djen_party_match',
  'djen_case_candidate'
));

-- ─── DJEN per-case party roster snapshot ──────────────────────────────────────
-- One row per (case, party) observed in DJEN communications. On each poll the
-- worker builds the current roster and only processes parties whose key is not
-- already present here, so a case with a stable roster does no repeated work.
-- party_key is the normalized "<NAME>|<polo>" (uppercase, accents stripped).

CREATE TABLE IF NOT EXISTS djen_party_snapshot (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_number   TEXT NOT NULL,           -- 20-digit numero_processo
  party_key     TEXT NOT NULL,           -- normalized "<NAME>|<polo>"
  nome          TEXT NOT NULL,           -- raw destinatario name as published
  polo          TEXT NOT NULL,           -- "A" (ativo) | "P" (passivo)
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (case_number, party_key)
);

CREATE INDEX IF NOT EXISTS idx_djen_snapshot_case ON djen_party_snapshot (case_number);
