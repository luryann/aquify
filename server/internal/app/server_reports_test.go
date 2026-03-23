package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func newReportsRouter(t *testing.T, memberSvc *members.Service, groupSvc *groups.Service, meetSvc *meets.Service, billingSvc *billing.Service, auditSvc *audit.Service) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithServices(
		Config{Auth: AuthConfig{EnableDevStubTokens: true}},
		logger,
		nil,
		memberSvc,
		groupSvc,
		meetSvc,
		nil,
		nil,
		nil,
		billingSvc,
		nil,
		nil,
		auditSvc,
		nil,
		nil,
		nil,
	)
}

func seedReportsFixture(t *testing.T, memberSvc *members.Service, groupSvc *groups.Service, meetSvc *meets.Service, billingSvc *billing.Service) (orgID string, principal *appmiddleware.Principal, household members.Household, swimmer members.Swimmer, group groups.Group, meet meets.Meet, invoice *billing.Invoice) {
	t.Helper()

	orgID = "org-reports"
	principal = &appmiddleware.Principal{UserID: "admin-user", OrgID: orgID, Roles: []string{"admin"}}

	var err error
	household, err = memberSvc.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:         orgID,
		HouseholdName: "Garcia",
		PrimaryEmail:  "garcia@example.com",
		Status:        "active",
	})
	if err != nil {
		t.Fatalf("create household: %v", err)
	}
	if _, err := memberSvc.AddContact(principal, orgID, household.ID, members.AddContactInput{
		Relationship: "parent",
		FirstName:    "Elena",
		LastName:     "Garcia",
		Email:        "elena@example.com",
		IsPrimary:    true,
	}); err != nil {
		t.Fatalf("add contact: %v", err)
	}
	swimmer, _, err = memberSvc.CreateSwimmer(principal, orgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Maya",
		LastName:           "Garcia",
		PreferredName:      "May",
		DateOfBirth:        "2012-04-10",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("create swimmer: %v", err)
	}

	group, _, err = groupSvc.CreateGroup(principal, orgID, groups.CreateGroupInput{
		Code: "senior",
		Name: "Senior Elite",
		Type: "competitive",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := memberSvc.AssignSwimmerToGroup(principal, orgID, swimmer.ID, members.AssignSwimmerGroupInput{
		GroupCode: group.Code,
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("assign swimmer group: %v", err)
	}
	groupSvc.SeedSwimmer(orgID, swimmer.ID, household.ID, swimmer.DateOfBirth, swimmer.Gender, swimmer.RegistrationStatus)
	reg, err := groupSvc.CreateRegistration(principal, orgID, groups.CreateRegistrationInput{
		HouseholdID:      household.ID,
		SwimmerID:        swimmer.ID,
		GroupID:          group.ID,
		Season:           "2026",
		RegistrationDate: "2026-03-01",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, _, err := groupSvc.TransitionRegistrationStatus(principal, orgID, reg.ID, "active"); err != nil {
		t.Fatalf("transition registration: %v", err)
	}
	if _, err := groupSvc.AcceptWaiver(principal, orgID, reg.ID, groups.AcceptWaiverInput{WaiverType: "liability"}); err != nil {
		t.Fatalf("accept waiver: %v", err)
	}

	meet, _, err = meetSvc.CreateMeet(principal, orgID, meets.CreateMeetInput{
		Name:          "Spring Invite",
		MeetType:      "invitational",
		Course:        "SCY",
		StartDate:     "2026-04-15",
		EndDate:       "2026-04-17",
		EntryDeadline: "2026-04-05",
	})
	if err != nil {
		t.Fatalf("create meet: %v", err)
	}

	invoice, err = billingSvc.CreateInvoice(orgID, principal.UserID, principal, billing.CreateInvoiceInput{
		HouseholdID:   household.ID,
		InvoiceNumber: "INV-1001",
		DueDate:       "2026-03-05",
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := billingSvc.AddInvoiceItem(orgID, principal.UserID, principal, invoice.ID, billing.AddInvoiceItemInput{
		Description:     "Spring Invite entry fees",
		Quantity:        2,
		UnitAmountCents: 1250,
		Category:        "meet_fee",
		SwimmerID:       swimmer.ID,
		MeetID:          meet.ID,
	}); err != nil {
		t.Fatalf("add meet fee item: %v", err)
	}
	if _, err := billingSvc.AddInvoiceItem(orgID, principal.UserID, principal, invoice.ID, billing.AddInvoiceItemInput{
		Description:     "April dues",
		Quantity:        1,
		UnitAmountCents: 9000,
		Category:        "program_fee",
		SwimmerID:       swimmer.ID,
	}); err != nil {
		t.Fatalf("add program fee item: %v", err)
	}
	if _, _, err := billingSvc.IssueInvoice(orgID, principal.UserID, principal, invoice.ID); err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	payment, _, err := billingSvc.RecordPayment(orgID, principal.UserID, principal, invoice.ID, billing.RecordPaymentInput{
		AmountCents:           5000,
		Method:                "stripe",
		StripePaymentIntentID: "pi_reports_001",
	})
	if err != nil {
		t.Fatalf("record payment: %v", err)
	}
	if _, _, err := billingSvc.CreateRefund(orgID, principal.UserID, principal, invoice.ID, billing.CreateRefundInput{
		PaymentID:   payment.ID,
		AmountCents: 1000,
		Reason:      "lane fee adjustment",
	}); err != nil {
		t.Fatalf("create refund: %v", err)
	}

	return orgID, principal, household, swimmer, group, meet, invoice
}

func TestReportsEndpointsGenerateJSONAndAuditEvents(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()
	auditSvc := audit.NewService()

	orgID, _, household, swimmer, _, meet, invoice := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, auditSvc)
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "membership", path: "/api/v1/reports/membership-status", want: swimmer.ID},
		{name: "meet fees", path: "/api/v1/reports/meet-fees", want: meet.ID},
		{name: "invoice aging", path: "/api/v1/reports/invoice-aging?as_of=2026-03-18", want: invoice.InvoiceNumber},
		{name: "payment reconciliation", path: "/api/v1/reports/payment-reconciliation?stripe_reported_cents=5000", want: "stripe_reported_cents"},
		{name: "registration", path: "/api/v1/reports/registration-status", want: household.HouseholdName},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Org-Id", orgID)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("expected response to contain %q, got %s", tc.want, rr.Body.String())
			}
		})
	}

	events, err := auditSvc.Query(audit.Filter{OrgID: orgID})
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	found := map[string]bool{}
	for _, event := range events {
		found[event.Action] = true
	}
	for _, action := range []string{
		audit.ActionReportMembershipStatusRun,
		audit.ActionReportMeetFeesRun,
		audit.ActionReportInvoiceAgingRun,
		audit.ActionReportPaymentReconciliationRun,
		audit.ActionReportRegistrationStatusRun,
	} {
		if !found[action] {
			t.Fatalf("expected audit action %q to be emitted", action)
		}
	}
}

func TestReportsEndpointsCSVExport(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()

	orgID, _, _, _, _, _, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"billing"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/invoice-aging?format=csv&as_of=2026-03-18", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Format   string `json:"format"`
		Delivery string `json:"delivery"`
		Artifact struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			StorageKey  string `json:"storage_key"`
			DownloadURL string `json:"download_url"`
			SizeBytes   int    `json:"size_bytes"`
		} `json:"artifact"`
		Report struct {
			RowCount int `json:"row_count"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Format != "csv" {
		t.Fatalf("format = %q, want csv", payload.Format)
	}
	if payload.Delivery != "artifact" {
		t.Fatalf("delivery = %q, want artifact", payload.Delivery)
	}
	if payload.Artifact.ID == "" || payload.Artifact.StorageKey == "" || payload.Artifact.DownloadURL == "" {
		t.Fatalf("expected durable artifact metadata, got %#v", payload.Artifact)
	}
	if !strings.Contains(payload.Artifact.Filename, "invoice-aging") {
		t.Fatalf("filename = %q, want invoice-aging csv", payload.Artifact.Filename)
	}
	if !strings.Contains(payload.Artifact.ContentType, "text/csv") {
		t.Fatalf("content type = %q, want csv", payload.Artifact.ContentType)
	}
	if payload.Artifact.SizeBytes <= 0 {
		t.Fatalf("size_bytes = %d, want > 0", payload.Artifact.SizeBytes)
	}
	if payload.Report.RowCount == 0 {
		t.Fatalf("expected report row_count in response")
	}
}

func TestReportsEndpointsRejectParentAccess(t *testing.T) {
	router := newReportsRouter(t, members.NewService(), groups.NewService(), meets.NewService(), billing.NewService(), audit.NewService())
	token := issueSessionToken(t, router, "org-reports", []string{"parent"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/membership-status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", "org-reports")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPaymentReconciliationReportIncludesStripeTotals(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()

	orgID, _, _, _, _, _, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/payment-reconciliation?stripe_reported_cents=7000", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Report struct {
			StripeReportedCents  *int `json:"stripe_reported_cents"`
			StripeMismatchCents  *int `json:"stripe_mismatch_cents"`
			UnreconciledRowCount int  `json:"unreconciled_row_count"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Report.StripeReportedCents == nil || *payload.Report.StripeReportedCents != 7000 {
		t.Fatalf("expected stripe_reported_cents=7000, got %#v", payload.Report.StripeReportedCents)
	}
	if payload.Report.StripeMismatchCents == nil || *payload.Report.StripeMismatchCents == 0 {
		t.Fatalf("expected a non-zero stripe mismatch")
	}
}

func TestPaymentReconciliationReportOmitsStripeTotalsWhenNotProvided(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()

	orgID, _, _, _, _, _, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/payment-reconciliation", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Report struct {
			StripeReportedCents *int `json:"stripe_reported_cents"`
			StripeMismatchCents *int `json:"stripe_mismatch_cents"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Report.StripeReportedCents != nil {
		t.Fatalf("expected stripe_reported_cents to be omitted when not provided")
	}
	if payload.Report.StripeMismatchCents != nil {
		t.Fatalf("expected stripe_mismatch_cents to be omitted when not provided")
	}
}

func TestPaymentReconciliationReportIncludesZeroStripeTotals(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()

	orgID, _, _, _, _, _, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/payment-reconciliation?stripe_reported_cents=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Report struct {
			StripeReportedCents  *int  `json:"stripe_reported_cents"`
			StripeMismatchCents  *int  `json:"stripe_mismatch_cents"`
			StripeAlertTriggered *bool `json:"stripe_alert_triggered"`
			PaymentTotalCents    int   `json:"payment_total_cents"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Report.StripeReportedCents == nil || *payload.Report.StripeReportedCents != 0 {
		t.Fatalf("expected explicit zero stripe total, got %#v", payload.Report.StripeReportedCents)
	}
	if payload.Report.StripeMismatchCents == nil || *payload.Report.StripeMismatchCents != payload.Report.PaymentTotalCents {
		t.Fatalf("expected mismatch to equal payment total cents %d, got %#v", payload.Report.PaymentTotalCents, payload.Report.StripeMismatchCents)
	}
	if payload.Report.StripeAlertTriggered == nil {
		t.Fatalf("expected stripe_alert_triggered to be present for explicit zero stripe total")
	}
}

func TestPaymentReconciliationReportRejectsInvalidStripeTotals(t *testing.T) {
	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	billingSvc := billing.NewService()

	orgID, _, _, _, _, _, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	tests := []string{
		"/api/v1/reports/payment-reconciliation?stripe_reported_cents=-1",
		"/api/v1/reports/payment-reconciliation?stripe_reported_cents=abc",
	}

	for _, path := range tests {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", orgID)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", path, rr.Code, rr.Body.String())
		}
	}
}
