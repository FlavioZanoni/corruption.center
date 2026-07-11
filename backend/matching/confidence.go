// Package matching scores how strongly a record from an official source can be
// tied to a specific person in the graph.
//
// The problem it exists for: official sources are trustworthy about *what*
// happened but often silent about *who* it happened to. DJEN publishes party
// names with no document at all; CGU masks CPFs to their six middle digits. So
// linking "a person named X" to "our politician X" is an inference we make, not
// a fact the source states — and a wrong inference publishes a false accusation
// about a named individual, which no amount of source-linking cures.
//
// The score turns that inference into an explicit, auditable number. A link is
// written automatically only when the evidence is document-grade; anything
// weaker is queued for a human, and the reasons travel with it.
package matching

import (
	"strings"
	"unicode"
)

// AutoLinkThreshold is the score at or above which a link may be created without
// human review. It is calibrated so that a document signal is required: no
// combination of name-only evidence can reach it.
const AutoLinkThreshold = 0.90

// Signal names recorded on the edge, so any link can be explained after the fact.
const (
	SignalFullDocument = "full_document"      // exact CPF/CNPJ from the source
	SignalMaskedCPF    = "masked_cpf_middle6" // CGU's ***.NNN.NNN-** middle digits
	SignalExactName    = "exact_name"         // full name matches after normalization
	SignalLongName     = "long_name"          // 4+ tokens: far fewer people share it
	SignalAmbiguous    = "ambiguous_match"    // evidence fits more than one politician
)

// Evidence is what an official record gives us about identity.
type Evidence struct {
	// FullDocument is true when the source published a full CPF/CNPJ that matched.
	FullDocument bool
	// MaskedCPF is true when CGU's six visible middle digits matched the CPF we
	// hold for this politician (from TSE).
	MaskedCPF bool
	// SourceName / SubjectName are compared after normalization.
	SourceName  string
	SubjectName string
	// Candidates is how many distinct politicians this same evidence fits. More
	// than one means the evidence cannot identify anybody on its own.
	Candidates int
}

// Score returns a confidence in [0,1] and the signals that produced it.
//
// Weights are set so that identity requires a document:
//
//	masked CPF (0.60) + exact name (0.30) + long name (0.05) = 0.95 → auto-link
//	masked CPF alone, name differs                            = 0.60 → review
//	exact name alone, no document                             = 0.30 → review
//
// A name, however exact, can never reach the threshold by itself — homonyms are
// abundant (DJEN returns thousands of distinct people for a single common name),
// so a name-only match is a lead, not an identification.
func Score(e Evidence) (float64, []string) {
	if e.FullDocument {
		return 1.0, []string{SignalFullDocument}
	}

	score := 0.0
	signals := []string{}

	if e.MaskedCPF {
		score += 0.60
		signals = append(signals, SignalMaskedCPF)
	}

	if NamesMatch(e.SourceName, e.SubjectName) {
		score += 0.30
		signals = append(signals, SignalExactName)
		if len(strings.Fields(NormalizeName(e.SourceName))) >= 4 {
			score += 0.05
			signals = append(signals, SignalLongName)
		}
	}

	// Evidence that fits several people identifies none of them. Cap below the
	// threshold rather than zeroing it: the match is still worth a human's time.
	if e.Candidates > 1 {
		signals = append(signals, SignalAmbiguous)
		if score > 0.5 {
			score = 0.5
		}
	}

	if score > 1 {
		score = 1
	}
	return score, signals
}

// AutoLink reports whether evidence is strong enough to write an edge with no
// human in the loop.
func AutoLink(e Evidence) (bool, float64, []string) {
	score, signals := Score(e)
	return score >= AutoLinkThreshold, score, signals
}

// NamesMatch compares two names after normalization (accents, case, spacing).
func NamesMatch(a, b string) bool {
	na, nb := NormalizeName(a), NormalizeName(b)
	return na != "" && na == nb
}

// NormalizeName upper-cases, strips accents and collapses whitespace, so
// "José da Silva" and "JOSE DA SILVA" compare equal.
func NormalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if folded, ok := foldAccent(r); ok {
			b.WriteRune(folded)
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.FieldsFunc(b.String(), func(r rune) bool {
		return unicode.IsSpace(r)
	}), " ")
}

func foldAccent(r rune) (rune, bool) {
	switch r {
	case 'Á', 'À', 'Â', 'Ã', 'Ä':
		return 'A', true
	case 'É', 'È', 'Ê', 'Ë':
		return 'E', true
	case 'Í', 'Ì', 'Î', 'Ï':
		return 'I', true
	case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö':
		return 'O', true
	case 'Ú', 'Ù', 'Û', 'Ü':
		return 'U', true
	case 'Ç':
		return 'C', true
	case 'Ñ':
		return 'N', true
	}
	return r, false
}
