package comms

import "testing"

func TestRuntimeSnapshotRoundTripKeepsMessageData(t *testing.T) {
	src := NewService()
	msg, err := src.SendMessage(CreateMessageInput{OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1", Body: "hi", SenderUserID: "coach-1", SenderRole: "coach"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.EditMessage("org-1", msg.ID, "coach-1", "hi again"); err != nil {
		t.Fatal(err)
	}
	for emoji := range AllowedMessageReactions {
		if err := src.AddReaction("org-1", msg.ID, "coach-1", emoji); err != nil {
			t.Fatal(err)
		}
		break
	}
	data, err := src.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	dst := NewService()
	if err := dst.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if len(dst.messages) != 1 || len(dst.messageEdits) != 1 || len(dst.messageReactions) != 1 {
		t.Fatalf("round trip dropped chat data: messages=%d edits=%d reactions=%d", len(dst.messages), len(dst.messageEdits), len(dst.messageReactions))
	}
	// An older snapshot without chat fields must still leave writable maps.
	old := NewService()
	if err := old.LoadRuntimeSnapshot([]byte(`{"announcements":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := old.SendMessage(CreateMessageInput{OrgID: "org-1", HouseholdID: "hh-1", SwimmerID: "sw-1", Body: "x", SenderUserID: "u", SenderRole: "coach"}); err != nil {
		t.Fatalf("send after old snapshot load: %v", err)
	}
}
