package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/billing"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov2xFinOrg = "org-cov2x-fin"

var cov2xFinTokens = tokenResolver{
	"admin":    {UserID: "u-admin", OrgID: cov2xFinOrg, Roles: []string{"admin"}},
	"adminMFA": {UserID: "u-admin-mfa", OrgID: cov2xFinOrg, Roles: []string{"admin"}, MFAAuthenticated: true},
	"coach":    {UserID: "u-coach", OrgID: cov2xFinOrg, Roles: []string{"coach"}},
	"parentA":  {UserID: "u-pa", OrgID: cov2xFinOrg, HouseholdID: "hh-a", Roles: []string{"parent"}},
}

func cov2xDay(offset int) string {
	return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02")
}

func cov2xMonthStart(monthsAgo int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -monthsAgo, 0)
}

type cov2xFinFixture struct {
	t      *testing.T
	svc    *billing.Service
	audit  *audit.Service
	router http.Handler
}

func cov2xRoster(joined time.Time) billing.RosterSource {
	return func(string) (billing.RosterSnapshot, error) {
		return billing.RosterSnapshot{
			Households: []billing.RosterHousehold{{ID: "hh-a", Name: "Alpha", Status: "active"}, {ID: "hh-b", Name: "Bravo", Status: "active"}},
			Swimmers: []billing.RosterSwimmer{
				{ID: "sw-a1", HouseholdID: "hh-a", FirstName: "Ann", LastName: "Alpha", DateOfBirth: "2012-01-01", Status: "active"},
				{ID: "sw-a2", HouseholdID: "hh-a", FirstName: "Al", LastName: "Alpha", DateOfBirth: "2014-01-01", Status: "active"},
				{ID: "sw-b1", HouseholdID: "hh-b", FirstName: "Bo", LastName: "Bravo", DateOfBirth: "2013-01-01", Status: "active"},
			},
			GroupLinks: []billing.RosterGroupLink{
				{SwimmerID: "sw-a1", GroupCode: "GOLD", IsPrimary: true, AssignedAt: joined},
				{SwimmerID: "sw-a2", GroupCode: "GOLD", IsPrimary: true, AssignedAt: joined},
				{SwimmerID: "sw-b1", GroupCode: "GOLD", IsPrimary: true, AssignedAt: joined},
			},
		}, nil
	}
}

func cov2xFinRouter(h *BillingHandlers, mws ...func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		for _, mw := range mws {
			p.Use(mw)
		}
		h.RegisterRoutes(p)
		h.RegisterFinanceRoutes(p)
	})
	return r
}

func cov2xNewFinFixture(t *testing.T) *cov2xFinFixture {
	t.Helper()
	svc := billing.NewService()
	svc.SetHouseholdChecker(func(_, id string) bool { return id == "hh-a" || id == "hh-b" })
	svc.SetRosterSource(cov2xRoster(cov2xMonthStart(6)))
	auditSvc := audit.NewService()
	h := NewBillingHandlersWithAudit(svc, auditSvc)
	router := cov2xFinRouter(h, appmiddleware.Auth(cov2xFinTokens), appmiddleware.RequireOrgScope)
	return &cov2xFinFixture{t: t, svc: svc, audit: auditSvc, router: router}
}

func cov2xReq(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", cov2xFinOrg)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func (f *cov2xFinFixture) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov2xReq(f.router, method, path, token, body)
}

func (f *cov2xFinFixture) must(want int, method, path, token, body string) map[string]any {
	f.t.Helper()
	rr := f.do(method, path, token, body)
	if rr.Code != want {
		f.t.Fatalf("%s %s as %s: want %d, got %d: %s", method, path, token, want, rr.Code, rr.Body.String())
	}
	return cov2xDecode(f.t, rr)
}

func cov2xDecode(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	out := map[string]any{}
	if rr.Body.Len() == 0 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		return out
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return out
}

func cov2xErrCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func cov2xObj(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	o, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("missing object %q in %v", key, m)
	}
	return o
}

func cov2xStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, ok := m[key].(string)
	if !ok || s == "" {
		t.Fatalf("missing string %q in %v", key, m)
	}
	return s
}

func cov2xNum(m map[string]any, key string) int {
	f, _ := m[key].(float64)
	return int(f)
}

func cov2xAuditActions(t *testing.T, a *audit.Service) map[string]int {
	t.Helper()
	evs, err := a.Query(audit.Filter{OrgID: cov2xFinOrg, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, ev := range evs {
		out[ev.Action]++
	}
	return out
}

// mapFinanceError is the contract between billing sentinel errors and HTTP
// responses; every sentinel must keep its own status and machine code.
func TestCov2xMapFinanceErrorContract(t *testing.T) {
	t.Parallel()
	stale := &billing.StalePreviewError{Preview: &billing.RunPreview{Fingerprint: "fp-new"}}
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"field validation", &billing.FinanceValidationError{Field: "amount_cents", Message: "must be positive"}, http.StatusUnprocessableEntity, "validation_error"},
		{"wrapped field validation", fmt.Errorf("wrap: %w", &billing.FinanceValidationError{Field: "x", Message: "y"}), http.StatusUnprocessableEntity, "validation_error"},
		{"stale preview", stale, http.StatusConflict, "preview_stale"},
		{"schedule missing", billing.ErrFeeScheduleNotFound, http.StatusNotFound, "fee_schedule_not_found"},
		{"rule missing", billing.ErrDiscountRuleNotFound, http.StatusNotFound, "discount_rule_not_found"},
		{"adjustment missing", billing.ErrAdjustmentNotFound, http.StatusNotFound, "adjustment_not_found"},
		{"run missing", billing.ErrRunNotFound, http.StatusNotFound, "billing_run_not_found"},
		{"adjustment not pending", billing.ErrAdjustmentNotPending, http.StatusConflict, "adjustment_not_pending"},
		{"schedule locked", billing.ErrScheduleLocked, http.StatusConflict, "fee_schedule_locked"},
		{"nothing to bill", billing.ErrNothingToBill, http.StatusUnprocessableEntity, "nothing_to_bill"},
		{"insufficient credit", billing.ErrInsufficientCredit, http.StatusUnprocessableEntity, "insufficient_credit"},
		{"household required", billing.ErrHouseholdRequired, http.StatusUnprocessableEntity, "household_required"},
		{"credit payment refund", billing.ErrCreditPaymentRefundToCredit, http.StatusUnprocessableEntity, "credit_payment_refund_to_credit"},
		{"roster unavailable", billing.ErrRosterUnavailable, http.StatusServiceUnavailable, "roster_unavailable"},
		{"falls through to billing mapper", billing.ErrInvoiceNotFound, http.StatusNotFound, "invoice_not_found"},
		{"forbidden falls through", billing.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"unknown is a 500", errors.New("disk on fire"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			mapFinanceError(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.status, rr.Body.String())
			}
			body := cov2xDecode(t, rr)
			if got := cov2xErrCode(body); got != tc.code {
				t.Fatalf("code = %q, want %q (%s)", got, tc.code, rr.Body.String())
			}
			if tc.code == "preview_stale" {
				if p := cov2xObj(t, body, "preview"); p["fingerprint"] != "fp-new" {
					t.Fatalf("stale response must carry the refreshed preview: %v", p)
				}
			}
			if tc.code == "internal_error" && strings.Contains(rr.Body.String(), "disk on fire") {
				t.Fatalf("internal error details leaked to client: %s", rr.Body.String())
			}
		})
	}
}

type cov2xRoute struct {
	method, path string
	// parentOK marks routes a household-scoped parent may read.
	parentOK bool
}

func cov2xFinanceRoutes() []cov2xRoute {
	return []cov2xRoute{
		{http.MethodGet, "/billing/fee-schedules", false},
		{http.MethodPost, "/billing/fee-schedules", false},
		{http.MethodGet, "/billing/fee-schedules/x", false},
		{http.MethodPut, "/billing/fee-schedules/x", false},
		{http.MethodDelete, "/billing/fee-schedules/x", false},
		{http.MethodGet, "/billing/discount-rules", false},
		{http.MethodPost, "/billing/discount-rules", false},
		{http.MethodPut, "/billing/discount-rules/x", false},
		{http.MethodDelete, "/billing/discount-rules/x", false},
		{http.MethodGet, "/billing/adjustments", false},
		{http.MethodPost, "/billing/adjustments", false},
		{http.MethodPost, "/billing/adjustments/x/void", false},
		{http.MethodGet, "/billing/settings", false},
		{http.MethodPut, "/billing/settings", false},
		{http.MethodPost, "/billing/runs/preview", false},
		{http.MethodPost, "/billing/runs", false},
		{http.MethodGet, "/billing/runs", false},
		{http.MethodGet, "/billing/runs/x", false},
		{http.MethodPost, "/billing/late-fees/apply", false},
		{http.MethodGet, "/households/hh-b/credits", false},
		{http.MethodPost, "/households/hh-b/credits", false},
		{http.MethodPost, "/invoices/x/refunds-to-credit", false},
		{http.MethodGet, "/households/hh-b/statement", false},
		{http.MethodGet, "/households/hh-b/upcoming-charges", false},
		{http.MethodGet, "/billing/reports/aging", false},
		{http.MethodGet, "/billing/reports/revenue", false},
		{http.MethodGet, "/billing/reports/outstanding", false},
		{http.MethodGet, "/billing/reports/deposits", false},
		{http.MethodGet, "/billing/reports/quickbooks", false},
	}
}

// Without an authenticated principal, or without an org scope, no finance route
// may reach the service.
func TestCov2xFinanceRoutesRequirePrincipalAndOrgScope(t *testing.T) {
	t.Parallel()
	svc := billing.NewService()
	h := NewBillingHandlers(svc)
	noAuth := cov2xFinRouter(h)
	authOnly := cov2xFinRouter(h, appmiddleware.Auth(cov2xFinTokens))
	for _, rt := range cov2xFinanceRoutes() {
		rt := rt
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rr := cov2xReq(noAuth, rt.method, rt.path, "", "{}")
			if rr.Code != http.StatusUnauthorized || cov2xErrCode(cov2xDecode(t, rr)) != "unauthorized" {
				t.Fatalf("no principal: want 401 unauthorized, got %d %s", rr.Code, rr.Body.String())
			}
			rr = cov2xReq(authOnly, rt.method, rt.path, "adminMFA", "{}")
			if rr.Code != http.StatusBadRequest || cov2xErrCode(cov2xDecode(t, rr)) != "missing_org_scope" {
				t.Fatalf("no org scope: want 400 missing_org_scope, got %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

// Finance is admin-only (parents may read their own statement/credit only), so
// coaches are denied everywhere and parents everywhere except their own household reads.
func TestCov2xFinanceRoutesDenyNonAdmins(t *testing.T) {
	t.Parallel()
	f := cov2xNewFinFixture(t)
	for _, rt := range cov2xFinanceRoutes() {
		for _, tok := range []string{"coach", "parentA"} {
			rr := f.do(rt.method, rt.path, tok, "{}")
			if rr.Code != http.StatusForbidden {
				t.Fatalf("%s %s as %s: want 403, got %d %s", rt.method, rt.path, tok, rr.Code, rr.Body.String())
			}
		}
	}
	// Non-MFA admins cannot move money into or out of account credit.
	rr := f.do(http.MethodPost, "/households/hh-a/credits", "admin", `{"direction":"credit","amount_cents":100,"reason":"r"}`)
	if rr.Code != http.StatusForbidden || cov2xErrCode(cov2xDecode(t, rr)) != "mfa_required" {
		t.Fatalf("credit without MFA: %d %s", rr.Code, rr.Body.String())
	}
	rr = f.do(http.MethodPost, "/invoices/x/refunds-to-credit", "admin", `{"payment_id":"p","amount_cents":1,"reason":"r"}`)
	if rr.Code != http.StatusForbidden || cov2xErrCode(cov2xDecode(t, rr)) != "mfa_required" {
		t.Fatalf("refund-to-credit without MFA: %d %s", rr.Code, rr.Body.String())
	}
	if n := len(cov2xAuditActions(t, f.audit)); n != 0 {
		t.Fatalf("denied requests must not write audit events, got %d action kinds", n)
	}
}

// Malformed JSON is a 400 and schema violations a 422 on every write route.
func TestCov2xFinanceWriteRoutesRejectBadBodies(t *testing.T) {
	t.Parallel()
	f := cov2xNewFinFixture(t)
	writes := []cov2xRoute{
		{http.MethodPost, "/billing/fee-schedules", false},
		{http.MethodPut, "/billing/fee-schedules/x", false},
		{http.MethodPost, "/billing/discount-rules", false},
		{http.MethodPut, "/billing/discount-rules/x", false},
		{http.MethodPost, "/billing/adjustments", false},
		{http.MethodPut, "/billing/settings", false},
		{http.MethodPost, "/billing/runs/preview", false},
		{http.MethodPost, "/billing/runs", false},
		{http.MethodPost, "/households/hh-a/credits", false},
		{http.MethodPost, "/invoices/x/refunds-to-credit", false},
	}
	for _, rt := range writes {
		rr := f.do(rt.method, rt.path, "adminMFA", `{not json`)
		if rr.Code != http.StatusBadRequest || cov2xErrCode(cov2xDecode(t, rr)) != "invalid_request_body" {
			t.Fatalf("%s %s malformed: want 400 invalid_request_body, got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		rr = f.do(rt.method, rt.path, "adminMFA", `{}`)
		if rr.Code != http.StatusUnprocessableEntity || cov2xErrCode(cov2xDecode(t, rr)) != "validation_error" {
			t.Fatalf("%s %s empty: want 422 validation_error, got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		rr = f.do(rt.method, rt.path, "adminMFA", `{"unexpected_field":1}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s %s unknown field: want 400, got %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
	}
	if n := len(cov2xAuditActions(t, f.audit)); n != 0 {
		t.Fatalf("rejected writes must not be audited, got %d", n)
	}
}

func TestCov2xBillingRunWithoutRosterIsServiceUnavailable(t *testing.T) {
	t.Parallel()
	svc := billing.NewService() // no roster source wired
	h := NewBillingHandlers(svc)
	router := cov2xFinRouter(h, appmiddleware.Auth(cov2xFinTokens), appmiddleware.RequireOrgScope)
	rr := cov2xReq(router, http.MethodPost, "/billing/runs/preview", "adminMFA", `{"as_of":"`+cov2xDay(0)+`"}`)
	if rr.Code != http.StatusServiceUnavailable || cov2xErrCode(cov2xDecode(t, rr)) != "roster_unavailable" {
		t.Fatalf("want 503 roster_unavailable, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov2xFeeSchedulesDiscountRulesAndAdjustments(t *testing.T) {
	t.Parallel()
	f := cov2xNewFinFixture(t)
	start := cov2xMonthStart(3).Format("2006-01-02")
	schedule := func(name string, amount int) string {
		return fmt.Sprintf(`{"name":%q,"kind":"dues","group_code":"GOLD","frequency":"monthly","amount_cents":%d,"start_date":%q,"proration":"none"}`, name, amount, start)
	}

	// Fee schedules: create, read, list, update, archive.
	created := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/fee-schedules", "admin", schedule("Gold dues", 12000)), "fee_schedule")
	sid := cov2xStr(t, created, "id")
	list := f.must(http.StatusOK, http.MethodGet, "/billing/fee-schedules", "admin", "")
	if cov2xNum(list, "count") != 1 {
		t.Fatalf("list count: %v", list)
	}
	got := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/billing/fee-schedules/"+sid, "admin", ""), "fee_schedule")
	if got["name"] != "Gold dues" || cov2xNum(got, "amount_cents") != 12000 {
		t.Fatalf("get schedule: %v", got)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodGet, "/billing/fee-schedules/nope", "admin", "")); code != "fee_schedule_not_found" {
		t.Fatalf("code %q", code)
	}
	upd := cov2xObj(t, f.must(http.StatusOK, http.MethodPut, "/billing/fee-schedules/"+sid, "admin", schedule("Gold dues v2", 13000)), "fee_schedule")
	if upd["name"] != "Gold dues v2" || cov2xNum(upd, "amount_cents") != 13000 {
		t.Fatalf("update schedule: %v", upd)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodPut, "/billing/fee-schedules/nope", "admin", schedule("x", 100))); code != "fee_schedule_not_found" {
		t.Fatalf("code %q", code)
	}
	bad := fmt.Sprintf(`{"name":"No group","kind":"dues","frequency":"monthly","amount_cents":100,"start_date":%q}`, start)
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPut, "/billing/fee-schedules/"+sid, "admin", bad)); code != "validation_error" {
		t.Fatalf("dues without group on update: %q", code)
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/billing/fee-schedules", "admin", bad)); code != "validation_error" {
		t.Fatalf("dues without group on create: %q", code)
	}
	if n := cov2xNum(f.must(http.StatusOK, http.MethodGet, "/billing/fee-schedules", "admin", ""), "count"); n != 1 {
		t.Fatalf("a rejected create must not store a schedule, have %d", n)
	}
	extra := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/fee-schedules", "admin", schedule("Silver dues", 9000)), "fee_schedule")
	archived := cov2xObj(t, f.must(http.StatusOK, http.MethodDelete, "/billing/fee-schedules/"+cov2xStr(t, extra, "id"), "admin", ""), "fee_schedule")
	if archived["active"] != false {
		t.Fatalf("archived schedule must be inactive: %v", archived)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodDelete, "/billing/fee-schedules/nope", "admin", "")); code != "fee_schedule_not_found" {
		t.Fatalf("code %q", code)
	}

	// Discount rules.
	rule := `{"name":"Sibling","applies_to":"dues","min_position":2,"discount_type":"fixed","amount_cents":1500}`
	r1 := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/discount-rules", "admin", rule), "discount_rule")
	rid := cov2xStr(t, r1, "id")
	if n := cov2xNum(f.must(http.StatusOK, http.MethodGet, "/billing/discount-rules", "admin", ""), "count"); n != 1 {
		t.Fatalf("rules count %d", n)
	}
	for _, badRule := range []string{
		`{"name":"Bad max","applies_to":"dues","min_position":3,"max_position":2,"discount_type":"fixed","amount_cents":100}`,
		`{"name":"No percent","applies_to":"dues","min_position":2,"discount_type":"percent"}`,
	} {
		if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/billing/discount-rules", "admin", badRule)); code != "validation_error" {
			t.Fatalf("invalid rule %s: %q", badRule, code)
		}
	}
	if n := cov2xNum(f.must(http.StatusOK, http.MethodGet, "/billing/discount-rules", "admin", ""), "count"); n != 1 {
		t.Fatalf("rejected rules must not be stored, have %d", n)
	}
	upd = cov2xObj(t, f.must(http.StatusOK, http.MethodPut, "/billing/discount-rules/"+rid, "admin",
		`{"name":"Sibling %","applies_to":"all","min_position":2,"discount_type":"percent","percent_bps":1000}`), "discount_rule")
	if upd["name"] != "Sibling %" || upd["discount_type"] != "percent" || cov2xNum(upd, "percent_bps") != 1000 {
		t.Fatalf("update rule: %v", upd)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodPut, "/billing/discount-rules/nope", "admin", rule)); code != "discount_rule_not_found" {
		t.Fatalf("code %q", code)
	}
	archivedRule := cov2xObj(t, f.must(http.StatusOK, http.MethodDelete, "/billing/discount-rules/"+rid, "admin", ""), "discount_rule")
	if archivedRule["active"] != false {
		t.Fatalf("archived rule must be inactive: %v", archivedRule)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodDelete, "/billing/discount-rules/nope", "admin", "")); code != "discount_rule_not_found" {
		t.Fatalf("code %q", code)
	}

	// Adjustments.
	adjBody := func(reason string, cents int) string {
		return fmt.Sprintf(`{"household_id":"hh-a","amount_cents":%d,"reason":%q}`, cents, reason)
	}
	a1 := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/adjustments", "admin", adjBody("Late registration", 500)), "adjustment")
	a2 := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/adjustments", "admin", adjBody("Goodwill", -200)), "adjustment")
	if a1["status"] != "pending" || cov2xNum(a2, "amount_cents") != -200 {
		t.Fatalf("adjustments: %v %v", a1, a2)
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/billing/adjustments", "admin",
		`{"household_id":"hh-unknown","amount_cents":100,"reason":"x"}`)); code != "validation_error" {
		t.Fatalf("unknown household adjustment: %q", code)
	}
	voided := cov2xObj(t, f.must(http.StatusOK, http.MethodPost, "/billing/adjustments/"+cov2xStr(t, a2, "id")+"/void", "admin", ""), "adjustment")
	if voided["status"] != "voided" {
		t.Fatalf("void: %v", voided)
	}
	if code := cov2xErrCode(f.must(http.StatusConflict, http.MethodPost, "/billing/adjustments/"+cov2xStr(t, a2, "id")+"/void", "admin", "")); code != "adjustment_not_pending" {
		t.Fatalf("double void: %q", code)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodPost, "/billing/adjustments/nope/void", "admin", "")); code != "adjustment_not_found" {
		t.Fatalf("void unknown: %q", code)
	}
	pending := f.must(http.StatusOK, http.MethodGet, "/billing/adjustments?status=pending&household_id=hh-a", "admin", "")
	if cov2xNum(pending, "total") != 1 || cov2xNum(pending, "count") != 1 {
		t.Fatalf("pending filter: %v", pending)
	}
	paged := f.must(http.StatusOK, http.MethodGet, "/billing/adjustments?limit=1", "admin", "")
	if cov2xNum(paged, "total") != 2 || cov2xNum(paged, "count") != 1 || cov2xNum(paged, "limit") != 1 {
		t.Fatalf("pagination: %v", paged)
	}

	// Settings.
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPut, "/billing/settings", "admin",
		`{"late_fee_enabled":true,"late_fee_type":"flat","auto_run_day":1,"auto_run_invoice_status":"draft"}`)); code != "validation_error" {
		t.Fatalf("flat late fee without an amount: %q", code)
	}
	def := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/billing/settings", "admin", ""), "settings")
	if def["late_fee_enabled"] != false {
		t.Fatalf("default settings: %v", def)
	}
	saved := cov2xObj(t, f.must(http.StatusOK, http.MethodPut, "/billing/settings", "admin",
		`{"late_fee_enabled":true,"late_fee_grace_days":3,"late_fee_type":"flat","late_fee_flat_cents":500,"auto_run_day":5,"auto_run_invoice_status":"draft"}`), "settings")
	if saved["late_fee_enabled"] != true || cov2xNum(saved, "late_fee_flat_cents") != 500 || cov2xNum(saved, "auto_run_day") != 5 {
		t.Fatalf("saved settings: %v", saved)
	}
	again := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/billing/settings", "admin", ""), "settings")
	if cov2xNum(again, "late_fee_grace_days") != 3 {
		t.Fatalf("settings did not persist: %v", again)
	}

	actions := cov2xAuditActions(t, f.audit)
	for _, want := range []string{
		"billing.fee_schedule.create", "billing.fee_schedule.update", "billing.fee_schedule.archive",
		"billing.discount_rule.create", "billing.discount_rule.update", "billing.discount_rule.archive",
		"billing.adjustment.create", "billing.adjustment.void", "billing.settings.update",
	} {
		if actions[want] == 0 {
			t.Fatalf("missing audit action %s (have %v)", want, actions)
		}
	}
}

// End-to-end: preview, stale-preview protection, commit, late fees, credits,
// refund-to-credit, statements and every report in JSON and CSV.
func TestCov2xBillingRunLateFeesCreditsAndReports(t *testing.T) {
	t.Parallel()
	f := cov2xNewFinFixture(t)
	start := cov2xMonthStart(3).Format("2006-01-02")
	sched := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/billing/fee-schedules", "admin",
		fmt.Sprintf(`{"name":"Gold dues","kind":"dues","group_code":"GOLD","frequency":"monthly","amount_cents":10000,"start_date":%q,"proration":"none"}`, start)), "fee_schedule")
	sid := cov2xStr(t, sched, "id")
	f.must(http.StatusCreated, http.MethodPost, "/billing/discount-rules", "admin",
		`{"name":"Sibling","applies_to":"dues","min_position":2,"discount_type":"fixed","amount_cents":1500}`)
	f.must(http.StatusOK, http.MethodPut, "/billing/settings", "admin",
		`{"late_fee_enabled":true,"late_fee_grace_days":3,"late_fee_type":"flat","late_fee_flat_cents":500,"auto_run_day":1,"auto_run_invoice_status":"draft"}`)

	today, asOf, due := cov2xDay(0), cov2xDay(-30), cov2xDay(-20)
	runBody := func(fp string) string {
		return fmt.Sprintf(`{"as_of":%q,"invoice_status":"issued","due_date":%q,"expected_fingerprint":%q}`, asOf, due, fp)
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/billing/runs/preview", "admin", `{"as_of":"garbage"}`)); code != "validation_error" {
		t.Fatalf("bad as_of: %q", code)
	}
	preview := cov2xObj(t, f.must(http.StatusOK, http.MethodPost, "/billing/runs/preview", "admin", runBody("")), "preview")
	if cov2xNum(preview, "household_count") != 2 || cov2xNum(preview, "swimmer_count") != 3 {
		t.Fatalf("preview should bill both households: %v", preview)
	}
	fp := cov2xStr(t, preview, "fingerprint")

	stale := f.must(http.StatusConflict, http.MethodPost, "/billing/runs", "admin", runBody("deadbeef"))
	if cov2xErrCode(stale) != "preview_stale" || cov2xObj(t, stale, "preview")["fingerprint"] != fp {
		t.Fatalf("stale commit must return the refreshed preview: %v", stale)
	}
	if runs, err := f.svc.ListBillingRuns(cov2xFinOrg, cov2xFinTokens["admin"]); err != nil || len(runs) != 0 {
		t.Fatalf("stale commit must not create a run, got %d (%v)", len(runs), err)
	}

	res := f.must(http.StatusCreated, http.MethodPost, "/billing/runs", "admin", runBody(fp))
	run := cov2xObj(t, res, "run")
	if cov2xNum(run, "invoice_count") != 2 || cov2xNum(run, "net_cents") != cov2xNum(preview, "net_cents") {
		t.Fatalf("committed run differs from reviewed preview: run=%v preview=%v", run, preview)
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/billing/runs", "admin", runBody(""))); code != "nothing_to_bill" {
		t.Fatalf("re-run must bill nothing: %q", code)
	}
	if code := cov2xErrCode(f.must(http.StatusConflict, http.MethodPut, "/billing/fee-schedules/"+sid, "admin",
		fmt.Sprintf(`{"name":"Gold dues","kind":"dues","group_code":"GOLD","frequency":"quarterly","amount_cents":10000,"start_date":%q}`, start))); code != "fee_schedule_locked" {
		t.Fatalf("billed schedule must lock its frequency: %q", code)
	}

	runs := f.must(http.StatusOK, http.MethodGet, "/billing/runs", "admin", "")
	if cov2xNum(runs, "total") != 1 || cov2xNum(runs, "count") != 1 {
		t.Fatalf("runs list: %v", runs)
	}
	detail := f.must(http.StatusOK, http.MethodGet, "/billing/runs/"+cov2xStr(t, run, "id"), "admin", "")
	if cov2xObj(t, detail, "run")["id"] != run["id"] {
		t.Fatalf("run detail: %v", detail)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodGet, "/billing/runs/nope", "admin", "")); code != "billing_run_not_found" {
		t.Fatalf("code %q", code)
	}

	// Late fees: argument validation, then one fee per overdue invoice, once.
	if rr := f.do(http.MethodPost, "/billing/late-fees/apply", "admin", `{bad`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed late-fee body: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPost, "/billing/late-fees/apply", "admin", `{"as_of":"yesterday"}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad as_of: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPost, "/billing/late-fees/apply", "admin", fmt.Sprintf(`{"as_of":%q}`, cov2xDay(30))); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "future") {
		t.Fatalf("future as_of must be refused: %d %s", rr.Code, rr.Body.String())
	}
	fees := cov2xObj(t, f.must(http.StatusOK, http.MethodPost, "/billing/late-fees/apply", "admin", fmt.Sprintf(`{"as_of":%q}`, today)), "result")
	if cov2xNum(fees, "applied_count") != 2 || cov2xNum(fees, "total_cents") != 1000 {
		t.Fatalf("expected a 500c late fee on each of the 2 overdue invoices: %v", fees)
	}
	fees = cov2xObj(t, f.must(http.StatusOK, http.MethodPost, "/billing/late-fees/apply", "admin", ""), "result")
	if cov2xNum(fees, "applied_count") != 0 {
		t.Fatalf("late fees must be applied at most once per invoice: %v", fees)
	}

	// Credits.
	credit := `{"direction":"credit","amount_cents":2500,"reason":"Fundraiser","apply_to_open_invoices":true}`
	res = f.must(http.StatusCreated, http.MethodPost, "/households/hh-b/credits", "adminMFA", credit)
	if applied, _ := res["applied_invoice_ids"].([]any); len(applied) != 1 {
		t.Fatalf("credit should settle part of the open invoice: %v", res)
	}
	res = f.must(http.StatusCreated, http.MethodPost, "/households/hh-b/credits", "adminMFA", `{"direction":"credit","amount_cents":700,"reason":"Gift"}`)
	if applied, ok := res["applied_invoice_ids"].([]any); !ok || len(applied) != 0 {
		t.Fatalf("credit without apply flag must report an empty list, got %v", res["applied_invoice_ids"])
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/households/hh-b/credits", "adminMFA",
		`{"direction":"debit","amount_cents":99999,"reason":"Too much"}`)); code != "insufficient_credit" {
		t.Fatalf("overdraw: %q", code)
	}
	f.must(http.StatusCreated, http.MethodPost, "/households/hh-b/credits", "adminMFA", `{"direction":"debit","amount_cents":200,"reason":"Correction"}`)
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodPost, "/households/hh-ghost/credits", "adminMFA", credit)); code != "household_not_found" {
		t.Fatalf("unknown household credit: %q", code)
	}
	summary := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/households/hh-b/credits", "adminMFA", ""), "credit")
	if cov2xNum(summary, "balance_cents") != 500 {
		t.Fatalf("credit balance: %v", summary)
	}

	// Refund to credit needs a real payment.
	invoices, err := f.svc.ListInvoices(cov2xFinOrg, cov2xFinTokens["admin"], billing.ListInvoicesFilters{HouseholdID: "hh-a"})
	if err != nil || len(invoices) != 1 {
		t.Fatalf("expected one hh-a invoice, got %d (%v)", len(invoices), err)
	}
	inv := invoices[0].ID
	pay := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/invoices/"+inv+"/payments", "adminMFA",
		`{"amount_cents":3000,"method":"cash"}`), "payment")
	pid := cov2xStr(t, pay, "id")
	refund := func(amount int) string {
		return fmt.Sprintf(`{"payment_id":%q,"amount_cents":%d,"reason":"Overbilled"}`, pid, amount)
	}
	out := f.must(http.StatusCreated, http.MethodPost, "/invoices/"+inv+"/refunds-to-credit", "adminMFA", refund(1000))
	if cov2xObj(t, out, "refund")["invoice_id"] != inv || out["ledger_event"] == nil {
		t.Fatalf("refund-to-credit response: %v", out)
	}
	if bal := cov2xNum(cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/households/hh-a/credits", "adminMFA", ""), "credit"), "balance_cents"); bal != 1000 {
		t.Fatalf("refund must land in household credit, balance = %d", bal)
	}
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/invoices/"+inv+"/refunds-to-credit", "adminMFA", refund(99000))); code != "refund_exceeds_payment" {
		t.Fatalf("over-refund: %q", code)
	}
	if code := cov2xErrCode(f.must(http.StatusNotFound, http.MethodPost, "/invoices/ghost/refunds-to-credit", "adminMFA", refund(1))); code != "invoice_not_found" {
		t.Fatalf("unknown invoice: %q", code)
	}
	noHH := cov2xObj(t, f.must(http.StatusCreated, http.MethodPost, "/invoices", "adminMFA", `{"invoice_number":"INV-NOHH-1"}`), "invoice")
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, "/invoices/"+cov2xStr(t, noHH, "id")+"/refunds-to-credit", "adminMFA", refund(1))); code != "household_required" {
		t.Fatalf("household-less invoice: %q", code)
	}

	// Statements and upcoming charges, including parent household scoping.
	st := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/households/hh-a/statement?from="+cov2xDay(-120)+"&to="+today, "adminMFA", ""), "statement")
	if st["household_id"] != "hh-a" || len(st) < 2 {
		t.Fatalf("statement: %v", st)
	}
	f.must(http.StatusOK, http.MethodGet, "/households/hh-a/statement", "parentA", "")
	f.must(http.StatusOK, http.MethodGet, "/households/hh-a/credits", "parentA", "")
	for _, q := range []string{"?from=bad", "?to=2026-13-45"} {
		if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodGet, "/households/hh-a/statement"+q, "adminMFA", "")); code != "validation_error" {
			t.Fatalf("statement %s: %q", q, code)
		}
	}
	f.must(http.StatusOK, http.MethodGet, "/households/hh-a/upcoming-charges", "parentA", "")
	up := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, "/households/hh-a/upcoming-charges?days=30", "adminMFA", ""), "upcoming")
	if _, ok := up["charges"].([]any); !ok || up["household_id"] != "hh-a" {
		t.Fatalf("upcoming payload: %v", up)
	}
	f.must(http.StatusForbidden, http.MethodGet, "/households/hh-b/statement", "parentA", "")

	// Reports: JSON, CSV (attachment + audited), and range validation.
	f.must(http.StatusOK, http.MethodGet, "/billing/reports/aging?as_of="+today, "admin", "")
	f.must(http.StatusOK, http.MethodGet, "/billing/reports/revenue", "admin", "")
	f.must(http.StatusOK, http.MethodGet, "/billing/reports/outstanding", "admin", "")
	f.must(http.StatusOK, http.MethodGet, "/billing/reports/deposits?period=week", "admin", "")
	for _, path := range []string{
		"/billing/reports/aging?format=csv&as_of=" + today,
		"/billing/reports/revenue?format=csv&from=" + cov2xDay(-90) + "&to=" + today,
		"/billing/reports/outstanding?format=CSV",
		"/billing/reports/deposits?format=csv&period=day&from=" + cov2xDay(-30) + "&to=" + today,
		"/billing/reports/quickbooks?from=" + cov2xDay(-90) + "&to=" + today,
		"/billing/reports/quickbooks?kind=payments&from=" + cov2xDay(-90) + "&to=" + today,
	} {
		rr := f.do(http.MethodGet, path, "admin", "")
		if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/csv") ||
			!strings.HasPrefix(rr.Header().Get("Content-Disposition"), "attachment; filename=") ||
			rr.Header().Get("Cache-Control") != "no-store" || rr.Body.Len() == 0 {
			t.Fatalf("%s: expected csv attachment, got %d %v %q", path, rr.Code, rr.Header(), rr.Body.String())
		}
	}
	qb := f.do(http.MethodGet, "/billing/reports/quickbooks?kind=payments&from="+cov2xDay(-90)+"&to="+today, "admin", "")
	if !strings.Contains(qb.Header().Get("Content-Disposition"), "quickbooks-payments-") {
		t.Fatalf("payments export filename: %q", qb.Header().Get("Content-Disposition"))
	}
	qb = f.do(http.MethodGet, "/billing/reports/quickbooks?kind=weird&from="+cov2xDay(-90)+"&to="+today, "admin", "")
	if !strings.Contains(qb.Header().Get("Content-Disposition"), "quickbooks-invoices-") {
		t.Fatalf("unknown kind must fall back to invoices: %q", qb.Header().Get("Content-Disposition"))
	}
	for _, path := range []string{
		"/billing/reports/aging?as_of=nope",
		"/billing/reports/revenue?from=nope",
		"/billing/reports/revenue?from=" + today + "&to=" + cov2xDay(-5),
		"/billing/reports/deposits?to=nope",
		"/billing/reports/quickbooks?from=" + today + "&to=" + cov2xDay(-5),
	} {
		if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodGet, path, "admin", "")); code != "validation_error" {
			t.Fatalf("%s: %q", path, code)
		}
	}

	actions := cov2xAuditActions(t, f.audit)
	for action, min := range map[string]int{
		"billing.run.commit": 1, "billing.late_fees.apply": 1, "billing.credit.create": 3,
		"billing.refund_to_credit.create": 1, "billing.report.export": 6,
	} {
		if actions[action] < min {
			t.Fatalf("audit action %s: want >= %d, have %v", action, min, actions)
		}
	}
}
