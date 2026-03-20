package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/members"
)

// ─── Envelope helpers ─────────────────────────────────────────────────────────

type groupEnvelope struct {
	Group struct {
		ID          string `json:"id"`
		Code        string `json:"code"`
		CoachUserID string `json:"coach_user_id"`
		IsActive    bool   `json:"is_active"`
	} `json:"group"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type groupsListEnvelope struct {
	Count  int `json:"count"`
	Groups []struct {
		ID          string `json:"id"`
		Code        string `json:"code"`
		CoachUserID string `json:"coach_user_id"`
	} `json:"groups"`
}

type ruleEnvelope struct {
	Rule struct {
		ID       string `json:"id"`
		RuleType string `json:"rule_type"`
	} `json:"rule"`
}

type eligibilityEnvelope struct {
	Result struct {
		Eligible    bool `json:"eligible"`
		RuleResults []struct {
			Passed bool   `json:"passed"`
			Reason string `json:"reason"`
		} `json:"rule_results"`
	} `json:"result"`
}

type registrationEnvelope struct {
	Registration struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"registration"`
	AuditEvent struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	} `json:"audit_event"`
}

type waiverEnvelope struct {
	Waiver struct {
		ID         string `json:"id"`
		WaiverType string `json:"waiver_type"`
	} `json:"waiver"`
}

type registrationsListEnvelope struct {
	Count         int `json:"count"`
	Registrations []struct {
		ID         string `json:"id"`
		HouseholdID string `json:"household_id"`
		GroupID    string `json:"group_id"`
		Season     string `json:"season"`
		Status     string `json:"status"`
	} `json:"registrations"`
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestGroupsRejectUnauthenticated(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", nil)
	req.Header.Set("X-Org-Id", "org-demo")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestGroupCreateAndUpdate(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create group
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code":         "senior-a",
		"name":         "Senior A",
		"type":         "competitive",
		"description":  "Top competitive group",
	}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Org-Id", "org-demo")
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d body=%s", http.StatusCreated, createRes.Code, createRes.Body.String())
	}

	var env groupEnvelope
	if err := json.Unmarshal(createRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode group create response: %v", err)
	}
	if env.Group.ID == "" {
		t.Fatal("expected group id")
	}
	if env.Group.Code != "senior-a" {
		t.Fatalf("expected code senior-a, got %q", env.Group.Code)
	}
	if env.AuditEvent.ID == "" {
		t.Fatal("expected audit_event.id in group create response")
	}
	if env.AuditEvent.Action != "group.created" {
		t.Fatalf("expected audit action group.created, got %q", env.AuditEvent.Action)
	}

	// Update group — deactivate
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+env.Group.ID, encodeJSONBody(t, map[string]any{
		"is_active": false,
	}))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-Org-Id", "org-demo")
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected group update status %d, got %d body=%s", http.StatusOK, updateRes.Code, updateRes.Body.String())
	}

	var updateEnv groupEnvelope
	if err := json.Unmarshal(updateRes.Body.Bytes(), &updateEnv); err != nil {
		t.Fatalf("failed to decode group update response: %v", err)
	}
	if updateEnv.Group.IsActive {
		t.Fatal("expected group to be inactive after update")
	}
}

func TestGroupCreateDuplicateCodeRejected(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	body := encodeJSONBody(t, map[string]any{"code": "dup-group", "name": "Dup Group"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", body)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Org-Id", "org-demo")
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d", http.StatusCreated, createRes.Code)
	}

	body2 := encodeJSONBody(t, map[string]any{"code": "dup-group", "name": "Dup Group Again"})
	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", body2)
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.Header.Set("X-Org-Id", "org-demo")
	dupReq.Header.Set("Authorization", "Bearer "+adminToken)
	dupRes := httptest.NewRecorder()
	router.ServeHTTP(dupRes, dupReq)
	if dupRes.Code != http.StatusConflict {
		t.Fatalf("expected conflict status %d on duplicate, got %d body=%s", http.StatusConflict, dupRes.Code, dupRes.Body.String())
	}
}

func TestGroupCreateForbiddenForParent(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "pgroup",
		"name": "Parent Group",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestGroupScopedCoachVisibility(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	// Issue tokens for two distinct coaches.
	coach1Token := issueSessionTokenForUser(t, router, "user-coach-1", "org-demo", []string{"coach"}, false)
	coach2Token := issueSessionTokenForUser(t, router, "user-coach-2", "org-demo", []string{"coach"}, false)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Admin creates two groups: one coached by coach-1, one by coach-2.
	createGroup := func(code, coachUserID string) string {
		body := encodeJSONBody(t, map[string]any{
			"code":          code,
			"name":          code,
			"coach_user_id": coachUserID,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Org-Id", "org-demo")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected group create %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}
		var env groupEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		return env.Group.ID
	}

	createGroup("group-c1", "user-coach-1")
	createGroup("group-c2", "user-coach-2")

	// Coach 1 lists groups — should only see their own group.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+coach1Token)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}
	var list1 groupsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &list1); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if list1.Count != 1 {
		t.Fatalf("coach1 expected to see 1 group, got %d", list1.Count)
	}
	if list1.Groups[0].CoachUserID != "user-coach-1" {
		t.Fatalf("coach1 expected to see only their group, got coach_user_id=%q", list1.Groups[0].CoachUserID)
	}

	// Coach 2 lists groups — should only see their own group.
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	listReq2.Header.Set("X-Org-Id", "org-demo")
	listReq2.Header.Set("Authorization", "Bearer "+coach2Token)
	listRes2 := httptest.NewRecorder()
	router.ServeHTTP(listRes2, listReq2)
	if listRes2.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d body=%s", http.StatusOK, listRes2.Code, listRes2.Body.String())
	}
	var list2 groupsListEnvelope
	if err := json.Unmarshal(listRes2.Body.Bytes(), &list2); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if list2.Count != 1 {
		t.Fatalf("coach2 expected to see 1 group, got %d", list2.Count)
	}
	if list2.Groups[0].CoachUserID != "user-coach-2" {
		t.Fatalf("coach2 expected to see only their group, got coach_user_id=%q", list2.Groups[0].CoachUserID)
	}

	// Admin sees all groups.
	listReqAdmin := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	listReqAdmin.Header.Set("X-Org-Id", "org-demo")
	listReqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	listResAdmin := httptest.NewRecorder()
	router.ServeHTTP(listResAdmin, listReqAdmin)
	var listAdmin groupsListEnvelope
	if err := json.Unmarshal(listResAdmin.Body.Bytes(), &listAdmin); err != nil {
		t.Fatalf("failed to decode admin list: %v", err)
	}
	if listAdmin.Count != 2 {
		t.Fatalf("admin expected to see 2 groups, got %d", listAdmin.Count)
	}
}

func TestListRegistrationsReturnsLiveData(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, memberSvc, groupSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	parentToken := issueSessionToken(t, router, "org-demo", []string{"parent"}, false)

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

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-demo",
		"household_id":  household.Household.ID,
		"first_name":    "Ava",
		"last_name":     "Rivera",
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
	groupSvc.SeedSwimmer("org-demo", swimmer.Swimmer.ID, household.Household.ID, "2012-07-09", "female", "active")

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "senior-a",
		"name": "Senior A",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d body=%s", http.StatusCreated, createGroupRes.Code, createGroupRes.Body.String())
	}

	var group groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &group); err != nil {
		t.Fatalf("failed to decode group response: %v", err)
	}

	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, map[string]any{
		"household_id": household.Household.ID,
		"swimmer_id":   swimmer.Swimmer.ID,
		"group_id":     group.Group.ID,
		"season":       "2026-2027",
	}))
	regReq.Header.Set("Content-Type", "application/json")
	regReq.Header.Set("X-Org-Id", "org-demo")
	regReq.Header.Set("Authorization", "Bearer "+adminToken)
	regRes := httptest.NewRecorder()
	router.ServeHTTP(regRes, regReq)
	if regRes.Code != http.StatusCreated {
		t.Fatalf("expected registration create status %d, got %d body=%s", http.StatusCreated, regRes.Code, regRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/registrations", nil)
	listReq.Header.Set("X-Org-Id", "org-demo")
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected registrations list status %d, got %d body=%s", http.StatusOK, listRes.Code, listRes.Body.String())
	}

	var env registrationsListEnvelope
	if err := json.Unmarshal(listRes.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode registrations list response: %v", err)
	}
	if env.Count != 1 {
		t.Fatalf("expected 1 registration, got %d", env.Count)
	}
	if env.Registrations[0].HouseholdID != household.Household.ID || env.Registrations[0].GroupID != group.Group.ID {
		t.Fatalf("expected registration to include created ids, got %#v", env.Registrations[0])
	}
}

func TestEligibilityRuleAttachAndEvaluate(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	coachToken := issueSessionToken(t, router, "org-demo", []string{"coach"}, false)

	// Create group
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "age-group",
		"name": "Age Group 10-14",
		"type": "competitive",
	}))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Org-Id", "org-demo")
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected group create %d, got %d body=%s", http.StatusCreated, createRes.Code, createRes.Body.String())
	}
	var grpEnv groupEnvelope
	if err := json.Unmarshal(createRes.Body.Bytes(), &grpEnv); err != nil {
		t.Fatalf("failed to decode group: %v", err)
	}
	groupID := grpEnv.Group.ID

	// Attach age_range eligibility rule (only admin can do this)
	ruleReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID+"/eligibility-rules",
		encodeJSONBody(t, map[string]any{
			"rule_type":  "age_range",
			"parameters": map[string]any{"min_age": 10, "max_age": 14},
		}))
	ruleReq.Header.Set("Content-Type", "application/json")
	ruleReq.Header.Set("X-Org-Id", "org-demo")
	ruleReq.Header.Set("Authorization", "Bearer "+adminToken)
	ruleRes := httptest.NewRecorder()
	router.ServeHTTP(ruleRes, ruleReq)
	if ruleRes.Code != http.StatusCreated {
		t.Fatalf("expected rule attach %d, got %d body=%s", http.StatusCreated, ruleRes.Code, ruleRes.Body.String())
	}
	var ruleEnv ruleEnvelope
	if err := json.Unmarshal(ruleRes.Body.Bytes(), &ruleEnv); err != nil {
		t.Fatalf("failed to decode rule response: %v", err)
	}
	if ruleEnv.Rule.ID == "" {
		t.Fatal("expected rule id in response")
	}
	if ruleEnv.Rule.RuleType != "age_range" {
		t.Fatalf("expected rule_type age_range, got %q", ruleEnv.Rule.RuleType)
	}

	// Coach cannot attach rules (forbidden)
	ruleReqCoach := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID+"/eligibility-rules",
		encodeJSONBody(t, map[string]any{
			"rule_type":  "age_range",
			"parameters": map[string]any{"min_age": 8, "max_age": 12},
		}))
	ruleReqCoach.Header.Set("Content-Type", "application/json")
	ruleReqCoach.Header.Set("X-Org-Id", "org-demo")
	ruleReqCoach.Header.Set("Authorization", "Bearer "+coachToken)
	ruleResCoach := httptest.NewRecorder()
	router.ServeHTTP(ruleResCoach, ruleReqCoach)
	if ruleResCoach.Code != http.StatusForbidden {
		t.Fatalf("expected coach rule attach forbidden %d, got %d", http.StatusForbidden, ruleResCoach.Code)
	}

	// Seed an eligible swimmer (age within 10-14) and an ineligible one (age 8)
	groupsService.SeedSwimmer("org-demo", "swm-eligible", "hsh-1", "2013-05-10", "female", "active")
	groupsService.SeedSwimmer("org-demo", "swm-too-young", "hsh-1", "2020-01-01", "male", "active")

	// Evaluate eligible swimmer
	evalReq := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/eligibility/swm-eligible", nil)
	evalReq.Header.Set("X-Org-Id", "org-demo")
	evalReq.Header.Set("Authorization", "Bearer "+coachToken)
	evalRes := httptest.NewRecorder()
	router.ServeHTTP(evalRes, evalReq)
	if evalRes.Code != http.StatusOK {
		t.Fatalf("expected eval status %d, got %d body=%s", http.StatusOK, evalRes.Code, evalRes.Body.String())
	}
	var eligEnv eligibilityEnvelope
	if err := json.Unmarshal(evalRes.Body.Bytes(), &eligEnv); err != nil {
		t.Fatalf("failed to decode eligibility response: %v", err)
	}
	if !eligEnv.Result.Eligible {
		t.Fatal("expected eligible swimmer to pass eligibility check")
	}

	// Evaluate ineligible swimmer (too young)
	evalReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/eligibility/swm-too-young", nil)
	evalReq2.Header.Set("X-Org-Id", "org-demo")
	evalReq2.Header.Set("Authorization", "Bearer "+coachToken)
	evalRes2 := httptest.NewRecorder()
	router.ServeHTTP(evalRes2, evalReq2)
	if evalRes2.Code != http.StatusOK {
		t.Fatalf("expected eval status %d, got %d body=%s", http.StatusOK, evalRes2.Code, evalRes2.Body.String())
	}
	var eligEnv2 eligibilityEnvelope
	if err := json.Unmarshal(evalRes2.Body.Bytes(), &eligEnv2); err != nil {
		t.Fatalf("failed to decode eligibility response 2: %v", err)
	}
	if eligEnv2.Result.Eligible {
		t.Fatal("expected too-young swimmer to fail eligibility check")
	}
	if len(eligEnv2.Result.RuleResults) == 0 || eligEnv2.Result.RuleResults[0].Passed {
		t.Fatal("expected rule result to show failed for too-young swimmer")
	}
}

func TestRegistrationCreateAndTransition(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-1", "org-demo", []string{"parent"}, false)

	// Create a group
	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "reg-group",
		"name": "Registration Test Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("expected group create %d, got %d", http.StatusCreated, createGroupRes.Code)
	}
	var grpEnv groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &grpEnv); err != nil {
		t.Fatalf("failed to decode group: %v", err)
	}
	groupID := grpEnv.Group.ID

	// Seed swimmer + link to parent
	groupsService.SeedSwimmer("org-demo", "swm-reg-test", "hsh-reg", "2015-04-12", "female", "active")
	groupsService.SeedParentHousehold("user-parent-1", "hsh-reg")

	// Parent creates registration
	createRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, map[string]any{
		"household_id": "hsh-reg",
		"swimmer_id":   "swm-reg-test",
		"group_id":     groupID,
		"season":       "2026",
	}))
	createRegReq.Header.Set("Content-Type", "application/json")
	createRegReq.Header.Set("X-Org-Id", "org-demo")
	createRegReq.Header.Set("Authorization", "Bearer "+parentToken)
	createRegRes := httptest.NewRecorder()
	router.ServeHTTP(createRegRes, createRegReq)
	if createRegRes.Code != http.StatusCreated {
		t.Fatalf("expected registration create %d, got %d body=%s", http.StatusCreated, createRegRes.Code, createRegRes.Body.String())
	}
	var regEnv registrationEnvelope
	if err := json.Unmarshal(createRegRes.Body.Bytes(), &regEnv); err != nil {
		t.Fatalf("failed to decode registration: %v", err)
	}
	if regEnv.Registration.ID == "" {
		t.Fatal("expected registration id")
	}
	if regEnv.Registration.Status != "pending" {
		t.Fatalf("expected initial status pending, got %q", regEnv.Registration.Status)
	}
	regID := regEnv.Registration.ID

	// Admin activates registration
	transReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regID+"/transition", encodeJSONBody(t, map[string]any{
		"status": "active",
	}))
	transReq.Header.Set("Content-Type", "application/json")
	transReq.Header.Set("X-Org-Id", "org-demo")
	transReq.Header.Set("Authorization", "Bearer "+adminToken)
	transRes := httptest.NewRecorder()
	router.ServeHTTP(transRes, transReq)
	if transRes.Code != http.StatusOK {
		t.Fatalf("expected transition status %d, got %d body=%s", http.StatusOK, transRes.Code, transRes.Body.String())
	}
	var transEnv registrationEnvelope
	if err := json.Unmarshal(transRes.Body.Bytes(), &transEnv); err != nil {
		t.Fatalf("failed to decode transition response: %v", err)
	}
	if transEnv.Registration.Status != "active" {
		t.Fatalf("expected status active, got %q", transEnv.Registration.Status)
	}
	if transEnv.AuditEvent.ID == "" {
		t.Fatal("expected audit_event.id in transition response")
	}
	if transEnv.AuditEvent.Action != "registration.status_changed" {
		t.Fatalf("expected audit action registration.status_changed, got %q", transEnv.AuditEvent.Action)
	}

	// Parent can withdraw an active registration
	withdrawReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regID+"/transition", encodeJSONBody(t, map[string]any{
		"status": "withdrawn",
	}))
	withdrawReq.Header.Set("Content-Type", "application/json")
	withdrawReq.Header.Set("X-Org-Id", "org-demo")
	withdrawReq.Header.Set("Authorization", "Bearer "+parentToken)
	withdrawRes := httptest.NewRecorder()
	router.ServeHTTP(withdrawRes, withdrawReq)
	if withdrawRes.Code != http.StatusOK {
		t.Fatalf("expected withdraw status %d, got %d body=%s", http.StatusOK, withdrawRes.Code, withdrawRes.Body.String())
	}
	var withdrawEnv registrationEnvelope
	if err := json.Unmarshal(withdrawRes.Body.Bytes(), &withdrawEnv); err != nil {
		t.Fatalf("failed to decode withdraw response: %v", err)
	}
	if withdrawEnv.Registration.Status != "withdrawn" {
		t.Fatalf("expected status withdrawn, got %q", withdrawEnv.Registration.Status)
	}
}

func TestRegistrationDuplicateRejected(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)

	// Create group
	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "dup-reg-group",
		"name": "Dup Reg Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	var grpEnv groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &grpEnv); err != nil {
		t.Fatalf("failed to decode group: %v", err)
	}
	groupID := grpEnv.Group.ID

	groupsService.SeedSwimmer("org-demo", "swm-dup-reg", "hsh-dup", "2015-04-12", "female", "active")

	regBody := map[string]any{
		"household_id": "hsh-dup",
		"swimmer_id":   "swm-dup-reg",
		"group_id":     groupID,
		"season":       "2026",
	}

	// First registration
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, regBody))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Org-Id", "org-demo")
	req1.Header.Set("Authorization", "Bearer "+adminToken)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("expected first registration %d, got %d body=%s", http.StatusCreated, rr1.Code, rr1.Body.String())
	}

	// Duplicate registration
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, regBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Org-Id", "org-demo")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("expected duplicate registration conflict %d, got %d body=%s", http.StatusConflict, rr2.Code, rr2.Body.String())
	}
}

func TestParentCannotActivateRegistration(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-noact", "org-demo", []string{"parent"}, false)

	// Create group
	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "parent-noact-group",
		"name": "Parent No Activate Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	var grpEnv groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &grpEnv); err != nil {
		t.Fatalf("failed to decode group: %v", err)
	}

	groupsService.SeedSwimmer("org-demo", "swm-noact", "hsh-noact", "2015-01-01", "male", "active")
	groupsService.SeedParentHousehold("user-parent-noact", "hsh-noact")

	// Parent creates registration (allowed)
	createRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, map[string]any{
		"household_id": "hsh-noact",
		"swimmer_id":   "swm-noact",
		"group_id":     grpEnv.Group.ID,
		"season":       "2026",
	}))
	createRegReq.Header.Set("Content-Type", "application/json")
	createRegReq.Header.Set("X-Org-Id", "org-demo")
	createRegReq.Header.Set("Authorization", "Bearer "+parentToken)
	createRegRes := httptest.NewRecorder()
	router.ServeHTTP(createRegRes, createRegReq)
	if createRegRes.Code != http.StatusCreated {
		t.Fatalf("expected registration create %d, got %d body=%s", http.StatusCreated, createRegRes.Code, createRegRes.Body.String())
	}
	var regEnv registrationEnvelope
	if err := json.Unmarshal(createRegRes.Body.Bytes(), &regEnv); err != nil {
		t.Fatalf("failed to decode reg: %v", err)
	}

	// Parent tries to activate — should be forbidden (parents can only withdraw)
	transReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regEnv.Registration.ID+"/transition", encodeJSONBody(t, map[string]any{
		"status": "active",
	}))
	transReq.Header.Set("Content-Type", "application/json")
	transReq.Header.Set("X-Org-Id", "org-demo")
	transReq.Header.Set("Authorization", "Bearer "+parentToken)
	transRes := httptest.NewRecorder()
	router.ServeHTTP(transRes, transReq)
	if transRes.Code != http.StatusForbidden {
		t.Fatalf("expected parent activate forbidden %d, got %d body=%s", http.StatusForbidden, transRes.Code, transRes.Body.String())
	}
}

func TestWaiverAcceptanceAndIdempotency(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	groupsService := groups.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, groupsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	parentToken := issueSessionTokenForUser(t, router, "user-parent-wvr", "org-demo", []string{"parent"}, false)

	// Create group
	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "waiver-group",
		"name": "Waiver Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-demo")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	var grpEnv groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &grpEnv); err != nil {
		t.Fatalf("failed to decode group: %v", err)
	}

	groupsService.SeedSwimmer("org-demo", "swm-wvr", "hsh-wvr", "2015-01-01", "female", "active")
	groupsService.SeedParentHousehold("user-parent-wvr", "hsh-wvr")

	// Create registration
	createRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, map[string]any{
		"household_id": "hsh-wvr",
		"swimmer_id":   "swm-wvr",
		"group_id":     grpEnv.Group.ID,
		"season":       "2026",
	}))
	createRegReq.Header.Set("Content-Type", "application/json")
	createRegReq.Header.Set("X-Org-Id", "org-demo")
	createRegReq.Header.Set("Authorization", "Bearer "+parentToken)
	createRegRes := httptest.NewRecorder()
	router.ServeHTTP(createRegRes, createRegReq)
	if createRegRes.Code != http.StatusCreated {
		t.Fatalf("expected registration create %d, got %d body=%s", http.StatusCreated, createRegRes.Code, createRegRes.Body.String())
	}
	var regEnv registrationEnvelope
	if err := json.Unmarshal(createRegRes.Body.Bytes(), &regEnv); err != nil {
		t.Fatalf("failed to decode reg: %v", err)
	}
	regID := regEnv.Registration.ID

	// Parent accepts waiver
	waiverReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regID+"/waivers", encodeJSONBody(t, map[string]any{
		"waiver_type": "liability",
		"ip_address":  "192.0.2.1",
	}))
	waiverReq.Header.Set("Content-Type", "application/json")
	waiverReq.Header.Set("X-Org-Id", "org-demo")
	waiverReq.Header.Set("Authorization", "Bearer "+parentToken)
	waiverRes := httptest.NewRecorder()
	router.ServeHTTP(waiverRes, waiverReq)
	if waiverRes.Code != http.StatusCreated {
		t.Fatalf("expected waiver accept %d, got %d body=%s", http.StatusCreated, waiverRes.Code, waiverRes.Body.String())
	}
	var wvrEnv waiverEnvelope
	if err := json.Unmarshal(waiverRes.Body.Bytes(), &wvrEnv); err != nil {
		t.Fatalf("failed to decode waiver response: %v", err)
	}
	if wvrEnv.Waiver.WaiverType != "liability" {
		t.Fatalf("expected waiver_type liability, got %q", wvrEnv.Waiver.WaiverType)
	}

	// Duplicate waiver acceptance must return conflict
	waiverReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regID+"/waivers", encodeJSONBody(t, map[string]any{
		"waiver_type": "liability",
	}))
	waiverReq2.Header.Set("Content-Type", "application/json")
	waiverReq2.Header.Set("X-Org-Id", "org-demo")
	waiverReq2.Header.Set("Authorization", "Bearer "+parentToken)
	waiverRes2 := httptest.NewRecorder()
	router.ServeHTTP(waiverRes2, waiverReq2)
	if waiverRes2.Code != http.StatusConflict {
		t.Fatalf("expected duplicate waiver conflict %d, got %d body=%s", http.StatusConflict, waiverRes2.Code, waiverRes2.Body.String())
	}
}
