// Migration 005 — precomputed folded search name + label indexes.
//
// TWO independent performance fixes the improbidade import makes urgent.
//
// 1. LABEL INDEXES. Every `MATCH (x:Label)` was a full scan of all ~130k nodes
//    regardless of label, because the graph had zero label indexes. The worst
//    offender is /scandals, whose `OPTIONAL MATCH (p:Politician)` degrades as
//    scandals × all-nodes. Importing ~500k rows over a label that did not grow
//    still slows every endpoint that scans by label. A bare label index fixes
//    the whole class.
//
// 2. search_name. Name search folded case+accents at QUERY TIME, wrapping
//    node.name in ~48 nested replace() calls run on every node of every search
//    (see fold.go). That is a ~300x multiplier — an 8s scan for a rare name
//    over 130k nodes — and turns the ~500k-row import into a 40s+ query. This
//    backfills a folded `search_name` once; from now on every name-writer keeps
//    it current (foldQuery). search.go compares $q against search_name instead
//    of folding at query time. The replace() chain below is generated from the
//    same accentFold table foldExpr/foldQuery use, so the stored value and the
//    folded query agree exactly — fold one side only and search matches nothing.
//    Only name-bearing labels get it (Person, Politician, Organization,
//    Scandal); the low-cardinality secondary fields keep query-time folding.

CREATE INDEX ON :Person;
CREATE INDEX ON :Politician;
CREATE INDEX ON :Organization;
CREATE INDEX ON :Scandal;
CREATE INDEX ON :Sanction;
CREATE INDEX ON :LegalProceeding;

MATCH (n) WHERE n.name IS NOT NULL SET n.search_name = replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(toLower(n.name), 'Á', 'a'), 'á', 'a'), 'À', 'a'), 'à', 'a'), 'Â', 'a'), 'â', 'a'), 'Ã', 'a'), 'ã', 'a'), 'Ä', 'a'), 'ä', 'a'), 'É', 'e'), 'é', 'e'), 'È', 'e'), 'è', 'e'), 'Ê', 'e'), 'ê', 'e'), 'Ë', 'e'), 'ë', 'e'), 'Í', 'i'), 'í', 'i'), 'Ì', 'i'), 'ì', 'i'), 'Î', 'i'), 'î', 'i'), 'Ï', 'i'), 'ï', 'i'), 'Ó', 'o'), 'ó', 'o'), 'Ò', 'o'), 'ò', 'o'), 'Ô', 'o'), 'ô', 'o'), 'Õ', 'o'), 'õ', 'o'), 'Ö', 'o'), 'ö', 'o'), 'Ú', 'u'), 'ú', 'u'), 'Ù', 'u'), 'ù', 'u'), 'Û', 'u'), 'û', 'u'), 'Ü', 'u'), 'ü', 'u'), 'Ç', 'c'), 'ç', 'c'), 'Ñ', 'n'), 'ñ', 'n');

// Sanctions do not search by name — they search by registry/type/organ/process
// ref, folded together into search_text so /search stops folding four fields on
// every one of 64k+ sanctions at query time (that was the 3.4s of a 4s search).
MATCH (s:Sanction) SET s.search_text = replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(replace(toLower((coalesce(s.registry,"") + " " + coalesce(s.sanction_type,"") + " " + coalesce(s.organ,"") + " " + coalesce(s.process_ref,""))), 'Á', 'a'), 'á', 'a'), 'À', 'a'), 'à', 'a'), 'Â', 'a'), 'â', 'a'), 'Ã', 'a'), 'ã', 'a'), 'Ä', 'a'), 'ä', 'a'), 'É', 'e'), 'é', 'e'), 'È', 'e'), 'è', 'e'), 'Ê', 'e'), 'ê', 'e'), 'Ë', 'e'), 'ë', 'e'), 'Í', 'i'), 'í', 'i'), 'Ì', 'i'), 'ì', 'i'), 'Î', 'i'), 'î', 'i'), 'Ï', 'i'), 'ï', 'i'), 'Ó', 'o'), 'ó', 'o'), 'Ò', 'o'), 'ò', 'o'), 'Ô', 'o'), 'ô', 'o'), 'Õ', 'o'), 'õ', 'o'), 'Ö', 'o'), 'ö', 'o'), 'Ú', 'u'), 'ú', 'u'), 'Ù', 'u'), 'ù', 'u'), 'Û', 'u'), 'û', 'u'), 'Ü', 'u'), 'ü', 'u'), 'Ç', 'c'), 'ç', 'c'), 'Ñ', 'n'), 'ñ', 'n');
