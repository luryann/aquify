package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
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
	Total    int `json:"total"`
	Limit    int `json:"limit"`
	Offset   int `json:"offset"`
	Swimmers []struct {
		ID                   string   `json:"id"`
		MedicalFlags         []string `json:"medical_flags"`
		MedicalFlagsRedacted bool     `json:"medical_flags_redacted"`
	} `json:"swimmers"`
}

type swimmerGroupsEnvelope struct {
	Count         int `json:"count"`
	Total         int `json:"total"`
	Limit         int `json:"limit"`
	Offset        int `json:"offset"`
	SwimmerGroups []struct {
		ID        string `json:"id"`
		SwimmerID string `json:"swimmer_id"`
		GroupCode string `json:"group_code"`
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
	Total      int `json:"total"`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
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

func TestSwimmerCanUpdateOwnBioViaMembersAPI(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Bio API Family",
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
		"first_name":    "Ari",
		"last_name":     "Bio",
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

	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-bio-api", "org-demo", household.Household.ID, []string{"swimmer"}, false)
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, encodeJSONBody(t, map[string]any{
		"notes": "relay lead-off and sprint free fan",
	}))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-Org-Id", "org-demo")
	updateReq.Header.Set("Authorization", "Bearer "+swimmerToken)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer bio update status %d, got %d body=%s", http.StatusOK, updateRes.Code, updateRes.Body.String())
	}

	var payload struct {
		Swimmer struct {
			ID    string `json:"id"`
			Notes string `json:"notes"`
		} `json:"swimmer"`
	}
	if err := json.Unmarshal(updateRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode swimmer update response: %v", err)
	}
	if payload.Swimmer.Notes != "relay lead-off and sprint free fan" {
		t.Fatalf("expected saved bio notes, got %q", payload.Swimmer.Notes)
	}
}

func TestParentCanListOwnHouseholdSwimmerGroups(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "parent-groups", "org-demo", []string{"parent"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Parent Groups Family",
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
		"first_name":    "Ava",
		"last_name":     "Lane",
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

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "parent-household-group",
		"name": "Parent Household Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d body=%s", http.StatusCreated, createGroupRes.Code, createGroupRes.Body.String())
	}

	assignReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID+"/groups", encodeJSONBody(t, map[string]any{
		"group_code": "parent-household-group",
		"is_primary": true,
	}))
	assignReq.Header.Set("Content-Type", "application/json")
	assignReq.Header.Set("X-Org-Id", "org-demo")
	assignReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignRes := httptest.NewRecorder()
	router.ServeHTTP(assignRes, assignReq)
	if assignRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer group assign status %d, got %d body=%s", http.StatusCreated, assignRes.Code, assignRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmer-groups", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+parentToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected parent swimmer groups status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env swimmerGroupsEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode swimmer groups response: %v", err)
	}
	if env.Count != 1 || env.Total != 1 {
		t.Fatalf("expected one scoped swimmer group link, got count=%d total=%d", env.Count, env.Total)
	}
	if env.SwimmerGroups[0].SwimmerID != swimmer.Swimmer.ID || env.SwimmerGroups[0].GroupCode != "PARENT-HOUSEHOLD-GROUP" {
		t.Fatalf("unexpected swimmer group payload: %#v", env.SwimmerGroups[0])
	}
}

func TestSwimmerCanListOwnHouseholdSwimmerGroups(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Swimmer Groups Family",
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
		"first_name":    "Mila",
		"last_name":     "Stone",
		"date_of_birth": "2011-08-12",
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

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "swimmer-household-group",
		"name": "Swimmer Household Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d body=%s", http.StatusCreated, createGroupRes.Code, createGroupRes.Body.String())
	}

	assignReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID+"/groups", encodeJSONBody(t, map[string]any{
		"group_code": "swimmer-household-group",
		"is_primary": true,
	}))
	assignReq.Header.Set("Content-Type", "application/json")
	assignReq.Header.Set("X-Org-Id", "org-demo")
	assignReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignRes := httptest.NewRecorder()
	router.ServeHTTP(assignRes, assignReq)
	if assignRes.Code != http.StatusCreated {
		t.Fatalf("expected swimmer group assign status %d, got %d body=%s", http.StatusCreated, assignRes.Code, assignRes.Body.String())
	}

	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-groups", "org-demo", household.Household.ID, []string{"swimmer"}, false)
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/swimmer-groups", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+swimmerToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer swimmer-groups status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env swimmerGroupsEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode swimmer groups response: %v", err)
	}
	if env.Count != 1 || env.SwimmerGroups[0].SwimmerID != swimmer.Swimmer.ID {
		t.Fatalf("expected swimmer to see own household group link, got %#v", env)
	}
}

func TestSwimmerCannotEditProtectedFieldsViaMembersAPI(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Protected API Family",
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
		"first_name":    "Nia",
		"last_name":     "Protect",
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

	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-protect-api", "org-demo", household.Household.ID, []string{"swimmer"}, false)
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/members/swimmers/"+swimmer.Swimmer.ID, encodeJSONBody(t, map[string]any{
		"first_name": "Changed",
	}))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-Org-Id", "org-demo")
	updateReq.Header.Set("Authorization", "Bearer "+swimmerToken)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusForbidden {
		t.Fatalf("expected protected-field update to be forbidden, got %d body=%s", updateRes.Code, updateRes.Body.String())
	}
}

func TestCreateSwimmer_DoesNotCreateUserAccount(t *testing.T) {
	t.Parallel()

	kc := keycloak.NewStubClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		kc)
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
	if env.Total != 2 || env.Offset != 0 || env.Limit != 100 {
		t.Fatalf("expected default pagination metadata total=2 limit=100 offset=0, got total=%d limit=%d offset=%d", env.Total, env.Limit, env.Offset)
	}
	if env.Households[0].ID == "" || env.Households[0].HouseholdName == "" {
		t.Fatalf("expected populated household response, got %#v", env.Households[0])
	}
}

func TestListHouseholdsEnforcesPaginationCeiling(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	for _, name := range []string{"Alpha Family", "Bravo Family", "Charlie Family"} {
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

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/households?limit=999&offset=1", nil)
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
	if env.Total != 3 || env.Count != 2 {
		t.Fatalf("expected total=3 and count=2, got total=%d count=%d", env.Total, env.Count)
	}
	if env.Limit != 200 || env.Offset != 1 {
		t.Fatalf("expected clamped pagination limit=200 offset=1, got limit=%d offset=%d", env.Limit, env.Offset)
	}
}

func TestParentCanListOnlyOwnHouseholds(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, memberSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionTokenForUser(t, router, "admin-household-scope", "org-demo", []string{"admin"}, false)
	parentOneToken := issueSessionTokenForUser(t, router, "parent-household-one", "org-demo", []string{"parent"}, false)
	parentTwoToken := issueSessionTokenForUser(t, router, "parent-household-two", "org-demo", []string{"parent"}, false)

	createHousehold := func(token, name string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
			"org_id":         "org-demo",
			"household_name": name,
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected household create status %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}

		var env householdEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("failed to decode household response: %v", err)
		}
		return env.Household.ID
	}

	ownHouseholdID := createHousehold(parentOneToken, "Parent One Family")
	_ = createHousehold(parentTwoToken, "Parent Two Family")
	linkedHouseholdID := createHousehold(adminToken, "Associated Family")

	if err := memberSvc.AssociateParentToHousehold(&appmiddleware.Principal{
		UserID: "admin-household-scope",
		OrgID:  "org-demo",
		Roles:  []string{"admin"},
	}, "org-demo", "parent-household-one", linkedHouseholdID); err != nil {
		t.Fatalf("associate parent to existing household: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/households", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+parentOneToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected household list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env householdsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode household list response: %v", err)
	}
	if env.Total != 2 || env.Count != 2 {
		t.Fatalf("expected parent to see exactly 2 scoped households, got total=%d count=%d", env.Total, env.Count)
	}

	visible := map[string]bool{}
	for _, household := range env.Households {
		visible[household.ID] = true
	}
	if !visible[ownHouseholdID] || !visible[linkedHouseholdID] {
		t.Fatalf("expected parent household list to include own=%s and linked=%s, got %#v", ownHouseholdID, linkedHouseholdID, env.Households)
	}
}

func TestSwimmerCanListOwnHouseholdOnly(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionTokenForUser(t, router, "admin-swimmer-households", "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "parent-swimmer-households", "org-demo", []string{"parent"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Swimmer Scope Family",
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

	foreignHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-demo",
		"household_name": "Foreign Family",
	}))
	foreignHouseholdReq.Header.Set("Content-Type", "application/json")
	foreignHouseholdReq.Header.Set("X-Org-Id", "org-demo")
	foreignHouseholdReq.Header.Set("Authorization", "Bearer "+adminToken)
	foreignHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(foreignHouseholdRes, foreignHouseholdReq)
	if foreignHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("expected foreign household create status %d, got %d body=%s", http.StatusCreated, foreignHouseholdRes.Code, foreignHouseholdRes.Body.String())
	}

	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-household-user", "org-demo", household.Household.ID, []string{"swimmer"}, false)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/members/households", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+swimmerToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected swimmer household list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env householdsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode swimmer household list response: %v", err)
	}
	if env.Total != 1 || env.Count != 1 {
		t.Fatalf("expected swimmer to see exactly 1 household, got total=%d count=%d", env.Total, env.Count)
	}
	if env.Households[0].ID != household.Household.ID {
		t.Fatalf("expected swimmer to see own household %q, got %#v", household.Household.ID, env.Households)
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
	router := NewRouterWithServices(Config{}, logger, nil, membershipService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

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
	return issueSession(t, router, sessionSpec{UserID: userID, OrgID: orgID, Roles: roles, MFA: mfaAuthenticated})
}

func TestSwimmerGroupCodesMatchGroupCodeCasing(t *testing.T) {
	t.Parallel()

	svc := members.NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", OrgID: "org-case", Roles: []string{"admin"}}
	hh, err := svc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: "org-case", HouseholdName: "Case"})
	if err != nil {
		t.Fatalf("household: %v", err)
	}
	sw, _, err := svc.CreateSwimmer(admin, "org-case", members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "A", LastName: "B", DateOfBirth: "2012-01-01", Gender: "female"})
	if err != nil {
		t.Fatalf("swimmer: %v", err)
	}
	link, err := svc.AssignSwimmerToGroup(admin, "org-case", sw.ID, members.AssignSwimmerGroupInput{GroupCode: " senior "})
	if err != nil || link.GroupCode != "SENIOR" {
		t.Fatalf("want canonical SENIOR link, got %q (%v)", link.GroupCode, err)
	}
	// Re-assigning with different casing is idempotent, and group filters match.
	again, _ := svc.AssignSwimmerToGroup(admin, "org-case", sw.ID, members.AssignSwimmerGroupInput{GroupCode: "Senior"})
	if again.ID != link.ID {
		t.Fatal("expected the existing link to be reused regardless of casing")
	}
	list, err := svc.ListSwimmers(admin, "org-case", members.ListSwimmersInput{GroupCode: "senior"})
	if err != nil || len(list) != 1 {
		t.Fatalf("group filter should match case-insensitively, got %d (%v)", len(list), err)
	}
}

func TestApprovingRegistrationAddsSwimmerToGroupRoster(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	membersSvc := members.NewService()
	groupsSvc := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, membersSvc, groupsSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	admin := issueSessionTokenForUser(t, router, "admin-user", "org-roster", []string{"admin"}, true)

	do := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, encodeJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-roster")
		req.Header.Set("Authorization", "Bearer "+admin)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	var hh struct {
		Household struct{ ID string } `json:"household"`
	}
	_ = json.Unmarshal(do(http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-roster", "household_name": "Roster"}).Body.Bytes(), &hh)
	var sw struct {
		Swimmer struct{ ID string } `json:"swimmer"`
	}
	_ = json.Unmarshal(do(http.MethodPost, "/api/v1/members/swimmers", map[string]any{"org_id": "org-roster", "household_id": hh.Household.ID, "first_name": "R", "last_name": "S", "date_of_birth": "2012-01-01", "gender": "male"}).Body.Bytes(), &sw)
	var grp struct {
		Group struct{ ID, Code string } `json:"group"`
	}
	_ = json.Unmarshal(do(http.MethodPost, "/api/v1/groups", map[string]any{"code": "gold", "name": "Gold"}).Body.Bytes(), &grp)
	var reg struct {
		Registration struct{ ID string } `json:"registration"`
	}
	rr := do(http.MethodPost, "/api/v1/registrations", map[string]any{"household_id": hh.Household.ID, "swimmer_id": sw.Swimmer.ID, "group_id": grp.Group.ID, "season": "2026-27"})
	if err := json.Unmarshal(rr.Body.Bytes(), &reg); err != nil || reg.Registration.ID == "" {
		t.Fatalf("registration: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(http.MethodPost, "/api/v1/registrations/"+reg.Registration.ID+"/transition", map[string]any{"status": "active"}); rr.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}

	links, _ := membersSvc.ListSwimmerGroupLinks(&appmiddleware.Principal{UserID: "admin-user", OrgID: "org-roster", Roles: []string{"admin"}}, "org-roster")
	if len(links) != 1 || links[0].SwimmerID != sw.Swimmer.ID || links[0].GroupCode != grp.Group.Code || !links[0].IsPrimary {
		t.Fatalf("expected primary roster link to %q, got %+v", grp.Group.Code, links)
	}
}

func TestHouseholdUpdateEmitsAuditEvent(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditSvc := audit.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, members.NewService(), nil, nil, nil, nil, nil, nil, nil, auditSvc, nil, nil, nil, nil, nil)
	token := issueSessionTokenForUser(t, router, "admin-household-audit", "org-demo", []string{"admin"}, false)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, encodeJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	created := do(http.MethodPost, "/api/v1/members/households", map[string]any{"org_id": "org-demo", "household_name": "Audit Family"})
	var env householdEnvelope
	if err := json.Unmarshal(created.Body.Bytes(), &env); err != nil || env.Household.ID == "" {
		t.Fatalf("create household: %d %s", created.Code, created.Body.String())
	}
	if rr := do(http.MethodPatch, "/api/v1/members/households/"+env.Household.ID, map[string]any{"household_name": "Renamed Family"}); rr.Code != http.StatusOK {
		t.Fatalf("update household: %d %s", rr.Code, rr.Body.String())
	}

	events, err := auditSvc.Query(audit.Filter{OrgID: "org-demo", Action: "members.household.update"})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(events) != 1 || events[0].ResourceID != env.Household.ID {
		t.Fatalf("expected one members.household.update event for %s, got %#v", env.Household.ID, events)
	}
}
