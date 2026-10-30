package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

// messageThreads are two real org-1 household/swimmer pairs; staff may only
// message threads that exist on the org roster.
type messageThreads struct{ HH1, SW1, HH2, SW2 string }

func newMessagesRouter(t *testing.T, svc *comms.Service) (http.Handler, messageThreads) {
	t.Helper()
	membersSvc := members.NewService()
	admin := &appmiddleware.Principal{UserID: "seed-admin", OrgID: "org-1", Roles: []string{"admin"}}
	var th messageThreads
	seed := func(name string) (string, string) {
		hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: "org-1", HouseholdName: name})
		if err != nil {
			t.Fatalf("seed household: %v", err)
		}
		sw, _, err := membersSvc.CreateSwimmer(admin, "org-1", members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: name, LastName: "S", DateOfBirth: "2012-01-01", Gender: "female", RegistrationStatus: "active"})
		if err != nil {
			t.Fatalf("seed swimmer: %v", err)
		}
		return hh.ID, sw.ID
	}
	th.HH1, th.SW1 = seed("One")
	th.HH2, th.SW2 = seed("Two")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	originalNewObjectStorageClient := newObjectStorageClient
	newObjectStorageClient = func(_ storage.Config) (storage.ObjectStorage, error) {
		return storage.NewStubStorage(""), nil
	}
	t.Cleanup(func() {
		newObjectStorageClient = originalNewObjectStorageClient
	})
	return NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, RouterDeps{
		Comms:   svc,
		Members: membersSvc,
	}), th
}

func issueMessagesToken(t *testing.T, router http.Handler, userID, orgID, role, householdID string) string {
	t.Helper()
	return issueSession(t, router, sessionSpec{UserID: userID, OrgID: orgID, HouseholdID: householdID, Roles: []string{role}})
}

func TestMessagesEditForbiddenForNonSender(t *testing.T) {
	svc := comms.NewService()
	router, th := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")
	otherCoachToken := issueMessagesToken(t, router, "coach-2", orgID, "coach", "")

	sendResp := doOrgRequest(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "`+th.HH1+`", "swimmer_id": "`+th.SW1+`", "body": "hello"
	}`)
	if sendResp.Code != http.StatusCreated {
		t.Fatalf("send message: want 201, got %d: %s", sendResp.Code, sendResp.Body.String())
	}
	var sendBody struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(sendResp.Body.Bytes(), &sendBody); err != nil {
		t.Fatalf("decode send response: %v", err)
	}

	editResp := doOrgRequest(router, http.MethodPatch, "/api/v1/messages/"+sendBody.Message.ID, otherCoachToken, orgID, `{
		"body": "edited by someone else"
	}`)
	if editResp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 editing another user's message, got %d: %s", editResp.Code, editResp.Body.String())
	}

	okEditResp := doOrgRequest(router, http.MethodPatch, "/api/v1/messages/"+sendBody.Message.ID, coachToken, orgID, `{
		"body": "edited by sender"
	}`)
	if okEditResp.Code != http.StatusOK {
		t.Fatalf("expected 200 editing own message, got %d: %s", okEditResp.Code, okEditResp.Body.String())
	}
}

func TestMessagesReactionRejectsDisallowedEmoji(t *testing.T) {
	svc := comms.NewService()
	router, th := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	sendResp := doOrgRequest(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "`+th.HH1+`", "swimmer_id": "`+th.SW1+`", "body": "hello"
	}`)
	var sendBody struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	_ = json.Unmarshal(sendResp.Body.Bytes(), &sendBody)

	badReaction := doOrgRequest(router, http.MethodPost, "/api/v1/messages/"+sendBody.Message.ID+"/reactions", coachToken, orgID, `{"emoji": "🔥"}`)
	if badReaction.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for disallowed emoji, got %d: %s", badReaction.Code, badReaction.Body.String())
	}

	goodReaction := doOrgRequest(router, http.MethodPost, "/api/v1/messages/"+sendBody.Message.ID+"/reactions", coachToken, orgID, `{"emoji": "👍"}`)
	if goodReaction.Code != http.StatusCreated {
		t.Fatalf("expected 201 for allowed emoji, got %d: %s", goodReaction.Code, goodReaction.Body.String())
	}
}

func TestMessagesAttachmentRejectsMismatchedThreadObjectKey(t *testing.T) {
	svc := comms.NewService()
	router, th := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	// upload-url for hh-1/sw-1
	uploadResp := doOrgRequest(router, http.MethodPost, "/api/v1/messages/upload-url", coachToken, orgID, `{
		"household_id": "`+th.HH1+`", "swimmer_id": "`+th.SW1+`",
		"file_name": "photo.png", "file_mime": "image/png", "file_size_bytes": 1024
	}`)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("upload-url: want 201, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}
	var uploadBody struct {
		ObjectKey string `json:"object_key"`
	}
	_ = json.Unmarshal(uploadResp.Body.Bytes(), &uploadBody)

	// Try to send that object key into a *different* thread (hh-2/sw-2) — must be rejected.
	sendResp := doOrgRequest(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "`+th.HH2+`", "swimmer_id": "`+th.SW2+`", "body": "sneaky",
		"attachments": [{"object_key": "`+uploadBody.ObjectKey+`", "file_name": "photo.png", "file_mime": "image/png"}]
	}`)
	if sendResp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cross-thread object key, got %d: %s", sendResp.Code, sendResp.Body.String())
	}
}

func TestMessagesStreamUnavailableWithoutBroadcaster(t *testing.T) {
	svc := comms.NewService()
	router, _ := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	resp := doOrgRequest(router, http.MethodGet, "/api/v1/messages/stream", coachToken, orgID, "")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no broadcaster configured, got %d: %s", resp.Code, resp.Body.String())
	}
}
