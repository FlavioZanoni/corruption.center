package matching

import "testing"

func TestScore_AutoLinkRequiresADocument(t *testing.T) {
	cases := []struct {
		name     string
		ev       Evidence
		wantAuto bool
	}{
		{
			name:     "full document alone is enough",
			ev:       Evidence{FullDocument: true},
			wantAuto: true,
		},
		{
			name: "masked cpf + exact long name",
			ev: Evidence{
				MaskedCPF:   true,
				SourceName:  "JOSE ADELMARIO PINHEIRO FILHO",
				SubjectName: "José Adelmário Pinheiro Filho",
			},
			wantAuto: true,
		},
		{
			name: "masked cpf but a different person's name",
			ev: Evidence{
				MaskedCPF:   true,
				SourceName:  "MARIA DE FATIMA PEREIRA",
				SubjectName: "JOAO DA SILVA",
			},
			wantAuto: false,
		},
		{
			// The whole point: a name is a lead, never an identification.
			name: "exact name with no document never auto-links",
			ev: Evidence{
				SourceName:  "JOSE CARLOS DA SILVA SANTOS",
				SubjectName: "JOSE CARLOS DA SILVA SANTOS",
			},
			wantAuto: false,
		},
		{
			name: "strong evidence that fits two politicians identifies neither",
			ev: Evidence{
				MaskedCPF:   true,
				SourceName:  "JOSE DA SILVA",
				SubjectName: "JOSE DA SILVA",
				Candidates:  2,
			},
			wantAuto: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auto, score, signals := AutoLink(tc.ev)
			if auto != tc.wantAuto {
				t.Fatalf("AutoLink = %v (score %.2f, signals %v), want %v", auto, score, signals, tc.wantAuto)
			}
			if score < 0 || score > 1 {
				t.Fatalf("score %.2f out of range", score)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  José   da Conceição  "); got != "JOSE DA CONCEICAO" {
		t.Fatalf("NormalizeName = %q", got)
	}
	if !NamesMatch("Antônio Palocci Filho", "ANTONIO PALOCCI FILHO") {
		t.Fatal("expected accent-insensitive match")
	}
	if NamesMatch("JOSE SILVA", "ANTONIO JOSE SILVA VIANA") {
		t.Fatal("substring must not count as a name match")
	}
}
