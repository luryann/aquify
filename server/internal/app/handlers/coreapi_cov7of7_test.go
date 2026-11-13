package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/luryann/aquify/server/internal/testutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/invites"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgs"
)

const cov7xOrg = "org-cov7x"

func cov7xTokens() tokenResolver {
	return tokenResolver{
		"admin": {UserID: "u-admin", OrgID: cov7xOrg, Roles: []string{"admin"}},
		"coach": {UserID: "u-coach", OrgID: cov7xOrg, Roles: []string{"coach"}},
	}
}

// cov7xRouter mounts routes behind real Auth; withOrg adds RequireOrgScope.
func cov7xRouter(withOrg bool, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov7xTokens()))
		if withOrg {
			p.Use(appmiddleware.RequireOrgScope)
		}
		register(p)
	})
	return r
}

func cov7xDo(h http.Handler, method, path, token, org, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if org != "" {
		req.Header.Set("X-Org-Id", org)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func cov7xErrCode(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status = %d, want %d: %s", rr.Code, status, rr.Body.String())
	}
	if code != "" && !strings.Contains(rr.Body.String(), code) {
		t.Fatalf("body %s missing %q", rr.Body.String(), code)
	}
}

func TestCov7xAuthzPingAndSettingsRequirePrincipal(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	AuthzPing(rr, httptest.NewRequest(http.MethodGet, "/ping", nil))
	cov7xErrCode(t, rr, http.StatusUnauthorized, "unauthorized")

	rr = httptest.NewRecorder()
	NewSettingsHandlers(map[string]any{"a": 1}).runtimeSettings(rr, httptest.NewRequest(http.MethodGet, "/settings/runtime", nil))
	cov7xErrCode(t, rr, http.StatusUnauthorized, "unauthorized")

	// Authenticated but without an org header: settings must refuse.
	h := cov7xRouter(false, NewSettingsHandlers(map[string]any{"a": 1}).RegisterRoutes)
	rr = cov7xDo(h, http.MethodGet, "/settings/runtime", "admin", "", "")
	cov7xErrCode(t, rr, http.StatusBadRequest, "missing_org_scope")

	// Non-admins are forbidden; admins see the payload.
	h = cov7xRouter(true, NewSettingsHandlers(map[string]any{"a": 1}).RegisterRoutes)
	cov7xErrCode(t, cov7xDo(h, http.MethodGet, "/settings/runtime", "coach", cov7xOrg, ""), http.StatusForbidden, "forbidden")
	rr = cov7xDo(h, http.MethodGet, "/settings/runtime", "admin", cov7xOrg, "")
	cov7xErrCode(t, rr, http.StatusOK, "")
	var got struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got.Settings["a"] != float64(1) {
		t.Fatalf("settings payload = %s (%v)", rr.Body.String(), err)
	}
}

func TestCov7xHasAnyRole(t *testing.T) {
	t.Parallel()
	if hasAnyRole(nil, "admin") {
		t.Fatal("nil principal must hold no role")
	}
	p := &appmiddleware.Principal{Roles: []string{"Coach"}}
	if !hasAnyRole(p, "admin", "coach") {
		t.Fatal("role match must be case-insensitive")
	}
	if hasAnyRole(p, "admin") {
		t.Fatal("coach must not match admin")
	}
}

func TestCov7xAppendAuditNilService(t *testing.T) {
	t.Parallel()
	if err := appendAudit(nil, audit.AppendRequest{Action: "x"}); err != nil {
		t.Fatalf("nil service must be a no-op, got %v", err)
	}
}

func TestCov7xAuditListing(t *testing.T) {
	t.Parallel()
	svc := audit.NewService()
	for i := 0; i < 201; i++ {
		if _, err := svc.Append(audit.AppendRequest{OrgID: cov7xOrg, ActorUserID: "u-admin", Action: "test.action", ResourceType: "thing", ResourceID: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	h := cov7xRouter(true, NewAuditHandlers(svc).RegisterRoutes)

	type resp struct {
		Events     []audit.AuditEvent `json:"events"`
		NextCursor string             `json:"next_cursor"`
	}
	get := func(q string) resp {
		t.Helper()
		rr := cov7xDo(h, http.MethodGet, "/audit-events"+q, "admin", cov7xOrg, "")
		cov7xErrCode(t, rr, http.StatusOK, "")
		var out resp
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// limit above the cap is clamped to 200 and a full page yields a cursor.
	big := get("?limit=1000")
	if len(big.Events) != 200 || big.NextCursor != big.Events[199].ID {
		t.Fatalf("clamped page: %d events, cursor %q", len(big.Events), big.NextCursor)
	}
	// The cursor continues with the remaining event and then ends.
	rest := get("?limit=1000&cursor=" + big.NextCursor)
	if len(rest.Events) != 1 || rest.NextCursor != "" {
		t.Fatalf("after cursor: %d events, cursor %q", len(rest.Events), rest.NextCursor)
	}
	// Explicit small limit is honoured.
	if small := get("?limit=3"); len(small.Events) != 3 || small.NextCursor == "" {
		t.Fatalf("limit=3 returned %d events cursor %q", len(small.Events), small.NextCursor)
	}
	// Garbage and non-positive limits fall back to the default of 50.
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-4"} {
		if r := get(q); len(r.Events) != 50 {
			t.Fatalf("%s returned %d events, want default 50", q, len(r.Events))
		}
	}
	// Filters apply.
	if r := get("?resource_id=7"); len(r.Events) != 1 || r.Events[0].ResourceID != "7" {
		t.Fatalf("resource_id filter: %+v", r.Events)
	}

	cov7xErrCode(t, cov7xDo(h, http.MethodGet, "/audit-events", "coach", cov7xOrg, ""), http.StatusForbidden, "forbidden")
	cov7xErrCode(t, cov7xDo(h, http.MethodGet, "/audit-events?from=yesterday", "admin", cov7xOrg, ""), http.StatusBadRequest, "'from'")
	cov7xErrCode(t, cov7xDo(h, http.MethodGet, "/audit-events?to=tomorrow", "admin", cov7xOrg, ""), http.StatusBadRequest, "'to'")
	if r := get("?from=2000-01-01T00:00:00Z&to=2999-01-01T00:00:00Z&limit=1"); len(r.Events) != 1 {
		t.Fatalf("valid range returned %d events", len(r.Events))
	}

	// No principal in context at all.
	rr := httptest.NewRecorder()
	NewAuditHandlers(svc).listAuditEvents(rr, httptest.NewRequest(http.MethodGet, "/audit-events", nil))
	cov7xErrCode(t, rr, http.StatusUnauthorized, "unauthorized")
}

func TestCov7xAuditQueryFailureIs500(t *testing.T) {
	svc := audit.NewService()
	svc.SetPool(testutil.ClosedPool(t))
	h := cov7xRouter(true, NewAuditHandlers(svc).RegisterRoutes)
	rr := cov7xDo(h, http.MethodGet, "/audit-events", "admin", cov7xOrg, "")
	cov7xErrCode(t, rr, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rr.Body.String(), "closed pool") {
		t.Fatalf("cause leaked to client: %s", rr.Body.String())
	}
}

func TestCov7xInvitesLifecycleAndValidation(t *testing.T) {
	t.Parallel()
	auditSvc := audit.NewService()
	h := cov7xRouter(true, NewInviteHandlers(invites.NewService(), auditSvc).RegisterRoutes)

	rr := cov7xDo(h, http.MethodPost, "/invites/", "admin", cov7xOrg, `{"label":"Fall","role":" Parent ","max_uses":3,"ttl_days":7}`)
	cov7xErrCode(t, rr, http.StatusCreated, "")
	var created struct {
		Invite invites.Invite `json:"invite"`
		Code   string         `json:"code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil || created.Code == "" || created.Invite.Role != "parent" || created.Invite.MaxUses != 3 {
		t.Fatalf("created = %s (%v)", rr.Body.String(), err)
	}

	cov7xErrCode(t, cov7xDo(h, http.MethodPost, "/invites/", "admin", cov7xOrg, `{"role":"admin"}`), http.StatusBadRequest, "invalid_role")
	cov7xErrCode(t, cov7xDo(h, http.MethodPost, "/invites/", "admin", cov7xOrg, `{"role":"parent","max_uses":2000000000}`), http.StatusBadRequest, "invalid_request")
	cov7xErrCode(t, cov7xDo(h, http.MethodPost, "/invites/", "admin", cov7xOrg, `{"role":`), http.StatusBadRequest, "invalid_request")
	cov7xErrCode(t, cov7xDo(h, http.MethodPost, "/invites/", "coach", cov7xOrg, `{"role":"parent"}`), http.StatusForbidden, "forbidden")

	rr = cov7xDo(h, http.MethodGet, "/invites/", "admin", cov7xOrg, "")
	cov7xErrCode(t, rr, http.StatusOK, `"count":1`)
	if strings.Contains(rr.Body.String(), created.Code) {
		t.Fatal("list must never expose the plaintext code")
	}

	cov7xErrCode(t, cov7xDo(h, http.MethodDelete, "/invites/missing", "admin", cov7xOrg, ""), http.StatusNotFound, "not_found")
	cov7xErrCode(t, cov7xDo(h, http.MethodDelete, "/invites/"+created.Invite.ID, "admin", cov7xOrg, ""), http.StatusNoContent, "")

	evs, err := auditSvc.Query(audit.Filter{OrgID: cov7xOrg})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, e := range evs {
		actions[e.Action] = true
	}
	if !actions[audit.ActionOrgInviteCreate] || !actions[audit.ActionOrgInviteRevoke] {
		t.Fatalf("expected create+revoke audit events, got %v", actions)
	}
}

func TestCov7xInvitesRequireScope(t *testing.T) {
	t.Parallel()
	h := NewInviteHandlers(invites.NewService(), nil)

	// No principal at all.
	for name, fn := range map[string]http.HandlerFunc{"create": h.create, "list": h.list, "revoke": h.revoke} {
		rr := httptest.NewRecorder()
		fn(rr, httptest.NewRequest(http.MethodGet, "/invites", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s without principal = %d", name, rr.Code)
		}
	}
	// Principal but no org header.
	router := cov7xRouter(false, h.RegisterRoutes)
	cov7xErrCode(t, cov7xDo(router, http.MethodGet, "/invites/", "admin", "", ""), http.StatusBadRequest, "missing_org_scope")
	cov7xErrCode(t, cov7xDo(router, http.MethodDelete, "/invites/x", "admin", "", ""), http.StatusBadRequest, "missing_org_scope")
}

func TestCov7xInvitesStorageFailuresAre500(t *testing.T) {
	svc := invites.NewServiceWithPool(testutil.ClosedPool(t))
	h := cov7xRouter(true, NewInviteHandlers(svc, nil).RegisterRoutes)
	cov7xErrCode(t, cov7xDo(h, http.MethodGet, "/invites/", "admin", cov7xOrg, ""), http.StatusInternalServerError, "internal_error")
	cov7xErrCode(t, cov7xDo(h, http.MethodDelete, "/invites/00000000-0000-4000-8000-000000000001", "admin", cov7xOrg, ""), http.StatusInternalServerError, "internal_error")
}

func TestCov7xNormalizeJSONDecodeError(t *testing.T) {
	t.Parallel()
	if err := normalizeJSONDecodeError(&json.UnmarshalTypeError{Value: "string"}, 10); err == nil || err.Error() != "request body has the wrong type" {
		t.Fatalf("typeErr without field = %v", err)
	}
	if err := normalizeJSONDecodeError(&json.UnmarshalTypeError{Field: "age"}, 10); err == nil || err.Error() != `request body field "age" has the wrong type` {
		t.Fatalf("typeErr with field = %v", err)
	}
	if err := normalizeJSONDecodeError(errors.New("some reflect detail on main.secretStruct"), 10); err == nil || err.Error() != "request body is invalid" {
		t.Fatalf("fallback = %v", err)
	}
	if err := normalizeJSONDecodeError(errors.New(`json: unknown field "x"`), 10); err == nil || err.Error() != `json: unknown field "x"` {
		t.Fatalf("unknown field = %v", err)
	}
}

func TestCov7xParseIntHelpers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw         string
		fallback    int
		pos, nonNeg int
	}{
		{"", 7, 7, 7},
		{"0", 7, 7, 0},
		{"12", 7, 12, 12},
		{"-3", 7, 7, 7},
		{"1x", 7, 7, 7},
	}
	for _, c := range cases {
		if got := parsePositiveInt(c.raw, c.fallback); got != c.pos {
			t.Errorf("parsePositiveInt(%q) = %d, want %d", c.raw, got, c.pos)
		}
		if got := parseNonNegativeInt(c.raw, c.fallback); got != c.nonNeg {
			t.Errorf("parseNonNegativeInt(%q) = %d, want %d", c.raw, got, c.nonNeg)
		}
	}
	if got := parseNonNegativeInt(strings.Repeat("9", 40), 1); got != maxParsedInt {
		t.Errorf("huge value must clamp to maxParsedInt, got %d", got)
	}
}

func TestCov7xWriteServiceBadRequest(t *testing.T) {
	t.Parallel()
	do := func(err error) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		writeServiceBadRequest(rr, httptest.NewRequest(http.MethodPost, "/x", nil), "invalid_thing", err)
		return rr
	}
	rr := do(fmt.Errorf("wrap: %w", &pgconn.PgError{Code: "23505", Message: "dup key secret_idx"}))
	cov7xErrCode(t, rr, http.StatusConflict, "conflict")
	if strings.Contains(rr.Body.String(), "secret_idx") {
		t.Fatal("db detail leaked")
	}
	rr = do(&pgconn.PgError{Code: "42P01", Message: "no table"})
	cov7xErrCode(t, rr, http.StatusInternalServerError, "internal_error")
	rr = do(&net.DNSError{Err: "lookup failed", Name: "db.internal"})
	cov7xErrCode(t, rr, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rr.Body.String(), "db.internal") {
		t.Fatal("network detail leaked")
	}
	rr = do(context.DeadlineExceeded)
	cov7xErrCode(t, rr, http.StatusInternalServerError, "internal_error")
	rr = do(errors.New("name is required"))
	cov7xErrCode(t, rr, http.StatusBadRequest, "invalid_thing")
	if !strings.Contains(rr.Body.String(), "name is required") {
		t.Fatalf("validation message missing: %s", rr.Body.String())
	}
}

func TestCov7xPublicTenantRegistryFailureIs500(t *testing.T) {
	reg := orgs.NewServiceWithPool(testutil.ClosedPool(t))
	h := tenantRouter(t, reg, nil)
	code, body := getTenant(h, "slug=anything")
	if code != http.StatusInternalServerError || !strings.Contains(body, "internal_error") {
		t.Fatalf("got %d %s", code, body)
	}
}

func TestCov7xOpenAPIServesPackagedSpec(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	OpenAPIStub(rr, httptest.NewRequest(http.MethodGet, "/openapi", nil))
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/yaml") ||
		rr.Header().Get("Cache-Control") == "" || !strings.Contains(rr.Body.String(), "openapi:") {
		t.Fatalf("got %d %v %.60s", rr.Code, rr.Header(), rr.Body.String())
	}
}

func TestCov7xStaffPublicRole(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{"Coach": {"admin", "coach"}, "Administrator": {"admin"}, "": {"parent"}}
	for want, roles := range cases {
		if got := (StaffMember{Roles: roles}).PublicRole(); got != want {
			t.Errorf("PublicRole(%v) = %q, want %q", roles, got, want)
		}
	}
}

type cov7xNeverLister struct{}

func (cov7xNeverLister) ListStaff(context.Context, string) ([]StaffMember, error) {
	return nil, errors.New("inner must not be called")
}

func TestCov7xCachedStaffListerSharedInflightCall(t *testing.T) {
	t.Parallel()
	c := NewCachedStaffLister(cov7xNeverLister{}, 0, nil)

	// A caller that joins an in-flight lookup gets its error.
	failing := &staffCall{done: make(chan struct{}), err: errors.New("keycloak down")}
	close(failing.done)
	c.inflight["org-a"] = failing
	if _, err := c.ListStaff(context.Background(), "org-a"); err == nil || err.Error() != "keycloak down" {
		t.Fatalf("joined error = %v", err)
	}

	// A joined lookup that succeeds returns a copy of the shared result.
	ok := &staffCall{done: make(chan struct{}), staff: []StaffMember{{ID: "s1"}}}
	close(ok.done)
	c.inflight["org-b"] = ok
	got, err := c.ListStaff(context.Background(), "org-b")
	if err != nil || len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("joined result = %v %v", got, err)
	}
	got[0].ID = "mutated"
	if ok.staff[0].ID != "s1" {
		t.Fatal("result must be cloned")
	}

	// A caller whose context ends while waiting stops waiting.
	pending := &staffCall{done: make(chan struct{})}
	c.inflight["org-c"] = pending
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ListStaff(ctx, "org-c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}
}
