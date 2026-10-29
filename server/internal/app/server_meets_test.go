package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

// ─── Envelope helpers ─────────────────────────────────────────────────────────

type meetEnvelope struct {
	Meet struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Course string `json:"course"`
	} `json:"meet"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type meetListEnvelope struct {
	Count  int `json:"count"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Meets  []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"meets"`
}

type sessionEnvelope struct {
	Session struct {
		ID            string `json:"id"`
		SessionNumber int    `json:"session_number"`
	} `json:"session"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type meetEventEnvelope struct {
	Event struct {
		ID          string `json:"id"`
		EventNumber string `json:"event_number"`
		Stroke      string `json:"stroke"`
	} `json:"event"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type commitmentRequestEnvelope struct {
	Meet struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"meet"`
	Commitments []struct {
		ID        string `json:"id"`
		SwimmerID string `json:"swimmer_id"`
		Status    string `json:"status"`
	} `json:"commitments"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type commitmentEnvelope struct {
	Commitment struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"commitment"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type commitmentsListEnvelope struct {
	Count       int `json:"count"`
	Commitments []struct {
		ID        string `json:"id"`
		SwimmerID string `json:"swimmer_id"`
		Status    string `json:"status"`
	} `json:"commitments"`
}

type selectionEnvelope struct {
	Selection struct {
		ID          string `json:"id"`
		MeetEventID string `json:"meet_event_id"`
	} `json:"selection"`
}

type entryEnvelope struct {
	Entry struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		MeetEventID string `json:"meet_event_id"`
	} `json:"entry"`
	AuditEvent struct {
		ID       string         `json:"id"`
		Action   string         `json:"action"`
		Before   map[string]any `json:"before"`
		After    map[string]any `json:"after"`
		Metadata map[string]any `json:"metadata"`
	} `json:"audit_event"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func createMeet(t *testing.T, router http.Handler, orgID, token, name string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets", encodeJSONBody(t, map[string]any{
		"name":      name,
		"meet_type": "invitational",
		"course":    "SCY",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createMeet: expected %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
	}
	var env meetEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("createMeet: decode failed: %v", err)
	}
	return env.Meet.ID
}

func publishMeet(t *testing.T, router http.Handler, orgID, token, meetID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
		encodeJSONBody(t, map[string]any{"status": "published"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("publishMeet: expected %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
}

func createMeetEvent(t *testing.T, router http.Handler, orgID, token, meetID, eventNumber string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/meets/%s/events", meetID), encodeJSONBody(t, map[string]any{
		"event_number": eventNumber,
		"stroke":       "freestyle",
		"distance":     100,
		"course":       "SCY",
		"gender":       "O",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createMeetEvent: expected %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
	}
	var env meetEventEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("createMeetEvent: decode failed: %v", err)
	}
	return env.Event.ID
}

func issueMeetSessionTokenWithHousehold(t *testing.T, router http.Handler, userID, orgID, householdID string, roles []string, mfaAuthenticated bool) string {
	t.Helper()
	return issueSession(t, router, sessionSpec{UserID: userID, OrgID: orgID, HouseholdID: householdID, Roles: roles, MFA: mfaAuthenticated})
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestMeetsRejectUnauthenticated(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMeetCreateAndGet(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-demo", adminToken, "Spring Invitational 2026")

	// Get the meet.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID, nil)
	getReq.Header.Set("X-Org-Id", "org-demo")
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected get meet status %d, got %d body=%s", http.StatusOK, getRes.Code, getRes.Body.String())
	}

	var env meetEnvelope
	if err := json.Unmarshal(getRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Meet.Status != "draft" {
		t.Fatalf("expected status draft, got %q", env.Meet.Status)
	}
	if env.Meet.Course != "SCY" {
		t.Fatalf("expected course SCY, got %q", env.Meet.Course)
	}
}

func TestMeetCreateForbiddenForParent(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets", encodeJSONBody(t, map[string]any{
		"name": "Test Meet",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestMeetLifecycleTransition(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	meetID := createMeet(t, router, "org-demo", coachToken, "Transition Test Meet")

	// Transition: draft → published.
	transReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
		encodeJSONBody(t, map[string]any{"status": "published"}))
	transReq.Header.Set("Content-Type", "application/json")
	transReq.Header.Set("X-Org-Id", "org-demo")
	transReq.Header.Set("Authorization", "Bearer "+coachToken)
	transRes := httptest.NewRecorder()
	router.ServeHTTP(transRes, transReq)
	if transRes.Code != http.StatusOK {
		t.Fatalf("expected transition status %d, got %d body=%s", http.StatusOK, transRes.Code, transRes.Body.String())
	}

	var env meetEnvelope
	if err := json.Unmarshal(transRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Meet.Status != "published" {
		t.Fatalf("expected status published, got %q", env.Meet.Status)
	}
	if env.AuditEvent.ID == "" {
		t.Fatal("expected audit_event.id in transition response")
	}
	if env.AuditEvent.Action != "meet.status_transitioned" {
		t.Fatalf("expected action meet.status_transitioned, got %q", env.AuditEvent.Action)
	}

	// Invalid transition: published → archived (not allowed).
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
		encodeJSONBody(t, map[string]any{"status": "archived"}))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.Header.Set("X-Org-Id", "org-demo")
	badReq.Header.Set("Authorization", "Bearer "+coachToken)
	badRes := httptest.NewRecorder()
	router.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid transition status %d, got %d body=%s", http.StatusUnprocessableEntity, badRes.Code, badRes.Body.String())
	}
}

func TestMeetSessionAndEventCreation(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-demo", adminToken, "Events Test Meet")

	// Create session.
	sessReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/sessions", encodeJSONBody(t, map[string]any{
		"session_number": 1,
		"name":           "Session 1 Morning",
		"session_date":   "2026-06-15",
	}))
	sessReq.Header.Set("Content-Type", "application/json")
	sessReq.Header.Set("X-Org-Id", "org-demo")
	sessReq.Header.Set("Authorization", "Bearer "+adminToken)
	sessRes := httptest.NewRecorder()
	router.ServeHTTP(sessRes, sessReq)
	if sessRes.Code != http.StatusCreated {
		t.Fatalf("expected session create %d, got %d body=%s", http.StatusCreated, sessRes.Code, sessRes.Body.String())
	}
	var sessEnv sessionEnvelope
	if err := json.Unmarshal(sessRes.Body.Bytes(), &sessEnv); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if sessEnv.Session.ID == "" {
		t.Fatal("expected session id")
	}
	if sessEnv.AuditEvent.Action != "meet.session_created" {
		t.Fatalf("expected action meet.session_created, got %q", sessEnv.AuditEvent.Action)
	}

	// Create event.
	evtID := createMeetEvent(t, router, "org-demo", adminToken, meetID, "1")
	if evtID == "" {
		t.Fatal("expected event id")
	}

	// Duplicate event number should fail.
	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/events", encodeJSONBody(t, map[string]any{
		"event_number": "1",
		"stroke":       "backstroke",
		"distance":     100,
	}))
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.Header.Set("X-Org-Id", "org-demo")
	dupReq.Header.Set("Authorization", "Bearer "+adminToken)
	dupRes := httptest.NewRecorder()
	router.ServeHTTP(dupRes, dupReq)
	if dupRes.Code != http.StatusConflict {
		t.Fatalf("expected conflict %d on duplicate event number, got %d body=%s", http.StatusConflict, dupRes.Code, dupRes.Body.String())
	}
}

func TestMeetParseEntryFileDerivesSessionsFromBackendParser(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-parse", []string{"admin"}, false)

	pad := func(s string, n int) string {
		if len(s) >= n {
			return s[:n]
		}
		return s + strings.Repeat(" ", n-len(s))
	}
	sdifLine := func(parts ...any) string {
		var sb strings.Builder
		for i := 0; i+1 < len(parts); i += 2 {
			sb.WriteString(pad(parts[i].(string), parts[i+1].(int)))
		}
		line := sb.String()
		if len(line) < 160 {
			line += strings.Repeat(" ", 160-len(line))
		}
		return line[:160]
	}
	validSD3 := []byte(
		sdifLine(
			"A0", 2, "1", 1, "", 8,
			"02", 2, "", 30,
			"TestSoftware", 20, "v1.0", 10,
			"Test Contact", 20, "000-000-0000", 12,
			"03152026", 8, "", 42, "AZ", 2, "", 3,
		) + "\r\n" +
			sdifLine(
				"B1", 2, "1", 1, "", 8,
				"Spring Invitational", 30,
				"100 Pool Lane", 22, "", 22,
				"Springfield", 20, "AZ", 2, "85001", 10,
				"USA", 3, "1", 1,
				"03152026", 8, "03162026", 8,
				"0000", 4, "", 8,
				"2", 1, "", 10,
			) + "\r\n" +
			sdifLine("Z0", 2, "1", 1, "", 157) + "\r\n",
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "meet.sd3")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := fileWriter.Write(validSD3); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/parse-entry-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Org-Id", "org-parse")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("parse meet file: expected %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var payload struct {
		Parsed struct {
			MeetName  string `json:"meet_name"`
			StartDate string `json:"start_date"`
			EndDate   string `json:"end_date"`
			Course    string `json:"course"`
			Sessions  []struct {
				SessionNumber int    `json:"session_number"`
				Section       string `json:"section"`
				Name          string `json:"name"`
				SessionDate   string `json:"session_date"`
			} `json:"sessions"`
		} `json:"parsed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode parse response: %v", err)
	}

	if payload.Parsed.MeetName != "Spring Invitational" {
		t.Fatalf("meet_name = %q, want Spring Invitational", payload.Parsed.MeetName)
	}
	if payload.Parsed.StartDate != "2026-03-15" || payload.Parsed.EndDate != "2026-03-16" {
		t.Fatalf("unexpected parsed dates: start=%q end=%q", payload.Parsed.StartDate, payload.Parsed.EndDate)
	}
	if payload.Parsed.Course != "SCY" {
		t.Fatalf("course = %q, want SCY", payload.Parsed.Course)
	}
	if len(payload.Parsed.Sessions) != 2 {
		t.Fatalf("expected 2 derived sessions, got %d", len(payload.Parsed.Sessions))
	}
	if payload.Parsed.Sessions[0].SessionDate != "2026-03-15" || payload.Parsed.Sessions[1].SessionDate != "2026-03-16" {
		t.Fatalf("unexpected session dates: %+v", payload.Parsed.Sessions)
	}
}

func TestCommitmentRequestAndParentResponse(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-test", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-1", "org-test", []string{"parent"}, false)

	// Seed swimmer.
	meetsService.SeedSwimmer("org-test", "swm-001", "hsh-001")
	meetsService.SeedParentHousehold("user-parent-1", "hsh-001")

	// Create meet and publish it.
	meetID := createMeet(t, router, "org-test", adminToken, "Commitment Flow Meet")
	publishMeet(t, router, "org-test", adminToken, meetID)

	// Publish commitment request.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-test")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	if pubRes.Code != http.StatusOK {
		t.Fatalf("expected commitment-request status %d, got %d body=%s", http.StatusOK, pubRes.Code, pubRes.Body.String())
	}

	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode commitment-request: %v", err)
	}
	if pubEnv.Meet.Status != "commitment_open" {
		t.Fatalf("expected meet status commitment_open, got %q", pubEnv.Meet.Status)
	}
	if len(pubEnv.Commitments) != 1 {
		t.Fatalf("expected 1 commitment created, got %d", len(pubEnv.Commitments))
	}
	commitmentID := pubEnv.Commitments[0].ID
	if pubEnv.AuditEvent.Action != "meet.commitment_request_published" {
		t.Fatalf("expected audit action meet.commitment_request_published, got %q", pubEnv.AuditEvent.Action)
	}

	// Parent responds yes.
	respReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{
			"response": "committed_yes",
			"notes":    "Will attend",
		}))
	respReq.Header.Set("Content-Type", "application/json")
	respReq.Header.Set("X-Org-Id", "org-test")
	respReq.Header.Set("Authorization", "Bearer "+parentToken)
	respRes := httptest.NewRecorder()
	router.ServeHTTP(respRes, respReq)
	if respRes.Code != http.StatusOK {
		t.Fatalf("expected commitment respond status %d, got %d body=%s", http.StatusOK, respRes.Code, respRes.Body.String())
	}

	var cmtEnv commitmentEnvelope
	if err := json.Unmarshal(respRes.Body.Bytes(), &cmtEnv); err != nil {
		t.Fatalf("decode commitment response: %v", err)
	}
	if cmtEnv.Commitment.Status != "committed_yes" {
		t.Fatalf("expected status committed_yes, got %q", cmtEnv.Commitment.Status)
	}
	if cmtEnv.AuditEvent.Action != "commitment.responded" {
		t.Fatalf("expected audit action commitment.responded, got %q", cmtEnv.AuditEvent.Action)
	}
}

func TestCommitmentListUsesParentHouseholdIDFromSession(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-parent-scope", []string{"admin"}, false)
	parentToken := issueMeetSessionTokenWithHousehold(t, router, "user-parent-scope", "org-parent-scope", "hsh-parent-scope", []string{"parent"}, false)

	meetsService.SeedSwimmer("org-parent-scope", "swm-parent-scope", "hsh-parent-scope")

	meetID := createMeet(t, router, "org-parent-scope", adminToken, "Parent Household Scope Meet")
	publishMeet(t, router, "org-parent-scope", adminToken, meetID)

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-parent-scope")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	if pubRes.Code != http.StatusOK {
		t.Fatalf("expected commitment-request status %d, got %d body=%s", http.StatusOK, pubRes.Code, pubRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/commitments", nil)
	listReq.Header.Set("X-Org-Id", "org-parent-scope")
	listReq.Header.Set("Authorization", "Bearer "+parentToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected parent commitment list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env commitmentsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode parent commitment list: %v", err)
	}
	if len(env.Commitments) != 1 {
		t.Fatalf("expected 1 commitment for scoped household, got %d", len(env.Commitments))
	}
	if env.Commitments[0].SwimmerID != "swm-parent-scope" {
		t.Fatalf("expected swimmer swm-parent-scope, got %q", env.Commitments[0].SwimmerID)
	}
}

func TestParentEventSelectionFlow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-sel", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-sel", "org-sel", []string{"parent"}, false)

	meetsService.SeedSwimmer("org-sel", "swm-sel", "hsh-sel")
	meetsService.SeedParentHousehold("user-parent-sel", "hsh-sel")

	meetID := createMeet(t, router, "org-sel", adminToken, "Selection Flow Meet")
	publishMeet(t, router, "org-sel", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-sel", adminToken, meetID, "1")

	// Publish commitment request.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-sel")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	if pubRes.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, pubRes.Code, pubRes.Body.String())
	}
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	commitmentID := pubEnv.Commitments[0].ID

	// Parent responds yes.
	respReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_yes"}))
	respReq.Header.Set("Content-Type", "application/json")
	respReq.Header.Set("X-Org-Id", "org-sel")
	respReq.Header.Set("Authorization", "Bearer "+parentToken)
	respRes := httptest.NewRecorder()
	router.ServeHTTP(respRes, respReq)
	if respRes.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, respRes.Code, respRes.Body.String())
	}

	// Parent selects event.
	selReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections",
		encodeJSONBody(t, map[string]any{"meet_event_id": eventID}))
	selReq.Header.Set("Content-Type", "application/json")
	selReq.Header.Set("X-Org-Id", "org-sel")
	selReq.Header.Set("Authorization", "Bearer "+parentToken)
	selRes := httptest.NewRecorder()
	router.ServeHTTP(selRes, selReq)
	if selRes.Code != http.StatusCreated {
		t.Fatalf("expected event selection %d, got %d body=%s", http.StatusCreated, selRes.Code, selRes.Body.String())
	}

	var selEnv selectionEnvelope
	if err := json.Unmarshal(selRes.Body.Bytes(), &selEnv); err != nil {
		t.Fatalf("decode selection: %v", err)
	}
	if selEnv.Selection.ID == "" {
		t.Fatal("expected selection id")
	}
	if selEnv.Selection.MeetEventID != eventID {
		t.Fatalf("expected meet_event_id %q, got %q", eventID, selEnv.Selection.MeetEventID)
	}
	selectionID := selEnv.Selection.ID

	// Duplicate selection should return conflict.
	dupReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections",
		encodeJSONBody(t, map[string]any{"meet_event_id": eventID}))
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.Header.Set("X-Org-Id", "org-sel")
	dupReq.Header.Set("Authorization", "Bearer "+parentToken)
	dupRes := httptest.NewRecorder()
	router.ServeHTTP(dupRes, dupReq)
	if dupRes.Code != http.StatusConflict {
		t.Fatalf("expected duplicate selection conflict %d, got %d", http.StatusConflict, dupRes.Code)
	}

	// Remove selection.
	removeReq := httptest.NewRequest(http.MethodDelete,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections/"+selectionID, nil)
	removeReq.Header.Set("X-Org-Id", "org-sel")
	removeReq.Header.Set("Authorization", "Bearer "+parentToken)
	removeRes := httptest.NewRecorder()
	router.ServeHTTP(removeRes, removeReq)
	if removeRes.Code != http.StatusOK {
		t.Fatalf("expected remove selection %d, got %d body=%s", http.StatusOK, removeRes.Code, removeRes.Body.String())
	}
}

func TestEventSelectionLockedAfterCommitmentsLocked(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-lock", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-lock", "org-lock", []string{"parent"}, false)

	meetsService.SeedSwimmer("org-lock", "swm-lock", "hsh-lock")
	meetsService.SeedParentHousehold("user-parent-lock", "hsh-lock")

	meetID := createMeet(t, router, "org-lock", adminToken, "Lock Test Meet")
	publishMeet(t, router, "org-lock", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-lock", adminToken, meetID, "1")

	// Publish and respond yes.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-lock")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	commitmentID := pubEnv.Commitments[0].ID

	respReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_yes"}))
	respReq.Header.Set("Content-Type", "application/json")
	respReq.Header.Set("X-Org-Id", "org-lock")
	respReq.Header.Set("Authorization", "Bearer "+parentToken)
	respRes := httptest.NewRecorder()
	router.ServeHTTP(respRes, respReq)
	if respRes.Code != http.StatusOK {
		t.Fatalf("respond: %d body=%s", respRes.Code, respRes.Body.String())
	}

	// Lock commitments.
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-lock")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	lockRes := httptest.NewRecorder()
	router.ServeHTTP(lockRes, lockReq)
	if lockRes.Code != http.StatusOK {
		t.Fatalf("lock: expected %d, got %d body=%s", http.StatusOK, lockRes.Code, lockRes.Body.String())
	}

	// Parent tries to select event after lock — should fail.
	selReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections",
		encodeJSONBody(t, map[string]any{"meet_event_id": eventID}))
	selReq.Header.Set("Content-Type", "application/json")
	selReq.Header.Set("X-Org-Id", "org-lock")
	selReq.Header.Set("Authorization", "Bearer "+parentToken)
	selRes := httptest.NewRecorder()
	router.ServeHTTP(selRes, selReq)
	if selRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected locked selection rejection %d, got %d body=%s", http.StatusUnprocessableEntity, selRes.Code, selRes.Body.String())
	}
}

func TestCoachCannotRespondToOtherHouseholdCommitment(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-scope", []string{"admin"}, false)
	otherParentToken := issueSessionTokenForUser(t, router, "user-parent-other", "org-scope", []string{"parent"}, false)

	meetsService.SeedSwimmer("org-scope", "swm-scope", "hsh-scope-1")
	// user-parent-other is NOT linked to hsh-scope-1.

	meetID := createMeet(t, router, "org-scope", adminToken, "Scope Test Meet")
	publishMeet(t, router, "org-scope", adminToken, meetID)

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-scope")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	commitmentID := pubEnv.Commitments[0].ID

	// Other parent tries to respond — should be forbidden.
	respReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_yes"}))
	respReq.Header.Set("Content-Type", "application/json")
	respReq.Header.Set("X-Org-Id", "org-scope")
	respReq.Header.Set("Authorization", "Bearer "+otherParentToken)
	respRes := httptest.NewRecorder()
	router.ServeHTTP(respRes, respReq)
	if respRes.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden %d, got %d body=%s", http.StatusForbidden, respRes.Code, respRes.Body.String())
	}
}

func TestEntryProposeOverrideApprove(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-entry", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-entry", adminToken, "Entry Flow Meet")
	publishMeet(t, router, "org-entry", adminToken, meetID)
	eventID1 := createMeetEvent(t, router, "org-entry", adminToken, meetID, "1")
	eventID2 := createMeetEvent(t, router, "org-entry", adminToken, meetID, "2")

	// Publish and lock commitments to enter commitment_closed state.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-entry")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	if pubRes.Code != http.StatusOK {
		t.Fatalf("publish: expected %d, got %d body=%s", http.StatusOK, pubRes.Code, pubRes.Body.String())
	}

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-entry")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	lockRes := httptest.NewRecorder()
	router.ServeHTTP(lockRes, lockReq)
	if lockRes.Code != http.StatusOK {
		t.Fatalf("lock: expected %d, got %d body=%s", http.StatusOK, lockRes.Code, lockRes.Body.String())
	}

	// Propose entry.
	seedTime := 54321
	propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{
			"swimmer_id":    "swm-entry-test",
			"meet_event_id": eventID1,
			"seed_time_ms":  seedTime,
		}))
	propReq.Header.Set("Content-Type", "application/json")
	propReq.Header.Set("X-Org-Id", "org-entry")
	propReq.Header.Set("Authorization", "Bearer "+adminToken)
	propRes := httptest.NewRecorder()
	router.ServeHTTP(propRes, propReq)
	if propRes.Code != http.StatusCreated {
		t.Fatalf("propose entry: expected %d, got %d body=%s", http.StatusCreated, propRes.Code, propRes.Body.String())
	}

	var propEnv entryEnvelope
	if err := json.Unmarshal(propRes.Body.Bytes(), &propEnv); err != nil {
		t.Fatalf("decode propose: %v", err)
	}
	if propEnv.Entry.Status != "submitted" {
		t.Fatalf("expected entry status submitted, got %q", propEnv.Entry.Status)
	}
	if propEnv.AuditEvent.Action != "entry.proposed" {
		t.Fatalf("expected action entry.proposed, got %q", propEnv.AuditEvent.Action)
	}
	entryID := propEnv.Entry.ID

	// Coach override: change event with reason.
	overReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/override",
		encodeJSONBody(t, map[string]any{
			"new_meet_event_id": eventID2,
			"reason":            "Swimmer qualifies for event 2 instead",
		}))
	overReq.Header.Set("Content-Type", "application/json")
	overReq.Header.Set("X-Org-Id", "org-entry")
	overReq.Header.Set("Authorization", "Bearer "+adminToken)
	overRes := httptest.NewRecorder()
	router.ServeHTTP(overRes, overReq)
	if overRes.Code != http.StatusOK {
		t.Fatalf("override entry: expected %d, got %d body=%s", http.StatusOK, overRes.Code, overRes.Body.String())
	}

	var overEnv entryEnvelope
	if err := json.Unmarshal(overRes.Body.Bytes(), &overEnv); err != nil {
		t.Fatalf("decode override: %v", err)
	}
	if overEnv.Entry.MeetEventID != eventID2 {
		t.Fatalf("expected new event_id %q, got %q", eventID2, overEnv.Entry.MeetEventID)
	}
	if overEnv.AuditEvent.Action != "entry.coach_override" {
		t.Fatalf("expected action entry.coach_override, got %q", overEnv.AuditEvent.Action)
	}
	// Verify before/after diff is present in the audit event.
	if overEnv.AuditEvent.Before == nil || overEnv.AuditEvent.After == nil {
		t.Fatal("expected before/after diff in coach_override audit event")
	}
	if overEnv.AuditEvent.Metadata["reason"] != "Swimmer qualifies for event 2 instead" {
		t.Fatalf("expected reason in metadata, got %v", overEnv.AuditEvent.Metadata)
	}

	// Approve entry.
	appReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/approve", nil)
	appReq.Header.Set("X-Org-Id", "org-entry")
	appReq.Header.Set("Authorization", "Bearer "+adminToken)
	appRes := httptest.NewRecorder()
	router.ServeHTTP(appRes, appReq)
	if appRes.Code != http.StatusOK {
		t.Fatalf("approve entry: expected %d, got %d body=%s", http.StatusOK, appRes.Code, appRes.Body.String())
	}

	var appEnv entryEnvelope
	if err := json.Unmarshal(appRes.Body.Bytes(), &appEnv); err != nil {
		t.Fatalf("decode approve: %v", err)
	}
	if appEnv.Entry.Status != "approved" {
		t.Fatalf("expected entry status approved, got %q", appEnv.Entry.Status)
	}
	if appEnv.AuditEvent.Action != "entry.approved" {
		t.Fatalf("expected action entry.approved, got %q", appEnv.AuditEvent.Action)
	}
	if appEnv.AuditEvent.Metadata["approved_by"] == "" {
		t.Fatal("expected approved_by in audit event metadata")
	}
}

func TestEntryOverrideRequiresReason(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-reason", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-reason", adminToken, "Override Reason Test Meet")
	publishMeet(t, router, "org-reason", adminToken, meetID)
	eventID1 := createMeetEvent(t, router, "org-reason", adminToken, meetID, "1")
	eventID2 := createMeetEvent(t, router, "org-reason", adminToken, meetID, "2")

	// Publish and lock.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-reason")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-reason")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose entry.
	propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{
			"swimmer_id":    "swm-reason",
			"meet_event_id": eventID1,
		}))
	propReq.Header.Set("Content-Type", "application/json")
	propReq.Header.Set("X-Org-Id", "org-reason")
	propReq.Header.Set("Authorization", "Bearer "+adminToken)
	propRes := httptest.NewRecorder()
	router.ServeHTTP(propRes, propReq)
	var propEnv entryEnvelope
	json.Unmarshal(propRes.Body.Bytes(), &propEnv) //nolint
	entryID := propEnv.Entry.ID

	// Override without reason should fail validation.
	overReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/override",
		encodeJSONBody(t, map[string]any{
			"new_meet_event_id": eventID2,
			// No reason provided.
		}))
	overReq.Header.Set("Content-Type", "application/json")
	overReq.Header.Set("X-Org-Id", "org-reason")
	overReq.Header.Set("Authorization", "Bearer "+adminToken)
	overRes := httptest.NewRecorder()
	router.ServeHTTP(overRes, overReq)
	if overRes.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request when reason missing, got %d body=%s", overRes.Code, overRes.Body.String())
	}
}

func TestEntryApproveOnlySubmitted(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-state", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-state", adminToken, "State Machine Test Meet")
	publishMeet(t, router, "org-state", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-state", adminToken, meetID, "1")

	// Publish and lock.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-state")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-state")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose and approve.
	propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{
			"swimmer_id":    "swm-state",
			"meet_event_id": eventID,
		}))
	propReq.Header.Set("Content-Type", "application/json")
	propReq.Header.Set("X-Org-Id", "org-state")
	propReq.Header.Set("Authorization", "Bearer "+adminToken)
	propRes := httptest.NewRecorder()
	router.ServeHTTP(propRes, propReq)
	var propEnv entryEnvelope
	json.Unmarshal(propRes.Body.Bytes(), &propEnv) //nolint
	entryID := propEnv.Entry.ID

	appReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/approve", nil)
	appReq.Header.Set("X-Org-Id", "org-state")
	appReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), appReq)

	// Try to approve again — should fail (entry is now approved, not submitted).
	app2Req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/approve", nil)
	app2Req.Header.Set("X-Org-Id", "org-state")
	app2Req.Header.Set("Authorization", "Bearer "+adminToken)
	app2Res := httptest.NewRecorder()
	router.ServeHTTP(app2Res, app2Req)
	if app2Res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected unprocessable %d when approving non-submitted entry, got %d body=%s",
			http.StatusUnprocessableEntity, app2Res.Code, app2Res.Body.String())
	}
}

func TestDuplicateEntryRejected(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-dup-entry", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-dup-entry", adminToken, "Duplicate Entry Test")
	publishMeet(t, router, "org-dup-entry", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-dup-entry", adminToken, meetID, "1")

	// Publish and lock.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-dup-entry")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-dup-entry")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	body := map[string]any{"swimmer_id": "swm-dup", "meet_event_id": eventID}

	// First entry.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries", encodeJSONBody(t, body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Org-Id", "org-dup-entry")
	req1.Header.Set("Authorization", "Bearer "+adminToken)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first entry: expected %d, got %d body=%s", http.StatusCreated, rr1.Code, rr1.Body.String())
	}

	// Duplicate entry should conflict.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries", encodeJSONBody(t, body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-dup-entry")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("duplicate entry: expected conflict %d, got %d body=%s", http.StatusConflict, rr2.Code, rr2.Body.String())
	}
}

// TestCommitmentRequestRequiresPublished verifies CRITICAL-1: draft meets cannot open commitments.
func TestCommitmentRequestRequiresPublished(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-crit1", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-crit1", adminToken, "CRITICAL-1 Test Meet")
	// Meet is draft — commitment-request must fail.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	req.Header.Set("X-Org-Id", "org-crit1")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("commitment-request on draft: expected %d, got %d body=%s", http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	}
}

// TestTransitionCannotReachCommitmentOpen verifies MEDIUM: generic /transition cannot move to commitment_open.
func TestTransitionCannotReachCommitmentOpen(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-medium", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-medium", adminToken, "Medium Test Meet")
	publishMeet(t, router, "org-medium", adminToken, meetID)

	// Try to transition to commitment_open via /transition — must be rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
		encodeJSONBody(t, map[string]any{"status": "commitment_open"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-medium")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("transition to commitment_open: expected %d, got %d body=%s", http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	}

	// Try to transition to commitment_closed via /transition — must also be rejected.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
		encodeJSONBody(t, map[string]any{"status": "commitment_closed"}))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-medium")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("transition to commitment_closed: expected %d, got %d body=%s", http.StatusUnprocessableEntity, rr2.Code, rr2.Body.String())
	}
}

// TestCoachCannotRespondToCommitment verifies SEC-2: coaches cannot respond to commitments.
func TestCoachCannotRespondToCommitment(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-sec2", []string{"admin"}, false)
	coachToken := issueSessionToken(t, router, "org-sec2", []string{"coach"}, false)

	meetsService.SeedSwimmer("org-sec2", "swm-sec2", "hsh-sec2")

	meetID := createMeet(t, router, "org-sec2", adminToken, "SEC-2 Test Meet")
	publishMeet(t, router, "org-sec2", adminToken, meetID)

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-sec2")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	commitmentID := pubEnv.Commitments[0].ID

	// Coach tries to respond — must be forbidden.
	respReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_yes"}))
	respReq.Header.Set("Content-Type", "application/json")
	respReq.Header.Set("X-Org-Id", "org-sec2")
	respReq.Header.Set("Authorization", "Bearer "+coachToken)
	respRes := httptest.NewRecorder()
	router.ServeHTTP(respRes, respReq)
	if respRes.Code != http.StatusForbidden {
		t.Fatalf("coach respond: expected forbidden %d, got %d body=%s", http.StatusForbidden, respRes.Code, respRes.Body.String())
	}
}

// TestRespondNoRemovesEventSelections verifies HIGH-4: switching to committed_no removes event selections.
func TestRespondNoRemovesEventSelections(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-high4", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-high4", "org-high4", []string{"parent"}, false)

	meetsService.SeedSwimmer("org-high4", "swm-high4", "hsh-high4")
	meetsService.SeedParentHousehold("user-parent-high4", "hsh-high4")

	meetID := createMeet(t, router, "org-high4", adminToken, "HIGH-4 Test Meet")
	publishMeet(t, router, "org-high4", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-high4", adminToken, meetID, "1")

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-high4")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	pubRes := httptest.NewRecorder()
	router.ServeHTTP(pubRes, pubReq)
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pubRes.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	commitmentID := pubEnv.Commitments[0].ID

	// Parent responds yes.
	yesReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_yes"}))
	yesReq.Header.Set("Content-Type", "application/json")
	yesReq.Header.Set("X-Org-Id", "org-high4")
	yesReq.Header.Set("Authorization", "Bearer "+parentToken)
	router.ServeHTTP(httptest.NewRecorder(), yesReq)

	// Parent selects event.
	selReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections",
		encodeJSONBody(t, map[string]any{"meet_event_id": eventID}))
	selReq.Header.Set("Content-Type", "application/json")
	selReq.Header.Set("X-Org-Id", "org-high4")
	selReq.Header.Set("Authorization", "Bearer "+parentToken)
	selRes := httptest.NewRecorder()
	router.ServeHTTP(selRes, selReq)
	if selRes.Code != http.StatusCreated {
		t.Fatalf("select event: expected %d, got %d body=%s", http.StatusCreated, selRes.Code, selRes.Body.String())
	}

	// Parent flips to committed_no — selections must be deleted.
	noReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/respond",
		encodeJSONBody(t, map[string]any{"response": "committed_no"}))
	noReq.Header.Set("Content-Type", "application/json")
	noReq.Header.Set("X-Org-Id", "org-high4")
	noReq.Header.Set("Authorization", "Bearer "+parentToken)
	router.ServeHTTP(httptest.NewRecorder(), noReq)

	// Verify selections are gone.
	listReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/meets/"+meetID+"/commitments/"+commitmentID+"/event-selections", nil)
	listReq.Header.Set("X-Org-Id", "org-high4")
	listReq.Header.Set("Authorization", "Bearer "+parentToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list selections: expected %d, got %d", http.StatusOK, listRes.Code)
	}
	var listEnv struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listEnv.Count != 0 {
		t.Fatalf("expected 0 selections after committed_no, got %d", listEnv.Count)
	}
}

// TestListEntriesParentScope verifies HIGH-1: parents only see entries for their own household.
func TestListEntriesParentScope(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-high1", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-high1", "org-high1", []string{"parent"}, false)

	// Two swimmers in different households.
	meetsService.SeedSwimmer("org-high1", "swm-high1-a", "hsh-high1-a")
	meetsService.SeedSwimmer("org-high1", "swm-high1-b", "hsh-high1-b")
	// Parent only linked to household A.
	meetsService.SeedParentHousehold("user-parent-high1", "hsh-high1-a")

	meetID := createMeet(t, router, "org-high1", adminToken, "HIGH-1 Test Meet")
	publishMeet(t, router, "org-high1", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-high1", adminToken, meetID, "1")

	// Open and lock commitments.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-high1")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-high1")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose entries for both swimmers.
	for _, swmID := range []string{"swm-high1-a", "swm-high1-b"} {
		propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
			encodeJSONBody(t, map[string]any{"swimmer_id": swmID, "meet_event_id": eventID}))
		propReq.Header.Set("Content-Type", "application/json")
		propReq.Header.Set("X-Org-Id", "org-high1")
		propReq.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, propReq)
		if rr.Code != http.StatusCreated {
			t.Fatalf("propose entry for %s: expected %d, got %d body=%s", swmID, http.StatusCreated, rr.Code, rr.Body.String())
		}
	}

	// Admin sees both entries.
	listAdminReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/entries", nil)
	listAdminReq.Header.Set("X-Org-Id", "org-high1")
	listAdminReq.Header.Set("Authorization", "Bearer "+adminToken)
	listAdminRes := httptest.NewRecorder()
	router.ServeHTTP(listAdminRes, listAdminReq)
	var adminEnv struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listAdminRes.Body.Bytes(), &adminEnv); err != nil {
		t.Fatalf("decode admin entries: %v", err)
	}
	if adminEnv.Count != 2 {
		t.Fatalf("admin should see 2 entries, got %d", adminEnv.Count)
	}

	// Parent only sees their household's entry.
	listParentReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/entries", nil)
	listParentReq.Header.Set("X-Org-Id", "org-high1")
	listParentReq.Header.Set("Authorization", "Bearer "+parentToken)
	listParentRes := httptest.NewRecorder()
	router.ServeHTTP(listParentRes, listParentReq)
	var parentEnv struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listParentRes.Body.Bytes(), &parentEnv); err != nil {
		t.Fatalf("decode parent entries: %v", err)
	}
	if parentEnv.Count != 1 {
		t.Fatalf("parent should see 1 entry (own household), got %d", parentEnv.Count)
	}
}

// TestListMeetSessions verifies INCOMPLETENESS-1: GET /meets/{meetID}/sessions endpoint.
func TestListMeetSessions(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-sess", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-sess", adminToken, "Session List Test Meet")

	// Create two sessions.
	for i, name := range []string{"Morning Session", "Afternoon Session"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/sessions",
			encodeJSONBody(t, map[string]any{"session_number": i + 1, "name": name}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-sess")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create session %d: expected %d, got %d body=%s", i+1, http.StatusCreated, rr.Code, rr.Body.String())
		}
	}

	// List sessions.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+meetID+"/sessions", nil)
	listReq.Header.Set("X-Org-Id", "org-sess")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list sessions: expected %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}
	var env struct {
		Count    int `json:"count"`
		Total    int `json:"total"`
		Limit    int `json:"limit"`
		Offset   int `json:"offset"`
		Sessions []struct {
			ID            string `json:"id"`
			SessionNumber int    `json:"session_number"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("expected 2 sessions, got %d", env.Count)
	}
	if env.Total != 2 || env.Limit != 100 || env.Offset != 0 {
		t.Fatalf("expected default pagination metadata total=2 limit=100 offset=0, got total=%d limit=%d offset=%d", env.Total, env.Limit, env.Offset)
	}
}

func TestListMeetsEnforcesPaginationCeiling(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-paginated-meets", []string{"admin"}, false)

	for _, name := range []string{"Meet A", "Meet B", "Meet C"} {
		createMeet(t, router, "org-paginated-meets", adminToken, name)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets?limit=999&offset=1", nil)
	listReq.Header.Set("X-Org-Id", "org-paginated-meets")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected meet list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env meetListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode meet list: %v", err)
	}
	if env.Total != 3 || env.Count != 2 {
		t.Fatalf("expected total=3 and count=2, got total=%d count=%d", env.Total, env.Count)
	}
	if env.Limit != 200 || env.Offset != 1 {
		t.Fatalf("expected clamped pagination limit=200 offset=1, got limit=%d offset=%d", env.Limit, env.Offset)
	}
}

// TestOverrideEntryDuplicateConflict verifies CRITICAL-4: override rejects duplicate entry.
func TestOverrideEntryDuplicateConflict(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-crit4", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-crit4", adminToken, "CRITICAL-4 Test Meet")
	publishMeet(t, router, "org-crit4", adminToken, meetID)
	eventID1 := createMeetEvent(t, router, "org-crit4", adminToken, meetID, "1")
	eventID2 := createMeetEvent(t, router, "org-crit4", adminToken, meetID, "2")

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-crit4")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-crit4")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose two entries — swimmer A in event 1, swimmer A in event 2.
	prop1 := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-crit4", "meet_event_id": eventID1}))
	prop1.Header.Set("Content-Type", "application/json")
	prop1.Header.Set("X-Org-Id", "org-crit4")
	prop1.Header.Set("Authorization", "Bearer "+adminToken)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, prop1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("propose entry 1: %d body=%s", rr1.Code, rr1.Body.String())
	}
	var env1 entryEnvelope
	json.Unmarshal(rr1.Body.Bytes(), &env1) //nolint
	entryID1 := env1.Entry.ID

	prop2 := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-crit4", "meet_event_id": eventID2}))
	prop2.Header.Set("Content-Type", "application/json")
	prop2.Header.Set("X-Org-Id", "org-crit4")
	prop2.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), prop2)

	// Override entry 1 to event 2 — should conflict because swimmer already has an entry in event 2.
	overReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID1+"/override",
		encodeJSONBody(t, map[string]any{
			"new_meet_event_id": eventID2,
			"reason":            "Testing duplicate check",
		}))
	overReq.Header.Set("Content-Type", "application/json")
	overReq.Header.Set("X-Org-Id", "org-crit4")
	overReq.Header.Set("Authorization", "Bearer "+adminToken)
	overRes := httptest.NewRecorder()
	router.ServeHTTP(overRes, overReq)
	if overRes.Code != http.StatusConflict {
		t.Fatalf("override to duplicate event: expected conflict %d, got %d body=%s", http.StatusConflict, overRes.Code, overRes.Body.String())
	}
}

// TestOverrideEntryBlockedOnFinalizedMeet verifies HIGH-2: override rejected on entry_finalized, completed, and archived meets.
func TestOverrideEntryBlockedOnFinalizedMeet(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-high2", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-high2", adminToken, "HIGH-2 Test Meet")
	publishMeet(t, router, "org-high2", adminToken, meetID)
	eventID1 := createMeetEvent(t, router, "org-high2", adminToken, meetID, "1")
	eventID2 := createMeetEvent(t, router, "org-high2", adminToken, meetID, "2")

	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-high2")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-high2")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose entry in commitment_closed.
	prop := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-high2", "meet_event_id": eventID1}))
	prop.Header.Set("Content-Type", "application/json")
	prop.Header.Set("X-Org-Id", "org-high2")
	prop.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, prop)
	if rr.Code != http.StatusCreated {
		t.Fatalf("propose entry: expected %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
	}
	var propEnv entryEnvelope
	json.Unmarshal(rr.Body.Bytes(), &propEnv) //nolint
	entryID := propEnv.Entry.ID

	overrideBody := encodeJSONBody(t, map[string]any{
		"new_meet_event_id": eventID2,
		"reason":            "Should be blocked",
	})

	doOverride := func(t *testing.T) int {
		t.Helper()
		overrideBody = encodeJSONBody(t, map[string]any{
			"new_meet_event_id": eventID2,
			"reason":            "Should be blocked",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/override", overrideBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-high2")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code
	}

	transition := func(t *testing.T, toStatus string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
			encodeJSONBody(t, map[string]any{"status": toStatus}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-high2")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("transition to %s: expected %d, got %d body=%s", toStatus, http.StatusOK, res.Code, res.Body.String())
		}
	}

	// entry_finalized: override must be rejected.
	transition(t, "entry_finalized")
	if code := doOverride(t); code != http.StatusConflict {
		t.Fatalf("override in entry_finalized: expected conflict %d, got %d", http.StatusConflict, code)
	}

	// completed: override must be rejected.
	transition(t, "completed")
	if code := doOverride(t); code != http.StatusConflict {
		t.Fatalf("override in completed: expected conflict %d, got %d", http.StatusConflict, code)
	}

	// archived: override must be rejected.
	transition(t, "archived")
	if code := doOverride(t); code != http.StatusConflict {
		t.Fatalf("override in archived: expected conflict %d, got %d", http.StatusConflict, code)
	}
}

// TestApproveEntryBlockedOnInvalidMeetState verifies HIGH-3: approve succeeds in valid states
// and is rejected in invalid states (completed, archived).
func TestApproveEntryBlockedOnInvalidMeetState(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-high3", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-high3", adminToken, "HIGH-3 Test Meet")
	publishMeet(t, router, "org-high3", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-high3", adminToken, meetID, "1")

	// Open and lock commitments.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-high3")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-high3")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), lockReq)

	// Propose entry — should succeed in commitment_closed.
	propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-high3", "meet_event_id": eventID}))
	propReq.Header.Set("Content-Type", "application/json")
	propReq.Header.Set("X-Org-Id", "org-high3")
	propReq.Header.Set("Authorization", "Bearer "+adminToken)
	propRes := httptest.NewRecorder()
	router.ServeHTTP(propRes, propReq)
	if propRes.Code != http.StatusCreated {
		t.Fatalf("propose: expected %d, got %d body=%s", http.StatusCreated, propRes.Code, propRes.Body.String())
	}
	var propEnv entryEnvelope
	json.Unmarshal(propRes.Body.Bytes(), &propEnv) //nolint
	entryID := propEnv.Entry.ID

	// POSITIVE: Approve in commitment_closed — must succeed.
	appReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID+"/approve", nil)
	appReq.Header.Set("X-Org-Id", "org-high3")
	appReq.Header.Set("Authorization", "Bearer "+adminToken)
	appRes := httptest.NewRecorder()
	router.ServeHTTP(appRes, appReq)
	if appRes.Code != http.StatusOK {
		t.Fatalf("approve in commitment_closed: expected %d, got %d body=%s", http.StatusOK, appRes.Code, appRes.Body.String())
	}

	// Propose a second entry (un-approved) to test the negative path.
	eventID2 := createMeetEvent(t, router, "org-high3", adminToken, meetID, "2")
	prop2Req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-high3-b", "meet_event_id": eventID2}))
	prop2Req.Header.Set("Content-Type", "application/json")
	prop2Req.Header.Set("X-Org-Id", "org-high3")
	prop2Req.Header.Set("Authorization", "Bearer "+adminToken)
	prop2Res := httptest.NewRecorder()
	router.ServeHTTP(prop2Res, prop2Req)
	if prop2Res.Code != http.StatusCreated {
		t.Fatalf("propose entry 2: expected %d, got %d body=%s", http.StatusCreated, prop2Res.Code, prop2Res.Body.String())
	}
	var prop2Env entryEnvelope
	json.Unmarshal(prop2Res.Body.Bytes(), &prop2Env) //nolint
	entryID2 := prop2Env.Entry.ID

	// Advance meet to completed (commitment_closed → entry_finalized → completed).
	for _, toStatus := range []string{"entry_finalized", "completed"} {
		tr := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/transition",
			encodeJSONBody(t, map[string]any{"status": toStatus}))
		tr.Header.Set("Content-Type", "application/json")
		tr.Header.Set("X-Org-Id", "org-high3")
		tr.Header.Set("Authorization", "Bearer "+adminToken)
		trRes := httptest.NewRecorder()
		router.ServeHTTP(trRes, tr)
		if trRes.Code != http.StatusOK {
			t.Fatalf("transition to %s: expected %d, got %d body=%s", toStatus, http.StatusOK, trRes.Code, trRes.Body.String())
		}
	}

	// NEGATIVE: Approve in completed — must be rejected (409).
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries/"+entryID2+"/approve", nil)
	badReq.Header.Set("X-Org-Id", "org-high3")
	badReq.Header.Set("Authorization", "Bearer "+adminToken)
	badRes := httptest.NewRecorder()
	router.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusConflict {
		t.Fatalf("approve in completed: expected conflict %d, got %d body=%s", http.StatusConflict, badRes.Code, badRes.Body.String())
	}
}

// TestProposeEntryBlockedDuringCommitmentOpen verifies CRITICAL-3: entries cannot be proposed while commitments are open.
func TestProposeEntryBlockedDuringCommitmentOpen(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})

	adminToken := issueSessionToken(t, router, "org-crit3", []string{"admin"}, false)

	meetID := createMeet(t, router, "org-crit3", adminToken, "CRITICAL-3 Test Meet")
	publishMeet(t, router, "org-crit3", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-crit3", adminToken, meetID, "1")

	// Open commitments.
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	pubReq.Header.Set("X-Org-Id", "org-crit3")
	pubReq.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(httptest.NewRecorder(), pubReq)

	// Try to propose entry while commitments are open — must be rejected.
	propReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries",
		encodeJSONBody(t, map[string]any{"swimmer_id": "swm-crit3", "meet_event_id": eventID}))
	propReq.Header.Set("Content-Type", "application/json")
	propReq.Header.Set("X-Org-Id", "org-crit3")
	propReq.Header.Set("Authorization", "Bearer "+adminToken)
	propRes := httptest.NewRecorder()
	router.ServeHTTP(propRes, propReq)
	if propRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("propose during commitment_open: expected %d, got %d body=%s", http.StatusUnprocessableEntity, propRes.Code, propRes.Body.String())
	}
}

// TestInvalidPathParametersReturn400 verifies HIGH-5: whitespace-only path params are rejected with 400.
func TestInvalidPathParametersReturn400(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-high5", []string{"admin"}, false)

	// URL-encode a single space so the HTTP request is valid, but the meet ID
	// after URL-decoding and TrimSpace is empty — validateID must reject it with 400.
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/meets/%20/sessions"},
		{http.MethodGet, "/api/v1/meets/%20/events"},
		{http.MethodPost, "/api/v1/meets/%20/lock-commitments"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-Org-Id", "org-high5")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: expected 400 for whitespace meet ID, got %d body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
}

func TestSwimmerCreatedWhileCommitmentsOpenIsInvited(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	membersService := members.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Members: membersService,
		Meets:   meetsService,
	})
	adminToken := issueSessionToken(t, router, "org-test", []string{"admin"}, false)

	do := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", "org-test")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	meetID := createMeet(t, router, "org-test", adminToken, "Late Roster Meet")
	publishMeet(t, router, "org-test", adminToken, meetID)
	if rr := do(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil); rr.Code != http.StatusOK {
		t.Fatalf("commitment-request: %d %s", rr.Code, rr.Body.String())
	}

	hh := do(http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-test", "household_name": "Late Family"})
	if hh.Code != http.StatusCreated {
		t.Fatalf("create household: %d %s", hh.Code, hh.Body.String())
	}
	var hhEnv struct {
		Household struct {
			ID string `json:"id"`
		} `json:"household"`
	}
	if err := json.Unmarshal(hh.Body.Bytes(), &hhEnv); err != nil {
		t.Fatalf("decode household: %v", err)
	}
	sw := do(http.MethodPost, "/api/v1/members/swimmers", map[string]any{
		"org_id": "org-test", "household_id": hhEnv.Household.ID, "first_name": "Late", "last_name": "Joiner",
		"date_of_birth": "2013-05-01", "gender": "female",
	})
	if sw.Code != http.StatusCreated {
		t.Fatalf("create swimmer: %d %s", sw.Code, sw.Body.String())
	}
	var swEnv struct {
		Swimmer struct {
			ID string `json:"id"`
		} `json:"swimmer"`
	}
	if err := json.Unmarshal(sw.Body.Bytes(), &swEnv); err != nil {
		t.Fatalf("decode swimmer: %v", err)
	}

	list := do(http.MethodGet, "/api/v1/meets/"+meetID+"/commitments", nil)
	var listEnv struct {
		Commitments []struct {
			SwimmerID   string `json:"swimmer_id"`
			HouseholdID string `json:"household_id"`
			Status      string `json:"status"`
		} `json:"commitments"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode commitments: %v", err)
	}
	if len(listEnv.Commitments) != 1 || listEnv.Commitments[0].SwimmerID != swEnv.Swimmer.ID ||
		listEnv.Commitments[0].Status != "invited" || listEnv.Commitments[0].HouseholdID != hhEnv.Household.ID {
		t.Fatalf("expected one invited commitment for late swimmer, got %s", list.Body.String())
	}
}

func TestLockCommitmentsCreatesEntriesAndPreservesDeclines(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsService := meets.NewService()
	router := NewRouterWithServices(Config{}, logger, RouterDeps{
		Meets: meetsService,
	})
	adminToken := issueSessionToken(t, router, "org-test", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-1", "org-test", []string{"parent"}, false)
	meetsService.SeedSwimmer("org-test", "swm-yes", "hsh-001")
	meetsService.SeedSwimmer("org-test", "swm-no", "hsh-001")
	meetsService.SeedSwimmer("org-test", "swm-silent", "hsh-001")
	meetsService.SeedParentHousehold("user-parent-1", "hsh-001")

	do := func(method, path, token string, body map[string]any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, encodeJSONBody(t, body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("X-Org-Id", "org-test")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	meetID := createMeet(t, router, "org-test", adminToken, "Lock Meet")
	ev1 := createMeetEvent(t, router, "org-test", adminToken, meetID, "1")
	ev2 := createMeetEvent(t, router, "org-test", adminToken, meetID, "2")
	publishMeet(t, router, "org-test", adminToken, meetID)
	pub := do(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", adminToken, nil)
	var pubEnv commitmentRequestEnvelope
	if err := json.Unmarshal(pub.Body.Bytes(), &pubEnv); err != nil {
		t.Fatalf("decode commitment-request: %v", err)
	}
	bySwimmer := map[string]string{}
	for _, c := range pubEnv.Commitments {
		bySwimmer[c.SwimmerID] = c.ID
	}
	yesID, noID := bySwimmer["swm-yes"], bySwimmer["swm-no"]
	if rr := do(http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+yesID+"/respond", parentToken, map[string]any{"response": "committed_yes"}); rr.Code != http.StatusOK {
		t.Fatalf("respond yes: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+noID+"/respond", parentToken, map[string]any{"response": "committed_no"}); rr.Code != http.StatusOK {
		t.Fatalf("respond no: %d %s", rr.Code, rr.Body.String())
	}
	for _, ev := range []string{ev1, ev2} {
		if rr := do(http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+yesID+"/event-selections", parentToken, map[string]any{"meet_event_id": ev}); rr.Code != http.StatusCreated {
			t.Fatalf("select %s: %d %s", ev, rr.Code, rr.Body.String())
		}
	}
	if rr := do(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", adminToken, nil); rr.Code != http.StatusOK {
		t.Fatalf("lock: %d %s", rr.Code, rr.Body.String())
	}

	var entriesEnv struct {
		Entries []struct {
			SwimmerID string `json:"swimmer_id"`
			Status    string `json:"status"`
		} `json:"entries"`
	}
	entries := do(http.MethodGet, "/api/v1/meets/"+meetID+"/entries", adminToken, nil)
	if err := json.Unmarshal(entries.Body.Bytes(), &entriesEnv); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entriesEnv.Entries) != 2 {
		t.Fatalf("expected 2 entries from selections, got %s", entries.Body.String())
	}
	for _, e := range entriesEnv.Entries {
		if e.SwimmerID != "swm-yes" || e.Status != "submitted" {
			t.Fatalf("unexpected entry %+v", e)
		}
	}

	var listEnv struct {
		Commitments []struct {
			SwimmerID string `json:"swimmer_id"`
			Status    string `json:"status"`
		} `json:"commitments"`
	}
	list := do(http.MethodGet, "/api/v1/meets/"+meetID+"/commitments", adminToken, nil)
	if err := json.Unmarshal(list.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode commitments: %v", err)
	}
	want := map[string]string{"swm-yes": "locked", "swm-no": "committed_no", "swm-silent": "invited"}
	for _, c := range listEnv.Commitments {
		if want[c.SwimmerID] != c.Status {
			t.Fatalf("swimmer %s: want status %q, got %q", c.SwimmerID, want[c.SwimmerID], c.Status)
		}
	}
}
