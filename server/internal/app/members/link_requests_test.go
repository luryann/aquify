package members

import (
	"testing"
	"time"
)

func TestListPendingLinkRequestsIsOldestFirst(t *testing.T) {
	t.Parallel()
	svc := NewService()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"c", "a", "e", "b", "d"} {
		svc.linkRequests[id] = SwimmerLinkRequest{ID: id, OrgID: "org", Status: "pending", CreatedAt: base.Add(time.Duration(i) * time.Minute)}
	}
	got, err := svc.ListPendingLinkRequests("org")
	if err != nil || len(got) != 5 {
		t.Fatalf("got %d err %v", len(got), err)
	}
	for i, id := range []string{"c", "a", "e", "b", "d"} {
		if got[i].ID != id {
			t.Fatalf("position %d = %s, want %s (oldest first)", i, got[i].ID, id)
		}
	}
}
