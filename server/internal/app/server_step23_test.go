package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/dareaquatics/aquify/server/internal/storage"
)

func issueSessionTokenWithHousehold(t *testing.T, router http.Handler, userID, orgID, householdID string, roles []string, mfaAuthenticated bool) string {
	t.Helper()

	payload := map[string]any{
		"user_id":           userID,
		"org_id":            orgID,
		"household_id":      householdID,
		"roles":             roles,
		"mfa_authenticated": mfaAuthenticated,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue session with household: expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func TestHY3ImportFinalizationAndReportGeneration(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewStubStorage("")
	importsService := imports.NewServiceWithStorage(store)
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionToken(t, router, "org-hy3", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-hy3", adminToken, "championship.hy3", "hy3")
	seedUploadedImport(t, store, "org-hy3", importID, "championship.hy3", "application/zip", buildZipFixture(t, "results.sd3", loadImportFixture(t, "valid.sd3")))

	finalizeReq := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
	finalizeReq.Header.Set("X-Org-Id", "org-hy3")
	finalizeReq.Header.Set("Authorization", "Bearer "+adminToken)
	finalizeRes := httptest.NewRecorder()
	router.ServeHTTP(finalizeRes, finalizeReq)
	if finalizeRes.Code != http.StatusOK {
		t.Fatalf("expected finalize 200, got %d body=%s", finalizeRes.Code, finalizeRes.Body.String())
	}

	var finalizeEnv importEnvelope
	if err := json.Unmarshal(finalizeRes.Body.Bytes(), &finalizeEnv); err != nil {
		t.Fatalf("decode finalize response: %v", err)
	}
	if finalizeEnv.Import.FileType != "hy3" {
		t.Fatalf("expected hy3 file type, got %q", finalizeEnv.Import.FileType)
	}
	if finalizeEnv.Import.Status != "conflicts_pending" {
		t.Fatalf("expected conflicts_pending status, got %q", finalizeEnv.Import.Status)
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/api/v1/imports/"+importID+"/report", nil)
	reportReq.Header.Set("X-Org-Id", "org-hy3")
	reportReq.Header.Set("Authorization", "Bearer "+adminToken)
	reportRes := httptest.NewRecorder()
	router.ServeHTTP(reportRes, reportReq)
	if reportRes.Code != http.StatusOK {
		t.Fatalf("expected report 200, got %d body=%s", reportRes.Code, reportRes.Body.String())
	}

	var reportEnv importReportEnvelope
	if err := json.Unmarshal(reportRes.Body.Bytes(), &reportEnv); err != nil {
		t.Fatalf("decode report response: %v", err)
	}
	if reportEnv.Report == nil {
		t.Fatal("expected import report for finalized HY3 import")
	}
	if reportEnv.Report.ConflictRows == 0 {
		t.Fatal("expected finalized HY3 import report to record conflict rows")
	}
}

func TestSensitiveRouteRBACMatrix(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, nil, nil, nil, billing.NewService(), nil, nil, nil, nil, nil, nil, nil, nil)

	tokens := map[string]string{
		"owner":     issueSessionTokenForUser(t, router, "owner-user", "org-rbac", []string{"owner"}, true),
		"admin":     issueSessionTokenForUser(t, router, "admin-user", "org-rbac", []string{"admin"}, true),
		"coach":     issueSessionTokenForUser(t, router, "coach-user", "org-rbac", []string{"coach"}, false),
		"billing":   issueSessionTokenForUser(t, router, "billing-user", "org-rbac", []string{"billing"}, true),
		"parent":    issueSessionTokenForUser(t, router, "parent-user", "org-rbac", []string{"parent"}, false),
		"swimmer":   issueSessionTokenForUser(t, router, "swimmer-user", "org-rbac", []string{"swimmer"}, false),
		"read-only": issueSessionTokenForUser(t, router, "readonly-user", "org-rbac", []string{"read-only"}, false),
	}

	type routeCase struct {
		name         string
		method       string
		path         string
		body         string
		allowedRoles map[string]bool
	}

	cases := []routeCase{
		{
			name:   "import upload url",
			method: http.MethodPost,
			path:   "/api/v1/imports/upload-url",
			body:   `{"filename":"rbac.sd3","file_type":"sd3","size_bytes":1024,"mime_type":"application/octet-stream"}`,
			allowedRoles: map[string]bool{
				"owner": true, "admin": true, "coach": true,
			},
		},
		{
			name:   "invoice creation",
			method: http.MethodPost,
			path:   "/api/v1/invoices",
			body:   `{"household_id":"hh-rbac","invoice_number":"INV-RBAC-1"}`,
			allowedRoles: map[string]bool{
				"owner": true, "admin": true, "billing": true,
			},
		},
		{
			name:   "ledger listing",
			method: http.MethodGet,
			path:   "/api/v1/ledger-events",
			allowedRoles: map[string]bool{
				"owner": true, "admin": true, "billing": true,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for role, token := range tokens {
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				if tc.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-Org-Id", "org-rbac")
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)

				allowed := tc.allowedRoles[role]
				if allowed && rr.Code == http.StatusForbidden {
					t.Fatalf("%s should be allowed on %s, got 403 body=%s", role, tc.name, rr.Body.String())
				}
				if !allowed && rr.Code != http.StatusForbidden {
					t.Fatalf("%s should be forbidden on %s, got %d body=%s", role, tc.name, rr.Code, rr.Body.String())
				}
			}
		})
	}
}

func TestParentAndSwimmerPortalReadMatrix(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetsSvc := meets.NewService()
	perfSvc := performance.NewService()
	commsSvc := comms.NewService()
	trainingSvc := training.NewService()
	router := NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil,
		memberSvc,
		groupSvc,
		meetsSvc,
		nil,
		nil,
		perfSvc,
		nil,
		nil,
		commsSvc,
		nil,
		nil,
		nil,
		trainingSvc,
		nil,
		nil,
	)

	adminToken := issueSessionTokenForUser(t, router, "admin-portal-matrix", "org-portal", []string{"admin"}, true)
	parentToken := issueSessionTokenForUser(t, router, "parent-portal-matrix", "org-portal", []string{"parent"}, false)

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-portal",
		"household_name": "Portal Family",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-portal")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("create parent household: expected %d, got %d body=%s", http.StatusCreated, createHouseholdRes.Code, createHouseholdRes.Body.String())
	}
	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-portal",
		"household_id":  household.Household.ID,
		"first_name":    "Riley",
		"last_name":     "Portal",
		"date_of_birth": "2012-05-17",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-portal")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+adminToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("create swimmer: expected %d, got %d body=%s", http.StatusCreated, createSwimmerRes.Code, createSwimmerRes.Body.String())
	}
	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("decode swimmer response: %v", err)
	}

	groupSvc.SeedSwimmer("org-portal", swimmer.Swimmer.ID, household.Household.ID, "2012-05-17", "female", "active")
	groupSvc.SeedParentHousehold("parent-portal-matrix", household.Household.ID)
	meetsSvc.SeedSwimmer("org-portal", swimmer.Swimmer.ID, household.Household.ID)
	meetsSvc.SeedParentHousehold("parent-portal-matrix", household.Household.ID)
	swimmerToken := issueSessionTokenWithHousehold(t, router, "swimmer-portal-matrix", "org-portal", household.Household.ID, []string{"swimmer"}, false)

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "portal-group",
		"name": "Portal Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-portal")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("create group: expected %d, got %d body=%s", http.StatusCreated, createGroupRes.Code, createGroupRes.Body.String())
	}

	standardReq := httptest.NewRequest(http.MethodPost, "/api/v1/time-standards", strings.NewReader(minimalTimeStandardBody("BB")))
	standardReq.Header.Set("Content-Type", "application/json")
	standardReq.Header.Set("X-Org-Id", "org-portal")
	standardReq.Header.Set("Authorization", "Bearer "+adminToken)
	standardRes := httptest.NewRecorder()
	router.ServeHTTP(standardRes, standardReq)
	if standardRes.Code != http.StatusCreated {
		t.Fatalf("create time standard: expected %d, got %d body=%s", http.StatusCreated, standardRes.Code, standardRes.Body.String())
	}
	var standardEnv timeStandardEnvelope
	if err := json.Unmarshal(standardRes.Body.Bytes(), &standardEnv); err != nil {
		t.Fatalf("decode time standard response: %v", err)
	}

	type routeCheck struct {
		name   string
		path   string
		token  string
		status int
	}

	checks := []routeCheck{
		{name: "parent households", path: "/api/v1/members/households", token: parentToken, status: http.StatusOK},
		{name: "parent swimmers", path: "/api/v1/members/swimmers", token: parentToken, status: http.StatusOK},
		{name: "parent swimmer groups", path: "/api/v1/members/swimmer-groups", token: parentToken, status: http.StatusOK},
		{name: "parent registrations", path: "/api/v1/registrations", token: parentToken, status: http.StatusOK},
		{name: "parent groups", path: "/api/v1/groups", token: parentToken, status: http.StatusOK},
		{name: "parent meets", path: "/api/v1/meets", token: parentToken, status: http.StatusOK},
		{name: "parent practices", path: "/api/v1/practices", token: parentToken, status: http.StatusOK},
		{name: "parent announcements", path: "/api/v1/announcements?status=published", token: parentToken, status: http.StatusOK},
		{name: "swimmer swimmers", path: "/api/v1/members/swimmers", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer swimmer groups", path: "/api/v1/members/swimmer-groups", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer groups", path: "/api/v1/groups", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer meets", path: "/api/v1/meets", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer practices", path: "/api/v1/practices", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer announcements", path: "/api/v1/announcements?status=published", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer best times", path: "/api/v1/swimmers/" + swimmer.Swimmer.ID + "/best-times", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer progression", path: "/api/v1/swimmers/" + swimmer.Swimmer.ID + "/progression", token: swimmerToken, status: http.StatusOK},
		{name: "swimmer cut qualification", path: "/api/v1/swimmers/" + swimmer.Swimmer.ID + "/cut-qualification?standard_id=" + standardEnv.TimeStandard.ID, token: swimmerToken, status: http.StatusOK},
	}

	for _, check := range checks {
		req := httptest.NewRequest(http.MethodGet, check.path, nil)
		req.Header.Set("X-Org-Id", "org-portal")
		req.Header.Set("Authorization", "Bearer "+check.token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != check.status {
			t.Fatalf("%s: expected %d, got %d body=%s", check.name, check.status, rr.Code, rr.Body.String())
		}
	}

	foreignHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-portal",
		"household_name": "Foreign Family",
	}))
	foreignHouseholdReq.Header.Set("Content-Type", "application/json")
	foreignHouseholdReq.Header.Set("X-Org-Id", "org-portal")
	foreignHouseholdReq.Header.Set("Authorization", "Bearer "+adminToken)
	foreignHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(foreignHouseholdRes, foreignHouseholdReq)
	if foreignHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("create foreign household: expected %d, got %d body=%s", http.StatusCreated, foreignHouseholdRes.Code, foreignHouseholdRes.Body.String())
	}
	var foreignHousehold householdEnvelope
	if err := json.Unmarshal(foreignHouseholdRes.Body.Bytes(), &foreignHousehold); err != nil {
		t.Fatalf("decode foreign household response: %v", err)
	}

	foreignSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-portal",
		"household_id":  foreignHousehold.Household.ID,
		"first_name":    "Casey",
		"last_name":     "Foreign",
		"date_of_birth": "2011-02-01",
		"gender":        "male",
	}))
	foreignSwimmerReq.Header.Set("Content-Type", "application/json")
	foreignSwimmerReq.Header.Set("X-Org-Id", "org-portal")
	foreignSwimmerReq.Header.Set("Authorization", "Bearer "+adminToken)
	foreignSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(foreignSwimmerRes, foreignSwimmerReq)
	if foreignSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("create foreign swimmer: expected %d, got %d body=%s", http.StatusCreated, foreignSwimmerRes.Code, foreignSwimmerRes.Body.String())
	}
	var foreignSwimmer swimmerEnvelope
	if err := json.Unmarshal(foreignSwimmerRes.Body.Bytes(), &foreignSwimmer); err != nil {
		t.Fatalf("decode foreign swimmer response: %v", err)
	}

	foreignBestTimesReq := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/"+foreignSwimmer.Swimmer.ID+"/best-times", nil)
	foreignBestTimesReq.Header.Set("X-Org-Id", "org-portal")
	foreignBestTimesReq.Header.Set("Authorization", "Bearer "+swimmerToken)
	foreignBestTimesRes := httptest.NewRecorder()
	router.ServeHTTP(foreignBestTimesRes, foreignBestTimesReq)
	if foreignBestTimesRes.Code != http.StatusForbidden {
		t.Fatalf("expected swimmer foreign best-times to be forbidden, got %d body=%s", foreignBestTimesRes.Code, foreignBestTimesRes.Body.String())
	}
}

func TestP0RegistrationToPaymentFlow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	billingSvc := billing.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, memberSvc, groupSvc, nil, nil, nil, nil, billingSvc, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-p0-reg", []string{"admin"}, true)
	parentToken := issueSessionTokenForUser(t, router, "parent-user", "org-p0-reg", []string{"parent"}, false)
	billingToken := issueSessionTokenForUser(t, router, "billing-user", "org-p0-reg", []string{"billing"}, true)

	createGroupReq := httptest.NewRequest(http.MethodPost, "/api/v1/groups", encodeJSONBody(t, map[string]any{
		"code": "gold",
		"name": "Gold Group",
	}))
	createGroupReq.Header.Set("Content-Type", "application/json")
	createGroupReq.Header.Set("X-Org-Id", "org-p0-reg")
	createGroupReq.Header.Set("Authorization", "Bearer "+adminToken)
	createGroupRes := httptest.NewRecorder()
	router.ServeHTTP(createGroupRes, createGroupReq)
	if createGroupRes.Code != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d body=%s", createGroupRes.Code, createGroupRes.Body.String())
	}
	var groupEnv groupEnvelope
	if err := json.Unmarshal(createGroupRes.Body.Bytes(), &groupEnv); err != nil {
		t.Fatalf("decode group response: %v", err)
	}

	createHouseholdReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/households", encodeJSONBody(t, map[string]any{
		"org_id":         "org-p0-reg",
		"household_name": "Lane Family",
		"primary_email":  "lane@example.com",
	}))
	createHouseholdReq.Header.Set("Content-Type", "application/json")
	createHouseholdReq.Header.Set("X-Org-Id", "org-p0-reg")
	createHouseholdReq.Header.Set("Authorization", "Bearer "+parentToken)
	createHouseholdRes := httptest.NewRecorder()
	router.ServeHTTP(createHouseholdRes, createHouseholdReq)
	if createHouseholdRes.Code != http.StatusCreated {
		t.Fatalf("create household: expected 201, got %d body=%s", createHouseholdRes.Code, createHouseholdRes.Body.String())
	}
	var household householdEnvelope
	if err := json.Unmarshal(createHouseholdRes.Body.Bytes(), &household); err != nil {
		t.Fatalf("decode household response: %v", err)
	}

	createSwimmerReq := httptest.NewRequest(http.MethodPost, "/api/v1/members/swimmers", encodeJSONBody(t, map[string]any{
		"org_id":        "org-p0-reg",
		"household_id":  household.Household.ID,
		"first_name":    "Avery",
		"last_name":     "Lane",
		"date_of_birth": "2012-07-04",
		"gender":        "female",
	}))
	createSwimmerReq.Header.Set("Content-Type", "application/json")
	createSwimmerReq.Header.Set("X-Org-Id", "org-p0-reg")
	createSwimmerReq.Header.Set("Authorization", "Bearer "+parentToken)
	createSwimmerRes := httptest.NewRecorder()
	router.ServeHTTP(createSwimmerRes, createSwimmerReq)
	if createSwimmerRes.Code != http.StatusCreated {
		t.Fatalf("create swimmer: expected 201, got %d body=%s", createSwimmerRes.Code, createSwimmerRes.Body.String())
	}
	var swimmer swimmerEnvelope
	if err := json.Unmarshal(createSwimmerRes.Body.Bytes(), &swimmer); err != nil {
		t.Fatalf("decode swimmer response: %v", err)
	}
	groupSvc.SeedSwimmer("org-p0-reg", swimmer.Swimmer.ID, household.Household.ID, "2012-07-04", "female", "active")
	groupSvc.SeedParentHousehold("parent-user", household.Household.ID)

	createRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations", encodeJSONBody(t, map[string]any{
		"household_id": household.Household.ID,
		"swimmer_id":   swimmer.Swimmer.ID,
		"group_id":     groupEnv.Group.ID,
		"season":       "2026",
	}))
	createRegReq.Header.Set("Content-Type", "application/json")
	createRegReq.Header.Set("X-Org-Id", "org-p0-reg")
	createRegReq.Header.Set("Authorization", "Bearer "+parentToken)
	createRegRes := httptest.NewRecorder()
	router.ServeHTTP(createRegRes, createRegReq)
	if createRegRes.Code != http.StatusCreated {
		t.Fatalf("create registration: expected 201, got %d body=%s", createRegRes.Code, createRegRes.Body.String())
	}
	var regEnv registrationEnvelope
	if err := json.Unmarshal(createRegRes.Body.Bytes(), &regEnv); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}

	activateReq := httptest.NewRequest(http.MethodPost, "/api/v1/registrations/"+regEnv.Registration.ID+"/transition", encodeJSONBody(t, map[string]any{
		"status": "active",
	}))
	activateReq.Header.Set("Content-Type", "application/json")
	activateReq.Header.Set("X-Org-Id", "org-p0-reg")
	activateReq.Header.Set("Authorization", "Bearer "+adminToken)
	activateRes := httptest.NewRecorder()
	router.ServeHTTP(activateRes, activateReq)
	if activateRes.Code != http.StatusOK {
		t.Fatalf("activate registration: expected 200, got %d body=%s", activateRes.Code, activateRes.Body.String())
	}

	invoice := createInvoiceForHousehold(t, router, billingToken, "org-p0-reg", household.Household.ID, "INV-P0-REG")
	addItem(t, router, billingToken, "org-p0-reg", invoice.Invoice.ID, 9000)
	issueInvoiceReq(t, router, billingToken, "org-p0-reg", invoice.Invoice.ID)
	recordPaymentReq(t, router, billingToken, "org-p0-reg", invoice.Invoice.ID, 9000)

	principal := billingAdminPrincipalForFlow("org-p0-reg")
	stored, err := billingSvc.GetInvoice("org-p0-reg", &principal, invoice.Invoice.ID)
	if err != nil {
		t.Fatalf("get stored invoice: %v", err)
	}
	if stored.Status != "paid" || stored.AmountDueCents != 0 {
		t.Fatalf("expected paid invoice with zero amount due, got status=%q amount_due=%d", stored.Status, stored.AmountDueCents)
	}
}

func TestP0MeetCommitToSD3ExportFlow(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	meetsSvc := meets.NewService()
	exportsSvc := exports.NewService()
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, meetsSvc, nil, exportsSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	adminToken := issueSessionTokenForUser(t, router, "admin-user", "org-p0-meet", []string{"admin"}, true)
	parentToken := issueSessionTokenWithHousehold(t, router, "parent-user", "org-p0-meet", "household-p0", []string{"parent"}, false)

	meetsSvc.SeedSwimmer("org-p0-meet", "swimmer-p0", "household-p0")
	meetsSvc.SeedParentHousehold("parent-user", "household-p0")

	meetID := createMeet(t, router, "org-p0-meet", adminToken, "P0 Meet")
	publishMeet(t, router, "org-p0-meet", adminToken, meetID)
	eventID := createMeetEvent(t, router, "org-p0-meet", adminToken, meetID, "1")

	publishCommitmentsReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitment-request", nil)
	publishCommitmentsReq.Header.Set("X-Org-Id", "org-p0-meet")
	publishCommitmentsReq.Header.Set("Authorization", "Bearer "+adminToken)
	publishCommitmentsRes := httptest.NewRecorder()
	router.ServeHTTP(publishCommitmentsRes, publishCommitmentsReq)
	if publishCommitmentsRes.Code != http.StatusOK {
		t.Fatalf("publish commitment request: expected 200, got %d body=%s", publishCommitmentsRes.Code, publishCommitmentsRes.Body.String())
	}
	var commitments commitmentRequestEnvelope
	if err := json.Unmarshal(publishCommitmentsRes.Body.Bytes(), &commitments); err != nil {
		t.Fatalf("decode commitments response: %v", err)
	}
	if len(commitments.Commitments) != 1 {
		t.Fatalf("expected 1 commitment, got %d", len(commitments.Commitments))
	}

	respondReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+commitments.Commitments[0].ID+"/respond", encodeJSONBody(t, map[string]any{
		"response": "committed_yes",
	}))
	respondReq.Header.Set("Content-Type", "application/json")
	respondReq.Header.Set("X-Org-Id", "org-p0-meet")
	respondReq.Header.Set("Authorization", "Bearer "+parentToken)
	respondRes := httptest.NewRecorder()
	router.ServeHTTP(respondRes, respondReq)
	if respondRes.Code != http.StatusOK {
		t.Fatalf("respond commitment: expected 200, got %d body=%s", respondRes.Code, respondRes.Body.String())
	}

	selectReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/commitments/"+commitments.Commitments[0].ID+"/event-selections", encodeJSONBody(t, map[string]any{
		"meet_event_id": eventID,
	}))
	selectReq.Header.Set("Content-Type", "application/json")
	selectReq.Header.Set("X-Org-Id", "org-p0-meet")
	selectReq.Header.Set("Authorization", "Bearer "+parentToken)
	selectRes := httptest.NewRecorder()
	router.ServeHTTP(selectRes, selectReq)
	if selectRes.Code != http.StatusCreated {
		t.Fatalf("select event: expected 201, got %d body=%s", selectRes.Code, selectRes.Body.String())
	}

	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/lock-commitments", nil)
	lockReq.Header.Set("X-Org-Id", "org-p0-meet")
	lockReq.Header.Set("Authorization", "Bearer "+adminToken)
	lockRes := httptest.NewRecorder()
	router.ServeHTTP(lockRes, lockReq)
	if lockRes.Code != http.StatusOK {
		t.Fatalf("lock commitments: expected 200, got %d body=%s", lockRes.Code, lockRes.Body.String())
	}

	proposeReq := httptest.NewRequest(http.MethodPost, "/api/v1/meets/"+meetID+"/entries", encodeJSONBody(t, map[string]any{
		"swimmer_id":    "swimmer-p0",
		"meet_event_id": eventID,
		"commitment_id": commitments.Commitments[0].ID,
	}))
	proposeReq.Header.Set("Content-Type", "application/json")
	proposeReq.Header.Set("X-Org-Id", "org-p0-meet")
	proposeReq.Header.Set("Authorization", "Bearer "+adminToken)
	proposeRes := httptest.NewRecorder()
	router.ServeHTTP(proposeRes, proposeReq)
	if proposeRes.Code != http.StatusCreated {
		t.Fatalf("propose entry: expected 201, got %d body=%s", proposeRes.Code, proposeRes.Body.String())
	}

	exportRes := doGenerateExport(t, router, "org-p0-meet", adminToken, meetID, "event_report")
	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+exportRes.Export.ID+"/download", nil)
	downloadReq.Header.Set("X-Org-Id", "org-p0-meet")
	downloadReq.Header.Set("Authorization", "Bearer "+adminToken)
	downloadRes := httptest.NewRecorder()
	router.ServeHTTP(downloadRes, downloadReq)
	if downloadRes.Code != http.StatusOK {
		t.Fatalf("download report: expected 200, got %d body=%s", downloadRes.Code, downloadRes.Body.String())
	}
	if !strings.HasPrefix(downloadRes.Body.String(), "%PDF-") {
		t.Fatalf("expected PDF report payload, got %q", downloadRes.Body.String()[:min(16, downloadRes.Body.Len())])
	}
}

func TestP0ResultsImportToBestTimeUpdateFlow(t *testing.T) {
	t.Parallel()

	perfSvc := performance.NewService()
	meetsSvc := meets.NewService()
	perfSvc.SetEntrySeedTimeUpdater(meetsSvc)
	router := newPerfAndMeetsRouter(t, perfSvc, meetsSvc)
	token := issueSessionTokenForUser(t, router, "coach-user", "org-p0-results", []string{"coach"}, false)

	entrySeedMs := 61500
	meetsSvc.SeedEntry("org-p0-results", "swimmer-p0-results", "meet-future", "freestyle", "SCY", 100, &entrySeedMs)

	resultReq := httptest.NewRequest(http.MethodPost, "/api/v1/results", strings.NewReader(`{
		"swimmer_id":"swimmer-p0-results",
		"distance":100,
		"stroke":"free",
		"course":"SCY",
		"gender":"F",
		"fin_time_ms":58900,
		"is_official":true
	}`))
	resultReq.Header.Set("Content-Type", "application/json")
	resultReq.Header.Set("X-Org-Id", "org-p0-results")
	resultReq.Header.Set("Authorization", "Bearer "+token)
	resultRes := httptest.NewRecorder()
	router.ServeHTTP(resultRes, resultReq)
	if resultRes.Code != http.StatusCreated {
		t.Fatalf("upsert result: expected 201, got %d body=%s", resultRes.Code, resultRes.Body.String())
	}

	bestTimesReq := httptest.NewRequest(http.MethodGet, "/api/v1/swimmers/swimmer-p0-results/best-times?course=SCY", nil)
	bestTimesReq.Header.Set("X-Org-Id", "org-p0-results")
	bestTimesReq.Header.Set("Authorization", "Bearer "+token)
	bestTimesRes := httptest.NewRecorder()
	router.ServeHTTP(bestTimesRes, bestTimesReq)
	if bestTimesRes.Code != http.StatusOK {
		t.Fatalf("list best times: expected 200, got %d body=%s", bestTimesRes.Code, bestTimesRes.Body.String())
	}
	if !strings.Contains(bestTimesRes.Body.String(), "58900") {
		t.Fatalf("expected best-time response to contain 58900ms, got %s", bestTimesRes.Body.String())
	}
}

func TestDuplicateImportFinalizeChaos(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewStubStorage("")
	importsService := imports.NewServiceWithStorage(store)
	router := NewRouterWithServices(Config{}, logger, nil, nil, nil, nil, importsService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	adminToken := issueSessionToken(t, router, "org-chaos", []string{"admin"}, false)
	importID := requestUploadURL(t, router, "org-chaos", adminToken, "duplicate.sd3", "sd3")
	seedUploadedImport(t, store, "org-chaos", importID, "duplicate.sd3", "application/octet-stream", loadImportFixture(t, "valid.sd3"))

	var wg sync.WaitGroup
	results := make(chan int, 2)
	finalize := func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+importID+"/finalize", nil)
		req.Header.Set("X-Org-Id", "org-chaos")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		results <- rr.Code
	}

	wg.Add(2)
	go finalize()
	go finalize()
	wg.Wait()
	close(results)

	var success, conflict int
	for code := range results {
		if code == http.StatusOK {
			success++
		}
		if code == http.StatusConflict {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", success, conflict)
	}
}

func billingAdminPrincipalForFlow(orgID string) appmiddleware.Principal {
	return appmiddleware.Principal{
		UserID: "billing-user",
		OrgID:  orgID,
		Roles:  []string{"admin"},
	}
}
