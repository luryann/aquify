package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
)

// ─── Router helper ────────────────────────────────────────────────────────────

func newCommsRouter(t *testing.T, svc *comms.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil,
		nil,
	)
}

// issueCommsToken creates a real session token with the given role for org-demo.
func issueCommsToken(t *testing.T, router http.Handler, role string) string {
	t.Helper()
	payload := map[string]any{
		"user_id": "test-user-" + role,
		"org_id":  "org-demo",
		"roles":   []string{role},
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issueCommsToken: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const commsOrgID = "org-demo"

func commsBody(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

func doCommsReq(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("{}")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Org-Id", commsOrgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func doPublicCommsReq(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// ─── Announcement tests ───────────────────────────────────────────────────────

func TestAnnouncements_CreateAndList(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Create announcement.
	body := commsBody(map[string]any{
		"title":    "Welcome back!",
		"body":     "Season starts Monday.",
		"audience": "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/announcements", token, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create announcement: want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["id"] == nil || created["id"] == "" {
		t.Fatal("expected non-empty id")
	}
	if created["status"] != "draft" {
		t.Fatalf("expected status=draft, got %v", created["status"])
	}

	// List announcements.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/announcements", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list announcements: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var listResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	anns, ok := listResp["announcements"].([]any)
	if !ok || len(anns) != 1 {
		t.Fatalf("expected 1 announcement, got %v", listResp["announcements"])
	}
}

func TestAnnouncements_PublishAndArchive(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Create.
	body := commsBody(map[string]any{
		"title":    "Test",
		"body":     "Body",
		"audience": "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/announcements", token, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var ann map[string]any
	json.NewDecoder(rr.Body).Decode(&ann)
	id := ann["id"].(string)

	// Publish.
	rr = doCommsReq(router, http.MethodPost, "/api/v1/announcements/"+id+"/publish", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("publish: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var published map[string]any
	json.NewDecoder(rr.Body).Decode(&published)
	if published["status"] != "published" {
		t.Fatalf("want status=published, got %v", published["status"])
	}

	// Publish again should fail (invalid transition).
	rr = doCommsReq(router, http.MethodPost, "/api/v1/announcements/"+id+"/publish", token, "")
	if rr.Code != http.StatusConflict {
		t.Fatalf("re-publish: want 409, got %d", rr.Code)
	}

	// Archive.
	rr = doCommsReq(router, http.MethodPost, "/api/v1/announcements/"+id+"/archive", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("archive: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var archived map[string]any
	json.NewDecoder(rr.Body).Decode(&archived)
	if archived["status"] != "archived" {
		t.Fatalf("want status=archived, got %v", archived["status"])
	}
}

func TestAnnouncements_NotFound(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	rr := doCommsReq(router, http.MethodGet, "/api/v1/announcements/nonexistent", token, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestAnnouncements_RBACEnforcement(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	parentToken := issueCommsToken(t, router, "parent")

	// Parent cannot create announcements.
	body := commsBody(map[string]any{"title": "x", "body": "y", "audience": "all"})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/announcements", parentToken, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent create: want 403, got %d", rr.Code)
	}
}

func TestAnnouncements_GroupAudienceRequiresGroupID(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	body := commsBody(map[string]any{
		"title":    "Test",
		"body":     "Body",
		"audience": "group",
		// group_id intentionally omitted
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/announcements", token, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("group audience without group_id: want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Campaign tests ───────────────────────────────────────────────────────────

func TestCampaigns_CreateListGet(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Create campaign.
	body := commsBody(map[string]any{
		"name":       "Fall Newsletter",
		"subject":    "Season Update",
		"body_html":  "<p>Hello!</p>",
		"from_name":  "DARE Aquatics",
		"from_email": "noreply@dareaquatics.com",
		"audience":   "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", token, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create campaign: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var campaign map[string]any
	json.NewDecoder(rr.Body).Decode(&campaign)
	if campaign["status"] != "draft" {
		t.Fatalf("want status=draft, got %v", campaign["status"])
	}
	campaignID := campaign["id"].(string)

	// List.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/email-campaigns", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", rr.Code)
	}
	var listResp map[string]any
	json.NewDecoder(rr.Body).Decode(&listResp)
	campaigns := listResp["campaigns"].([]any)
	if len(campaigns) != 1 {
		t.Fatalf("want 1 campaign, got %d", len(campaigns))
	}

	// Get by ID.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/email-campaigns/"+campaignID, token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", rr.Code)
	}
}

func TestCampaigns_SendWithSuppression(t *testing.T) {
	t.Setenv("COMMS_ALLOW_ARBITRARY_RECIPIENTS", "true")
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Add a suppressed email.
	suppBody := commsBody(map[string]any{"email": "bounce@example.com", "reason": "bounce"})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/suppression", token, suppBody)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add suppression: want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Create campaign.
	cBody := commsBody(map[string]any{
		"name":       "Test Send",
		"subject":    "Hello",
		"body_html":  "<p>Hi</p>",
		"from_name":  "DARE",
		"from_email": "noreply@dare.swim",
		"audience":   "all",
	})
	rr = doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", token, cBody)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var campaign map[string]any
	json.NewDecoder(rr.Body).Decode(&campaign)
	campaignID := campaign["id"].(string)

	// Send with 3 recipients — 1 suppressed, 2 live.
	sendBody := commsBody(map[string]any{
		"recipients": []map[string]any{
			{"email": "alice@example.com"},
			{"email": "bob@example.com"},
			{"email": "bounce@example.com"},
		},
	})
	rr = doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns/"+campaignID+"/send", token, sendBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("send: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var sent map[string]any
	json.NewDecoder(rr.Body).Decode(&sent)
	if sent["status"] != "sent" {
		t.Fatalf("want status=sent, got %v", sent["status"])
	}
	if int(sent["sent_count"].(float64)) != 2 {
		t.Fatalf("want sent_count=2, got %v", sent["sent_count"])
	}
	if int(sent["suppressed_count"].(float64)) != 1 {
		t.Fatalf("want suppressed_count=1, got %v", sent["suppressed_count"])
	}
	if int(sent["total_recipients"].(float64)) != 3 {
		t.Fatalf("want total_recipients=3, got %v", sent["total_recipients"])
	}

	// Re-send should fail (not in draft).
	rr = doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns/"+campaignID+"/send", token, sendBody)
	if rr.Code != http.StatusConflict {
		t.Fatalf("re-send: want 409, got %d", rr.Code)
	}
}

func TestCampaigns_ListDeliveries(t *testing.T) {
	t.Setenv("COMMS_ALLOW_ARBITRARY_RECIPIENTS", "true")
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Create + send.
	cBody := commsBody(map[string]any{
		"name":       "Delivery test",
		"subject":    "Test",
		"body_html":  "<p>x</p>",
		"from_name":  "X",
		"from_email": "x@x.com",
		"audience":   "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", token, cBody)
	var campaign map[string]any
	json.NewDecoder(rr.Body).Decode(&campaign)
	campaignID := campaign["id"].(string)

	sendBody := commsBody(map[string]any{
		"recipients": []map[string]any{
			{"email": "a@a.com"},
			{"email": "b@b.com"},
		},
	})
	doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns/"+campaignID+"/send", token, sendBody)

	// List deliveries.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/email-campaigns/"+campaignID+"/deliveries", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list deliveries: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var listResp map[string]any
	json.NewDecoder(rr.Body).Decode(&listResp)
	deliveries := listResp["deliveries"].([]any)
	if len(deliveries) != 2 {
		t.Fatalf("want 2 deliveries, got %d", len(deliveries))
	}
}

func TestCampaigns_RBACEnforcement(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	parentToken := issueCommsToken(t, router, "parent")

	// Parent cannot create campaigns.
	body := commsBody(map[string]any{
		"name": "x", "subject": "x", "body_html": "<p>x</p>",
		"from_name": "X", "from_email": "x@x.com", "audience": "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", parentToken, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent create campaign: want 403, got %d", rr.Code)
	}
}

func TestCampaigns_NotFoundForOtherOrg(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Create a campaign.
	cBody := commsBody(map[string]any{
		"name":       "OrgA Campaign",
		"subject":    "Hi",
		"body_html":  "<p>Hi</p>",
		"from_name":  "A",
		"from_email": "a@a.com",
		"audience":   "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", token, cBody)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d", rr.Code)
	}
	var campaign map[string]any
	json.NewDecoder(rr.Body).Decode(&campaign)
	campaignID := campaign["id"].(string)

	// Access with non-existent ID returns 404.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/email-campaigns/"+campaignID+"x", token, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("bad id: want 404, got %d", rr.Code)
	}
}

// ─── Suppression tests ────────────────────────────────────────────────────────

func TestSuppression_AddListRemove(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	// Add.
	body := commsBody(map[string]any{"email": "unsubscribe@example.com", "reason": "manual"})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/suppression", token, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var entry map[string]any
	json.NewDecoder(rr.Body).Decode(&entry)
	entryID := entry["id"].(string)
	if entry["email"] != "unsubscribe@example.com" {
		t.Fatalf("email mismatch: %v", entry["email"])
	}

	// Duplicate add.
	rr = doCommsReq(router, http.MethodPost, "/api/v1/suppression", token, body)
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d", rr.Code)
	}

	// List.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/suppression", token, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", rr.Code)
	}
	var listResp map[string]any
	json.NewDecoder(rr.Body).Decode(&listResp)
	suppression := listResp["suppression"].([]any)
	if len(suppression) != 1 {
		t.Fatalf("want 1 entry, got %d", len(suppression))
	}

	// Remove.
	rr = doCommsReq(router, http.MethodDelete, "/api/v1/suppression/"+entryID, token, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("remove: want 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// List again — should be empty.
	rr = doCommsReq(router, http.MethodGet, "/api/v1/suppression", token, "")
	json.NewDecoder(rr.Body).Decode(&listResp)
	suppression = listResp["suppression"].([]any)
	if len(suppression) != 0 {
		t.Fatalf("want 0 entries after remove, got %d", len(suppression))
	}
}

func TestSuppression_RBACEnforcement(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	coachToken := issueCommsToken(t, router, "coach")

	// Coach cannot manage suppression list.
	body := commsBody(map[string]any{"email": "x@x.com", "reason": "manual"})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/suppression", coachToken, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("coach add suppression: want 403, got %d", rr.Code)
	}
}

func TestPostmarkWebhookBounceUpdatesDeliveryAndSuppression(t *testing.T) {
	t.Setenv("COMMS_ALLOW_ARBITRARY_RECIPIENTS", "true")
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	cBody := commsBody(map[string]any{
		"name":       "Webhook Test",
		"subject":    "Hello",
		"body_html":  "<p>Hi</p>",
		"from_name":  "DARE",
		"from_email": "noreply@dare.swim",
		"audience":   "all",
	})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns", token, cBody)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create campaign: want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var campaign map[string]any
	json.NewDecoder(rr.Body).Decode(&campaign)
	campaignID := campaign["id"].(string)

	sendBody := commsBody(map[string]any{
		"recipients": []map[string]any{{"email": "alice@example.com"}},
	})
	rr = doCommsReq(router, http.MethodPost, "/api/v1/email-campaigns/"+campaignID+"/send", token, sendBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("send campaign: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	deliveries, err := svc.ListDeliveries(commsOrgID, campaignID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d err=%v", len(deliveries), err)
	}

	webhookBody := commsBody(map[string]any{
		"RecordType": "Bounce",
		"MessageID":  deliveries[0].ProviderMessageID,
		"Recipient":  "alice@example.com",
		"BouncedAt":  "2026-03-20T10:00:00Z",
		"Details":    "Hard bounce",
		"Metadata": map[string]any{
			"org_id": commsOrgID,
		},
	})
	rr = doPublicCommsReq(router, http.MethodPost, "/api/v1/webhooks/postmark", webhookBody, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("postmark bounce webhook: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	updatedDeliveries, err := svc.ListDeliveries(commsOrgID, campaignID)
	if err != nil {
		t.Fatalf("list deliveries after webhook: %v", err)
	}
	if updatedDeliveries[0].Status != "bounced" {
		t.Fatalf("expected delivery status bounced, got %q", updatedDeliveries[0].Status)
	}

	suppression, err := svc.ListSuppression(commsOrgID)
	if err != nil {
		t.Fatalf("list suppression: %v", err)
	}
	if len(suppression) != 1 || suppression[0].Reason != "bounce" {
		t.Fatalf("expected 1 bounce suppression entry, got %#v", suppression)
	}
}

func TestPostmarkWebhookSubscriptionChangeUnsuppressesEmail(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)
	token := issueCommsToken(t, router, "admin")

	body := commsBody(map[string]any{"email": "bounce@example.com", "reason": "bounce"})
	rr := doCommsReq(router, http.MethodPost, "/api/v1/suppression", token, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed suppression: want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	webhookBody := commsBody(map[string]any{
		"RecordType":      "SubscriptionChange",
		"Recipient":       "bounce@example.com",
		"ChangedAt":       "2026-03-20T11:00:00Z",
		"SuppressSending": false,
		"Metadata": map[string]any{
			"org_id": commsOrgID,
		},
	})
	rr = doPublicCommsReq(router, http.MethodPost, "/api/v1/webhooks/postmark", webhookBody, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("postmark subscription webhook: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	suppression, err := svc.ListSuppression(commsOrgID)
	if err != nil {
		t.Fatalf("list suppression: %v", err)
	}
	if len(suppression) != 0 {
		t.Fatalf("expected suppression to be removed, got %#v", suppression)
	}
}

func TestPostmarkWebhookBounceWithoutDeliveryStillSuppressesWhenScoped(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)

	webhookBody := commsBody(map[string]any{
		"RecordType": "Bounce",
		"Recipient":  "scoped-bounce@example.com",
		"BouncedAt":  "2026-03-20T12:00:00Z",
		"Details":    "Hard bounce",
		"Metadata": map[string]any{
			"org_id": commsOrgID,
		},
	})
	rr := doPublicCommsReq(router, http.MethodPost, "/api/v1/webhooks/postmark", webhookBody, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("postmark bounce webhook without delivery: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	suppression, err := svc.ListSuppression(commsOrgID)
	if err != nil {
		t.Fatalf("list suppression: %v", err)
	}
	if len(suppression) != 1 || suppression[0].Email != "scoped-bounce@example.com" || suppression[0].Reason != "bounce" {
		t.Fatalf("expected scoped bounce suppression entry, got %#v", suppression)
	}
}

func TestPostmarkWebhookComplaintWithoutDeliveryStillSuppressesWhenScoped(t *testing.T) {
	svc := comms.NewService()
	router := newCommsRouter(t, svc)

	webhookBody := commsBody(map[string]any{
		"RecordType": "SpamComplaint",
		"Recipient":  "scoped-complaint@example.com",
		"ChangedAt":  "2026-03-20T12:05:00Z",
		"Details":    "Spam complaint",
		"Metadata": map[string]any{
			"org_id": commsOrgID,
		},
	})
	rr := doPublicCommsReq(router, http.MethodPost, "/api/v1/webhooks/postmark", webhookBody, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("postmark complaint webhook without delivery: want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	suppression, err := svc.ListSuppression(commsOrgID)
	if err != nil {
		t.Fatalf("list suppression: %v", err)
	}
	if len(suppression) != 1 || suppression[0].Email != "scoped-complaint@example.com" || suppression[0].Reason != "complaint" {
		t.Fatalf("expected scoped complaint suppression entry, got %#v", suppression)
	}
}
