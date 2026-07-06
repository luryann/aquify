package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

func newMessagesRouter(t *testing.T, svc *comms.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	originalNewObjectStorageClient := newObjectStorageClient
	newObjectStorageClient = func(_ storage.Config) (storage.ObjectStorage, error) {
		return storage.NewStubStorage(""), nil
	}
	t.Cleanup(func() {
		newObjectStorageClient = originalNewObjectStorageClient
	})
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil,
		svc,
		nil, nil, nil, nil, nil, nil)
}

func issueMessagesToken(t *testing.T, router http.Handler, userID, orgID, role, householdID string) string {
	t.Helper()
	payload := map[string]any{
		"user_id": userID,
		"org_id":  orgID,
		"roles":   []string{role},
	}
	if householdID != "" {
		payload["household_id"] = householdID
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueMessagesToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func doMessagesReq(router http.Handler, method, path, token, orgID, body string) *httptest.ResponseRecorder {
	var b *bytes.Reader
	if body != "" {
		b = bytes.NewReader([]byte(body))
	} else {
		b = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, b)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestMessagesEditForbiddenForNonSender(t *testing.T) {
	svc := comms.NewService()
	router := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")
	otherCoachToken := issueMessagesToken(t, router, "coach-2", orgID, "coach", "")

	sendResp := doMessagesReq(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "hh-1", "swimmer_id": "sw-1", "body": "hello"
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

	editResp := doMessagesReq(router, http.MethodPatch, "/api/v1/messages/"+sendBody.Message.ID, otherCoachToken, orgID, `{
		"body": "edited by someone else"
	}`)
	if editResp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 editing another user's message, got %d: %s", editResp.Code, editResp.Body.String())
	}

	okEditResp := doMessagesReq(router, http.MethodPatch, "/api/v1/messages/"+sendBody.Message.ID, coachToken, orgID, `{
		"body": "edited by sender"
	}`)
	if okEditResp.Code != http.StatusOK {
		t.Fatalf("expected 200 editing own message, got %d: %s", okEditResp.Code, okEditResp.Body.String())
	}
}

func TestMessagesReactionRejectsDisallowedEmoji(t *testing.T) {
	svc := comms.NewService()
	router := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	sendResp := doMessagesReq(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "hh-1", "swimmer_id": "sw-1", "body": "hello"
	}`)
	var sendBody struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	_ = json.Unmarshal(sendResp.Body.Bytes(), &sendBody)

	badReaction := doMessagesReq(router, http.MethodPost, "/api/v1/messages/"+sendBody.Message.ID+"/reactions", coachToken, orgID, `{"emoji": "🔥"}`)
	if badReaction.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for disallowed emoji, got %d: %s", badReaction.Code, badReaction.Body.String())
	}

	goodReaction := doMessagesReq(router, http.MethodPost, "/api/v1/messages/"+sendBody.Message.ID+"/reactions", coachToken, orgID, `{"emoji": "👍"}`)
	if goodReaction.Code != http.StatusCreated {
		t.Fatalf("expected 201 for allowed emoji, got %d: %s", goodReaction.Code, goodReaction.Body.String())
	}
}

func TestMessagesAttachmentRejectsMismatchedThreadObjectKey(t *testing.T) {
	svc := comms.NewService()
	router := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	// upload-url for hh-1/sw-1
	uploadResp := doMessagesReq(router, http.MethodPost, "/api/v1/messages/upload-url", coachToken, orgID, `{
		"household_id": "hh-1", "swimmer_id": "sw-1",
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
	sendResp := doMessagesReq(router, http.MethodPost, "/api/v1/messages", coachToken, orgID, `{
		"household_id": "hh-2", "swimmer_id": "sw-2", "body": "sneaky",
		"attachments": [{"object_key": "`+uploadBody.ObjectKey+`", "file_name": "photo.png", "file_mime": "image/png"}]
	}`)
	if sendResp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cross-thread object key, got %d: %s", sendResp.Code, sendResp.Body.String())
	}
}

func TestMessagesStreamUnavailableWithoutBroadcaster(t *testing.T) {
	svc := comms.NewService()
	router := newMessagesRouter(t, svc)
	orgID := "org-1"
	coachToken := issueMessagesToken(t, router, "coach-1", orgID, "coach", "")

	resp := doMessagesReq(router, http.MethodGet, "/api/v1/messages/stream", coachToken, orgID, "")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no broadcaster configured, got %d: %s", resp.Code, resp.Body.String())
	}
}
