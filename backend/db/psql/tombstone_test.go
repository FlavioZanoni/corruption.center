package psql

import "testing"

// The whole point of the aggressive fold: a person purged under one spelling of
// their name cannot slip back in under an interchangeable one on the next sync.
func TestTombstoneKeyName_FoldsInterchangeableSpellings(t *testing.T) {
	same := [][2]string{
		{"JOSÉ DA SILVA JUNIOR", "jose da silva jr."}, // suffix + accents + case + punct
		{"MARIA-JOSÉ SOUZA", "maria jose souza"},      // hyphen vs space
		{"João  Neto", "JOAO NETO"},                   // extra whitespace + accents
	}
	for _, p := range same {
		a, b := TombstoneKeyName(p[0]), TombstoneKeyName(p[1])
		if a != b {
			t.Errorf("expected same key for %q and %q, got %q vs %q", p[0], p[1], a, b)
		}
	}

	// Distinct people must NOT collapse just because the fold is aggressive.
	if TombstoneKeyName("JOSE DA SILVA") == TombstoneKeyName("JOSE DA COSTA") {
		t.Error("distinct names collapsed to the same tombstone key")
	}
}
