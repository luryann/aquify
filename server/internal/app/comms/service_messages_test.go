package comms

import (
	"testing"
	"time"
)

func TestEditMessageIsSenderOnly(t *testing.T) {
	s := NewService()
	msg, err := s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "hello", SenderUserID: "coach-1", SenderRole: "coach",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if _, err := s.EditMessage("org-1", msg.ID, "someone-else", "hacked"); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-sender edit, got %v", err)
	}

	edited, err := s.EditMessage("org-1", msg.ID, "coach-1", "hello again")
	if err != nil {
		t.Fatalf("sender edit failed: %v", err)
	}
	if edited.Body != "hello again" {
		t.Fatalf("expected updated body, got %q", edited.Body)
	}
	if edited.EditedAt == nil {
		t.Fatal("expected EditedAt to be set after edit")
	}

	if _, err := s.EditMessage("org-1", "does-not-exist", "coach-1", "x"); err != ErrMessageNotFound {
		t.Fatalf("expected ErrMessageNotFound, got %v", err)
	}
}

func TestReactionsAllowlistAndToggle(t *testing.T) {
	s := NewService()
	msg, _ := s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "hi", SenderUserID: "coach-1", SenderRole: "coach",
	})

	if err := s.AddReaction("org-1", msg.ID, "parent-1", "🔥"); err != ErrInvalidReaction {
		t.Fatalf("expected ErrInvalidReaction for disallowed emoji, got %v", err)
	}

	if err := s.AddReaction("org-1", msg.ID, "parent-1", "👍"); err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	// Adding the same reaction twice must be a no-op, not an error or duplicate.
	if err := s.AddReaction("org-1", msg.ID, "parent-1", "👍"); err != nil {
		t.Fatalf("AddReaction (repeat): %v", err)
	}

	views := s.ListMessages("org-1", "hh-1", "sw-1", "parent-1")
	if len(views) != 1 || len(views[0].Reactions) != 1 {
		t.Fatalf("expected exactly one aggregated reaction, got %+v", views)
	}
	if views[0].Reactions[0].Count != 1 || !views[0].Reactions[0].ReactedByMe {
		t.Fatalf("expected count=1 reacted_by_me=true, got %+v", views[0].Reactions[0])
	}

	// A different viewer should see the same reaction but reacted_by_me=false.
	otherViews := s.ListMessages("org-1", "hh-1", "sw-1", "coach-1")
	if otherViews[0].Reactions[0].ReactedByMe {
		t.Fatal("expected reacted_by_me=false for a viewer who did not react")
	}

	if err := s.RemoveReaction("org-1", msg.ID, "parent-1", "👍"); err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	views = s.ListMessages("org-1", "hh-1", "sw-1", "parent-1")
	if len(views[0].Reactions) != 0 {
		t.Fatalf("expected no reactions after removal, got %+v", views[0].Reactions)
	}
}

func TestUnreadCountTracksReadWatermark(t *testing.T) {
	s := NewService()
	_, _ = s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "first", SenderUserID: "parent-1", SenderRole: "parent",
	})
	_, _ = s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "second", SenderUserID: "parent-1", SenderRole: "parent",
	})

	threads := s.ListMessageThreads("org-1", "coach-1")
	if len(threads) != 1 || threads[0].UnreadCount != 2 {
		t.Fatalf("expected 2 unread for coach who hasn't read yet, got %+v", threads)
	}

	// The sender's own view of the thread should never count their own messages as unread.
	selfThreads := s.ListMessageThreads("org-1", "parent-1")
	if selfThreads[0].UnreadCount != 0 {
		t.Fatalf("expected 0 unread for the sender's own thread view, got %d", selfThreads[0].UnreadCount)
	}

	if err := s.MarkThreadRead("org-1", "hh-1", "sw-1", "coach-1"); err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	threads = s.ListMessageThreads("org-1", "coach-1")
	if threads[0].UnreadCount != 0 {
		t.Fatalf("expected 0 unread after marking read, got %d", threads[0].UnreadCount)
	}

	// A new message after the watermark should count again. Sleep briefly so its
	// CreatedAt is unambiguously after the read watermark set just above —
	// otherwise both timestamps can land in the same clock tick.
	time.Sleep(2 * time.Millisecond)
	_, _ = s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "third", SenderUserID: "parent-1", SenderRole: "parent",
	})
	threads = s.ListMessageThreads("org-1", "coach-1")
	if threads[0].UnreadCount != 1 {
		t.Fatalf("expected 1 unread after a new message post-watermark, got %d", threads[0].UnreadCount)
	}
}

func TestMessageThreadLookup(t *testing.T) {
	s := NewService()
	msg, _ := s.SendMessage(CreateMessageInput{
		OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1",
		Body: "hi", SenderUserID: "coach-1", SenderRole: "coach",
	})

	hh, sw, ok := s.MessageThread("org-1", msg.ID)
	if !ok || hh != "hh-1" || sw != "sw-1" {
		t.Fatalf("expected (hh-1, sw-1, true), got (%s, %s, %v)", hh, sw, ok)
	}

	if _, _, ok := s.MessageThread("org-2", msg.ID); ok {
		t.Fatal("expected lookup to fail for a mismatched org")
	}
	if _, _, ok := s.MessageThread("org-1", "missing"); ok {
		t.Fatal("expected lookup to fail for a missing message")
	}
}
