package memgraph

import (
	"fmt"
	"strings"
)

// proceedingLabel turns a LegalProceeding into something a reader can place.
//
// A case used to be labelled with its bare case number — "50833760520144047000".
// It is the intermediate hop between a scandal and the people in it, so on the
// canvas the user clicks a scandal and is shown a ring of twenty raw digits: the
// step that carries the whole meaning of the graph reads as noise, and looks more
// like a bug than a link.
//
// Returns "" when the node has no case number, so the caller can fall back.
func proceedingLabel(props map[string]any) string {
	number := formatCNJ(strProp(props, "case_number"))
	if number == "" {
		return ""
	}

	// The case number ALWAYS stays in the label. Three of our four cases share a
	// class and a court — all three are "Ação Penal" at the 13ª Vara Federal de
	// Curitiba — so a label built from those alone renders three different
	// prosecutions as three identical nodes. Naming a thing after its category
	// makes it unfindable.
	//
	// class_name comes from DataJud and is absent until it has polled the case,
	// which today is nearly all of them. The formatted number alone is still a real
	// answer, and the one a lawyer would recognise.
	if class := strProp(props, "class_name"); class != "" {
		return fmt.Sprintf("%s · %s", class, number)
	}
	return number
}

// formatCNJ renders a 20-digit case number in the form every Brazilian court,
// lawyer and journalist writes it: NNNNNNN-DD.AAAA.J.TR.OOOO (Resolução CNJ
// 65/2008). Anything that is not 20 digits is returned unchanged — a malformed
// number should stay visibly malformed rather than be silently reshaped into
// something that looks official.
func formatCNJ(s string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if len(digits) != 20 {
		return s
	}
	return fmt.Sprintf("%s-%s.%s.%s.%s.%s",
		digits[0:7],   // NNNNNNN sequential
		digits[7:9],   // DD      check digits
		digits[9:13],  // AAAA    year
		digits[13:14], // J       judicial branch
		digits[14:16], // TR      court
		digits[16:20], // OOOO    originating unit
	)
}
