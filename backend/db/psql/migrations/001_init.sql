-- 001_init.sql

CREATE TABLE IF NOT EXISTS schema_migrations (
  id         SERIAL PRIMARY KEY,
  target     TEXT NOT NULL CHECK (target IN ('psql', 'memgraph')),
  filename   TEXT NOT NULL,
  applied_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE (target, filename)
);

CREATE TABLE IF NOT EXISTS sources (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  url            TEXT NOT NULL UNIQUE,
  title          TEXT NOT NULL,
  publisher      TEXT NOT NULL,
  type           TEXT NOT NULL CHECK (type IN (
                   'news_outlet',
                   'government_agency',
                   'court_document',
                   'parliamentary',
                   'official_gazette',
                   'ngo_watchdog',
                   'academic'
                 )),
  reliability    TEXT NOT NULL CHECK (reliability IN ('high', 'medium', 'low')),
  date_published DATE,
  date_scraped   TIMESTAMPTZ DEFAULT now(),
  raw_content    TEXT,
  checksum       TEXT,
  active         BOOLEAN DEFAULT true
);

CREATE TABLE IF NOT EXISTS scraper_jobs (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  worker           TEXT NOT NULL,
  status           TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'skipped')),
  started_at       TIMESTAMPTZ DEFAULT now(),
  finished_at      TIMESTAMPTZ,
  records_upserted INT DEFAULT 0,
  error_message    TEXT
);

CREATE TABLE IF NOT EXISTS audit_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id    TEXT NOT NULL,
  action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  target_type TEXT NOT NULL,
  target_id   TEXT NOT NULL,
  metadata    JSONB,
  created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sources_type        ON sources (type);
CREATE INDEX IF NOT EXISTS idx_sources_reliability ON sources (reliability);
CREATE INDEX IF NOT EXISTS idx_scraper_jobs_worker ON scraper_jobs (worker);
CREATE INDEX IF NOT EXISTS idx_audit_log_target    ON audit_log (target_type, target_id);
