package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
)

type cov10gReportResp struct {
	Format   string `json:"format"`
	Delivery string `json:"delivery"`
	Artifact struct {
		Kind       string `json:"kind"`
		Filename   string `json:"filename"`
		SizeBytes  int    `json:"size_bytes"`
		StorageKey string `json:"storage_key"`
	} `json:"artifact"`
	Report struct {
		RowCount int `json:"row_count"`
	} `json:"report"`
}

func cov10gReportsGet(t *testing.T, router http.Handler, token, org, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", org)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestCov10gReportsCSVForEveryReport(t *testing.T) {
	memberSvc, groupSvc, meetSvc, billingSvc := members.NewService(), groups.NewService(), meets.NewService(), billing.NewService()
	auditSvc := audit.NewService()
	orgID, _, _, _, _, _, invoice := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, auditSvc)
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	cases := []struct {
		name, path, kind, filePrefix string
	}{
		{"membership", "/api/v1/reports/membership-status?format=csv", "membership_status", "membership-status-"},
		{"meet fees", "/api/v1/reports/meet-fees?format=csv", "meet_fees", "meet-fees-"},
		{"invoice aging", "/api/v1/reports/invoice-aging?format=csv&as_of=2026-03-18", "invoice_aging", "invoice-aging-"},
		{"invoice detail", "/api/v1/reports/invoice-detail?format=csv&invoice_id=" + invoice.ID, "invoice_detail", "invoice-" + invoice.InvoiceNumber + "-"},
		{"payment reconciliation", "/api/v1/reports/payment-reconciliation?format=csv&stripe_reported_cents=4000", "payment_reconciliation", "payment-reconciliation-"},
		{"registration", "/api/v1/reports/registration-status?format=csv", "registration_status", "registration-status-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := cov10gReportsGet(t, router, token, orgID, tc.path)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
			}
			var out cov10gReportResp
			if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Format != "csv" || out.Delivery != "artifact" || out.Artifact.Kind != tc.kind {
				t.Fatalf("unexpected envelope: %+v", out)
			}
			if !strings.HasPrefix(out.Artifact.Filename, tc.filePrefix) || !strings.HasSuffix(out.Artifact.Filename, ".csv") {
				t.Fatalf("filename %q, want prefix %q", out.Artifact.Filename, tc.filePrefix)
			}
			if !strings.Contains(out.Artifact.StorageKey, "reports/"+orgID+"/"+tc.kind+"/") {
				t.Fatalf("storage key %q not org/kind scoped", out.Artifact.StorageKey)
			}
			if out.Report.RowCount < 1 {
				t.Fatalf("fixture should yield rows, got %d", out.Report.RowCount)
			}
			// Header plus at least one row of data.
			if out.Artifact.SizeBytes < 60 {
				t.Fatalf("csv unexpectedly small: %d bytes", out.Artifact.SizeBytes)
			}
		})
	}

	events, err := auditSvc.Query(audit.Filter{OrgID: orgID})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]string{}
	for _, e := range events {
		seen[e.Action] = e.Metadata
	}
	if m := seen[audit.ActionReportInvoiceDetailRun]; m == nil || m["format"] != "csv" {
		t.Fatalf("invoice detail audit missing csv format: %v", m)
	}
	if m := seen[audit.ActionReportPaymentReconciliationRun]; m == nil || m["stripe_reported_cents"] != "4000" {
		t.Fatalf("reconciliation audit missing stripe total: %v", m)
	}
}

func TestCov10gInvoiceDetailReportJSONAndErrors(t *testing.T) {
	memberSvc, groupSvc, meetSvc, billingSvc := members.NewService(), groups.NewService(), meets.NewService(), billing.NewService()
	orgID, _, _, _, _, _, invoice := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	rr := cov10gReportsGet(t, router, token, orgID, "/api/v1/reports/invoice-detail?invoice_id="+invoice.ID)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), invoice.InvoiceNumber) {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	rr = cov10gReportsGet(t, router, token, orgID, "/api/v1/reports/invoice-detail")
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invoice_id_required") {
		t.Fatalf("missing id: status %d body %s", rr.Code, rr.Body.String())
	}

	rr = cov10gReportsGet(t, router, token, orgID, "/api/v1/reports/invoice-detail?invoice_id=does-not-exist")
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "invoice_not_found") {
		t.Fatalf("unknown id: status %d body %s", rr.Code, rr.Body.String())
	}

	rr = cov10gReportsGet(t, router, token, orgID, "/api/v1/reports/invoice-aging?as_of=03-18-2026")
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid_as_of") {
		t.Fatalf("bad as_of: status %d body %s", rr.Code, rr.Body.String())
	}

	parent := issueSessionToken(t, router, orgID, []string{"parent"}, false)
	rr = cov10gReportsGet(t, router, parent, orgID, "/api/v1/reports/invoice-detail?invoice_id="+invoice.ID)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("parent status %d, want 403", rr.Code)
	}
}

// A draft invoice has no issued_at, so its meet-fee row exports an empty
// timestamp cell instead of failing.
func TestCov10gMeetFeesCSVIncludesUnissuedInvoice(t *testing.T) {
	memberSvc, groupSvc, meetSvc, billingSvc := members.NewService(), groups.NewService(), meets.NewService(), billing.NewService()
	orgID, principal, household, swimmer, _, meet, _ := seedReportsFixture(t, memberSvc, groupSvc, meetSvc, billingSvc)
	router := newReportsRouter(t, memberSvc, groupSvc, meetSvc, billingSvc, audit.NewService())
	token := issueSessionToken(t, router, orgID, []string{"admin"}, false)

	count := func() int {
		rr := cov10gReportsGet(t, router, token, orgID, "/api/v1/reports/meet-fees?format=csv")
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		var out cov10gReportResp
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Report.RowCount
	}
	before := count()

	draft, err := billingSvc.CreateInvoice(orgID, principal.UserID, principal, billing.CreateInvoiceInput{
		HouseholdID: household.ID, InvoiceNumber: "INV-DRAFT-1", DueDate: "2026-05-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingSvc.AddInvoiceItem(orgID, principal.UserID, principal, draft.ID, billing.AddInvoiceItemInput{
		Description: "Late entry", Quantity: 1, UnitAmountCents: 700, Category: "meet_fee", SwimmerID: swimmer.ID, MeetID: meet.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if after := count(); after != before+1 {
		t.Fatalf("row count %d -> %d, want +1 for the unissued meet fee", before, after)
	}
}
