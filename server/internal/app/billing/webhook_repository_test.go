package billing

import (
	"context"
	"testing"
)

func TestLoadWebhookEventsNonUUIDOrgReturnsNothing(t *testing.T) {
	// A nil pool would panic if the query ran: a non-UUID org must short-circuit.
	r := &Repository{}
	got, err := r.loadWebhookEvents(context.Background(), "org-demo", 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty", got, err)
	}
}
