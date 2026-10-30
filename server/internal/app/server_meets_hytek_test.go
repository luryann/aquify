package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func loadMeetFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "internal", "meetfiles", "testdata", "fixtures", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func postParseMeetFile(t *testing.T, router http.Handler, orgID, token, filename string, data []byte) map[string]any {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := fileWriter.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets/parse-entry-file", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Org-Id", orgID)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("parse meet file: expected %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode parse response: %v", err)
	}
	return payload
}

func TestParseMeetFile_EV3IncludesEventsAndSignupSettings(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-parse", []string{"admin"}, false)

	payload := postParseMeetFile(t, router, "org-parse", adminToken, "sample.ev3", loadMeetFixture(t, "sample.ev3"))
	parsed := payload["parsed"].(map[string]any)

	if parsed["file_type"] != "ev3" {
		t.Fatalf("file_type = %v, want ev3", parsed["file_type"])
	}
	if parsed["location"] != "La Mirada Splash! Aquatic Complex" {
		t.Fatalf("location = %v", parsed["location"])
	}
	sessions := parsed["sessions"].([]any)
	if len(sessions) != 4 {
		t.Fatalf("len(sessions) = %d, want 4", len(sessions))
	}
	events := parsed["events"].([]any)
	if len(events) != 106 {
		t.Fatalf("len(events) = %d, want 106", len(events))
	}
	signupSettings := parsed["signup_settings"].(map[string]any)
	if signupSettings["self_signup_enabled"] != true {
		t.Fatalf("self_signup_enabled = %v, want true", signupSettings["self_signup_enabled"])
	}
	if signupSettings["max_entries_per_swimmer"].(float64) != 3 {
		t.Fatalf("max_entries_per_swimmer = %v, want 3", signupSettings["max_entries_per_swimmer"])
	}
	firstEvent := events[0].(map[string]any)
	if firstEvent["event_number"] != "1" || firstEvent["stroke"] != "individual_medley" {
		t.Fatalf("unexpected first event: %#v", firstEvent)
	}
}

func TestParseMeetFile_HYVIncludesEventsAndDerivedSessions(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-parse", []string{"admin"}, false)

	payload := postParseMeetFile(t, router, "org-parse", adminToken, "sample.hyv", loadMeetFixture(t, "sample.hyv"))
	parsed := payload["parsed"].(map[string]any)

	if parsed["file_type"] != "hyv" {
		t.Fatalf("file_type = %v, want hyv", parsed["file_type"])
	}
	sessions := parsed["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	events := parsed["events"].([]any)
	if len(events) != 106 {
		t.Fatalf("len(events) = %d, want 106", len(events))
	}
	firstEvent := events[0].(map[string]any)
	if _, ok := firstEvent["session_number"]; ok {
		t.Fatalf("expected HYV events to omit session_number when unassigned, got %#v", firstEvent)
	}
}

func TestCreateMeet_WithNestedSessionsEventsAndSignupSettings(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-create", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/meets", encodeJSONBody(t, map[string]any{
		"name":       "Parsed Setup Meet",
		"meet_type":  "invitational",
		"course":     "LCM",
		"start_date": "2026-04-11",
		"end_date":   "2026-04-12",
		"location":   "La Mirada Splash! Aquatic Complex",
		"sessions": []map[string]any{
			{
				"session_number": 1,
				"name":           "Session 1",
				"session_date":   "2026-04-11",
				"start_time":     "09:00",
			},
			{
				"session_number": 2,
				"name":           "Session 2",
				"session_date":   "2026-04-11",
				"start_time":     "13:00",
			},
		},
		"events": []map[string]any{
			{
				"session_number": 1,
				"event_number":   "1",
				"stroke":         "individual_medley",
				"distance":       200,
				"course":         "LCM",
				"gender":         "F",
				"min_age":        11,
				"max_age":        12,
				"is_relay":       false,
			},
			{
				"event_number": "24",
				"stroke":       "medley_relay",
				"distance":     200,
				"course":       "LCM",
				"gender":       "X",
				"min_age":      0,
				"max_age":      8,
				"is_relay":     true,
			},
		},
		"signup_settings": map[string]any{
			"self_signup_enabled":     true,
			"max_entries_per_swimmer": 4,
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-create")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create meet: expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var created struct {
		Meet struct {
			ID                   string `json:"id"`
			SelfSignupEnabled    bool   `json:"self_signup_enabled"`
			MaxEntriesPerSwimmer int    `json:"max_entries_per_swimmer"`
		} `json:"meet"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create meet: %v", err)
	}
	if created.Meet.ID == "" {
		t.Fatal("expected created meet id")
	}
	if !created.Meet.SelfSignupEnabled || created.Meet.MaxEntriesPerSwimmer != 4 {
		t.Fatalf("unexpected signup settings on create response: %+v", created.Meet)
	}

	sessionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+created.Meet.ID+"/sessions", nil)
	sessionsReq.Header.Set("X-Org-Id", "org-create")
	sessionsReq.Header.Set("Authorization", "Bearer "+adminToken)
	sessionsRes := httptest.NewRecorder()
	router.ServeHTTP(sessionsRes, sessionsReq)
	if sessionsRes.Code != http.StatusOK {
		t.Fatalf("list sessions: expected 200, got %d body=%s", sessionsRes.Code, sessionsRes.Body.String())
	}
	var sessionsPayload struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(sessionsRes.Body.Bytes(), &sessionsPayload); err != nil {
		t.Fatalf("decode sessions response: %v", err)
	}
	if len(sessionsPayload.Sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessionsPayload.Sessions))
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/meets/"+created.Meet.ID+"/events", nil)
	eventsReq.Header.Set("X-Org-Id", "org-create")
	eventsReq.Header.Set("Authorization", "Bearer "+adminToken)
	eventsRes := httptest.NewRecorder()
	router.ServeHTTP(eventsRes, eventsReq)
	if eventsRes.Code != http.StatusOK {
		t.Fatalf("list events: expected 200, got %d body=%s", eventsRes.Code, eventsRes.Body.String())
	}
	var eventsPayload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(eventsRes.Body.Bytes(), &eventsPayload); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	if len(eventsPayload.Events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(eventsPayload.Events))
	}
	for _, event := range eventsPayload.Events {
		// A missing session_id decodes to nil, so assert on the string value.
		if sessionID, _ := event["session_id"].(string); event["event_number"] == "1" && sessionID == "" {
			t.Fatalf("expected parsed session-bound event to retain session id: %#v", event)
		}
	}
}
