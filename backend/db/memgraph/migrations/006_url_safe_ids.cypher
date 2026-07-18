// Migration 006 — make name-derived node ids URL-safe.
//
// DJEN/QSA name-keyed ids kept the '/' from names like "ODEBRECHT S/A", and a
// '/' inside a path segment splits the route: /organizacao/org_djen_odebrecht_s/a
// is a 404 that no encoding rescues (%2F is rejected by both Next and gin), so
// every such search result was an unclickable dead end. The id generators now
// map URL-structural characters to '_' (db/memgraph nameSlug); this rewrites
// the existing ids with the SAME transform so the generator re-derives the
// identical id on the next sync — generator and data must move in lockstep or
// the next DJEN run mints duplicate nodes. Collisions were checked: none.
// Sanction ids (REGISTRY:entry) keep their ':' — it is legal in a path segment.

MATCH (n) WHERE (n:Person OR n:Organization) AND n.id CONTAINS '/' SET n.id = replace(n.id, '/', '_');
