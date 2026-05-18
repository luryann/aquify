package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

type householdEnvelope struct {
	Household struct {
		ID string `json:"id"`
	} `json:"household"`
}

type swimmerEnvelope struct {
	Swimmer struct {
		ID string `json:"id"`
	} `json:"swimmer"`
	AuditEvent struct {
		Action string `json:"action"`
	} `json:"audit_event"`
}

type swimmersListEnvelope struct {
	Count    int `json:"count"`
	Swimmers []struct {
		ID                   string   `json:"id"`
		MedicalFlags         []string `json:"medical_flags"`
		MedicalFlagsRedacted bool     `json:"medical_flags_redacted"`
	} `json:"swimmers"`
}

type swimmerGroupsEnvelope struct {
	Count         int `json:"count"`
	SwimmerGroups []struct {
		ID string `json:"id"`
	} `json:"swimmer_groups"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type householdsListEnvelope struct {
	Count      int `json:"count"`
	Households []struct {
		ID            string `json:"id"`
		HouseholdName string `json:"household_name"`
	} `json:"households"`
}

func TestMembersListRejectsUnauthenticatedRequests(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMembersMedicalVisibilityByRole(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	billingToken := issueSessionToken(t, router, "org-demo", []string{"billing"}, false)
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Rivera Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}
	if household.Household.ID == "" {
		t.Fatal("expected household id")
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Ava",
		"last_name":     "Rivera",
		"date_of_birth": "2012-07-09",
		"gender":        "female",
		"medical_flags": []string{"asthma"},
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	billingListReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers?include_medical=true", nil)
	billingListReq.Header.Set("X-Org-Id", "org-demo")
	billingListReq.Header.Set("Authorization", "Bearer "+billingToken)
	billingListRes := httptest.NewRecorder()
	router.ServeHTTP(billingListRes, billingListReq)
	if billingListRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer list status %d, got %d body=%s", http.StatusOK, billingListRes.Code, billingListRes.Body.String())
	}

	var billingList swimmersListEnvelope
	if err := json.Unmarshal(billingListRes.Body.Bytes(), &billingList); err != nil {
		t.Fatalf("failed to decode billing list response: %v", err)
	}
	if billingList.Count != 1 {
		t.Fatalf("expected 1 swimmer in billing list, got %d", billingList.Count)
	}
	if !billingList.Swimmers[0].MedicalFlagsRedacted {
		t.Fatal("expected billing list to redact medical flags")
	}
	if len(billingList.Swimmers[0].MedicalFlags) != 0 {
		t.Fatal("expected billing list medical flags to be empty")
	}

	coachListReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers?include_medical=true", nil)
	coachListReq.Header.Set("X-Org-Id", "org-demo")
	coachListReq.Header.Set("Authorization", "Bearer "+coachToken)
	coachListRes := httptest.NewRecorder()
	router.ServeHTTP(coachListRes, coachListReq)
	if coachListRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer list status %d, got %d body=%s", http.StatusOK, coachListRes.Code, coachListRes.Body.String())
	}

	var coachList swimmersListEnvelope
	if err := json.Unmarshal(coachListRes.Body.Bytes(), &coachList); err != nil {
		t.Fatalf("failed to decode coach list response: %v", err)
	}
	if coachList.Count != 1 {
		t.Fatalf("expected 1 swimmer in coach list, got %d", coachList.Count)
	}
	if coachList.Swimmers[0].MedicalFlagsRedacted {
		t.Fatal("expected coach list to include medical flags")
	}
	if len(coachList.Swimmers[0].MedicalFlags) != 1 || coachList.Swimmers[0].MedicalFlags[0] != "asthma" {
		t.Fatalf("expected coach list medical flags [asthma], got %#v", coachList.Swimmers[0].MedicalFlags)
	}
}

func TestDeleteHouseholdRejectsWhenSwimmersStillAssigned(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Delete Guard Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+adminToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Delete",
		"last_name":     "Guard",
		"date_of_birth": "2012-07-09",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/members/households/"+household.Household.ID, nil)
	deleteReq.Header.Set("X-Org-Id", "org-demo")
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRes := httptest.NewRecorder()
	router.ServeHTTP(deleteRes, deleteReq)

	if deleteRes.Code != http.StatusConflict {
		t.Fatalf("expected household delete status %d, got %d body=%s", http.StatusConflict, deleteRes.Code, deleteRes.Body.String())
	}

	var errEnv errorEnvelope
	if err := json.Unmarshal(deleteRes.Body.Bytes(), &errEnv); err != nil {
		t.Fatalf("failed to decode delete error response: %v", err)
	}
	if errEnv.Error.Code != "household_has_swimmers" {
		t.Fatalf("expected error code household_has_swimmers, got %q", errEnv.Error.Code)
	}
}

func TestDeleteSwimmerRemovesSwimmerAndGroupLinks(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Delete Swimmer Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+adminToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Delete",
		"last_name":     "Me",
		"date_of_birth": "2012-07-09",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+adminToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("failed to decode swimmer response: %v", err)
	}

	assignGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID+"/groups", encodeJSONBody(t, map[string]any{
		"group_code": "gold",
		"is_primary": true,
	}))
	assignGroupReq.Header.Set("Content-Type", "application/json")
	assignGroupReq.Header.Set("X-Org-Id", "org-demo")
	assignGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignGroupRes := httptest.NewRecorder()
	router.ServeHTTP(assignGroupRes, assignGroupReq)
	if assignGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer group assignment status %d, got %d body=%s", http.StatusCreated, assignGroupRes.Code, assignGroupRes.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, nil)
	deleteReq.Header.Set("X-Org-Id", "org-demo")
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRes := httptest.NewRecorder()
	router.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer delete status %d, got %d body=%s", http.StatusOK, deleteRes.Code, deleteRes.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, nil)
	getReq.Header.Set("X-Org-Id", "org-demo")
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusNotFound {
		t.Fatalf("expected swimmer get-after-delete status %d, got %d body=%s", http.StatusNotFound, getRes.Code, getRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmers", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var swimmerList swimmersListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &swimmerList); err != nil {
		t.Fatalf("failed to decode swimmer list response: %v", err)
	}
	if swimmerList.Count != 0 {
		t.Fatalf("expected 0 swimmers after delete, got %d", swimmerList.Count)
	}

	groupLinksReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmer-groups", nil)
	groupLinksReq.Header.Set("X-Org-Id", "org-demo")
	groupLinksReq.Header.Set("Authorization", "Bearer "+adminToken)
	groupLinksRes := httptest.NewRecorder()
	router.ServeHTTP(groupLinksRes, groupLinksReq)
	if groupLinksRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer groups status %d, got %d body=%s", http.StatusOK, groupLinksRes.Code, groupLinksRes.Body.String())
	}

	var swimmerGroups swimmerGroupsEnvelope
	if err := json.Unmarshal(groupLinksRes.Body.Bytes(), &swimmerGroups); err != nil {
		t.Fatalf("failed to decode swimmer groups response: %v", err)
	}
	if swimmerGroups.Count != 0 {
		t.Fatalf("expected 0 swimmer groups after delete, got %d", swimmerGroups.Count)
	}
}

func TestCreateSwimmer_DoesNotCreateUserAccount(t *testing.T) {
	t.Parallel()

	kc := keycloak.NewStubClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		kc,
	)
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Parker Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Mila",
		"last_name":     "Parker",
		"date_of_birth": "2014-02-03",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	users, err := kc.ListUsers(context.Background(), "org-demo", "", 50, 0)
	if err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected swimmer creation to leave user accounts unchanged, got %d users", len(users))
	}
}

func TestListHouseholdsReturnsLiveDataForPrivilegedRoles(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	for _, name := range []string{"Rivera Family", "Nguyen Family"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
			"org_id":         "org-demo",
			"household_name": name,
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+parentToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/households", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected household list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env householdsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode household list response: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("expected 2 households, got %d", env.Count)
	}
	if env.Households[0].ID == "" || env.Households[0].HouseholdName == "" {
		t.Fatalf("expected populated household response, got %#v", env.Households[0])
	}
}

func TestSwimmerUpdateReturnsAuditEvent(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Nguyen Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Noah",
		"last_name":     "Nguyen",
		"date_of_birth": "2011-03-18",
		"gender":        "male",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("failed to decode swimmer create response: %v", err)
	}
	if swimmer.Swimmer.ID == "" {
		t.Fatal("expected swimmer id")
	}
	if swimmer.AuditEvent.Action != "swimmer.profile_created" {
		t.Fatalf("expected swimmer.profile_created audit action, got %q", swimmer.AuditEvent.Action)
	}

	updateSwimmerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, encodeJSONBody(t, map[string]any{
		"preferred_name": "N",
		"notes":          "updated by coach",
	}))
	updateSwimmerReq.Header.Set("Content-Type", "application/json")
	updateSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	updateSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	updateSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(updateSwimmerRes, updateSwimmerReq)
	if updateSwimmerRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer update status %d, got %d body=%s", http.StatusOK, updateSwimmerRes.Code, updateSwimmerRes.Body.String())
	}

	var updateResponse swimmerEnvelope
	if err := json.Unmarshal(updateSwimmerRes.Body.Bytes(), &updateResponse); err != nil {
		t.Fatalf("failed to decode swimmer update response: %v", err)
	}
	if updateResponse.AuditEvent.Action != "swimmer.profile_updated" {
		t.Fatalf("expected swimmer.profile_updated audit action, got %q", updateResponse.AuditEvent.Action)
	}
}

func TestParentAssociatedToExistingHouseholdCanManageOwnSwimmer(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	membershipService := members.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, membershipService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "parent-user", "org-demo", []string{"parent"}, false)
	coachToken := issueSessionTokenForUser(t, router, "coach-user", "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Pre-Existing Household",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+adminToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	if err := membershipService.AssociateParentToHousehold(&appmiddleware.Principal{
		UserID: "admin-user",
		OrgID:  "org-demo",
		Roles:  []string{"admin"},
	}, "org-demo", "parent-user", household.Household.ID); err != nil {
		t.Fatalf("unexpected associate failure: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Mia",
		"last_name":     "Rivera",
		"date_of_birth": "2012-04-11",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("failed to decode swimmer response: %v", err)
	}

	parentUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, encodeJSONBody(t, map[string]any{
		"preferred_name": "Mi",
	}))
	parentUpdateReq.Header.Set("Content-Type", "application/json")
	parentUpdateReq.Header.Set("X-Org-Id", "org-demo")
	parentUpdateReq.Header.Set("Authorization", "Bearer "+parentToken)
	parentUpdateRes := httptest.NewRecorder()
	router.ServeHTTP(parentUpdateRes, parentUpdateReq)
	if parentUpdateRes.Code != http.StatusOK {
		t.Fatalf("expected parent update status %d, got %d body=%s", http.StatusOK, parentUpdateRes.Code, parentUpdateRes.Body.String())
	}
}

func TestParentCannotAccessUnassociatedHouseholdSwimmers(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentOneToken := issueSessionTokenForUser(t, router, "parent-one", "org-demo", []string{"parent"}, false)
	parentTwoToken := issueSessionTokenForUser(t, router, "parent-two", "org-demo", []string{"parent"}, false)
	coachToken := issueSessionTokenForUser(t, router, "coach-user", "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Household One",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentOneToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Liam",
		"last_name":     "Jones",
		"date_of_birth": "2011-01-04",
		"gender":        "male",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-demo")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+coachToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer create status %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}

	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("failed to decode swimmer response: %v", err)
	}

	parentTwoUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, encodeJSONBody(t, map[string]any{
		"preferred_name": "Blocked",
	}))
	parentTwoUpdateReq.Header.Set("Content-Type", "application/json")
	parentTwoUpdateReq.Header.Set("X-Org-Id", "org-demo")
	parentTwoUpdateReq.Header.Set("Authorization", "Bearer "+parentTwoToken)
	parentTwoUpdateRes := httptest.NewRecorder()
	router.ServeHTTP(parentTwoUpdateRes, parentTwoUpdateReq)
	if parentTwoUpdateRes.Code != http.StatusForbidden {
		t.Fatalf("expected parent update status %d, got %d body=%s", http.StatusForbidden, parentTwoUpdateRes.Code, parentTwoUpdateRes.Body.String())
	}
}

func TestSwimmerExternalIDUniquenessIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionTokenForUser(t, router, "parent-case", "org-demo", []string{"parent"}, false)
	coachToken := issueSessionTokenForUser(t, router, "coach-case", "org-demo", []string{"coach"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Case Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}

	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("failed to decode household response: %v", err)
	}

	firstCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":              "org-demo",
		"household_id":        household.Household.ID,
		"first_name":          "Nora",
		"last_name":           "Case",
		"date_of_birth":       "2013-05-21",
		"gender":              "female",
		"swimmer_external_id": "ABC123",
	}))
	firstCreateReq.Header.Set("Content-Type", "application/json")
	firstCreateReq.Header.Set("X-Org-Id", "org-demo")
	firstCreateReq.Header.Set("Authorization", "Bearer "+coachToken)
	firstCreateRes := httptest.NewRecorder()
	router.ServeHTTP(firstCreateRes, firstCreateReq)
	if firstCreateRes.Code != http.StatusCreated {
		t.Fatalf("expected first swimmer create status %d, got %d body=%s", http.StatusCreated, firstCreateRes.Code, firstCreateRes.Body.String())
	}

	secondCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":              "org-demo",
		"household_id":        household.Household.ID,
		"first_name":          "Nina",
		"last_name":           "Case",
		"date_of_birth":       "2014-05-21",
		"gender":              "female",
		"swimmer_external_id": "abc123",
	}))
	secondCreateReq.Header.Set("Content-Type", "application/json")
	secondCreateReq.Header.Set("X-Org-Id", "org-demo")
	secondCreateReq.Header.Set("Authorization", "Bearer "+coachToken)
	secondCreateRes := httptest.NewRecorder()
	router.ServeHTTP(secondCreateRes, secondCreateReq)
	if secondCreateRes.Code != http.StatusBadRequest {
		t.Fatalf("expected duplicate swimmer create status %d, got %d body=%s", http.StatusBadRequest, secondCreateRes.Code, secondCreateRes.Body.String())
	}
}

func issueSessionTokenForUser(t *testing.T, router http.Handler, userID, orgID string, roles []string, mfaAuthenticated bool) string {
	t.Helper()

	payload := map[string]any{
		"user_id":           userID,
		"org_id":            orgID,
		"roles":             roles,
		"mfa_authenticated": mfaAuthenticated,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected session issue status %d, got %d", http.StatusCreated, rr.Code)
	}

	return issuedAccessTokenFromResponse(t, rr)
}
