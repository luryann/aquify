package audit

import (
	"testing"
	"time"
)

// Events sharing a timestamp must page in the same total order Postgres uses
// (occurred_at DESC, id DESC); otherwise cursor pagination can skip or repeat.
func TestQueryInMemoryCursorOrderIsDeterministicOnTimestampTies(t *testing.T) {
	t.Parallel()
	s := NewService()
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s.events = []AuditEvent{
		{ID: "a", OrgID: "o", OccurredAt: at},
		{ID: "c", OrgID: "o", OccurredAt: at},
		{ID: "b", OrgID: "o", OccurredAt: at},
	}
	var got []string
	cursor := ""
	for i := 0; i < 3; i++ {
		page, err := s.Query(Filter{OrgID: "o", Limit: 1, Cursor: cursor})
		if err != nil || len(page) != 1 {
			t.Fatalf("page %d: %v %+v", i, err, page)
		}
		got = append(got, page[0].ID)
		cursor = page[0].ID
	}
	if got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Fatalf("order = %v, want [c b a]", got)
	}
}
