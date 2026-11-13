package comms

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// When the second attachment cannot be saved, the message and the attachment that
// already went in are both rolled back, in the database and in the cache.
func TestCov1xDBSendMessageRollsBackEarlierAttachments(t *testing.T) {
	e := cov1xNewEnv(t)
	cov1xFailWhen(t, e.pool, "badfile", "message_attachments", "INSERT", `NEW.filename = 'bad.png'`)
	_, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: "with files", SenderUserID: uuid.NewString(), SenderRole: "coach",
		Attachments: []AttachmentInput{
			{ObjectKey: "k/good.png", Filename: "good.png", MimeType: "image/png", SizeBytes: 1},
			{ObjectKey: "k/bad.png", Filename: "bad.png", MimeType: "image/png", SizeBytes: 1},
		}})
	cov1xWantErr(t, err, "persist message attachment")
	if n := e.count(t, `SELECT (SELECT count(*) FROM messages WHERE org_id = $1::uuid) + (SELECT count(*) FROM message_attachments WHERE org_id = $1::uuid)`, e.org); n != 0 {
		t.Fatalf("%d rows survived the rollback", n)
	}
	if len(e.svc.messages) != 0 || len(e.svc.messageAttachments) != 0 {
		t.Fatalf("cache kept messages=%d attachments=%d", len(e.svc.messages), len(e.svc.messageAttachments))
	}
	// A retry with valid attachments then succeeds without duplicating anything.
	if _, err := e.svc.SendMessage(CreateMessageInput{OrgID: e.org, HouseholdID: e.hh, SwimmerID: e.sw, Body: "retry", SenderUserID: uuid.NewString(), SenderRole: "coach",
		Attachments: []AttachmentInput{{ObjectKey: "k/good.png", Filename: "good.png", MimeType: "image/png", SizeBytes: 1}}}); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM messages WHERE org_id = $1::uuid`, e.org); n != 1 {
		t.Fatalf("messages = %d", n)
	}
}

// Child-table loaders accept an empty club to read every club's rows at once.
func TestCov1xDBUnscopedChildLoadersReadEveryClub(t *testing.T) {
	ctx := context.Background()
	e := cov1xNewEnv(t)
	other := e.sibling(t)
	user := uuid.NewString()
	for _, env := range []*cov1xEnv{e, other} {
		m := env.send(t, user, "hello", AttachmentInput{ObjectKey: "k/a.png", Filename: "a.png", MimeType: "image/png", SizeBytes: 1})
		if err := env.svc.AddReaction(env.org, m.ID, user, "👍"); err != nil {
			t.Fatal(err)
		}
		if _, err := env.svc.EditMessage(env.org, m.ID, user, "hello again"); err != nil {
			t.Fatal(err)
		}
	}
	edits, err := e.svc.repo.LoadMessageEdits(ctx, "")
	if err != nil || len(edits) != 2 {
		t.Fatalf("edits = %d err=%v", len(edits), err)
	}
	reactions, err := e.svc.repo.LoadMessageReactions(ctx, "")
	if err != nil || len(reactions) != 2 {
		t.Fatalf("reactions = %d err=%v", len(reactions), err)
	}
	atts, err := e.svc.repo.LoadMessageAttachments(ctx, "")
	if err != nil || len(atts) != 2 {
		t.Fatalf("attachments = %d err=%v", len(atts), err)
	}
	// Scoped to one club, only that club's rows come back.
	scoped, err := e.svc.repo.LoadMessageEdits(ctx, e.org)
	if err != nil || len(scoped) != 1 {
		t.Fatalf("scoped edits = %d err=%v", len(scoped), err)
	}
}
