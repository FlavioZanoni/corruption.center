package psql

import (
	"context"
	"strings"
	"testing"
)

// TestResolveRemovalRequest_RejectsInvalidStatus verifies the state-machine
// guard: only 'resolved' and 'rejected' are terminal states. The invalid-status
// branch returns before touching the connection, so a zero-value DB is fine.
func TestResolveRemovalRequest_RejectsInvalidStatus(t *testing.T) {
	db := &DB{}
	for _, bad := range []string{"pending", "purged", "", "deleted"} {
		err := db.ResolveRemovalRequest(context.Background(), "00000000-0000-0000-0000-000000000000", bad, "note", "op")
		if err == nil {
			t.Fatalf("status %q: expected error, got nil", bad)
		}
		if !strings.Contains(err.Error(), "invalid removal request resolution status") {
			t.Fatalf("status %q: unexpected error: %v", bad, err)
		}
	}
}

// TestCreateRemovalRequest_RequiresFields verifies required-field validation
// runs before any DB access.
func TestCreateRemovalRequest_RequiresFields(t *testing.T) {
	db := &DB{}
	cases := [][3]string{
		{"", "Person", "id1"},
		{"req", "", "id1"},
		{"req", "Person", ""},
	}
	for _, tc := range cases {
		_, err := db.CreateRemovalRequest(context.Background(), tc[0], tc[1], tc[2], "reason")
		if err == nil {
			t.Fatalf("inputs %v: expected validation error, got nil", tc)
		}
	}
}
