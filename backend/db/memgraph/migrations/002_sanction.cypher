// 002_sanction.cypher
//
// Sanction node — official Brazilian punishment registries (CGU CEIS/CNEP/CEAF/
// leniency + TCU irregular/inabilitado/inidôneo). id = registry + ":" + entry id.
// Written by the Sanctions Sync worker; linked to Politician/Person/Organization
// via SANCTIONED_IN edges.

// ─── Uniqueness constraint ────────────────────────────────────────────────────

CREATE CONSTRAINT ON (s:Sanction) ASSERT s.id IS UNIQUE;

// ─── Lookup indexes ───────────────────────────────────────────────────────────

CREATE INDEX ON :Sanction(registry);
CREATE INDEX ON :Sanction(process_ref);
