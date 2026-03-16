-- 001_init.sql

-- ─── Migration tracking ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS schema_migrations (
  id         SERIAL PRIMARY KEY,
  target     TEXT NOT NULL CHECK (target IN ('psql', 'memgraph')),
  filename   TEXT NOT NULL,
  applied_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE (target, filename)
);

-- ─── Worker job log ───────────────────────────────────────────────────────────
-- One row per worker run. Used for monitoring, debugging, and skipping
-- duplicate runs.

CREATE TABLE IF NOT EXISTS scraper_jobs (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  worker           TEXT NOT NULL,
  status           TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'skipped')),
  started_at       TIMESTAMPTZ DEFAULT now(),
  finished_at      TIMESTAMPTZ,
  records_upserted INT DEFAULT 0,
  error_message    TEXT
);

-- ─── Audit log ────────────────────────────────────────────────────────────────
-- Every create/update/delete on Memgraph nodes and edges, written by workers
-- and backoffice. actor_id is the worker name or backoffice user id.

CREATE TABLE IF NOT EXISTS audit_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id    TEXT NOT NULL,
  action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  target_type TEXT NOT NULL,
  target_id   TEXT NOT NULL,
  metadata    JSONB,
  created_at  TIMESTAMPTZ DEFAULT now()
);

-- ─── DataJud watcher tracking ─────────────────────────────────────────────────
-- One row per tracked LegalProceeding. The watcher loads this table on each
-- run to know what to poll and where it left off.

CREATE TABLE IF NOT EXISTS watcher_tracking (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_number         TEXT NOT NULL UNIQUE,
  tribunal_endpoint   TEXT NOT NULL,         -- e.g. "api_publica_trf4"
  scandal_id          TEXT NOT NULL,         -- Memgraph Scandal node id
  proceeding_id       TEXT NOT NULL,         -- Memgraph LegalProceeding node id
  last_movement_id    TEXT,                  -- last DataJud movimento id seen
  last_polled_at      TIMESTAMPTZ,
  status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'concluded', 'paused')),
  added_by            TEXT NOT NULL,         -- worker name or 'backoffice'
  added_at            TIMESTAMPTZ DEFAULT now()
);

-- ─── Pending aliases ──────────────────────────────────────────────────────────
-- Output of alias_extractor script. Candidates wait here until a human
-- approves or rejects them in the backoffice. Approved aliases are written
-- to the Politician node in Memgraph.

CREATE TABLE IF NOT EXISTS pending_aliases (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  politician_id  TEXT NOT NULL,              -- Memgraph Politician node id
  alias          TEXT NOT NULL,
  source         TEXT NOT NULL CHECK (source IN (
                   'tse_cross_match',        -- same CPF, different name across TSE years
                   'camara_senado_diff',     -- name differs between Câmara and Senado APIs
                   'wikipedia'              -- extracted from Wikipedia intro paragraph
                 )),
  status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'approved', 'rejected')),
  reviewed_by    TEXT,
  reviewed_at    TIMESTAMPTZ,
  created_at     TIMESTAMPTZ DEFAULT now(),
  UNIQUE (politician_id, alias)
);

-- ─── Pending review queue ─────────────────────────────────────────────────────
-- Generic queue for anything a worker flags for human attention before it
-- gets written to Memgraph. The backoffice surfaces these grouped by type.

CREATE TABLE IF NOT EXISTS pending_review (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type         TEXT NOT NULL CHECK (type IN (
                 'unknown_cpf',             -- DataJud found a case party CPF not in DB
                 'unknown_cnpj',            -- DataJud found a case party CNPJ not in DB
                 'cpf_partial_match',       -- masked CPF loosely matches a Politician
                 'cpf_controls_politician', -- possible masked CPF match to a Politician from QSA
                 'scandal_cluster',         -- watcher detected potential new scandal
                 'unlinked_spinoff'         -- new case with no processoRelacionado
               )),
  payload      JSONB NOT NULL,             -- all context needed to make the decision
  worker       TEXT NOT NULL,              -- which worker created this item
  status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'approved', 'rejected', 'deferred')),
  reviewed_by  TEXT,
  reviewed_at  TIMESTAMPTZ,
  created_at   TIMESTAMPTZ DEFAULT now()
);

-- ─── TSE import log ───────────────────────────────────────────────────────────
-- Tracks which election years have been imported so the worker can skip
-- already-processed years and resume interrupted imports.

CREATE TABLE IF NOT EXISTS tse_import_log (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  election_year    INT NOT NULL UNIQUE,
  status           TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
  records_imported INT DEFAULT 0,
  started_at       TIMESTAMPTZ DEFAULT now(),
  finished_at      TIMESTAMPTZ,
  error_message    TEXT
);

-- ─── Indexes ──────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_scraper_jobs_worker      ON scraper_jobs (worker);
CREATE INDEX IF NOT EXISTS idx_scraper_jobs_status      ON scraper_jobs (status);

CREATE INDEX IF NOT EXISTS idx_audit_log_target         ON audit_log (target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor          ON audit_log (actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created        ON audit_log (created_at);

CREATE INDEX IF NOT EXISTS idx_watcher_status           ON watcher_tracking (status);
CREATE INDEX IF NOT EXISTS idx_watcher_scandal          ON watcher_tracking (scandal_id);
CREATE INDEX IF NOT EXISTS idx_watcher_last_polled      ON watcher_tracking (last_polled_at);

CREATE INDEX IF NOT EXISTS idx_pending_aliases_pol      ON pending_aliases (politician_id);
CREATE INDEX IF NOT EXISTS idx_pending_aliases_status   ON pending_aliases (status);

CREATE INDEX IF NOT EXISTS idx_pending_review_type      ON pending_review (type);
CREATE INDEX IF NOT EXISTS idx_pending_review_status    ON pending_review (status);
CREATE INDEX IF NOT EXISTS idx_pending_review_created   ON pending_review (created_at);

CREATE INDEX IF NOT EXISTS idx_tse_import_year          ON tse_import_log (election_year);
