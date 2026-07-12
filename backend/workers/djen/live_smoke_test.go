package djen

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live check against the real DJEN. Skipped unless DJEN_LIVE=1.
//
// The fake-server unit tests cannot catch a contract mismatch with the real API,
// and every DJEN bug we have had was exactly that: the missing date range, the
// substring matching, the unservable records. None of them would fail a test
// against a server we wrote ourselves.
func TestLive_SearchByPartyName(t *testing.T) {
	if os.Getenv("DJEN_LIVE") != "1" {
		t.Skip("set DJEN_LIVE=1 to hit the real DJEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := NewClient("")

	// Lula's 2025 results contain a record DJEN cannot serve (index 36), so this
	// exercises the step-over path against the live API that produced it.
	for _, name := range []string{"LUIZ INACIO LULA DA SILVA", "IZALCI LUCAS FERREIRA"} {
		items, err := c.SearchByPartyName(ctx, name, 300)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		t.Logf("%s -> %d items (%d records DJEN refused)", name, len(items), c.SkippedRecords())
		if len(items) == 0 {
			t.Errorf("%s: expected hits, got none", name)
		}
	}
}
