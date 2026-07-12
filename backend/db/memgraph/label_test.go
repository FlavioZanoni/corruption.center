package memgraph

import "testing"

func TestFormatCNJ(t *testing.T) {
	// The real Lava Jato case on the graph.
	if got, want := formatCNJ("50833760520144047000"), "5083376-05.2014.4.04.7000"; got != want {
		t.Errorf("formatCNJ = %q, want %q", got, want)
	}
	// Already formatted: the digits are extracted and it round-trips.
	if got, want := formatCNJ("5083376-05.2014.4.04.7000"), "5083376-05.2014.4.04.7000"; got != want {
		t.Errorf("formatCNJ(formatted) = %q, want %q", got, want)
	}
	// Not 20 digits: left visibly wrong rather than reshaped into something that
	// looks like a real case number.
	for _, bad := range []string{"", "123", "abc", "508337605201440470001"} {
		if got := formatCNJ(bad); got != bad {
			t.Errorf("formatCNJ(%q) = %q, want it returned unchanged", bad, got)
		}
	}
}

func TestProceedingLabel(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
		want  string
	}{
		{
			name:  "class name once DataJud has polled the case",
			props: map[string]any{"case_number": "50833760520144047000", "class_name": "Ação Penal", "court": "TRF4"},
			want:  "Ação Penal · 5083376-05.2014.4.04.7000",
		},
		{
			// The state of nearly every case today: DataJud has not polled it, so
			// there is no class and no court. The formatted number is still a real
			// answer, and the one a lawyer would recognise.
			name:  "number alone when DataJud has not polled it",
			props: map[string]any{"case_number": "50833760520144047000"},
			want:  "5083376-05.2014.4.04.7000",
		},
		{
			// The class code alone ("283") is not a label, it is a TPU number, and
			// three of our four cases share it. The number must survive.
			name:  "raw class code is not used as a label",
			props: map[string]any{"case_number": "50833760520144047000", "type": "283", "court": "TRF4"},
			want:  "5083376-05.2014.4.04.7000",
		},
		{
			name:  "no case number at all: caller falls back",
			props: map[string]any{},
			want:  "",
		},
	}
	for _, c := range cases {
		if got := proceedingLabel(c.props); got != c.want {
			t.Errorf("%s: proceedingLabel = %q, want %q", c.name, got, c.want)
		}
	}
}

// Three of the four real cases are all "Ação Penal" at the 13ª Vara Federal de
// Curitiba. A label built from class and court alone renders them as three
// identical nodes, which is how the first attempt at this broke.
func TestProceedingLabel_CasesSharingAClassStayDistinguishable(t *testing.T) {
	shared := func(number string) map[string]any {
		return map[string]any{
			"case_number": number,
			"class_name":  "Ação Penal - Procedimento Ordinário",
			"court":       "13ª Vara Federal de Curitiba",
		}
	}
	seen := map[string]bool{}
	for _, number := range []string{
		"50833760520144047000",
		"50465129420164047000",
		"50213653220174047000",
	} {
		label := proceedingLabel(shared(number))
		if seen[label] {
			t.Fatalf("two different cases share the label %q: they render as the same node", label)
		}
		seen[label] = true
	}
}
