package audit

import "testing"

func TestQueryActionPrefixAppliesBeforeLimit(t *testing.T) {
	t.Parallel()
	s := NewService()
	add := func(action string) {
		if _, err := s.Append(AppendRequest{OrgID: "org-1", ActorUserID: "u", Action: action, ResourceType: "x", ResourceID: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	add("platform.user.disable")
	for i := 0; i < 10; i++ {
		add("billing.invoice.issue")
	}
	add("Platform.Org.Update") // prefix match ignores case, like Action does
	got, err := s.Query(Filter{OrgID: "org-1", ActionPrefix: "platform.", Limit: 2})
	if err != nil || len(got) != 2 {
		t.Fatalf("want both platform events within limit 2, got %d (%v)", len(got), err)
	}
	for _, ev := range got {
		if len(ev.Action) < 9 {
			t.Fatalf("unexpected event %q", ev.Action)
		}
	}
}
