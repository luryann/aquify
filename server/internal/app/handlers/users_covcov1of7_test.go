package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/invites"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
)

// ─── Fakes (true external boundaries only) ───────────────────────────────────

// cov1o7KC wraps the in-memory Keycloak stub and lets a test make individual
// admin-API calls fail (optionally after N successful calls).
type cov1o7KC struct {
	*keycloak.StubClient
	mu    sync.Mutex
	errs  map[string]error
	skips map[string]int
	calls map[string]int
	roles map[string][]keycloak.Role
}

func cov1o7NewKC() *cov1o7KC {
	return &cov1o7KC{
		StubClient: keycloak.NewStubClient(),
		errs:       map[string]error{},
		skips:      map[string]int{},
		calls:      map[string]int{},
		roles:      map[string][]keycloak.Role{},
	}
}

// failOn makes method return err once `skip` earlier calls have succeeded.
func (k *cov1o7KC) failOn(method string, err error, skip ...int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.errs[method] = err
	if len(skip) > 0 {
		k.skips[method] = skip[0]
	}
}

func (k *cov1o7KC) trip(method string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls[method]++
	if err := k.errs[method]; err != nil && k.calls[method] > k.skips[method] {
		return err
	}
	return nil
}

func (k *cov1o7KC) ListUsers(ctx context.Context, orgID, search string, limit, offset int) ([]keycloak.User, error) {
	if err := k.trip("ListUsers"); err != nil {
		return nil, err
	}
	return k.StubClient.ListUsers(ctx, orgID, search, limit, offset)
}

func (k *cov1o7KC) GetUser(ctx context.Context, id string) (*keycloak.User, error) {
	if err := k.trip("GetUser"); err != nil {
		return nil, err
	}
	return k.StubClient.GetUser(ctx, id)
}

func (k *cov1o7KC) CreateUser(ctx context.Context, email, first, last, org string) (string, error) {
	if err := k.trip("CreateUser"); err != nil {
		return "", err
	}
	return k.StubClient.CreateUser(ctx, email, first, last, org)
}

func (k *cov1o7KC) UpdateUser(ctx context.Context, id string, f keycloak.UpdateUserFields) error {
	if err := k.trip("UpdateUser"); err != nil {
		return err
	}
	return k.StubClient.UpdateUser(ctx, id, f)
}

func (k *cov1o7KC) DeleteUser(ctx context.Context, id string) error {
	if err := k.trip("DeleteUser"); err != nil {
		return err
	}
	return k.StubClient.DeleteUser(ctx, id)
}

func (k *cov1o7KC) DisableUser(ctx context.Context, id string) error {
	if err := k.trip("DisableUser"); err != nil {
		return err
	}
	return k.StubClient.DisableUser(ctx, id)
}

func (k *cov1o7KC) EnableUser(ctx context.Context, id string) error {
	if err := k.trip("EnableUser"); err != nil {
		return err
	}
	return k.StubClient.EnableUser(ctx, id)
}

func (k *cov1o7KC) ListUsersWithRealmRole(ctx context.Context, org, role string) ([]keycloak.User, error) {
	if err := k.trip("ListUsersWithRealmRole"); err != nil {
		return nil, err
	}
	return k.StubClient.ListUsersWithRealmRole(ctx, org, role)
}

func (k *cov1o7KC) GetUserRealmRoles(ctx context.Context, id string) ([]keycloak.Role, error) {
	if err := k.trip("GetUserRealmRoles"); err != nil {
		return nil, err
	}
	k.mu.Lock()
	override, ok := k.roles[id]
	k.mu.Unlock()
	if ok {
		return override, nil
	}
	return k.StubClient.GetUserRealmRoles(ctx, id)
}

func (k *cov1o7KC) AssignRealmRole(ctx context.Context, id, role string) error {
	if err := k.trip("AssignRealmRole"); err != nil {
		return err
	}
	return k.StubClient.AssignRealmRole(ctx, id, role)
}

func (k *cov1o7KC) RemoveRealmRole(ctx context.Context, id, role string) error {
	if err := k.trip("RemoveRealmRole"); err != nil {
		return err
	}
	return k.StubClient.RemoveRealmRole(ctx, id, role)
}

func (k *cov1o7KC) SendVerificationEmail(ctx context.Context, id string) error {
	if err := k.trip("SendVerificationEmail"); err != nil {
		return err
	}
	return k.StubClient.SendVerificationEmail(ctx, id)
}

func (k *cov1o7KC) SendPasswordResetEmail(ctx context.Context, id string) error {
	if err := k.trip("SendPasswordResetEmail"); err != nil {
		return err
	}
	return k.StubClient.SendPasswordResetEmail(ctx, id)
}

func (k *cov1o7KC) SetUserPassword(ctx context.Context, id, pw string, temp bool) error {
	if err := k.trip("SetUserPassword"); err != nil {
		return err
	}
	return k.StubClient.SetUserPassword(ctx, id, pw, temp)
}

type cov1o7Revoker struct {
	mu    sync.Mutex
	err   error
	calls []string
}

func (r *cov1o7Revoker) RevokeUserSessions(_ context.Context, orgID, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, orgID+"/"+userID)
	if r.err != nil {
		return 0, r.err
	}
	return 2, nil
}

type cov1o7Clubs struct {
	bySlug map[string]*orgsettings.OrgSettings
	err    error
}

func (c cov1o7Clubs) FindBySlug(slug string) (*orgsettings.OrgSettings, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.bySlug[slug], nil
}

type cov1o7Invites struct {
	mu         sync.Mutex
	redeem     func(orgID, code string) (*invites.Invite, error)
	released   []string
	releaseErr error
}

func (i *cov1o7Invites) Redeem(_ context.Context, orgID, code string) (*invites.Invite, error) {
	return i.redeem(orgID, code)
}

func (i *cov1o7Invites) Release(_ context.Context, orgID, id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.released = append(i.released, orgID+"/"+id)
	return i.releaseErr
}

// ─── Environment ──────────────────────────────────────────────────────────────

type cov1o7Env struct {
	t       *testing.T
	kc      *cov1o7KC
	h       *UserHandlers
	members *members.Service
	audit   *audit.Service
	revoker *cov1o7Revoker
	router  http.Handler // production routes, RBAC applied
	direct  http.Handler // handlers mounted without RBAC (tier / MFA guards)
	public  http.Handler
}

const cov1o7Org = "org-1"

var cov1o7Tokens = tokenResolver{
	"admin":   {UserID: "admin-1", OrgID: cov1o7Org, Roles: []string{"admin"}, MFAAuthenticated: true},
	"nomfa":   {UserID: "admin-nm", OrgID: cov1o7Org, Roles: []string{"admin"}},
	"ghost":   {UserID: "ghost-admin", OrgID: cov1o7Org, Roles: []string{"admin"}, MFAAuthenticated: true},
	"coach":   {UserID: "u-coach", OrgID: cov1o7Org, Roles: []string{"coach"}, MFAAuthenticated: true},
	"parent":  {UserID: "p-1", OrgID: cov1o7Org, Roles: []string{"parent"}},
	"swimmer": {UserID: "u-swim", OrgID: cov1o7Org, Roles: []string{"swimmer"}},
}

func cov1o7New(t *testing.T) *cov1o7Env {
	t.Helper()
	kc := cov1o7NewKC()
	add := func(id, role string, enabled bool, attrs map[string][]string) {
		if attrs == nil {
			attrs = map[string][]string{}
		}
		attrs["org_id"] = []string{cov1o7Org}
		kc.AddUser(keycloak.User{ID: id, Email: id + "@example.com", FirstName: strings.ToUpper(id[:1]) + id[1:], Enabled: enabled, Attributes: attrs})
		if role != "" {
			if err := kc.AssignRealmRole(context.Background(), id, role); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("admin-1", "admin", true, nil)
	add("admin-2", "admin", true, nil)
	add("u-coach", "coach", true, nil)
	add("p-1", "parent", true, nil)
	add("pend-1", "", false, map[string][]string{"registration_status": {"pending_approval"}, "requested_role": {"parent"}})
	kc.AddUser(keycloak.User{ID: "out-1", Email: "out-1@example.com", Enabled: true, Attributes: map[string][]string{"org_id": {"org-2"}}})

	auditSvc := audit.NewService()
	mem := members.NewService()
	rev := &cov1o7Revoker{}
	h := NewUserHandlersWithMembers(kc, auditSvc, mem)
	h.SetSessionRevoker(rev)

	e := &cov1o7Env{t: t, kc: kc, h: h, members: mem, audit: auditSvc, revoker: rev}
	e.router = e.build(func(r chi.Router) { h.RegisterRoutes(r) })
	e.direct = e.build(func(r chi.Router) {
		r.Post("/users", h.createUser)
		r.Post("/users/invite", h.inviteUser)
		r.Post("/users/{id}/approve", h.approveUser)
		r.Post("/users/{id}/roles", h.assignRole)
		r.Delete("/users/{id}/roles/{role}", h.removeRole)
	})
	pub := chi.NewRouter()
	h.RegisterPublicRoutes(pub)
	e.public = pub
	return e
}

func (e *cov1o7Env) build(register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(appmiddleware.Auth(cov1o7Tokens), appmiddleware.RequireOrgScope)
	register(r)
	return r
}

func cov1o7Do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Org-Id", cov1o7Org)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (e *cov1o7Env) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov1o7Do(e.router, method, path, token, body)
}

func (e *cov1o7Env) dodirect(method, path, token, body string) *httptest.ResponseRecorder {
	return cov1o7Do(e.direct, method, path, token, body)
}

func cov1o7Code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env.Error.Code
}

func cov1o7Expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if code != "" {
		if got := cov1o7Code(t, rec); got != code {
			t.Fatalf("error code = %q, want %q (body %s)", got, code, rec.Body.String())
		}
	}
}

func cov1o7JSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func (e *cov1o7Env) auditActions(action string) []audit.AuditEvent {
	e.t.Helper()
	evs, err := e.audit.Query(audit.Filter{Action: action})
	if err != nil {
		e.t.Fatal(err)
	}
	return evs
}

func (e *cov1o7Env) user(id string) *keycloak.User {
	e.t.Helper()
	u, err := e.kc.StubClient.GetUser(context.Background(), id)
	if err != nil {
		e.t.Fatalf("user %s: %v", id, err)
	}
	return u
}

func (e *cov1o7Env) realmRoles(id string) []string {
	e.t.Helper()
	roles, err := e.kc.StubClient.GetUserRealmRoles(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return normalizeRealmRoles(roles)
}

var (
	cov1o7Boom        = errors.New("boom")
	cov1o7Unavailable = &keycloak.StatusError{Op: "test", Status: 503, Body: "down"}
)

// ─── Pure helpers ─────────────────────────────────────────────────────────────

func TestCov1o7UserHelperFunctions(t *testing.T) {
	if validPersonName(strings.Repeat("a", 101)) {
		t.Error("101-character name must be rejected")
	}
	if !validPersonName(strings.Repeat("a", 100)) || !validPersonName("") {
		t.Error("100-character and empty names are valid")
	}
	if validPersonName("bad\x07name") || validPersonName("del\x7f") {
		t.Error("control characters must be rejected")
	}
	if validEmail("not-an-email") || validEmail(strings.Repeat("a", 250)+"@x.io") || !validEmail("a@b.io") {
		t.Error("validEmail misclassified an address")
	}

	roles := []keycloak.Role{{ID: "1", Name: " Parent "}, {ID: "2", Name: "parent"}, {ID: "3", Name: "  "}, {ID: "4", Name: "ADMIN"}}
	if got := normalizeRealmRoles(roles); strings.Join(got, ",") != "admin,parent" {
		t.Errorf("normalizeRealmRoles = %v", got)
	}
	objs := normalizeRealmRoleObjects(roles)
	if len(objs) != 2 || objs[0].Name != "admin" || objs[1].Name != "parent" || objs[1].ID != "1" {
		t.Errorf("normalizeRealmRoleObjects = %+v", objs)
	}

	pending := userListItem{Attributes: map[string][]string{"registration_status": {"pending_approval"}}}
	for _, tc := range []struct {
		item   userListItem
		status string
		want   bool
	}{
		{userListItem{Enabled: true}, "active", true},
		{userListItem{Enabled: true}, "disabled", false},
		{userListItem{Enabled: false}, "disabled", true},
		{pending, "pending_approval", true},
		{userListItem{}, "pending_approval", false},
		{userListItem{Enabled: true}, "bogus", false},
	} {
		if got := userMatchesStatus(tc.item, tc.status); got != tc.want {
			t.Errorf("userMatchesStatus(%+v,%q) = %v, want %v", tc.item, tc.status, got, tc.want)
		}
	}

	if keycloakSource(keycloak.NewStubClient()) != "stub" || keycloakSource(cov1o7NewKC()) != "keycloak" {
		t.Error("keycloakSource misreported the client kind")
	}

	// Attribute helpers.
	if attrs := cloneUserAttributes(nil); len(attrs) != 0 {
		t.Error("nil user clones to empty attrs")
	}
	if attrs := cloneUserAttributes(&keycloak.User{}); len(attrs) != 0 {
		t.Error("nil attribute map clones to empty attrs")
	}
	src := &keycloak.User{Attributes: map[string][]string{"k": {"v"}}}
	cp := cloneUserAttributes(src)
	cp["k"][0] = "changed"
	if src.Attributes["k"][0] != "v" {
		t.Error("clone must deep copy values")
	}
	m := map[string][]string{"keep": {"x"}}
	setUserAttribute(m, "  ", "ignored")
	setUserAttribute(m, "a", "  v  ")
	setUserAttribute(m, "keep", "   ")
	if _, ok := m["keep"]; ok || m["a"][0] != "v" || len(m) != 1 {
		t.Errorf("setUserAttribute produced %v", m)
	}
	if firstUserAttribute(nil, "k") != "" || firstUserAttribute(&keycloak.User{}, "k") != "" ||
		firstUserAttribute(&keycloak.User{Attributes: map[string][]string{"k": {}}}, "k") != "" ||
		firstUserAttribute(&keycloak.User{Attributes: map[string][]string{"k": {" v "}}}, "k") != "v" {
		t.Error("firstUserAttribute mismatch")
	}

	if actorMaxTier([]string{"parent", " Coach "}) != roleTier["coach"] || actorMaxTier(nil) != 0 {
		t.Error("actorMaxTier mismatch")
	}
}

func TestCov1o7AttemptLimiterWindowAndEviction(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newAttemptLimiter(1, time.Hour)
	l.now = func() time.Time { return now }

	if !l.allow("A@x.io") || l.allow(" a@x.io ") {
		t.Fatal("second attempt inside the window (case/space-insensitive) must be refused")
	}
	now = now.Add(2 * time.Hour)
	if !l.allow("a@x.io") {
		t.Fatal("attempt after the window expired must be allowed again")
	}

	// Flood with distinct keys: once the map exceeds 10000 entries, stale ones are evicted.
	l2 := newAttemptLimiter(1, time.Hour)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := t0
	l2.now = func() time.Time { return cur }
	for i := 0; i < 5000; i++ {
		l2.allow(fmt.Sprintf("old-%d", i))
	}
	cur = t0.Add(3 * time.Hour)
	for i := 0; i < 5001; i++ {
		l2.allow(fmt.Sprintf("new-%d", i))
	}
	if len(l2.seen) != 5001 {
		t.Fatalf("after eviction want 5001 live keys, got %d", len(l2.seen))
	}
}

func TestCov1o7ListAllUsersPagesToExhaustion(t *testing.T) {
	kc := cov1o7NewKC()
	for i := 0; i < 205; i++ {
		kc.AddUser(keycloak.User{ID: fmt.Sprintf("bulk-%03d", i), Email: fmt.Sprintf("b%d@example.com", i), Enabled: true, Attributes: map[string][]string{"org_id": {cov1o7Org}}})
	}
	h := NewUserHandlers(kc, nil)
	all, err := h.listAllUsers(context.Background(), cov1o7Org, "")
	if err != nil || len(all) != 205 {
		t.Fatalf("listAllUsers = %d users, err %v; want 205", len(all), err)
	}
	kc.failOn("ListUsers", cov1o7Boom, 1) // second page fails
	if _, err := h.listAllUsers(context.Background(), cov1o7Org, ""); !errors.Is(err, cov1o7Boom) {
		t.Fatalf("want page error propagated, got %v", err)
	}
}

func TestCov1o7WriteKeycloakAdminErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{keycloak.ErrInvalidConfig, 503, "keycloak_unavailable"},
		{&keycloak.StatusError{Status: 400}, 400, "invalid_request"},
		{&keycloak.StatusError{Status: 502}, 503, "keycloak_unavailable"},
		{keycloak.ErrUnauthorized, 503, "keycloak_unavailable"},
		{cov1o7Boom, 500, "keycloak_error"},
	} {
		rec := httptest.NewRecorder()
		writeKeycloakAdminError(rec, httptest.NewRequest(http.MethodGet, "/", nil), tc.err, "fallback")
		cov1o7Expect(t, rec, tc.status, tc.code)
	}
	rec := httptest.NewRecorder()
	writeKeycloakUserError(rec, httptest.NewRequest(http.MethodGet, "/", nil), keycloak.ErrUserNotFound, "x")
	cov1o7Expect(t, rec, 404, "user_not_found")
}

func TestCov1o7HandlersRequireAuthContext(t *testing.T) {
	e := cov1o7New(t)
	handlers := map[string]http.HandlerFunc{
		"listUsers": e.h.listUsers, "listStaff": e.h.listStaff, "getUser": e.h.getUser,
		"createUser": e.h.createUser, "inviteUser": e.h.inviteUser, "updateUser": e.h.updateUser,
		"disableUser": e.h.disableUser, "enableUser": e.h.enableUser, "approveUser": e.h.approveUser,
		"rejectUser": e.h.rejectUser, "deleteUser": e.h.deleteUser, "listUserRoles": e.h.listUserRoles,
		"assignRole": e.h.assignRole, "removeRole": e.h.removeRole, "passwordReset": e.h.passwordReset,
		"getUserHousehold": e.h.getUserHousehold, "linkUserHousehold": e.h.linkUserHousehold,
		"unlinkUserHousehold": e.h.unlinkUserHousehold, "getUserSwimmer": e.h.getUserSwimmer,
		"linkUserSwimmer": e.h.linkUserSwimmer, "unlinkUserSwimmer": e.h.unlinkUserSwimmer,
		"createSwimmerLinkRequest": e.h.createSwimmerLinkRequest, "listSwimmerLinkRequests": e.h.listSwimmerLinkRequests,
		"resolveSwimmerLinkRequest": e.h.resolveSwimmerLinkRequest,
	}
	for name, fn := range handlers {
		t.Run(name+"/no-principal", func(t *testing.T) {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
			cov1o7Expect(t, rec, http.StatusUnauthorized, "unauthorized")
		})
		t.Run(name+"/no-org", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			req = req.WithContext(appmiddleware.ContextWithPrincipal(req.Context(), cov1o7Tokens["admin"]))
			rec := httptest.NewRecorder()
			fn(rec, req)
			cov1o7Expect(t, rec, http.StatusBadRequest, "missing_org_scope")
		})
	}
}

// ─── Listing and reads ────────────────────────────────────────────────────────

func TestCov1o7ListUsers(t *testing.T) {
	type listResp struct {
		Count  int    `json:"count"`
		Total  int    `json:"total"`
		Source string `json:"source"`
		Users  []struct {
			ID    string   `json:"id"`
			Roles []string `json:"roles"`
		} `json:"users"`
	}
	get := func(e *cov1o7Env, path string) (*httptest.ResponseRecorder, listResp) {
		rec := e.do(http.MethodGet, path, "admin", "")
		var out listResp
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	t.Run("unfiltered short page reports offset plus length", func(t *testing.T) {
		e := cov1o7New(t)
		rec, out := get(e, "/users?limit=50")
		cov1o7Expect(t, rec, 200, "")
		if out.Total != 5 || out.Count != 5 || out.Source != "keycloak" {
			t.Fatalf("got %+v", out)
		}
	})
	t.Run("full page walks the org for the real total", func(t *testing.T) {
		e := cov1o7New(t)
		_, out := get(e, "/users?limit=2")
		if out.Count != 2 || out.Total != 5 {
			t.Fatalf("got %+v", out)
		}
	})
	t.Run("status filters", func(t *testing.T) {
		e := cov1o7New(t)
		for status, want := range map[string]int{"active": 4, "disabled": 1, "pending_approval": 1, "bogus": 0} {
			_, out := get(e, "/users?status="+status)
			if out.Total != want || out.Count != want {
				t.Errorf("status=%s: got %+v, want %d", status, out, want)
			}
		}
	})
	t.Run("role filter normalizes role names", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.roles["p-1"] = []keycloak.Role{{Name: " Parent "}, {Name: "parent"}}
		_, out := get(e, "/users?role=PARENT&status=active")
		if out.Total != 1 || out.Users[0].ID != "p-1" || len(out.Users[0].Roles) != 1 || out.Users[0].Roles[0] != "parent" {
			t.Fatalf("got %+v", out)
		}
	})
	t.Run("directory failures are surfaced", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("ListUsers", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodGet, "/users", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("ListUsers", cov1o7Unavailable, 1) // first page ok, walk fails
		cov1o7Expect(t, e.do(http.MethodGet, "/users?limit=2", "admin", ""), 503, "keycloak_unavailable")

		e = cov1o7New(t)
		e.kc.failOn("ListUsers", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodGet, "/users?status=active", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodGet, "/users?role=parent", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodGet, "/users", "admin", ""), 500, "keycloak_error")
	})
	t.Run("non-admin is denied", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodGet, "/users", "coach", ""), 403, "forbidden")
	})
}

func TestCov1o7ListStaffAndGetUser(t *testing.T) {
	e := cov1o7New(t)
	rec := e.do(http.MethodGet, "/users/staff", "coach", "")
	cov1o7Expect(t, rec, 200, "")
	body := cov1o7JSON(t, rec)
	if body["count"].(float64) != 3 {
		t.Fatalf("staff = %v, want 3 enabled admin/coach accounts", body["staff"])
	}
	e.kc.failOn("ListUsersWithRealmRole", cov1o7Boom)
	cov1o7Expect(t, e.do(http.MethodGet, "/users/staff", "coach", ""), 500, "keycloak_error")

	e = cov1o7New(t)
	rec = e.do(http.MethodGet, "/users/p-1", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	roles := cov1o7JSON(t, rec)["roles"].([]any)
	if len(roles) != 1 || roles[0].(map[string]any)["name"] != "parent" {
		t.Fatalf("roles = %v", roles)
	}
	cov1o7Expect(t, e.do(http.MethodGet, "/users/nobody", "admin", ""), 404, "user_not_found")
	cov1o7Expect(t, e.do(http.MethodGet, "/users/out-1", "admin", ""), 403, "forbidden")
	e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
	cov1o7Expect(t, e.do(http.MethodGet, "/users/p-1", "admin", ""), 500, "keycloak_error")
}

// ─── Public self-registration ────────────────────────────────────────────────

func cov1o7Register(e *cov1o7Env, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.public.ServeHTTP(rec, req)
	return rec
}

func (e *cov1o7Env) userByEmail(email string) *keycloak.User {
	e.t.Helper()
	users, err := e.kc.StubClient.ListUsers(context.Background(), "", email, 5, 0)
	if err != nil || len(users) != 1 {
		e.t.Fatalf("user %s: %v %v", email, users, err)
	}
	return &users[0]
}

func TestCov1o7SelfRegisterValidationAndTenant(t *testing.T) {
	clubs := cov1o7Clubs{bySlug: map[string]*orgsettings.OrgSettings{
		"blue":  {OrgID: cov1o7Org},
		"blank": {OrgID: "  "},
	}}
	cases := []struct {
		name   string
		clubs  cov1o7Clubs
		useRes bool
		allow  bool
		body   string
		status int
		code   string
	}{
		{"bad json", clubs, true, false, `{`, 400, "invalid_request"},
		{"missing email", clubs, true, false, `{"club_slug":"blue"}`, 400, "invalid_request"},
		{"invalid email", clubs, true, false, `{"email":"nope","club_slug":"blue"}`, 400, "invalid_request"},
		{"long name", clubs, true, false, `{"email":"a@b.io","club_slug":"blue","first_name":"` + strings.Repeat("x", 101) + `"}`, 400, "invalid_request"},
		{"slug without resolver", clubs, false, false, `{"email":"a@b.io","club_slug":"blue"}`, 404, "club_not_found"},
		{"unknown slug", clubs, true, false, `{"email":"a@b.io","club_slug":"nope"}`, 404, "club_not_found"},
		{"blank org slug", clubs, true, false, `{"email":"a@b.io","club_slug":"blank"}`, 404, "club_not_found"},
		{"resolver error", cov1o7Clubs{err: cov1o7Boom}, true, false, `{"email":"a@b.io","club_slug":"blue"}`, 500, "internal_error"},
		{"org mismatch", clubs, true, false, `{"email":"a@b.io","club_slug":"blue","org_id":"org-9"}`, 400, "invalid_request"},
		{"raw org refused outside dev", clubs, true, false, `{"email":"a@b.io","org_id":"org-1"}`, 400, "invalid_request"},
		{"no tenant handle", clubs, true, true, `{"email":"a@b.io"}`, 400, "invalid_request"},
		{"unknown role", clubs, true, false, `{"email":"a@b.io","club_slug":"blue","requested_role":"owner"}`, 400, "invalid_role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := cov1o7New(t)
			if tc.useRes {
				e.h.SetClubResolver(tc.clubs, tc.allow)
			}
			cov1o7Expect(t, cov1o7Register(e, tc.body), tc.status, tc.code)
			if len(e.kc.InvitedEmails()) != 0 {
				t.Fatalf("a rejected registration must not create an account")
			}
		})
	}
}

func TestCov1o7SelfRegisterCreatesPendingAccountAndAudits(t *testing.T) {
	e := cov1o7New(t)
	e.h.SetClubResolver(cov1o7Clubs{}, true) // legacy raw org_id allowed (dev/test)
	rec := cov1o7Register(e, `{"email":" New@Example.com ","first_name":"Nia","last_name":"Lee","org_id":"org-1","requested_role":"Coach","club_code":"abc","note":"hi","swimmer_name":"Kid"}`)
	cov1o7Expect(t, rec, 201, "")
	if cov1o7JSON(t, rec)["status"] != "pending_approval" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	u := e.userByEmail("new@example.com")
	if u.Enabled != true || firstUserAttribute(u, "registration_status") != "pending_approval" ||
		firstUserAttribute(u, "requested_role") != "coach" || firstUserAttribute(u, "registration_club_code") != "ABC" ||
		firstUserAttribute(u, "registration_note") != "hi" || firstUserAttribute(u, "registration_swimmer_name") != "Kid" ||
		u.OrgID() != cov1o7Org {
		t.Fatalf("registration attributes = %v", u.Attributes)
	}
	evs := e.auditActions(audit.ActionAuthUserSelfRegister)
	if len(evs) != 1 || evs[0].Metadata["requested_role"] != "coach" || evs[0].Metadata["club_code"] != "ABC" || evs[0].Metadata["swimmer_name"] != "Kid" {
		t.Fatalf("audit = %+v", evs)
	}
}

func TestCov1o7SelfRegisterDoesNotRevealExistingAccountsAndRateLimits(t *testing.T) {
	e := cov1o7New(t)
	e.h.SetClubResolver(cov1o7Clubs{bySlug: map[string]*orgsettings.OrgSettings{"blue": {OrgID: cov1o7Org}}}, false)
	body := `{"email":"p-1@example.com","club_slug":"blue"}`
	var firstID string
	for i := 0; i < 3; i++ {
		rec := cov1o7Register(e, body)
		cov1o7Expect(t, rec, 201, "")
		out := cov1o7JSON(t, rec)
		if out["status"] != "pending_approval" || out["user_id"] == "" || out["user_id"] == "p-1" {
			t.Fatalf("existing email must look like a fresh registration: %v", out)
		}
		if i == 0 {
			firstID = out["user_id"].(string)
		} else if out["user_id"] == firstID {
			t.Fatalf("decoy ids should be fresh each time")
		}
	}
	cov1o7Expect(t, cov1o7Register(e, body), 429, "rate_limited")
}

func TestCov1o7SelfRegisterInviteCodes(t *testing.T) {
	clubs := cov1o7Clubs{bySlug: map[string]*orgsettings.OrgSettings{"blue": {OrgID: cov1o7Org}}}
	body := `{"email":"inv@example.com","club_slug":"blue","club_code":"join","requested_role":"parent"}`
	okInvite := func(orgID, code string) (*invites.Invite, error) {
		return &invites.Invite{ID: "inv-1", OrgID: orgID, Role: "swimmer"}, nil
	}

	t.Run("invalid code", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		e.h.SetInvites(&cov1o7Invites{redeem: func(string, string) (*invites.Invite, error) { return nil, invites.ErrInvalidCode }})
		cov1o7Expect(t, cov1o7Register(e, body), 400, "invalid_invite")
	})
	t.Run("redeem failure", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		e.h.SetInvites(&cov1o7Invites{redeem: func(string, string) (*invites.Invite, error) { return nil, cov1o7Boom }})
		cov1o7Expect(t, cov1o7Register(e, body), 500, "internal_error")
	})
	t.Run("success overrides the requested role", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		inv := &cov1o7Invites{redeem: okInvite}
		e.h.SetInvites(inv)
		cov1o7Expect(t, cov1o7Register(e, body), 201, "")
		u := e.userByEmail("inv@example.com")
		if firstUserAttribute(u, "requested_role") != "swimmer" || firstUserAttribute(u, "registration_invite_id") != "inv-1" || len(inv.released) != 0 {
			t.Fatalf("attrs = %v released = %v", u.Attributes, inv.released)
		}
	})
	t.Run("existing email releases the consumed use", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		inv := &cov1o7Invites{redeem: okInvite}
		e.h.SetInvites(inv)
		cov1o7Expect(t, cov1o7Register(e, `{"email":"p-1@example.com","club_slug":"blue","club_code":"join"}`), 201, "")
		if len(inv.released) != 1 || inv.released[0] != "org-1/inv-1" {
			t.Fatalf("released = %v", inv.released)
		}
	})
	t.Run("keycloak failure releases the use", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		inv := &cov1o7Invites{redeem: okInvite}
		e.h.SetInvites(inv)
		e.kc.failOn("CreateUser", cov1o7Unavailable)
		cov1o7Expect(t, cov1o7Register(e, body), 503, "keycloak_unavailable")
		if len(inv.released) != 1 {
			t.Fatalf("released = %v", inv.released)
		}
	})
	t.Run("half-registered account is removed and use released even when cleanup fails", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		inv := &cov1o7Invites{redeem: okInvite, releaseErr: cov1o7Boom}
		e.h.SetInvites(inv)
		e.kc.failOn("UpdateUser", cov1o7Boom)
		e.kc.failOn("DeleteUser", cov1o7Boom)
		cov1o7Expect(t, cov1o7Register(e, body), 500, "keycloak_error")
		if len(inv.released) != 1 {
			t.Fatalf("released = %v", inv.released)
		}
	})
	t.Run("update failure deletes the half-registered account", func(t *testing.T) {
		e := cov1o7New(t)
		e.h.SetClubResolver(clubs, false)
		e.kc.failOn("UpdateUser", cov1o7Boom)
		cov1o7Expect(t, cov1o7Register(e, `{"email":"half@example.com","club_slug":"blue"}`), 500, "keycloak_error")
		users, _ := e.kc.StubClient.ListUsers(context.Background(), "", "half@example.com", 5, 0)
		if len(users) != 0 {
			t.Fatalf("half-registered user left behind: %+v", users)
		}
	})
}

// ─── Admin create / invite ───────────────────────────────────────────────────

func TestCov1o7CreateUser(t *testing.T) {
	valid := `{"email":"New@Example.com","first_name":"Nia","last_name":"Lee","password":"longenough1","role":"parent"}`
	t.Run("validation", func(t *testing.T) {
		e := cov1o7New(t)
		for name, tc := range map[string]struct{ body, code string }{
			"bad json":       {`{`, "invalid_request"},
			"no email":       {`{"password":"longenough1"}`, "invalid_request"},
			"bad email":      {`{"email":"x","password":"longenough1"}`, "invalid_request"},
			"no password":    {`{"email":"a@b.io"}`, "invalid_request"},
			"short password": {`{"email":"a@b.io","password":"short"}`, "invalid_request"},
			"unknown role":   {`{"email":"a@b.io","password":"longenough1","role":"owner"}`, "invalid_role"},
		} {
			cov1o7Expect(t, e.do(http.MethodPost, "/users", "admin", tc.body), 400, tc.code)
			_ = name
		}
		if len(e.kc.InvitedEmails()) != 0 {
			t.Fatal("no account may be created for a rejected request")
		}
	})
	t.Run("tier and MFA guards", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users", "coach", `{"email":"a@b.io","password":"longenough1","role":"admin"}`), 403, "forbidden")
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users", "nomfa", `{"email":"a@b.io","password":"longenough1","role":"coach"}`), 403, "mfa_required")
	})
	t.Run("keycloak failures", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users", "admin", `{"email":"p-1@example.com","password":"longenough1"}`), 409, "user_already_exists")
		e.kc.failOn("CreateUser", cov1o7Unavailable)
		cov1o7Expect(t, e.do(http.MethodPost, "/users", "admin", valid), 503, "keycloak_unavailable")

		e = cov1o7New(t)
		e.kc.failOn("SetUserPassword", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodPost, "/users", "admin", valid), 500, "keycloak_error")
		users, _ := e.kc.StubClient.ListUsers(context.Background(), "", "new@example.com", 5, 0)
		if len(users) != 0 {
			t.Fatalf("account with unusable password must be rolled back, found %+v", users)
		}
	})
	t.Run("success with household and role", func(t *testing.T) {
		e := cov1o7New(t)
		rec := e.do(http.MethodPost, "/users", "admin", `{"email":"New@Example.com","first_name":"Nia","last_name":"Lee","password":"longenough1","role":"Parent","create_household":true}`)
		cov1o7Expect(t, rec, 201, "")
		out := cov1o7JSON(t, rec)
		id := out["user_id"].(string)
		if out["status"] != "created" || out["household_id"] == nil {
			t.Fatalf("body = %v", out)
		}
		if roles := e.realmRoles(id); len(roles) != 1 || roles[0] != "parent" {
			t.Fatalf("roles = %v", roles)
		}
		hh, ok := e.members.GetHouseholdByKeycloakUserID(id)
		if !ok || hh.HouseholdName != "Nia Lee" || hh.OrgID != cov1o7Org {
			t.Fatalf("linked household = %+v ok=%v", hh, ok)
		}
		if evs := e.auditActions(audit.ActionAuthUserInvite); len(evs) != 1 || evs[0].Metadata["method"] != "direct_create" {
			t.Fatalf("audit = %+v", evs)
		}
	})
	t.Run("nameless account gets a household named after the email", func(t *testing.T) {
		e := cov1o7New(t)
		rec := e.do(http.MethodPost, "/users", "admin", `{"email":"solo@example.com","password":"longenough1","create_household":true}`)
		cov1o7Expect(t, rec, 201, "")
		hh, ok := e.members.GetHouseholdByKeycloakUserID(cov1o7JSON(t, rec)["user_id"].(string))
		if !ok || hh.HouseholdName != "solo@example.com" {
			t.Fatalf("household = %+v", hh)
		}
	})
	t.Run("role assignment and household failures are non-fatal", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("AssignRealmRole", cov1o7Boom)
		rec := e.do(http.MethodPost, "/users", "admin", valid)
		cov1o7Expect(t, rec, 201, "")
		if evs := e.auditActions(audit.ActionAuthRoleAssign); len(evs) != 1 || evs[0].Metadata["error"] != "boom" {
			t.Fatalf("role-assign failure must be audited: %+v", evs)
		}

		// A swimmer-role caller cannot create households, so the linked household fails.
		e = cov1o7New(t)
		rec = e.dodirect(http.MethodPost, "/users", "swimmer", `{"email":"hh@example.com","password":"longenough1","create_household":true}`)
		cov1o7Expect(t, rec, 201, "")
		out := cov1o7JSON(t, rec)
		if out["household_created"] != false || out["warning"] == nil || out["household_id"] != nil {
			t.Fatalf("body = %v", out)
		}
		found := false
		for _, ev := range e.auditActions(audit.ActionAuthUserInvite) {
			if ev.Metadata["household_error"] != "" {
				found = true
			}
		}
		if !found {
			t.Fatal("household failure must be audited")
		}
	})
}

func TestCov1o7InviteUser(t *testing.T) {
	t.Run("validation and guards", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/invite", "admin", `{`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/invite", "admin", `{}`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/invite", "admin", `{"email":"bad"}`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/invite", "admin", `{"email":"a@b.io","role":"owner"}`), 400, "invalid_role")
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users/invite", "coach", `{"email":"a@b.io","role":"admin"}`), 403, "forbidden")
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users/invite", "nomfa", `{"email":"a@b.io","role":"admin"}`), 403, "mfa_required")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/invite", "admin", `{"email":"p-1@example.com"}`), 409, "user_already_exists")
	})
	t.Run("success sends the verification email and creates a household", func(t *testing.T) {
		e := cov1o7New(t)
		rec := e.do(http.MethodPost, "/users/invite", "admin", `{"email":"inv@example.com","first_name":"Ivy","role":"parent","create_household":true}`)
		cov1o7Expect(t, rec, 201, "")
		out := cov1o7JSON(t, rec)
		id := out["user_id"].(string)
		if out["household_id"] == nil || out["warnings"] != nil {
			t.Fatalf("body = %v", out)
		}
		if got := e.kc.VerificationsSent(); len(got) != 1 || got[0] != id {
			t.Fatalf("verification emails = %v", got)
		}
	})
	t.Run("email failure is reported but the account stands", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("SendVerificationEmail", cov1o7Boom)
		rec := e.do(http.MethodPost, "/users/invite", "admin", `{"email":"inv@example.com","role":"parent"}`)
		cov1o7Expect(t, rec, 201, "")
		warnings, _ := cov1o7JSON(t, rec)["warnings"].([]any)
		if len(warnings) != 1 || warnings[0] != "verification_email_failed" {
			t.Fatalf("body = %s", rec.Body.String())
		}
		if evs := e.auditActions(audit.ActionAuthUserInvite); len(evs) != 1 || evs[0].Metadata["verification_email_failed"] != "true" {
			t.Fatalf("audit = %+v", evs)
		}
	})
	t.Run("household failure is flagged", func(t *testing.T) {
		e := cov1o7New(t)
		rec := e.dodirect(http.MethodPost, "/users/invite", "swimmer", `{"email":"inv@example.com","create_household":true}`)
		cov1o7Expect(t, rec, 201, "")
		if cov1o7JSON(t, rec)["household_created"] != false {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})
}

// ─── Update / disable / enable ───────────────────────────────────────────────

func TestCov1o7UpdateDisableEnable(t *testing.T) {
	t.Run("update", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPut, "/users/p-1", "admin", `{`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPut, "/users/p-1", "admin", `{"email":"bad"}`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPut, "/users/nobody", "admin", `{}`), 404, "user_not_found")
		cov1o7Expect(t, e.do(http.MethodPut, "/users/out-1", "admin", `{}`), 403, "forbidden")
		cov1o7Expect(t, e.do(http.MethodPut, "/users/p-1", "admin", `{"first_name":" Pat ","email":" NEW@x.io "}`), 200, "")
		if u := e.user("p-1"); u.FirstName != "Pat" || u.Email != "new@x.io" {
			t.Fatalf("user = %+v", u)
		}
		e.kc.failOn("UpdateUser", keycloak.ErrUserNotFound)
		cov1o7Expect(t, e.do(http.MethodPut, "/users/p-1", "admin", `{}`), 404, "user_not_found")
	})
	t.Run("disable", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/admin-1/disable", "admin", ""), 409, "cannot_disable_self")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/disable", "admin", ""), 403, "forbidden")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/disable", "admin", ""), 200, "")
		if e.user("p-1").Enabled {
			t.Fatal("user must be disabled")
		}
		if len(e.revoker.calls) != 1 || e.revoker.calls[0] != "org-1/p-1" {
			t.Fatalf("revoked = %v", e.revoker.calls)
		}
		if evs := e.auditActions(audit.ActionAuthSessionRevoke); len(evs) != 1 || evs[0].Metadata["reason"] != "user_disabled" || evs[0].Metadata["revoked_count"] != "2" {
			t.Fatalf("revoke audit = %+v", evs)
		}
	})
	t.Run("disable refuses the last enabled admin and surfaces lookup failures", func(t *testing.T) {
		e := cov1o7New(t)
		_ = e.kc.StubClient.DisableUser(context.Background(), "admin-2")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/admin-1/disable", "ghost", ""), 409, "last_admin")

		e = cov1o7New(t)
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/disable", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("ListUsersWithRealmRole", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/admin-2/disable", "admin", ""), 500, "keycloak_error")
	})
	t.Run("disable failures", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("DisableUser", keycloak.ErrUserNotFound)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/disable", "admin", ""), 404, "user_not_found")

		e = cov1o7New(t)
		e.revoker.err = cov1o7Boom
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/disable", "admin", ""), 503, "session_revocation_failed")
		if e.user("p-1").Enabled {
			t.Fatal("the disable stands even when revocation fails")
		}
		if evs := e.auditActions(audit.ActionAuthSessionRevoke); len(evs) != 1 || evs[0].Metadata["error"] != "revoke_failed" {
			t.Fatalf("failed revocation must be audited: %+v", evs)
		}
	})
	t.Run("enable", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/enable", "admin", ""), 200, "")
		if !e.user("pend-1").Enabled {
			t.Fatal("user must be enabled")
		}
		cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/enable", "admin", ""), 403, "forbidden")
		e.kc.failOn("EnableUser", cov1o7Unavailable)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/enable", "admin", ""), 503, "keycloak_unavailable")
	})
}

// ─── Approve / reject / delete ───────────────────────────────────────────────

func TestCov1o7ApproveUser(t *testing.T) {
	t.Run("default parent role, enables, and sends the email", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/approve", "admin", ""), 200, "")
		u := e.user("pend-1")
		if !u.Enabled || firstUserAttribute(u, "registration_status") != "active" || u.OrgID() != cov1o7Org {
			t.Fatalf("user = %+v", u)
		}
		if roles := e.realmRoles("pend-1"); len(roles) != 1 || roles[0] != "parent" {
			t.Fatalf("roles = %v", roles)
		}
		if len(e.kc.VerificationsSent()) != 1 {
			t.Fatal("approval must send the verification email")
		}
	})
	t.Run("existing app role is kept and blank requested role falls back to parent", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.AddUser(keycloak.User{ID: "has-role", Email: "hr@example.com", Attributes: map[string][]string{"org_id": {cov1o7Org}}})
		_ = e.kc.StubClient.AssignRealmRole(context.Background(), "has-role", "coach")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/has-role/approve", "admin", ""), 200, "")
		if roles := e.realmRoles("has-role"); len(roles) != 1 || roles[0] != "coach" {
			t.Fatalf("roles = %v", roles)
		}
	})
	t.Run("guards", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.AddUser(keycloak.User{ID: "wants-admin", Email: "wa@example.com", Attributes: map[string][]string{"org_id": {cov1o7Org}, "requested_role": {"admin"}}})
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users/wants-admin/approve", "coach", ""), 403, "insufficient_role_tier")
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users/wants-admin/approve", "nomfa", ""), 403, "mfa_required")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/approve", "admin", ""), 403, "forbidden")
		if e.user("wants-admin").Enabled {
			t.Fatal("a refused approval must leave the account untouched")
		}
	})
	t.Run("each step failure is reported", func(t *testing.T) {
		for _, step := range []struct {
			method string
			err    error
			status int
			code   string
		}{
			{"EnableUser", keycloak.ErrUserNotFound, 404, "user_not_found"},
			{"UpdateUser", cov1o7Boom, 500, "keycloak_error"},
			{"GetUserRealmRoles", cov1o7Boom, 500, "keycloak_error"},
			{"AssignRealmRole", cov1o7Boom, 500, "keycloak_error"},
		} {
			e := cov1o7New(t)
			e.kc.failOn(step.method, step.err)
			cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/approve", "admin", ""), step.status, step.code)
		}
	})
	t.Run("verification email failure is a warning", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("SendVerificationEmail", cov1o7Boom)
		rec := e.do(http.MethodPost, "/users/pend-1/approve", "admin", "")
		cov1o7Expect(t, rec, 200, "")
		if w, _ := cov1o7JSON(t, rec)["warnings"].([]any); len(w) != 1 {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})
}

func TestCov1o7RejectUser(t *testing.T) {
	t.Run("rejects, records the reason, revokes sessions", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/reject", "admin", `{"reason":" spam "}`), 200, "")
		u := e.user("pend-1")
		if u.Enabled || firstUserAttribute(u, "registration_status") != "rejected" {
			t.Fatalf("user = %+v", u)
		}
		if evs := e.auditActions(audit.ActionAuthUserReject); len(evs) != 1 || evs[0].Metadata["reason"] != "spam" {
			t.Fatalf("audit = %+v", evs)
		}
		if len(e.revoker.calls) != 1 {
			t.Fatalf("revoked = %v", e.revoker.calls)
		}
	})
	t.Run("guards", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/admin-1/reject", "admin", ""), 409, "cannot_reject_self")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/reject", "admin", ""), 403, "forbidden")
		_ = e.kc.StubClient.DisableUser(context.Background(), "admin-2")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/admin-1/reject", "ghost", ""), 409, "last_admin")
	})
	t.Run("failures", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("DisableUser", keycloak.ErrUserNotFound)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/reject", "admin", ""), 404, "user_not_found")

		e = cov1o7New(t)
		e.kc.failOn("UpdateUser", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/reject", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.revoker.err = cov1o7Boom
		cov1o7Expect(t, e.do(http.MethodPost, "/users/pend-1/reject", "admin", ""), 503, "session_revocation_failed")
	})
}

func TestCov1o7DeleteUser(t *testing.T) {
	t.Run("guards", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1", "nomfa", ""), 403, "mfa_required")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-1", "admin", ""), 409, "cannot_delete_self")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/nobody", "admin", ""), 404, "user_not_found")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/out-1", "admin", ""), 403, "forbidden")
		_ = e.kc.StubClient.DisableUser(context.Background(), "admin-2")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-1", "ghost", ""), 409, "last_admin")
	})
	t.Run("lookup failures leave the user in place", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("ListUsersWithRealmRole", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-2", "admin", ""), 500, "keycloak_error")
		if e.user("admin-2") == nil {
			t.Fatal("admin must still exist")
		}
	})
	t.Run("deleting a non-last admin and a plain user", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-2", "admin", ""), 200, "")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1", "admin", ""), 200, "")
		if _, err := e.kc.StubClient.GetUser(context.Background(), "p-1"); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("user must be gone: %v", err)
		}
		if len(e.revoker.calls) != 2 {
			t.Fatalf("revoked = %v", e.revoker.calls)
		}
	})
	t.Run("delete and revoke failures", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("DeleteUser", keycloak.ErrUserNotFound)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1", "admin", ""), 404, "user_not_found")

		e = cov1o7New(t)
		e.revoker.err = cov1o7Boom
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1", "admin", ""), 503, "session_revocation_failed")
	})
}

// ─── Roles and password reset ────────────────────────────────────────────────

func TestCov1o7ListAssignRemoveRoles(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		e := cov1o7New(t)
		rec := e.do(http.MethodGet, "/users/pend-1/roles", "admin", "")
		cov1o7Expect(t, rec, 200, "")
		if roles := cov1o7JSON(t, rec)["roles"].([]any); len(roles) != 0 {
			t.Fatalf("roles = %v", roles)
		}
		e.kc.roles["pend-1"] = nil
		cov1o7Expect(t, e.do(http.MethodGet, "/users/pend-1/roles", "admin", ""), 200, "")
		cov1o7Expect(t, e.do(http.MethodGet, "/users/out-1/roles", "admin", ""), 403, "forbidden")
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodGet, "/users/p-1/roles", "admin", ""), 500, "keycloak_error")
	})
	t.Run("assign", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "nomfa", `{"role":"coach"}`), 403, "mfa_required")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/roles", "admin", `{"role":"coach"}`), 403, "forbidden")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"  "}`), 400, "invalid_request")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"owner"}`), 400, "invalid_role")
		cov1o7Expect(t, e.dodirect(http.MethodPost, "/users/p-1/roles", "coach", `{"role":"admin"}`), 403, "forbidden")
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"Coach"}`), 200, "")
		if roles := e.realmRoles("p-1"); strings.Join(roles, ",") != "coach,parent" {
			t.Fatalf("roles = %v", roles)
		}
		if len(e.revoker.calls) != 1 {
			t.Fatalf("role change must revoke sessions: %v", e.revoker.calls)
		}
	})
	t.Run("assign failures", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("AssignRealmRole", keycloak.ErrRoleNotFound)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"coach"}`), 400, "role_not_found")

		e = cov1o7New(t)
		e.kc.failOn("AssignRealmRole", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"coach"}`), 500, "keycloak_error")

		e = cov1o7New(t)
		e.revoker.err = cov1o7Boom
		cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/roles", "admin", `{"role":"coach"}`), 503, "session_revocation_failed")
	})
	t.Run("remove guards", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1/roles/parent", "nomfa", ""), 403, "mfa_required")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/out-1/roles/parent", "admin", ""), 403, "forbidden")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1/roles/owner", "admin", ""), 400, "invalid_role")
		cov1o7Expect(t, e.dodirect(http.MethodDelete, "/users/admin-2/roles/admin", "coach", ""), 403, "forbidden")
		_ = e.kc.StubClient.DisableUser(context.Background(), "admin-2")
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-1/roles/admin", "ghost", ""), 409, "last_admin")
	})
	t.Run("remove lookup failures", func(t *testing.T) {
		e := cov1o7New(t)
		e.kc.failOn("GetUserRealmRoles", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-2/roles/admin", "admin", ""), 500, "keycloak_error")

		e = cov1o7New(t)
		e.kc.failOn("ListUsersWithRealmRole", cov1o7Boom)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-2/roles/admin", "admin", ""), 500, "keycloak_error")
	})
	t.Run("remove outcomes", func(t *testing.T) {
		e := cov1o7New(t)
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/admin-2/roles/Admin", "admin", ""), 200, "")
		if roles := e.realmRoles("admin-2"); len(roles) != 0 {
			t.Fatalf("roles = %v", roles)
		}
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{keycloak.ErrRoleNotFound, 400, "role_not_found"},
			{keycloak.ErrLastAdmin, 409, "last_admin"},
			{cov1o7Boom, 500, "keycloak_error"},
		} {
			e := cov1o7New(t)
			e.kc.failOn("RemoveRealmRole", tc.err)
			cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1/roles/parent", "admin", ""), tc.status, tc.code)
		}
		e = cov1o7New(t)
		e.revoker.err = cov1o7Boom
		cov1o7Expect(t, e.do(http.MethodDelete, "/users/p-1/roles/parent", "admin", ""), 503, "session_revocation_failed")
	})
}

func TestCov1o7PasswordReset(t *testing.T) {
	e := cov1o7New(t)
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/password-reset", "admin", ""), 200, "")
	if got := e.kc.PasswordResetsSent(); len(got) != 1 || got[0] != "p-1" {
		t.Fatalf("resets = %v", got)
	}
	cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/password-reset", "admin", ""), 403, "forbidden")
	e.kc.failOn("SendPasswordResetEmail", keycloak.ErrUserNotFound)
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/password-reset", "admin", ""), 404, "user_not_found")
}

// ─── Household / swimmer linkage ─────────────────────────────────────────────

func (e *cov1o7Env) household(org, name string) members.Household {
	e.t.Helper()
	hh, err := e.members.CreateHousehold(&appmiddleware.Principal{UserID: "admin-1", OrgID: org, Roles: []string{"admin"}},
		members.CreateHouseholdInput{OrgID: org, HouseholdName: name, PrimaryEmail: strings.ToLower(name) + "@example.com"})
	if err != nil {
		e.t.Fatal(err)
	}
	return hh
}

func (e *cov1o7Env) swimmer(org, householdID, first, dob string) members.Swimmer {
	e.t.Helper()
	sw, _, err := e.members.CreateSwimmer(&appmiddleware.Principal{UserID: "admin-1", OrgID: org, Roles: []string{"admin"}}, org,
		members.CreateSwimmerInput{HouseholdID: householdID, FirstName: first, LastName: "Test", DateOfBirth: dob, Gender: "female", RegistrationStatus: "active"})
	if err != nil {
		e.t.Fatal(err)
	}
	return sw
}

func TestCov1o7HouseholdLinkage(t *testing.T) {
	e := cov1o7New(t)
	hh := e.household(cov1o7Org, "Alpha")
	other := e.household(cov1o7Org, "Beta")

	// Unlinked user: household is null.
	rec := e.do(http.MethodGet, "/users/p-1/household", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["household"] != nil {
		t.Fatalf("body = %s", rec.Body.String())
	}
	cov1o7Expect(t, e.do(http.MethodGet, "/users/out-1/household", "admin", ""), 403, "forbidden")

	// Link validation.
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{"household_id":" "}`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{"household_id":"missing"}`), 404, "household_not_found")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/out-1/household/link", "admin", `{"household_id":"x"}`), 403, "forbidden")

	rec = e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{"household_id":"`+hh.ID+`"}`)
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["status"] != "linked" || len(e.revoker.calls) != 1 {
		t.Fatalf("body %s revoked %v", rec.Body.String(), e.revoker.calls)
	}
	// The same user cannot be linked to a second household.
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{"household_id":"`+other.ID+`"}`), 409, "link_conflict")

	rec = e.do(http.MethodGet, "/users/p-1/household", "admin", "")
	hhJSON, _ := cov1o7JSON(t, rec)["household"].(map[string]any)
	if hhJSON == nil || hhJSON["id"] != hh.ID {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// A household in another org is never shown.
	foreign := e.household("org-2", "Foreign")
	if err := e.members.LinkHouseholdToUser("org-2", foreign.ID, "u-coach"); err != nil {
		t.Fatal(err)
	}
	rec = e.do(http.MethodGet, "/users/u-coach/household", "admin", "")
	if cov1o7JSON(t, rec)["household"] != nil {
		t.Fatalf("cross-org household leaked: %s", rec.Body.String())
	}

	// Unlink guards.
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/unlink", "admin", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/unlink", "admin", `{"household_id":"`+other.ID+`"}`), 409, "not_linked")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/unlink", "admin", `{"household_id":"`+hh.ID+`"}`), 200, "")
	if _, ok := e.members.GetHouseholdByKeycloakUserID("p-1"); ok {
		t.Fatal("household must be unlinked")
	}

	// Revocation failure after a link.
	e.revoker.err = cov1o7Boom
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/link", "admin", `{"household_id":"`+hh.ID+`"}`), 503, "session_revocation_failed")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/household/unlink", "admin", `{"household_id":"`+hh.ID+`"}`), 503, "session_revocation_failed")
}

func TestCov1o7SwimmerLinkage(t *testing.T) {
	e := cov1o7New(t)
	hh := e.household(cov1o7Org, "Alpha")
	adult := e.swimmer(cov1o7Org, hh.ID, "Ada", "2000-01-01")
	adult2 := e.swimmer(cov1o7Org, hh.ID, "Bea", "2001-01-01")
	child := e.swimmer(cov1o7Org, hh.ID, "Cy", time.Now().UTC().AddDate(-8, 0, 0).Format("2006-01-02"))

	rec := e.do(http.MethodGet, "/users/u-swim-x/swimmer", "admin", "")
	cov1o7Expect(t, rec, 404, "user_not_found")
	rec = e.do(http.MethodGet, "/users/p-1/swimmer", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["swimmer"] != nil {
		t.Fatalf("body = %s", rec.Body.String())
	}

	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{}`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{"swimmer_id":"missing"}`), 404, "swimmer_not_found")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{"swimmer_id":"`+child.ID+`"}`), 409, "guardian_consent_required")

	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{"swimmer_id":"`+adult.ID+`"}`), 200, "")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{"swimmer_id":"`+adult2.ID+`"}`), 409, "link_conflict")

	rec = e.do(http.MethodGet, "/users/p-1/swimmer", "admin", "")
	if sw, _ := cov1o7JSON(t, rec)["swimmer"].(map[string]any); sw == nil || sw["id"] != adult.ID {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// A swimmer record in another org is never shown for this org's user.
	foreignHH := e.household("org-2", "Foreign")
	foreign := e.swimmer("org-2", foreignHH.ID, "Fay", "2000-05-05")
	if err := e.members.LinkSwimmerToUser("org-2", foreign.ID, "u-coach"); err != nil {
		t.Fatal(err)
	}
	rec = e.do(http.MethodGet, "/users/u-coach/swimmer", "admin", "")
	if cov1o7JSON(t, rec)["swimmer"] != nil {
		t.Fatalf("cross-org swimmer leaked: %s", rec.Body.String())
	}

	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/unlink", "admin", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/unlink", "admin", `{"swimmer_id":"`+adult2.ID+`"}`), 409, "not_linked")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/unlink", "admin", `{"swimmer_id":"`+adult.ID+`"}`), 200, "")
	if _, ok := e.members.GetSwimmerByKeycloakUserID("p-1"); ok {
		t.Fatal("swimmer must be unlinked")
	}

	e.revoker.err = cov1o7Boom
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/link", "admin", `{"swimmer_id":"`+adult.ID+`"}`), 503, "session_revocation_failed")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/p-1/swimmer/unlink", "admin", `{"swimmer_id":"`+adult.ID+`"}`), 503, "session_revocation_failed")
}

func TestCov1o7LinkEndpointsNeedMembershipService(t *testing.T) {
	e := cov1o7New(t)
	bare := NewUserHandlers(e.kc, nil)
	router := e.build(func(r chi.Router) { bare.RegisterRoutes(r) })
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/users/p-1/household", ""},
		{http.MethodPost, "/users/p-1/household/link", `{"household_id":"h"}`},
		{http.MethodPost, "/users/p-1/household/unlink", `{"household_id":"h"}`},
		{http.MethodGet, "/users/p-1/swimmer", ""},
		{http.MethodPost, "/users/p-1/swimmer/link", `{"swimmer_id":"s"}`},
		{http.MethodPost, "/users/p-1/swimmer/unlink", `{"swimmer_id":"s"}`},
		{http.MethodPost, "/users/me/swimmer-link-requests", `{}`},
		{http.MethodGet, "/users/swimmer-link-requests", ""},
		{http.MethodPost, "/users/swimmer-link-requests/r1/resolve", `{"swimmer_id":"s"}`},
	} {
		token := "admin"
		if strings.Contains(tc.path, "/me/") {
			token = "parent"
		}
		cov1o7Expect(t, cov1o7Do(router, tc.method, tc.path, token, tc.body), 503, "service_unavailable")
	}
}

func TestCov1o7SwimmerLinkRequests(t *testing.T) {
	e := cov1o7New(t)
	hh := e.household(cov1o7Org, "Alpha")
	adult := e.swimmer(cov1o7Org, hh.ID, "Ada", "2000-01-01")
	adult2 := e.swimmer(cov1o7Org, hh.ID, "Bea", "2001-01-01")
	child := e.swimmer(cov1o7Org, hh.ID, "Cy", time.Now().UTC().AddDate(-8, 0, 0).Format("2006-01-02"))

	// Creation.
	cov1o7Expect(t, e.do(http.MethodPost, "/users/me/swimmer-link-requests", "parent", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/me/swimmer-link-requests", "parent", `{"requested_swimmer_name":"`+strings.Repeat("n", 101)+`"}`), 400, "invalid_request")
	rec := e.do(http.MethodPost, "/users/me/swimmer-link-requests", "parent", `{"requested_swimmer_name":" Ada ","note":"my login"}`)
	cov1o7Expect(t, rec, 201, "")
	reqID := cov1o7JSON(t, rec)["request"].(map[string]any)["id"].(string)
	// A repeat returns the same pending request.
	rec = e.do(http.MethodPost, "/users/me/swimmer-link-requests", "parent", `{}`)
	if cov1o7JSON(t, rec)["request"].(map[string]any)["id"] != reqID {
		t.Fatalf("repeat request must return the pending one: %s", rec.Body.String())
	}
	if len(e.auditActions("user.swimmer_link_request.create")) != 2 {
		t.Fatal("each submission is audited")
	}

	// Listing is admin-only.
	cov1o7Expect(t, e.do(http.MethodGet, "/users/swimmer-link-requests", "parent", ""), 403, "forbidden")
	rec = e.do(http.MethodGet, "/users/swimmer-link-requests", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	if reqs := cov1o7JSON(t, rec)["requests"].([]any); len(reqs) != 1 {
		t.Fatalf("requests = %v", reqs)
	}

	// Resolve validation.
	path := "/users/swimmer-link-requests/" + reqID + "/resolve"
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":" "}`), 400, "invalid_request")
	cov1o7Expect(t, e.do(http.MethodPost, "/users/swimmer-link-requests/nope/resolve", "admin", `{"swimmer_id":"`+adult.ID+`"}`), 404, "request_not_found")
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":"missing"}`), 404, "swimmer_not_found")
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":"`+child.ID+`"}`), 409, "guardian_consent_required")

	// Another swimmer already linked to the requesting user -> conflict.
	if err := e.members.LinkSwimmerToUser(cov1o7Org, adult2.ID, "p-1"); err != nil {
		t.Fatal(err)
	}
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":"`+adult.ID+`"}`), 409, "link_conflict")
	if err := e.members.UnlinkSwimmerFromUser(cov1o7Org, adult2.ID); err != nil {
		t.Fatal(err)
	}

	// Revocation failure still leaves the link resolved, and is reported.
	e.revoker.err = cov1o7Boom
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":"`+adult.ID+`"}`), 503, "session_revocation_failed")
	e.revoker.err = nil

	// The request is already resolved, so it is gone from the queue.
	cov1o7Expect(t, e.do(http.MethodPost, path, "admin", `{"swimmer_id":"`+adult.ID+`"}`), 404, "request_not_found")

	// Happy path with a fresh request from another user.
	rec = cov1o7Do(e.build(func(r chi.Router) { e.h.RegisterRoutes(r) }), http.MethodPost, "/users/me/swimmer-link-requests", "swimmer", `{}`)
	cov1o7Expect(t, rec, 201, "")
	id2 := cov1o7JSON(t, rec)["request"].(map[string]any)["id"].(string)
	rec = e.do(http.MethodPost, "/users/swimmer-link-requests/"+id2+"/resolve", "admin", `{"swimmer_id":"`+adult2.ID+`"}`)
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["status"] != "resolved" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if sw, ok := e.members.GetSwimmerByKeycloakUserID("u-swim"); !ok || sw.ID != adult2.ID {
		t.Fatalf("swimmer link = %+v ok=%v", sw, ok)
	}
	if pending, _ := e.members.ListPendingLinkRequests(cov1o7Org); len(pending) != 0 {
		t.Fatalf("pending = %+v", pending)
	}
}
