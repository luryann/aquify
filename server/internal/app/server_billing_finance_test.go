package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
)

func financeRequest(t *testing.T, router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("X-Org-Id", "org-demo")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestBillingFinanceRoutesRBACAndAudit(t *testing.T) {
	t.Parallel()
	auditSvc := audit.NewService()
	router := newBillingRouterWithAudit(t, billing.NewService(), auditSvc)
	admin := issueSessionToken(t, router, "org-demo", []string{"admin"}, false)
	adminMFA := issueSessionToken(t, router, "org-demo", []string{"admin"}, true)
	coach := issueSessionToken(t, router, "org-demo", []string{"coach"}, true)
	parent := issueSessionTokenWithHousehold(t, router, "parent-1", "org-demo", "hh-own", []string{"parent"}, false)

	schedule := `{"name":"Gold dues","kind":"dues","group_code":"GOLD","frequency":"monthly","amount_cents":12000,"start_date":"2026-09-01","proration":"daily"}`
	for _, tok := range []string{coach, parent} {
		if rr := financeRequest(t, router, http.MethodPost, "/api/v1/billing/fee-schedules", tok, schedule); rr.Code != http.StatusForbidden {
			t.Fatalf("non-admin create schedule: expected 403, got %d", rr.Code)
		}
		if rr := financeRequest(t, router, http.MethodGet, "/api/v1/billing/reports/aging", tok, ""); rr.Code != http.StatusForbidden {
			t.Fatalf("non-admin aging report: expected 403, got %d", rr.Code)
		}
		if rr := financeRequest(t, router, http.MethodPost, "/api/v1/billing/runs/preview", tok, `{"as_of":"2026-11-01"}`); rr.Code != http.StatusForbidden {
			t.Fatalf("non-admin run preview: expected 403, got %d", rr.Code)
		}
	}
	rr := financeRequest(t, router, http.MethodPost, "/api/v1/billing/fee-schedules", admin, schedule)
	if rr.Code != http.StatusCreated {
		t.Fatalf("admin create schedule: %d %s", rr.Code, rr.Body.String())
	}
	if rr := financeRequest(t, router, http.MethodPost, "/api/v1/billing/fee-schedules", admin, `{"name":"Bad","kind":"dues","frequency":"monthly","amount_cents":100,"start_date":"2026-09-01"}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dues without group must be 422, got %d", rr.Code)
	}
	if rr := financeRequest(t, router, http.MethodPost, "/api/v1/billing/runs", admin, `{"as_of":"2026-11-01"}`); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "nothing_to_bill") {
		t.Fatalf("empty roster run: expected 422 nothing_to_bill, got %d %s", rr.Code, rr.Body.String())
	}

	// Manual credit requires admin + MFA.
	credit := `{"direction":"credit","amount_cents":2500,"reason":"Fundraiser"}`
	if rr := financeRequest(t, router, http.MethodPost, "/api/v1/households/hh-own/credits", admin, credit); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "mfa_required") {
		t.Fatalf("credit without MFA: %d %s", rr.Code, rr.Body.String())
	}
	if rr := financeRequest(t, router, http.MethodPost, "/api/v1/households/hh-own/credits", parent, credit); rr.Code != http.StatusForbidden {
		t.Fatalf("parent credit: expected 403, got %d", rr.Code)
	}
	if rr := financeRequest(t, router, http.MethodPost, "/api/v1/households/hh-own/credits", adminMFA, credit); rr.Code != http.StatusCreated {
		t.Fatalf("admin MFA credit: %d %s", rr.Code, rr.Body.String())
	}

	// Parent sees own statement/credit; never another household; coach never.
	rr = financeRequest(t, router, http.MethodGet, "/api/v1/households/hh-own/statement", parent, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("parent own statement: %d %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Statement billing.Statement `json:"statement"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Statement.ClosingBalanceCents != -2500 || env.Statement.CreditBalanceCents != 2500 {
		t.Fatalf("statement after credit: %+v", env.Statement)
	}
	for _, path := range []string{"/api/v1/households/hh-other/statement", "/api/v1/households/hh-other/credits", "/api/v1/households/hh-other/upcoming-charges"} {
		if rr := financeRequest(t, router, http.MethodGet, path, parent, ""); rr.Code != http.StatusForbidden {
			t.Fatalf("parent foreign %s: expected 403, got %d", path, rr.Code)
		}
	}
	if rr := financeRequest(t, router, http.MethodGet, "/api/v1/households/hh-own/statement", coach, ""); rr.Code != http.StatusForbidden {
		t.Fatalf("coach statement: expected 403, got %d", rr.Code)
	}

	// CSV export is served as text/csv and audited.
	rr = financeRequest(t, router, http.MethodGet, "/api/v1/billing/reports/outstanding?format=csv", admin, "")
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rr.Body.String(), "Account credit") {
		t.Fatalf("outstanding csv: %d %q", rr.Code, rr.Header().Get("Content-Type"))
	}

	actions := map[string]bool{}
	events, _ := auditSvc.Query(audit.Filter{OrgID: "org-demo", Limit: 100})
	for _, ev := range events {
		actions[ev.Action] = true
	}
	for _, want := range []string{"billing.fee_schedule.create", "billing.credit.create", "billing.report.export"} {
		if !actions[want] {
			t.Fatalf("missing audit action %s (have %v)", want, actions)
		}
	}
}
