// 004_sanction_retraction.cypher
//
// Split a sanction's IDENTITY from its PUBLICATION STATE.
//
//   :SanctionRecord — permanent. The node's identity. Never removed.
//   :Sanction       — "currently published". Removed when the source retracts it.
//
// Why two labels instead of a `retracted` flag: eight separate queries surface a
// Sanction to the public, and the ninth arrives the day someone adds an endpoint.
// A `WHERE retracted_at IS NULL` that one of them forgets is a republished
// falsehood about a named person. Matching on a LABEL means every query that
// says (:Sanction) excludes retracted records for free — including the queries
// nobody has written yet. The safe thing happens by default.
//
// And why the identity label is needed at all: upserts MERGE on it. If they
// merged on :Sanction, a record that had been retracted (label gone) would not
// be found on the next sync and a SECOND node would be created with the same id.

CREATE CONSTRAINT ON (s:SanctionRecord) ASSERT s.id IS UNIQUE;
CREATE INDEX ON :SanctionRecord(id);
CREATE INDEX ON :SanctionRecord(registry);
CREATE INDEX ON :RetractedSanction(registry);

// Backfill: every sanction we already hold is published, and keeps its identity.
MATCH (s:Sanction) SET s:SanctionRecord;
