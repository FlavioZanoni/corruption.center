-- ─── Data removal request queue ───────────────────────────────────────────────
-- LGPD art. 18 removal requests. One row per incoming request. The backoffice
-- surfaces these, lets an operator resolve them (including a one-click purge of
-- the targeted Person node), and every resolution is mirrored into audit_log.
--
-- Politicians are public officials (LGPD art. 23) and are never purgeable — the
-- purge path refuses them in code; a request against a politician is resolved
-- with a 'rejected' status and a documented justification.

CREATE TABLE IF NOT EXISTS removal_request (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  requester     TEXT NOT NULL,                 -- requester identity (name and/or email)
  target_type   TEXT NOT NULL,                 -- targeted node label / 'node' / 'edge'
  target_id     TEXT NOT NULL,                 -- Memgraph node/edge id
  reason        TEXT,                          -- free-text request details
  status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'resolved', 'rejected')),
  resolution    TEXT,                          -- what the operator did / why refused
  received_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at   TIMESTAMPTZ,
  resolved_by   TEXT
);

CREATE INDEX IF NOT EXISTS idx_removal_request_status   ON removal_request (status);
CREATE INDEX IF NOT EXISTS idx_removal_request_received ON removal_request (received_at);
CREATE INDEX IF NOT EXISTS idx_removal_request_target   ON removal_request (target_type, target_id);
