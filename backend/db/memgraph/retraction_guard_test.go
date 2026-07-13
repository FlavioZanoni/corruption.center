package memgraph

import "testing"

// guardVerdict is the pure decision the sweep makes before it writes anything:
// given held/missing and a guard, does it refuse? Extracted so it can be tested
// without a live graph, because it is the line between "the state withdrew a
// sanction" and "our fetch broke" — the most consequential branch in the file.
func guardVerdict(held, missing int, g RetractionGuard) (refuse bool) {
	if held < g.MinRecords {
		return true
	}
	if float64(missing)/float64(held) > g.MaxFraction {
		return true
	}
	return false
}

func TestRetractionGuard(t *testing.T) {
	g := DefaultRetractionGuard // 5%, min 100
	cases := []struct {
		name           string
		held, missing  int
		wantRefuse     bool
	}{
		{"trickle retraction is allowed", 1000, 20, false},         // 2%
		{"exactly at the ceiling is allowed", 1000, 50, false},     // 5%
		{"over the ceiling refuses (truncated fetch)", 1000, 60, true}, // 6%
		{"a mass wipe refuses hard", 64779, 40000, true},
		{"too few records refuses", 50, 1, true},                   // under MinRecords
		{"nothing missing, nothing refused", 1000, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := guardVerdict(c.held, c.missing, g); got != c.wantRefuse {
				t.Fatalf("held=%d missing=%d: refuse=%v, want %v", c.held, c.missing, got, c.wantRefuse)
			}
		})
	}
}
