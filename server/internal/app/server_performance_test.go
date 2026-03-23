package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/meets"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

// ─── Envelope helpers ─────────────────────────────────────────────────────────

type resultEnvelope struct {
	Result struct {
		ID        string `json:"id"`
		SwimmerID string `json:"swimmer_id"`
		Distance  int    `json:"distance"`
		Stroke    string `json:"stroke"`
		Course    string `json:"course"`
		Status    string `json:"status"`
	} `json:"result"`
}

type bestTimesEnvelope struct {
	SwimmerID string `json:"swimmer_id"`
	Count     int    `json:"count"`
	BestTimes []struct {
		ID            string     `json:"id"`
		EventDistance int        `json:"event_distance"`
		Stroke        string     `json:"stroke"`
		Course        string     `json:"course"`
		BestTimeMs    *int       `json:"best_time_ms"`
		RecordedAt    *time.Time `json:"recorded_at"`
	} `json:"best_times"`
}

type bestTimeOverrideEnvelope struct {
	BestTime struct {
		ID               string `json:"id"`
		IsManualOverride bool   `json:"is_manual_override"`
		BestTimeMs       *int   `json:"best_time_ms"`
	} `json:"best_time"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type progressionEnvelope struct {
	SwimmerID string `json:"swimmer_id"`
	Count     int    `json:"count"`
	Results   []struct {
		ID       string `json:"id"`
		Distance int    `json:"distance"`
		Stroke   string `json:"stroke"`
	} `json:"results"`
}

type orgBestTimesEnvelope struct {
	Count     int `json:"count"`
	BestTimes []struct {
		SwimmerID string `json:"swimmer_id"`
	} `json:"best_times"`
}

type orgProgressionEnvelope struct {
	Count   int `json:"count"`
	Results []struct {
		SwimmerID string `json:"swimmer_id"`
	} `json:"results"`
}

type cutQualEnvelope struct {
	Qualification struct {
		SwimmerID  string `json:"swimmer_id"`
		StandardID string `json:"standard_id"`
		Qualified  bool   `json:"qualified"`
		CutTimeMs  int    `json:"cut_time_ms"`
		BestTimeMs *int   `json:"best_time_ms"`
	} `json:"qualification"`
}

type timeStandardEnvelope struct {
	TimeStandard struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		StandardCode string `json:"standard_code"`
		SeasonYear   int    `json:"season_year"`
		TimeMs       int    `json:"time_ms"`
		IsActive     bool   `json:"is_active"`
	} `json:"time_standard"`
}

type timeStandardsListEnvelope struct {
	Count         int `json:"count"`
	TimeStandards []struct {
		ID           string `json:"id"`
		StandardCode string `json:"standard_code"`
	} `json:"time_standards"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newPerfRouter(t *testing.T, svc *performance.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, nil, nil, nil, nil, nil, nil, svc, nil, nil, nil, nil, nil, nil, nil)
}

// newPerfAndMeetsRouter builds a router with both performance and meets
// services pre-wired so that seed-time back-fill is exercised end-to-end.
func newPerfAndMeetsRouter(t *testing.T, perfSvc *performance.Service, meetsSvc *meets.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, logger, nil, nil, nil, meetsSvc, nil, nil, perfSvc, nil, nil, nil, nil, nil, nil, nil)
}

func minimalResultBody(swimmerID string) string {
	return fmt.Sprintf(`{
		"swimmer_id": %q,
		"distance": 100,
		"stroke": "free",
		"course": "SCY",
		"gender": "M",
		"fin_time_ms": 59100,
		"is_official": true
	}`, swimmerID)
}

func minimalTimeStandardBody(code string) string {
	return fmt.Sprintf(`{
		"name": "2026 USAS Motivational",
		"season_year": 2026,
		"standard_code": %q,
		"event_distance": 100,
		"stroke": "free",
		"course": "SCY",
		"gender": "M",
		"time_ms": 60000,
		"is_active": true
	}`, code)
}

// ─── Tests: upsert result ─────────────────────────────────────────────────────

func TestUpsertResultRequiresAuth(t *testing.T) {
	t.Parallel()
	router := newPerfRouter(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-1")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestUpsertResultCoachCanCreate(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)

	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-abc")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var env resultEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Result.ID == "" {
		t.Fatal("expected result.id to be populated")
	}
	if env.Result.SwimmerID != "swimmer-abc" {
		t.Fatalf("expected swimmer_id=swimmer-abc, got %q", env.Result.SwimmerID)
	}
}

func TestUpsertResultParentForbidden(t *testing.T) {
	t.Parallel()
	router := newPerfRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-1")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

// TestRelayStrokeDoesNotTriggerBestTimeRecompute verifies that upserting an
// official relay result never creates a best_time record, since the best_times
// table only permits individual strokes.
func TestRelayStrokeDoesNotTriggerBestTimeRecompute(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	relayBody := `{
		"swimmer_id": "swimmer-relay",
		"distance": 200,
		"stroke": "free_relay",
		"course": "SCY",
		"gender": "M",
		"fin_time_ms": 90000,
		"is_official": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(relayBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert relay result: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// List best times — must be empty because relay strokes must not populate best_times.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-relay/best-times", nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("list best-times: expected 200, got %d", rr2.Code)
	}
	var env bestTimesEnvelope
	if err := json.NewDecoder(rr2.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 {
		t.Fatalf("relay stroke must not create a best_time record; got %d entries", env.Count)
	}
}

// ─── Tests: best times ────────────────────────────────────────────────────────

func TestListBestTimesEmptyForNewSwimmer(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-new/best-times", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env bestTimesEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 0 {
		t.Fatalf("expected 0 best times, got %d", env.Count)
	}
}

func TestUpsertResultTriggersBestTimeRecompute(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	// Upsert a result
	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-bt")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert result: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// List best times — expect one entry
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-bt/best-times", nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("list best-times: expected 200, got %d", rr2.Code)
	}
	var env bestTimesEnvelope
	if err := json.NewDecoder(rr2.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 1 {
		t.Fatalf("expected 1 best time, got %d", env.Count)
	}
	if env.BestTimes[0].BestTimeMs == nil {
		t.Fatal("expected best_time_ms to be set")
	}
	if *env.BestTimes[0].BestTimeMs != 59100 {
		t.Fatalf("expected best_time_ms=59100, got %d", *env.BestTimes[0].BestTimeMs)
	}
}

// TestAutoComputedBestTimeHasRecordedAt verifies that auto-computed best times
// populate recorded_at from the result's timestamp.
func TestAutoComputedBestTimeHasRecordedAt(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-rcat")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert result: expected 201, got %d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-rcat/best-times", nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
	var env bestTimesEnvelope
	if err := json.NewDecoder(rr2.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 1 {
		t.Fatalf("expected 1 best time, got %d", env.Count)
	}
	if env.BestTimes[0].RecordedAt == nil {
		t.Fatal("auto-computed best time must have recorded_at set from the result timestamp")
	}
}

// TestUpsertResultEmitsAuditEvent verifies that UpsertResult appends a
// result.upserted audit event to the service's event log.
func TestUpsertResultEmitsAuditEvent(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()

	principal := &appmiddleware.Principal{UserID: "actor-1", OrgID: "org-test", Roles: []string{"coach"}}
	input := performance.UpsertResultInput{
		SwimmerID:  "swimmer-audit",
		Distance:   100,
		Stroke:     "free",
		Course:     "SCY",
		Gender:     "M",
		IsOfficial: true,
	}
	ms := 58000
	input.FinTimeMs = &ms

	_, err := svc.UpsertResult("org-test", "actor-1", principal, input)
	if err != nil {
		t.Fatalf("UpsertResult: %v", err)
	}

	events := svc.AuditEvents()
	var found bool
	for _, e := range events {
		if e.Action == "result.upserted" {
			found = true
			if e.OrgID != "org-test" {
				t.Errorf("expected org_id=org-test, got %q", e.OrgID)
			}
			if e.ActorID != "actor-1" {
				t.Errorf("expected actor_id=actor-1, got %q", e.ActorID)
			}
			meta := e.Meta
			if meta["swimmer_id"] != "swimmer-audit" {
				t.Errorf("expected meta.swimmer_id=swimmer-audit, got %v", meta["swimmer_id"])
			}
		}
	}
	if !found {
		t.Fatal("expected result.upserted audit event to be emitted")
	}
}

// ─── Tests: read endpoint role checks ────────────────────────────────────────

// TestBillingRoleCannotReadBestTimes verifies that the billing role is denied
// access to swimmer best-time data (only owner/admin/coach/parent may read).
func TestBillingRoleCannotReadBestTimes(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-x/best-times", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for billing role on best-times, got %d", rr.Code)
	}
}

// TestBillingRoleCannotReadProgression verifies that the billing role is denied
// access to swimmer progression data.
func TestBillingRoleCannotReadProgression(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-x/progression", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for billing role on progression, got %d", rr.Code)
	}
}

func TestOrgPerformanceSummaryEndpointsReturnAggregatedData(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	for _, swimmerID := range []string{"swimmer-one", "swimmer-two"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/results", strings.NewReader(minimalResultBody(swimmerID)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201 creating result for %s, got %d: %s", swimmerID, rr.Code, rr.Body.String())
		}
	}

	bestTimesReq := httptest.NewRequest(http.MethodGet, "/api/v1/performance/best-times", nil)
	bestTimesReq.Header.Set("X-Org-Id", "org-demo")
	bestTimesReq.Header.Set("Authorization", "Bearer "+token)
	bestTimesRes := httptest.NewRecorder()
	router.ServeHTTP(bestTimesRes, bestTimesReq)
	if bestTimesRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for org best-times, got %d: %s", bestTimesRes.Code, bestTimesRes.Body.String())
	}

	var bestTimesEnv orgBestTimesEnvelope
	if err := json.NewDecoder(bestTimesRes.Body).Decode(&bestTimesEnv); err != nil {
		t.Fatalf("decode org best-times: %v", err)
	}
	if bestTimesEnv.Count != 2 {
		t.Fatalf("expected 2 best-time rows, got %d", bestTimesEnv.Count)
	}

	progressionReq := httptest.NewRequest(http.MethodGet, "/api/v1/performance/progression", nil)
	progressionReq.Header.Set("X-Org-Id", "org-demo")
	progressionReq.Header.Set("Authorization", "Bearer "+token)
	progressionRes := httptest.NewRecorder()
	router.ServeHTTP(progressionRes, progressionReq)
	if progressionRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for org progression, got %d: %s", progressionRes.Code, progressionRes.Body.String())
	}

	var progressionEnv orgProgressionEnvelope
	if err := json.NewDecoder(progressionRes.Body).Decode(&progressionEnv); err != nil {
		t.Fatalf("decode org progression: %v", err)
	}
	if progressionEnv.Count != 2 {
		t.Fatalf("expected 2 progression rows, got %d", progressionEnv.Count)
	}
}

// TestBillingRoleCannotReadCutQualification verifies that the billing role is
// denied access to cut qualification evaluation.
func TestBillingRoleCannotReadCutQualification(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	billingToken := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)

	// Create a standard to have a valid standard_id.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("AA")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var createEnv timeStandardEnvelope
	json.NewDecoder(rr.Body).Decode(&createEnv)
	standardID := createEnv.TimeStandard.ID

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-x/cut-qualification?standard_id="+standardID, nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+billingToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for billing role on cut-qualification, got %d", rr2.Code)
	}
}

// TestParentRoleCanReadBestTimes verifies that the parent role has read access.
func TestParentRoleCanReadBestTimes(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-x/best-times", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for parent role on best-times, got %d", rr.Code)
	}
}

// ─── Tests: best-time override ────────────────────────────────────────────────

func TestOverrideBestTimeRequiresAdminRole(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	body := `{"event_distance":100,"stroke":"free","course":"SCY","best_time_ms":58000,"reason":"manual entry"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/swimmers/swimmer-x/best-times/override",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestOverrideBestTimeAdminSuccess(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	ms := 57500
	bodyMap := map[string]any{
		"event_distance": 100,
		"stroke":         "free",
		"course":         "SCY",
		"best_time_ms":   ms,
		"reason":         "Historical result entry",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/swimmers/swimmer-ovr/best-times/override",
		encodeJSONBody(t, bodyMap))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var env bestTimeOverrideEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.BestTime.IsManualOverride {
		t.Fatal("expected is_manual_override=true")
	}
	if env.BestTime.BestTimeMs == nil || *env.BestTime.BestTimeMs != ms {
		t.Fatalf("expected best_time_ms=%d", ms)
	}
	if env.AuditEvent.Action != "best_time.manual_override" {
		t.Fatalf("expected audit action best_time.manual_override, got %q", env.AuditEvent.Action)
	}
}

func TestOverrideBestTimeRequiresReason(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	body := `{"event_distance":100,"stroke":"free","course":"SCY","best_time_ms":58000,"reason":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/swimmers/swimmer-x/best-times/override",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

// TestOverrideBestTimeWhitespaceReasonReturns422 verifies that a whitespace-only
// reason bypasses the validator's `required` check but is still rejected by the
// service with 422 (not 400).
func TestOverrideBestTimeWhitespaceReasonReturns422(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	body := `{"event_distance":100,"stroke":"free","course":"SCY","best_time_ms":58000,"reason":"   "}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/swimmers/swimmer-x/best-times/override",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for whitespace-only reason, got %d", rr.Code)
	}
}

// ─── Tests: progression ───────────────────────────────────────────────────────

func TestListProgressionReturnsResultsInOrder(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	// Upsert two results for the same swimmer
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
			strings.NewReader(minimalResultBody("swimmer-prog")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("upsert result: expected 201, got %d", rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-prog/progression?stroke=free&course=SCY&distance=100", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env progressionEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("expected 2 results, got %d", env.Count)
	}
}

// ─── Tests: time standards ────────────────────────────────────────────────────

func TestCreateTimeStandardRequiresAdmin(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("AA")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCreateAndGetTimeStandard(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("AA")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var createEnv timeStandardEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&createEnv); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	standardID := createEnv.TimeStandard.ID
	if standardID == "" {
		t.Fatal("expected standard ID to be set")
	}

	// Get
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/time-standards/"+standardID, nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rr2.Code)
	}
	var getEnv timeStandardEnvelope
	if err := json.NewDecoder(rr2.Body).Decode(&getEnv); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if getEnv.TimeStandard.StandardCode != "AA" {
		t.Fatalf("expected standard_code=AA, got %q", getEnv.TimeStandard.StandardCode)
	}
}

func TestListTimeStandards(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create two standards
	for _, code := range []string{"AA", "A"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
			strings.NewReader(minimalTimeStandardBody(code)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", code, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/time-standards", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var env timeStandardsListEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("expected 2 standards, got %d", env.Count)
	}
}

func TestUpdateTimeStandard(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("BB")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}
	var createEnv timeStandardEnvelope
	json.NewDecoder(rr.Body).Decode(&createEnv)
	standardID := createEnv.TimeStandard.ID

	// Update time_ms
	newTimeMs := 62000
	updateBody := fmt.Sprintf(`{"time_ms": %d}`, newTimeMs)
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/time-standards/"+standardID,
		strings.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var updateEnv timeStandardEnvelope
	json.NewDecoder(rr2.Body).Decode(&updateEnv)
	if updateEnv.TimeStandard.TimeMs != newTimeMs {
		t.Fatalf("expected time_ms=%d, got %d", newTimeMs, updateEnv.TimeStandard.TimeMs)
	}
}

func TestDeleteTimeStandard(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("CC")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var createEnv timeStandardEnvelope
	json.NewDecoder(rr.Body).Decode(&createEnv)
	standardID := createEnv.TimeStandard.ID

	// Delete
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/time-standards/"+standardID, nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", rr2.Code)
	}

	// Get after delete should 404
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/time-standards/"+standardID, nil)
	req3.Header.Set("X-Org-Id", "org-demo")
	req3.Header.Set("Authorization", "Bearer "+token)
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", rr3.Code)
	}
}

// ─── Tests: cut qualification ─────────────────────────────────────────────────

func TestEvaluateCutQualificationNotQualifiedWhenNT(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create standard with a fast cut
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("AA")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var createEnv timeStandardEnvelope
	json.NewDecoder(rr.Body).Decode(&createEnv)
	standardID := createEnv.TimeStandard.ID

	// Evaluate swimmer with no results
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-nt/cut-qualification?standard_id="+standardID, nil)
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
	var env cutQualEnvelope
	json.NewDecoder(rr2.Body).Decode(&env)
	if env.Qualification.Qualified {
		t.Fatal("expected qualified=false for NT swimmer")
	}
	if env.Qualification.BestTimeMs != nil {
		t.Fatal("expected best_time_ms=nil for NT swimmer")
	}
}

func TestEvaluateCutQualificationQualifiedWhenUnderCut(t *testing.T) {
	t.Parallel()
	svc := performance.NewService()
	router := newPerfRouter(t, svc)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create standard: cut = 60000ms
	req := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards",
		strings.NewReader(minimalTimeStandardBody("AA")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var createEnv timeStandardEnvelope
	json.NewDecoder(rr.Body).Decode(&createEnv)
	standardID := createEnv.TimeStandard.ID

	// Post result with fin_time_ms=59100 (under 60000)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(minimalResultBody("swimmer-qual")))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+token)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("upsert: expected 201, got %d", rr2.Code)
	}

	// Evaluate cut qualification
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-qual/cut-qualification?standard_id="+standardID, nil)
	req3.Header.Set("X-Org-Id", "org-demo")
	req3.Header.Set("Authorization", "Bearer "+token)
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}
	var env cutQualEnvelope
	json.NewDecoder(rr3.Body).Decode(&env)
	if !env.Qualification.Qualified {
		t.Fatalf("expected qualified=true (59100 < 60000), got %+v", env.Qualification)
	}
}

func TestEvaluateCutQualificationMissingStandardIDReturns400(t *testing.T) {
	t.Parallel()
	router := newPerfRouter(t, nil)
	token := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-x/cut-qualification", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// ─── Tests: seed-time back-fill ───────────────────────────────────────────────

// TestUpsertResultUpdatesFutureEntrySeedTime verifies the end-to-end seed-time
// back-fill: upserting a faster official result must update the seed time on
// any draft/submitted meet entry for the same swimmer and event specification.
func TestUpsertResultUpdatesFutureEntrySeedTime(t *testing.T) {
	t.Parallel()

	perfSvc := performance.NewService()
	meetsSvc := meets.NewService()

	// Wire the services together (mirrors what NewRouterWithServices does).
	perfSvc.SetEntrySeedTimeUpdater(meetsSvc)

	router := newPerfAndMeetsRouter(t, perfSvc, meetsSvc)
	token := issueSessionToken(t, router, "org-seed", []string{"coach"}, false)

	// Seed a submitted entry with an initial seed time of 62000ms (100 free SCY).
	initialSeedMs := 62000
	entryID := meetsSvc.SeedEntry(
		"org-seed",
		"swimmer-seed-time",
		"meet-future-1",
		"freestyle", // meets-domain stroke name
		"SCY",
		100,
		&initialSeedMs,
	)

	// Verify the entry was seeded correctly.
	seedMs, ok := meetsSvc.GetEntrySeedTime(entryID)
	if !ok {
		t.Fatal("seeded entry not found")
	}
	if seedMs == nil || *seedMs != initialSeedMs {
		t.Fatalf("expected initial seed_time_ms=%d, got %v", initialSeedMs, seedMs)
	}

	// Upsert a faster official result (59000ms < 62000ms initial seed).
	body := `{
		"swimmer_id": "swimmer-seed-time",
		"distance": 100,
		"stroke": "free",
		"course": "SCY",
		"gender": "M",
		"fin_time_ms": 59000,
		"is_official": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-seed")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert result: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// The entry's seed time must now reflect the new best time.
	updatedSeedMs, ok := meetsSvc.GetEntrySeedTime(entryID)
	if !ok {
		t.Fatal("entry not found after result upsert")
	}
	if updatedSeedMs == nil {
		t.Fatal("expected seed_time_ms to be updated to 59000, got nil")
	}
	if *updatedSeedMs != 59000 {
		t.Fatalf("expected seed_time_ms=59000 after best-time recompute, got %d", *updatedSeedMs)
	}
}

// TestUpsertResultDoesNotUpdateApprovedEntries verifies that approved/exported
// entries are not modified by the seed-time back-fill — only draft and
// submitted entries should be touched.
func TestUpsertResultDoesNotUpdateApprovedEntries(t *testing.T) {
	t.Parallel()

	perfSvc := performance.NewService()
	meetsSvc := meets.NewService()
	perfSvc.SetEntrySeedTimeUpdater(meetsSvc)

	router := newPerfAndMeetsRouter(t, perfSvc, meetsSvc)
	token := issueSessionToken(t, router, "org-approved", []string{"coach"}, false)

	// Seed an approved entry — seed time should not be touched.
	initialSeedMs := 63000
	entryID := meetsSvc.SeedEntry(
		"org-approved",
		"swimmer-approved",
		"meet-future-2",
		"freestyle",
		"SCY",
		100,
		&initialSeedMs,
	)
	// Manually move the entry to "approved" status.
	meetsSvc.SetEntryStatusForTest(entryID, "approved")

	// Upsert a result for the swimmer.
	body := `{
		"swimmer_id": "swimmer-approved",
		"distance": 100,
		"stroke": "free",
		"course": "SCY",
		"gender": "M",
		"fin_time_ms": 58000,
		"is_official": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/results",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-approved")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upsert result: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Approved entry seed time must remain unchanged.
	seedMs, ok := meetsSvc.GetEntrySeedTime(entryID)
	if !ok {
		t.Fatal("entry not found")
	}
	if seedMs == nil || *seedMs != initialSeedMs {
		t.Fatalf("approved entry seed_time_ms must not be changed; expected %d, got %v", initialSeedMs, seedMs)
	}
}
