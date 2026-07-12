package memgraph

import (
	"fmt"
	"strings"
)

// Search has to ignore accents, because the data cannot agree on them. The same
// person is "JOSÉ" in one source and "JOSE" in another, and Brazilians type both.
// Today the three spellings of one query return three different, incomplete sets:
// "jose" finds 59 nodes, "josé" finds 37, "JOSÉ" finds 21, and none of them is
// the whole answer. Scandals are worse — they are stored accented ("Operação Lava
// Jato"), so "operacao" finds nothing at all.
//
// This is not only an accent problem. Memgraph's toLower() is ASCII-only:
//
//	toLower('OPERAÇÃO JOSÉ')  =>  'operaÇÃo josÉ'
//
// The accented capitals pass straight through it. So `toLower(name) CONTAINS
// toLower($q)` was never even case-insensitive for an accented name — which is
// why the counts above disagree. Folding case and accents together, on both
// sides, is what makes the comparison mean what it looks like it means.

// accentFold maps every non-ASCII letter that appears in the data to its ASCII
// base, in BOTH cases. Both cases are listed deliberately: toLower will not
// deliver the lowercase form (see above), so the capitals must be folded here or
// not at all.
//
// An ordered slice, not a map: the Cypher expression is built by nesting these in
// sequence, and a map's iteration order would reshuffle the generated query on
// every process start for no reason.
var accentFold = [][2]string{
	{"Á", "a"}, {"á", "a"}, {"À", "a"}, {"à", "a"}, {"Â", "a"}, {"â", "a"},
	{"Ã", "a"}, {"ã", "a"}, {"Ä", "a"}, {"ä", "a"},
	{"É", "e"}, {"é", "e"}, {"È", "e"}, {"è", "e"}, {"Ê", "e"}, {"ê", "e"},
	{"Ë", "e"}, {"ë", "e"},
	{"Í", "i"}, {"í", "i"}, {"Ì", "i"}, {"ì", "i"}, {"Î", "i"}, {"î", "i"},
	{"Ï", "i"}, {"ï", "i"},
	{"Ó", "o"}, {"ó", "o"}, {"Ò", "o"}, {"ò", "o"}, {"Ô", "o"}, {"ô", "o"},
	{"Õ", "o"}, {"õ", "o"}, {"Ö", "o"}, {"ö", "o"},
	{"Ú", "u"}, {"ú", "u"}, {"Ù", "u"}, {"ù", "u"}, {"Û", "u"}, {"û", "u"},
	{"Ü", "u"}, {"ü", "u"},
	{"Ç", "c"}, {"ç", "c"},
	{"Ñ", "n"}, {"ñ", "n"},
}

// foldExpr wraps a Cypher string expression so it compares without case or
// accents. Used on the STORED side of every search comparison.
//
// ponytail: this folds at query time, on every node, every search — no stored
// column to maintain and so no way for one of the eight writers that set a name
// to forget it and silently make its entities unfindable. The price is a full
// scan with ~48 nested replaces, which measures around half a second over the
// 36k named nodes we have. If the base grows an order of magnitude, the upgrade
// is a folded `search_name` property written once and indexed — at which point
// maintaining it across every writer becomes the cost worth paying.
func foldExpr(expr string) string {
	out := "toLower(" + expr + ")"
	for _, f := range accentFold {
		out = fmt.Sprintf("replace(%s, '%s', '%s')", out, f[0], f[1])
	}
	return out
}

// foldQuery folds the user's search term the same way, in Go. It must agree with
// foldExpr exactly: fold one side and not the other and the search matches
// nothing, which looks like "no results" rather than like a bug.
func foldQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	for _, f := range accentFold {
		q = strings.ReplaceAll(q, f[0], f[1])
	}
	return q
}
