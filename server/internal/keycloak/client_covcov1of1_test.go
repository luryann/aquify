package keycloak_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/keycloak"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

const cov1xTokenSuffix = "/protocol/openid-connect/token"

// cov1xServer starts a fake Keycloak. Token requests are answered with
// tok1, tok2, ... (counted in the returned counter); everything else goes to h.
func cov1xServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var tokens atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, cov1xTokenSuffix) {
			n := tokens.Add(1)
			cov1xJSON(w, http.StatusOK, map[string]any{"access_token": fmt.Sprintf("tok%d", n), "expires_in": 3600})
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &tokens
}

func cov1xClientFor(t *testing.T, base string) *keycloak.AdminClient {
	t.Helper()
	c, err := keycloak.NewAdminClient(keycloak.AdminConfig{
		AdminURL: base, Realm: "r", ClientID: "id", ClientSecret: "secret", HTTPTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	return c
}

func cov1xClient(t *testing.T, h http.HandlerFunc) *keycloak.AdminClient {
	t.Helper()
	srv, _ := cov1xServer(t, h)
	return cov1xClientFor(t, srv.URL)
}

func cov1xJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cov1xStatus(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// cov1xScenario builds a handler for a failure mode.
func cov1xScenario(kind string) http.HandlerFunc {
	switch kind {
	case "abort":
		return func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }
	case "truncated":
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("short"))
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
	case "500":
		return cov1xStatus(http.StatusInternalServerError, "boom")
	case "404":
		return cov1xStatus(http.StatusNotFound, "")
	case "badjson":
		return cov1xStatus(http.StatusOK, "{not json")
	case "ok200":
		return cov1xStatus(http.StatusOK, "")
	}
	panic("unknown scenario " + kind)
}

// cov1xWithRole serves role lookups successfully and delegates the rest.
func cov1xWithRole(inner http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles/") {
			cov1xJSON(w, http.StatusOK, keycloak.Role{ID: "rid", Name: "admin"})
			return
		}
		inner(w, r)
	}
}

type cov1xOp struct {
	name     string
	opName   string // StatusError.Op expected on 5xx
	notFound error  // expected mapping of a 404, nil means StatusError
	hasJSON  bool   // decodes a JSON body on 200
	ok200    bool   // 200 with empty body is success
	wrapRole bool   // needs the role lookup to succeed first
	call     func(c *keycloak.AdminClient) error
}

func cov1xOps() []cov1xOp {
	ctx := context.Background()
	return []cov1xOp{
		{name: "GetUser", opName: "get user", notFound: keycloak.ErrUserNotFound, hasJSON: true,
			call: func(c *keycloak.AdminClient) error { _, err := c.GetUser(ctx, "u1"); return err }},
		{name: "SendVerificationEmail", opName: "send actions email", notFound: keycloak.ErrUserNotFound, ok200: true,
			call: func(c *keycloak.AdminClient) error { return c.SendVerificationEmail(ctx, "u1") }},
		{name: "SendPasswordResetEmail", opName: "send actions email", notFound: keycloak.ErrUserNotFound, ok200: true,
			call: func(c *keycloak.AdminClient) error { return c.SendPasswordResetEmail(ctx, "u1") }},
		{name: "SetUserPassword", opName: "set password", notFound: keycloak.ErrUserNotFound, ok200: true,
			call: func(c *keycloak.AdminClient) error { return c.SetUserPassword(ctx, "u1", "pw", true) }},
		{name: "UpdateUser", opName: "update user", notFound: keycloak.ErrUserNotFound, ok200: true,
			call: func(c *keycloak.AdminClient) error {
				return c.UpdateUser(ctx, "u1", keycloak.UpdateUserFields{FirstName: "Ann"})
			}},
		{name: "DeleteUser", opName: "delete user", notFound: keycloak.ErrUserNotFound, ok200: true,
			call: func(c *keycloak.AdminClient) error { return c.DeleteUser(ctx, "u1") }},
		{name: "GetUserRealmRoles", opName: "get user roles", notFound: keycloak.ErrUserNotFound, hasJSON: true,
			call: func(c *keycloak.AdminClient) error { _, err := c.GetUserRealmRoles(ctx, "u1"); return err }},
		{name: "ListUsers", opName: "list users", hasJSON: true,
			call: func(c *keycloak.AdminClient) error { _, err := c.ListUsers(ctx, "", "", 10, 0); return err }},
		{name: "ListUsersWithRealmRole", opName: "list role members", hasJSON: true,
			call: func(c *keycloak.AdminClient) error { _, err := c.ListUsersWithRealmRole(ctx, "", "admin"); return err }},
		{name: "CreateUser", opName: "create user",
			call: func(c *keycloak.AdminClient) error {
				_, err := c.CreateUser(ctx, "a@example.com", "A", "B", "org-1")
				return err
			}},
		{name: "AssignRealmRole", opName: "assign role", notFound: keycloak.ErrUserNotFound, ok200: true, wrapRole: true,
			call: func(c *keycloak.AdminClient) error { return c.AssignRealmRole(ctx, "u1", "admin") }},
		{name: "RemoveRealmRole", opName: "remove role", notFound: keycloak.ErrUserNotFound, ok200: true, wrapRole: true,
			call: func(c *keycloak.AdminClient) error { return c.RemoveRealmRole(ctx, "u1", "admin") }},
	}
}

func (op cov1xOp) handler(h http.HandlerFunc) http.HandlerFunc {
	if op.wrapRole {
		return cov1xWithRole(h)
	}
	return h
}

// ─── error classification ────────────────────────────────────────────────────

func TestCov1xStatusErrorAndOrgID(t *testing.T) {
	se := &keycloak.StatusError{Op: "get user", Status: 418, Body: "teapot"}
	if got, want := se.Error(), "keycloak: get user returned 418: teapot"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	if (&keycloak.User{}).OrgID() != "" {
		t.Fatal("nil attributes must yield empty org")
	}
	if (&keycloak.User{Attributes: map[string][]string{"org_id": {}}}).OrgID() != "" {
		t.Fatal("empty org_id slice must yield empty org")
	}
	if got := (&keycloak.User{Attributes: map[string][]string{"org_id": {"o1", "o2"}}}).OrgID(); got != "o1" {
		t.Fatalf("OrgID = %q, want o1", got)
	}
}

func TestCov1xIsUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"status 503", &keycloak.StatusError{Status: 503}, true},
		{"status 400", &keycloak.StatusError{Status: 400}, false},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"url error", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("boom")}, true},
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), true},
		{"unexpected eof", fmt.Errorf("wrapped: %w", io.ErrUnexpectedEOF), true},
		{"plain", errors.New("plain"), false},
		{"not found", keycloak.ErrUserNotFound, false},
	}
	for _, tc := range cases {
		if got := keycloak.IsUnavailable(tc.err); got != tc.want {
			t.Errorf("%s: IsUnavailable = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !keycloak.IsBadRequest(&keycloak.StatusError{Status: 400}) || keycloak.IsBadRequest(errors.New("x")) {
		t.Fatal("IsBadRequest misclassified")
	}
}

// ─── UnavailableClient ───────────────────────────────────────────────────────

func TestCov1xUnavailableClientFailsEverything(t *testing.T) {
	ctx := context.Background()
	custom := errors.New("keycloak down")
	for name, c := range map[string]*keycloak.UnavailableClient{
		"custom":  keycloak.NewUnavailableClient(custom),
		"default": keycloak.NewUnavailableClient(nil),
		"nilrecv": nil,
	} {
		want := custom
		if name != "custom" {
			want = keycloak.ErrInvalidConfig
		}
		var k keycloak.Client = c
		errs := map[string]error{
			"Ready":                  c.Ready(ctx),
			"SendVerificationEmail":  k.SendVerificationEmail(ctx, "u"),
			"UpdateUser":             k.UpdateUser(ctx, "u", keycloak.UpdateUserFields{}),
			"DeleteUser":             k.DeleteUser(ctx, "u"),
			"DisableUser":            k.DisableUser(ctx, "u"),
			"EnableUser":             k.EnableUser(ctx, "u"),
			"AssignRealmRole":        k.AssignRealmRole(ctx, "u", "r"),
			"RemoveRealmRole":        k.RemoveRealmRole(ctx, "u", "r"),
			"EnsureRealmRolesExist":  k.EnsureRealmRolesExist(ctx, []string{"r"}),
			"SendPasswordResetEmail": k.SendPasswordResetEmail(ctx, "u"),
			"SetUserPassword":        k.SetUserPassword(ctx, "u", "p", false),
		}
		if users, err := k.ListUsers(ctx, "o", "", 1, 0); users != nil {
			t.Errorf("%s ListUsers returned users", name)
		} else {
			errs["ListUsers"] = err
		}
		if u, err := k.GetUser(ctx, "u"); u != nil {
			t.Errorf("%s GetUser returned user", name)
		} else {
			errs["GetUser"] = err
		}
		if id, err := k.CreateUser(ctx, "e", "f", "l", "o"); id != "" {
			t.Errorf("%s CreateUser returned id", name)
		} else {
			errs["CreateUser"] = err
		}
		if r, err := k.GetUserRealmRoles(ctx, "u"); r != nil {
			t.Errorf("%s GetUserRealmRoles returned roles", name)
		} else {
			errs["GetUserRealmRoles"] = err
		}
		if us, err := k.ListUsersWithRealmRole(ctx, "o", "r"); us != nil {
			t.Errorf("%s ListUsersWithRealmRole returned users", name)
		} else {
			errs["ListUsersWithRealmRole"] = err
		}
		for method, err := range errs {
			if !errors.Is(err, want) {
				t.Errorf("%s.%s err = %v, want %v", name, method, err, want)
			}
		}
	}
}

// ─── token handling ──────────────────────────────────────────────────────────

func TestCov1xReadyTokenFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(cov1xStatus(http.StatusUnauthorized, ""))
		defer srv.Close()
		if err := cov1xClientFor(t, srv.URL).Ready(ctx); !errors.Is(err, keycloak.ErrUnauthorized) {
			t.Fatalf("err = %v, want ErrUnauthorized", err)
		}
	})
	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(cov1xStatus(http.StatusServiceUnavailable, "down"))
		defer srv.Close()
		err := cov1xClientFor(t, srv.URL).Ready(ctx)
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Status != 503 || se.Op != "token request" || se.Body != "down" {
			t.Fatalf("err = %#v, want token request 503 down", err)
		}
		if !keycloak.IsUnavailable(err) {
			t.Fatal("503 must be unavailable")
		}
	})
	t.Run("bad json", func(t *testing.T) {
		srv := httptest.NewServer(cov1xStatus(http.StatusOK, "{nope"))
		defer srv.Close()
		err := cov1xClientFor(t, srv.URL).Ready(ctx)
		if err == nil || !strings.Contains(err.Error(), "decode token response") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("empty access token", func(t *testing.T) {
		srv := httptest.NewServer(cov1xStatus(http.StatusOK, `{"access_token":"","expires_in":60}`))
		defer srv.Close()
		err := cov1xClientFor(t, srv.URL).Ready(ctx)
		if err == nil || !strings.Contains(err.Error(), "empty access_token") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("transport", func(t *testing.T) {
		srv := httptest.NewServer(cov1xStatus(http.StatusOK, ""))
		base := srv.URL
		srv.Close()
		err := cov1xClientFor(t, base).Ready(ctx)
		if err == nil || !strings.Contains(err.Error(), "token request") || !keycloak.IsUnavailable(err) {
			t.Fatalf("err = %v, want unavailable token request error", err)
		}
	})
	t.Run("nil context", func(t *testing.T) {
		var nilCtx context.Context
		err := cov1xClientFor(t, "http://127.0.0.1:1").Ready(nilCtx)
		if err == nil || !strings.Contains(err.Error(), "build token request") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCov1xDoBuildRequestFailureAfterTokenCached(t *testing.T) {
	c := cov1xClient(t, func(w http.ResponseWriter, _ *http.Request) {
		cov1xJSON(w, http.StatusOK, keycloak.User{ID: "u1"})
	})
	if _, err := c.GetUser(context.Background(), "u1"); err != nil {
		t.Fatalf("warm-up: %v", err)
	}
	var nilCtx context.Context
	_, err := c.GetUser(nilCtx, "u1")
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("err = %v, want build request error", err)
	}
}

func TestCov1xRepeated401ReturnsStatusError(t *testing.T) {
	srv, tokens := cov1xServer(t, cov1xStatus(http.StatusUnauthorized, "denied"))
	c := cov1xClientFor(t, srv.URL)
	_, err := c.GetUser(context.Background(), "u1")
	var se *keycloak.StatusError
	if !errors.As(err, &se) || se.Status != 401 || se.Body != "denied" {
		t.Fatalf("err = %#v, want 401 StatusError", err)
	}
	if got := tokens.Load(); got != 2 {
		t.Fatalf("token requests = %d, want 2 (initial + one forced refresh)", got)
	}
}

func TestCov1xForcedRefreshFailureIsReturned(t *testing.T) {
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, cov1xTokenSuffix) {
			if tokenCalls.Add(1) == 1 {
				cov1xJSON(w, http.StatusOK, map[string]any{"access_token": "tok1", "expires_in": 3600})
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := cov1xClientFor(t, srv.URL).GetUser(context.Background(), "u1")
	var se *keycloak.StatusError
	if !errors.As(err, &se) || se.Op != "token request" || se.Status != 502 {
		t.Fatalf("err = %#v, want token request 502", err)
	}
}

// A concurrent caller rotates the token while another request is mid-flight;
// the loser must reuse the fresh token instead of fetching a third one.
func TestCov1xForcedRefreshReusesTokenRotatedByConcurrentCaller(t *testing.T) {
	var c *keycloak.AdminClient
	var tok1Seen atomic.Int32
	var nestedErr error
	var nestedUser *keycloak.User
	srv, tokens := cov1xServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer tok1" {
			if tok1Seen.Add(1) == 1 {
				// While the outer request is in flight, a second caller hits the
				// stale token, refreshes it, and succeeds.
				nestedUser, nestedErr = c.GetUser(context.Background(), "u1")
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		cov1xJSON(w, http.StatusOK, keycloak.User{ID: "u1", Email: "a@example.com"})
	})
	c = cov1xClientFor(t, srv.URL)

	u, err := c.GetUser(context.Background(), "u1")
	if err != nil || u == nil || u.Email != "a@example.com" {
		t.Fatalf("outer GetUser = %+v, %v", u, err)
	}
	if nestedErr != nil || nestedUser == nil || nestedUser.ID != "u1" {
		t.Fatalf("nested GetUser = %+v, %v", nestedUser, nestedErr)
	}
	if got := tokens.Load(); got != 2 {
		t.Fatalf("token requests = %d, want 2", got)
	}
}

// ─── shared failure matrix for single-request operations ────────────────────

func TestCov1xOperationFailureModes(t *testing.T) {
	for _, op := range cov1xOps() {
		op := op
		t.Run(op.name, func(t *testing.T) {
			t.Run("transport failure is unavailable", func(t *testing.T) {
				err := op.call(cov1xClient(t, op.handler(cov1xScenario("abort"))))
				if err == nil || !keycloak.IsUnavailable(err) {
					t.Fatalf("err = %v, want unavailable transport error", err)
				}
			})
			t.Run("truncated body", func(t *testing.T) {
				err := op.call(cov1xClient(t, op.handler(cov1xScenario("truncated"))))
				if err == nil {
					t.Fatal("expected body read error")
				}
			})
			t.Run("server error", func(t *testing.T) {
				err := op.call(cov1xClient(t, op.handler(cov1xScenario("500"))))
				var se *keycloak.StatusError
				if !errors.As(err, &se) || se.Op != op.opName || se.Status != 500 || se.Body != "boom" {
					t.Fatalf("err = %#v, want %s 500 boom", err, op.opName)
				}
			})
			t.Run("not found", func(t *testing.T) {
				err := op.call(cov1xClient(t, op.handler(cov1xScenario("404"))))
				if op.notFound != nil {
					if !errors.Is(err, op.notFound) {
						t.Fatalf("err = %v, want %v", err, op.notFound)
					}
					return
				}
				if op.name == "ListUsersWithRealmRole" {
					if err != nil {
						t.Fatalf("unknown role must yield no error, got %v", err)
					}
					return
				}
				var se *keycloak.StatusError
				if !errors.As(err, &se) || se.Status != 404 {
					t.Fatalf("err = %v, want 404 StatusError", err)
				}
			})
			if op.hasJSON {
				t.Run("malformed json", func(t *testing.T) {
					err := op.call(cov1xClient(t, op.handler(cov1xScenario("badjson"))))
					if err == nil || !strings.Contains(err.Error(), "decode") {
						t.Fatalf("err = %v, want decode error", err)
					}
				})
			}
			if op.ok200 {
				t.Run("200 is success", func(t *testing.T) {
					if err := op.call(cov1xClient(t, op.handler(cov1xScenario("ok200")))); err != nil {
						t.Fatalf("err = %v", err)
					}
				})
			}
		})
	}
}

func TestCov1xRoleLookupFailures(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"abort", "truncated", "500", "badjson"} {
		c := cov1xClient(t, cov1xScenario(kind))
		for name, err := range map[string]error{
			"assign": c.AssignRealmRole(ctx, "u1", "admin"),
			"remove": c.RemoveRealmRole(ctx, "u1", "admin"),
		} {
			if err == nil {
				t.Fatalf("%s/%s: expected error from role lookup", name, kind)
			}
		}
	}
	// Role lookup 404 maps to ErrRoleNotFound.
	c := cov1xClient(t, cov1xScenario("404"))
	if err := c.AssignRealmRole(ctx, "u1", "ghost"); !errors.Is(err, keycloak.ErrRoleNotFound) {
		t.Fatalf("assign err = %v", err)
	}
	if err := c.RemoveRealmRole(ctx, "u1", "ghost"); !errors.Is(err, keycloak.ErrRoleNotFound) {
		t.Fatalf("remove err = %v", err)
	}
	// Unsafe user IDs are rejected after the role resolves.
	c = cov1xClient(t, cov1xWithRole(cov1xScenario("500")))
	if err := c.AssignRealmRole(ctx, "../x", "admin"); !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("assign unsafe id err = %v", err)
	}
	if err := c.RemoveRealmRole(ctx, "a/b", "admin"); !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("remove unsafe id err = %v", err)
	}
}

// ─── request shape ───────────────────────────────────────────────────────────

func TestCov1xWriteRequestsCarryExpectedPayloads(t *testing.T) {
	ctx := context.Background()
	type seen struct {
		method, path, auth string
		body               map[string]any
	}
	var last seen
	c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		last = seen{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization")}
		_ = json.Unmarshal(raw, &last.body)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.SetUserPassword(ctx, "u1", "s3cret", true); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if last.method != http.MethodPut || last.path != "/admin/realms/r/users/u1/reset-password" || last.auth != "Bearer tok1" {
		t.Fatalf("request = %+v", last)
	}
	if last.body["type"] != "password" || last.body["value"] != "s3cret" || last.body["temporary"] != true {
		t.Fatalf("password payload = %v", last.body)
	}

	enabled := false
	err := c.UpdateUser(ctx, "u1", keycloak.UpdateUserFields{
		FirstName: " Ann ", LastName: "Lee", Email: " Ann@Example.com ", Enabled: &enabled,
		Attributes: map[string][]string{"org_id": {"o1"}},
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if last.body["firstName"] != "Ann" || last.body["lastName"] != "Lee" ||
		last.body["email"] != "Ann@Example.com" || last.body["username"] != "ann@example.com" ||
		last.body["enabled"] != false || last.body["attributes"] == nil {
		t.Fatalf("update payload = %v", last.body)
	}

	// An empty update sends nothing and never touches the network.
	last = seen{}
	if err := c.UpdateUser(ctx, "u1", keycloak.UpdateUserFields{FirstName: "  "}); err != nil || last.method != "" {
		t.Fatalf("empty update: err=%v request=%+v", err, last)
	}
	// An empty update does not even validate the ID.
	if err := c.UpdateUser(ctx, "bad/id", keycloak.UpdateUserFields{}); err != nil {
		t.Fatalf("empty update with bad id: %v", err)
	}

	if err := c.DisableUser(ctx, "u1"); err != nil || last.body["enabled"] != false {
		t.Fatalf("DisableUser: err=%v body=%v", err, last.body)
	}
	if err := c.EnableUser(ctx, "u1"); err != nil || last.body["enabled"] != true {
		t.Fatalf("EnableUser: err=%v body=%v", err, last.body)
	}

	for _, id := range []string{"", "a/b", "a..b", "a b", "a%2Fb"} {
		last = seen{}
		if err := c.UpdateUser(ctx, id, keycloak.UpdateUserFields{FirstName: "X"}); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("UpdateUser(%q) err = %v", id, err)
		}
		if err := c.SetUserPassword(ctx, id, "p", false); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("SetUserPassword(%q) err = %v", id, err)
		}
		if last.method != "" {
			t.Fatalf("unsafe id %q reached Keycloak: %+v", id, last)
		}
	}
}

func TestCov1xCreateUserParsesLocationHeader(t *testing.T) {
	ctx := context.Background()
	c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["username"] != "mixed@example.com" || body["email"] != "Mixed@Example.com" {
			t.Errorf("create payload = %v", body)
		}
		w.Header().Set("Location", "http://kc/admin/realms/r/users/new-id/")
		w.WriteHeader(http.StatusCreated)
	})
	id, err := c.CreateUser(ctx, " Mixed@Example.com ", " A ", " B ", "org-1")
	if err != nil || id != "new-id" {
		t.Fatalf("CreateUser = %q, %v", id, err)
	}

	c = cov1xClient(t, cov1xStatus(http.StatusConflict, ""))
	if _, err := c.CreateUser(ctx, "a@example.com", "A", "B", "o"); !errors.Is(err, keycloak.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

// ─── ListUsers ───────────────────────────────────────────────────────────────

func cov1xUserWithOrg(id, org string) keycloak.User {
	return keycloak.User{ID: id, Attributes: map[string][]string{"org_id": {org}}}
}

func TestCov1xListUsersClampsPagingAndSendsSearch(t *testing.T) {
	var q url.Values
	c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		cov1xJSON(w, http.StatusOK, []keycloak.User{{ID: "a"}})
	})
	users, err := c.ListUsers(context.Background(), "", "ann", 9999, -5)
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers = %v, %v", users, err)
	}
	if q.Get("max") != "500" || q.Get("first") != "0" || q.Get("search") != "ann" || q.Get("q") != "" {
		t.Fatalf("query = %v", q)
	}
	if _, err := c.ListUsers(context.Background(), "", "", 0, 0); err != nil || q.Get("max") != "50" || q.Has("search") {
		t.Fatalf("default limit not applied: err=%v query=%v", err, q)
	}
}

func TestCov1xListUsersPastEndOfOrgReturnsEmptyWithoutRealmScan(t *testing.T) {
	var calls atomic.Int32
	c := cov1xClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		cov1xJSON(w, http.StatusOK, []keycloak.User{})
	})
	users, err := c.ListUsers(context.Background(), "o1", "", 10, 20)
	if err != nil || users == nil || len(users) != 0 {
		t.Fatalf("ListUsers = %#v, %v", users, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("list calls = %d, want 1", calls.Load())
	}
}

func TestCov1xListUsersErrorPropagation(t *testing.T) {
	ctx := context.Background()
	t.Run("first page error", func(t *testing.T) {
		c := cov1xClient(t, cov1xStatus(http.StatusInternalServerError, "boom"))
		if _, err := c.ListUsers(ctx, "o1", "", 10, 0); !keycloak.IsUnavailable(err) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("hydrate error", func(t *testing.T) {
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/users") {
				cov1xJSON(w, http.StatusOK, []keycloak.User{{ID: "u1"}})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, err := c.ListUsers(ctx, "o1", "", 10, 0)
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Op != "get user" {
			t.Fatalf("err = %v, want get user status error", err)
		}
	})
	t.Run("fallback page error", func(t *testing.T) {
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("q") {
				cov1xJSON(w, http.StatusOK, []keycloak.User{})
				return
			}
			w.WriteHeader(http.StatusBadGateway)
		})
		_, err := c.ListUsers(ctx, "o1", "", 10, 0)
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Op != "list users" || se.Status != 502 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("fallback hydrate error", func(t *testing.T) {
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Query().Has("q"):
				cov1xJSON(w, http.StatusOK, []keycloak.User{})
			case strings.HasSuffix(r.URL.Path, "/users"):
				cov1xJSON(w, http.StatusOK, []keycloak.User{{ID: "u9"}})
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		})
		_, err := c.ListUsers(ctx, "o1", "", 10, 0)
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Op != "get user" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCov1xListUsersSkipsHydrationTargetsThatVanished(t *testing.T) {
	// The brief listing has two users without attributes: one disappears
	// before hydration (404, dropped), the other hydrates into the org.
	c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/realms/r/users":
			if r.URL.Query().Has("q") {
				cov1xJSON(w, http.StatusOK, []keycloak.User{{ID: "gone"}, {ID: "ok"}})
			} else {
				cov1xJSON(w, http.StatusOK, []keycloak.User{})
			}
		case "/admin/realms/r/users/gone":
			w.WriteHeader(http.StatusNotFound)
		case "/admin/realms/r/users/ok":
			cov1xJSON(w, http.StatusOK, cov1xUserWithOrg("ok", "o1"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	})
	users, err := c.ListUsers(context.Background(), "o1", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	// "gone" stays attribute-less so the all-match shortcut fails and the
	// realm fallback (empty here) is what is returned.
	if len(users) != 0 {
		t.Fatalf("users = %+v, want empty fallback result", users)
	}
}

func TestCov1xListUsersFallbackWindowing(t *testing.T) {
	realm := []keycloak.User{
		cov1xUserWithOrg("a", "o1"), cov1xUserWithOrg("b", "o2"),
		cov1xUserWithOrg("c", "o1"), cov1xUserWithOrg("d", "o1"),
	}
	c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("q") {
			// Server-side attribute filtering is unreliable: it returns a
			// user from another org, forcing the local fallback.
			cov1xJSON(w, http.StatusOK, []keycloak.User{cov1xUserWithOrg("b", "o2")})
			return
		}
		cov1xJSON(w, http.StatusOK, realm)
	})
	ids := func(us []keycloak.User) string {
		var out []string
		for _, u := range us {
			out = append(out, u.ID)
		}
		return strings.Join(out, ",")
	}
	ctx := context.Background()

	if us, err := c.ListUsers(ctx, "o1", "", 2, 0); err != nil || ids(us) != "a,c" {
		t.Fatalf("limit window = %q, %v", ids(us), err)
	}
	if us, err := c.ListUsers(ctx, "o1", "", 10, 1); err != nil || ids(us) != "c,d" {
		t.Fatalf("offset window = %q, %v", ids(us), err)
	}
	us, err := c.ListUsers(ctx, "o1", "", 10, 7)
	if err != nil || us == nil || len(us) != 0 {
		t.Fatalf("offset beyond matches = %#v, %v", us, err)
	}
}

func TestCov1xListUsersWithRealmRole(t *testing.T) {
	ctx := context.Background()

	t.Run("filters by org and escapes role", func(t *testing.T) {
		var gotPath string
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			cov1xJSON(w, http.StatusOK, []keycloak.User{cov1xUserWithOrg("a", "o1"), cov1xUserWithOrg("b", "o2")})
		})
		us, err := c.ListUsersWithRealmRole(ctx, "o1", "club admin")
		if err != nil || len(us) != 1 || us[0].ID != "a" {
			t.Fatalf("users = %+v, %v", us, err)
		}
		if gotPath != "/admin/realms/r/roles/club%20admin/users" {
			t.Fatalf("path = %q", gotPath)
		}
	})

	t.Run("unknown role is empty", func(t *testing.T) {
		c := cov1xClient(t, cov1xScenario("404"))
		us, err := c.ListUsersWithRealmRole(ctx, "o1", "ghost")
		if err != nil || us == nil || len(us) != 0 {
			t.Fatalf("users = %#v, %v", us, err)
		}
	})

	t.Run("hydrate error", func(t *testing.T) {
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/roles/") {
				cov1xJSON(w, http.StatusOK, []keycloak.User{{ID: "u1"}})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		})
		if _, err := c.ListUsersWithRealmRole(ctx, "o1", "admin"); !keycloak.IsUnavailable(err) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("pages until a short page", func(t *testing.T) {
		var firsts []string
		full := make([]keycloak.User, 200)
		for i := range full {
			full[i] = cov1xUserWithOrg(fmt.Sprintf("p%d", i), "o1")
		}
		c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
			first := r.URL.Query().Get("first")
			firsts = append(firsts, first)
			if first == "0" {
				cov1xJSON(w, http.StatusOK, full)
				return
			}
			cov1xJSON(w, http.StatusOK, []keycloak.User{cov1xUserWithOrg("last", "o1")})
		})
		us, err := c.ListUsersWithRealmRole(ctx, "o1", "admin")
		if err != nil || len(us) != 201 || strings.Join(firsts, ",") != "0,200" {
			t.Fatalf("len=%d err=%v firsts=%v", len(us), err, firsts)
		}
	})

	t.Run("runaway membership hits scan limit", func(t *testing.T) {
		full := make([]keycloak.User, 200)
		for i := range full {
			full[i] = keycloak.User{ID: fmt.Sprintf("p%d", i)}
		}
		var calls atomic.Int32
		c := cov1xClient(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			cov1xJSON(w, http.StatusOK, full)
		})
		_, err := c.ListUsersWithRealmRole(ctx, "", "admin")
		if err == nil || !strings.Contains(err.Error(), "scan limit") {
			t.Fatalf("err = %v, want scan limit error", err)
		}
		if calls.Load() != 500 {
			t.Fatalf("pages fetched = %d, want 500", calls.Load())
		}
	})
}

// ─── EnsureRealmRolesExist ───────────────────────────────────────────────────

func TestCov1xEnsureRealmRolesExist(t *testing.T) {
	ctx := context.Background()

	roleHandler := func(createStatus int, created *[]string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles/exists"):
				cov1xJSON(w, http.StatusOK, keycloak.Role{ID: "1", Name: "exists"})
			case r.Method == http.MethodGet:
				w.WriteHeader(http.StatusNotFound)
			case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/r/roles":
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				*created = append(*created, body["name"])
				w.WriteHeader(createStatus)
			default:
				w.WriteHeader(http.StatusTeapot)
			}
		}
	}

	t.Run("creates only missing roles", func(t *testing.T) {
		var created []string
		c := cov1xClient(t, roleHandler(http.StatusCreated, &created))
		if err := c.EnsureRealmRolesExist(ctx, []string{"exists", "new1", "new2"}); err != nil {
			t.Fatalf("err = %v", err)
		}
		if strings.Join(created, ",") != "new1,new2" {
			t.Fatalf("created = %v", created)
		}
	})
	t.Run("200 on create accepted", func(t *testing.T) {
		var created []string
		c := cov1xClient(t, roleHandler(http.StatusOK, &created))
		if err := c.EnsureRealmRolesExist(ctx, []string{"new"}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("create conflict is a benign race", func(t *testing.T) {
		var created []string
		c := cov1xClient(t, roleHandler(http.StatusConflict, &created))
		if err := c.EnsureRealmRolesExist(ctx, []string{"new"}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("create failure is wrapped", func(t *testing.T) {
		var created []string
		c := cov1xClient(t, roleHandler(http.StatusInternalServerError, &created))
		err := c.EnsureRealmRolesExist(ctx, []string{"new"})
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Op != "create role" || !strings.Contains(err.Error(), `ensure role "new"`) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("lookup failure other than not found", func(t *testing.T) {
		c := cov1xClient(t, cov1xScenario("500"))
		err := c.EnsureRealmRolesExist(ctx, []string{"x"})
		var se *keycloak.StatusError
		if !errors.As(err, &se) || se.Op != "get role" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("create transport failure and truncated response", func(t *testing.T) {
		for _, kind := range []string{"abort", "truncated"} {
			c := cov1xClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				cov1xScenario(kind)(w, r)
			})
			if err := c.EnsureRealmRolesExist(ctx, []string{"x"}); err == nil {
				t.Fatalf("%s: expected error", kind)
			}
		}
	})
}

// ─── StubClient ──────────────────────────────────────────────────────────────

func cov1xStubWithUser(orgID string, id string) *keycloak.StubClient {
	s := keycloak.NewStubClient()
	s.AddUser(keycloak.User{ID: id, Email: id + "@example.com", FirstName: "First" + id, LastName: "Last" + id,
		Attributes: map[string][]string{"org_id": {orgID}}})
	return s
}

func TestCov1xStubListUsersPagingAndSearch(t *testing.T) {
	ctx := context.Background()
	s := keycloak.NewStubClient()
	for _, id := range []string{"c", "a", "b"} {
		s.AddUser(keycloak.User{ID: id, Email: id + "@example.com", FirstName: "Fn" + id, LastName: "Ln" + id,
			Attributes: map[string][]string{"org_id": {"o1"}}})
	}
	s.AddUser(keycloak.User{ID: "z", Email: "z@other.com", Attributes: map[string][]string{"org_id": {"o2"}}})

	ids := func(us []keycloak.User) string {
		var out []string
		for _, u := range us {
			out = append(out, u.ID)
		}
		return strings.Join(out, ",")
	}
	if us, err := s.ListUsers(ctx, "o1", "", 0, -3); err != nil || ids(us) != "a,b,c" {
		t.Fatalf("default paging = %q, %v", ids(us), err)
	}
	if us, _ := s.ListUsers(ctx, "o1", "", 1, 1); ids(us) != "b" {
		t.Fatalf("window = %q", ids(us))
	}
	if us, _ := s.ListUsers(ctx, "", "", 10, 0); len(us) != 4 {
		t.Fatalf("all orgs = %d users", len(us))
	}
	// Search matches email, first name or last name, case-insensitively.
	if us, _ := s.ListUsers(ctx, "o1", "EXAMPLE.COM", 10, 0); len(us) != 3 {
		t.Fatalf("email search = %d", len(us))
	}
	if us, _ := s.ListUsers(ctx, "o1", "fnb", 10, 0); ids(us) != "b" {
		t.Fatalf("first-name search = %q", ids(us))
	}
	if us, _ := s.ListUsers(ctx, "o1", "lnc", 10, 0); ids(us) != "c" {
		t.Fatalf("last-name search = %q", ids(us))
	}
	if us, _ := s.ListUsers(ctx, "o1", "nomatch", 10, 0); len(us) != 0 {
		t.Fatalf("no-match search = %d", len(us))
	}
	if us, err := s.ListUsers(ctx, "o1", "", 10, 50); err != nil || us == nil || len(us) != 0 {
		t.Fatalf("offset past end = %#v, %v", us, err)
	}
}

func TestCov1xStubErrorOverridesAndNotFound(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("injected")
	s := cov1xStubWithUser("o1", "u1")

	if _, err := s.GetUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	s.GetUserErr = boom
	if _, err := s.GetUser(ctx, "u1"); !errors.Is(err, boom) {
		t.Fatalf("GetUser err = %v", err)
	}
	s.ListUsersErr = boom
	if _, err := s.ListUsers(ctx, "o1", "", 10, 0); !errors.Is(err, boom) {
		t.Fatalf("ListUsers err = %v", err)
	}
	s.SendVerificationEmailErr = boom
	if err := s.SendVerificationEmail(ctx, "u1"); !errors.Is(err, boom) {
		t.Fatalf("SendVerificationEmail err = %v", err)
	}
	s.UpdateUserErr = boom
	if err := s.UpdateUser(ctx, "u1", keycloak.UpdateUserFields{FirstName: "X"}); !errors.Is(err, boom) {
		t.Fatalf("UpdateUser err = %v", err)
	}
	s.DeleteUserErr = boom
	if err := s.DeleteUser(ctx, "u1"); !errors.Is(err, boom) {
		t.Fatalf("DeleteUser err = %v", err)
	}
	s.GetUserRealmRolesErr = boom
	if _, err := s.GetUserRealmRoles(ctx, "u1"); !errors.Is(err, boom) {
		t.Fatalf("GetUserRealmRoles err = %v", err)
	}
	if _, err := s.ListUsersWithRealmRole(ctx, "o1", "admin"); !errors.Is(err, boom) {
		t.Fatalf("ListUsersWithRealmRole err = %v", err)
	}
	s.AssignRealmRoleErr = boom
	if err := s.AssignRealmRole(ctx, "u1", "admin"); !errors.Is(err, boom) {
		t.Fatalf("AssignRealmRole err = %v", err)
	}
	s.RemoveRealmRoleErr = boom
	if err := s.RemoveRealmRole(ctx, "u1", "admin"); !errors.Is(err, boom) {
		t.Fatalf("RemoveRealmRole err = %v", err)
	}
	s.EnsureRealmRolesExistErr = boom
	if err := s.EnsureRealmRolesExist(ctx, []string{"x"}); !errors.Is(err, boom) {
		t.Fatalf("EnsureRealmRolesExist err = %v", err)
	}
	s.SendPasswordResetErr = boom
	if err := s.SendPasswordResetEmail(ctx, "u1"); !errors.Is(err, boom) {
		t.Fatalf("SendPasswordResetEmail err = %v", err)
	}

	// Unknown users on a fresh stub.
	f := keycloak.NewStubClient()
	checks := map[string]error{
		"SendVerificationEmail":  f.SendVerificationEmail(ctx, "nope"),
		"UpdateUser":             f.UpdateUser(ctx, "nope", keycloak.UpdateUserFields{FirstName: "X"}),
		"AssignRealmRole":        f.AssignRealmRole(ctx, "nope", "admin"),
		"RemoveRealmRole":        f.RemoveRealmRole(ctx, "nope", "admin"),
		"SendPasswordResetEmail": f.SendPasswordResetEmail(ctx, "nope"),
		"SetUserPassword":        f.SetUserPassword(ctx, "nope", "pw", false),
	}
	if _, err := f.GetUserRealmRoles(ctx, "nope"); !errors.Is(err, keycloak.ErrUserNotFound) {
		t.Fatalf("GetUserRealmRoles err = %v", err)
	}
	for name, err := range checks {
		if !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Errorf("%s err = %v, want ErrUserNotFound", name, err)
		}
	}
}

func TestCov1xStubRoleAndPasswordWorkflows(t *testing.T) {
	ctx := context.Background()
	s := keycloak.NewStubClient()
	s.AddUser(keycloak.User{ID: "a", Attributes: map[string][]string{"org_id": {"o1"}}})
	s.AddUser(keycloak.User{ID: "b", Attributes: map[string][]string{"org_id": {"o1"}}})
	s.AddUser(keycloak.User{ID: "c", Attributes: map[string][]string{"org_id": {"o2"}}})
	// A user added without any attributes map is updated lazily.
	s.AddUser(keycloak.User{ID: "bare"})

	for _, id := range []string{"a", "c"} {
		if err := s.AssignRealmRole(ctx, id, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AssignRealmRole(ctx, "b", "coach"); err != nil {
		t.Fatal(err)
	}
	admins, err := s.ListUsersWithRealmRole(ctx, "o1", "admin")
	if err != nil || len(admins) != 1 || admins[0].ID != "a" {
		t.Fatalf("o1 admins = %+v, %v", admins, err)
	}
	all, _ := s.ListUsersWithRealmRole(ctx, "", "admin")
	if len(all) != 2 || all[0].ID != "a" || all[1].ID != "c" {
		t.Fatalf("all admins = %+v", all)
	}

	if err := s.RemoveRealmRole(ctx, "a", "ghost-role"); !errors.Is(err, keycloak.ErrRoleNotFound) {
		t.Fatalf("remove unknown role err = %v", err)
	}
	if err := s.RemoveRealmRole(ctx, "a", "admin"); err != nil {
		t.Fatal(err)
	}
	if roles, _ := s.GetUserRealmRoles(ctx, "a"); len(roles) != 0 {
		t.Fatalf("roles after removal = %+v", roles)
	}

	if err := s.UpdateUser(ctx, "bare", keycloak.UpdateUserFields{
		LastName: "L", Email: "Bare@Example.com", Attributes: map[string][]string{"org_id": {"o9"}},
	}); err != nil {
		t.Fatal(err)
	}
	u, _ := s.GetUser(ctx, "bare")
	if u.LastName != "L" || u.Username != "bare@example.com" || u.OrgID() != "o9" {
		t.Fatalf("bare user = %+v", u)
	}

	if err := s.SetUserPassword(ctx, "a", "pw", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationEmail(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SendPasswordResetEmail(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	var sawPassword bool
	for _, act := range s.Actions() {
		if act.Action == "set_password" && act.UserID == "a" {
			sawPassword = true
		}
	}
	if !sawPassword {
		t.Fatalf("set_password not recorded: %+v", s.Actions())
	}
	if got := s.VerificationsSent(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("VerificationsSent = %v", got)
	}
	if got := s.PasswordResetsSent(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("PasswordResetsSent = %v", got)
	}
}

func TestCov1xStubInvitedEmailsIsSnapshot(t *testing.T) {
	ctx := context.Background()
	s := keycloak.NewStubClient()
	if got := s.InvitedEmails(); len(got) != 0 {
		t.Fatalf("fresh InvitedEmails = %v", got)
	}
	if _, err := s.CreateUser(ctx, "x@example.com", "X", "Y", "o1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "y@example.com", "X", "Y", "o1"); err != nil {
		t.Fatal(err)
	}
	got := s.InvitedEmails()
	if strings.Join(got, ",") != "x@example.com,y@example.com" {
		t.Fatalf("InvitedEmails = %v", got)
	}
	got[0] = "mutated"
	if s.InvitedEmails()[0] != "x@example.com" {
		t.Fatal("InvitedEmails must return a copy")
	}
}

func TestCov1xFirstTokenFetchFailureAbortsCall(t *testing.T) {
	srv := httptest.NewServer(cov1xStatus(http.StatusServiceUnavailable, "no tokens"))
	defer srv.Close()
	_, err := cov1xClientFor(t, srv.URL).GetUser(context.Background(), "u1")
	var se *keycloak.StatusError
	if !errors.As(err, &se) || se.Op != "token request" || se.Status != 503 {
		t.Fatalf("err = %v, want token request 503", err)
	}
}

func TestCov1xUnsafeIDsRejectedForEmailAndRoleReads(t *testing.T) {
	ctx := context.Background()
	var hits atomic.Int32
	c := cov1xClient(t, func(http.ResponseWriter, *http.Request) { hits.Add(1) })
	for _, id := range []string{"", "a/b", "..", "x y"} {
		if err := c.SendVerificationEmail(ctx, id); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("SendVerificationEmail(%q) err = %v", id, err)
		}
		if err := c.SendPasswordResetEmail(ctx, id); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("SendPasswordResetEmail(%q) err = %v", id, err)
		}
		if _, err := c.GetUserRealmRoles(ctx, id); !errors.Is(err, keycloak.ErrUserNotFound) {
			t.Fatalf("GetUserRealmRoles(%q) err = %v", id, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("unsafe ids reached Keycloak %d times", hits.Load())
	}
}

func TestCov1xStubUpdateUserAppliesNames(t *testing.T) {
	s := cov1xStubWithUser("o1", "u1")
	if err := s.UpdateUser(context.Background(), "u1", keycloak.UpdateUserFields{FirstName: "Zed"}); err != nil {
		t.Fatal(err)
	}
	u, _ := s.GetUser(context.Background(), "u1")
	if u.FirstName != "Zed" || u.LastName != "Lastu1" {
		t.Fatalf("user = %+v", u)
	}
}
