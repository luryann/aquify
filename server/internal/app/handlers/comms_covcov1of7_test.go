package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/workers"
)

// ─── Fakes at true external boundaries ───────────────────────────────────────

// cov1o7Store wraps the in-memory object store with injectable failures.
type cov1o7Store struct {
	*storage.StubStorage
	existsErr, sizeErr, getErr, putErr, presignErr error
	sizeOverride                                   *int64
}

func (s *cov1o7Store) Exists(ctx context.Context, key string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.StubStorage.Exists(ctx, key)
}

func (s *cov1o7Store) ObjectSize(ctx context.Context, key string) (int64, error) {
	if s.sizeErr != nil {
		return 0, s.sizeErr
	}
	if s.sizeOverride != nil {
		return *s.sizeOverride, nil
	}
	return s.StubStorage.ObjectSize(ctx, key)
}

func (s *cov1o7Store) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	if s.getErr != nil {
		return nil, "", s.getErr
	}
	return s.StubStorage.GetObject(ctx, key)
}

func (s *cov1o7Store) PutObject(ctx context.Context, key, ct string, data []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.StubStorage.PutObject(ctx, key, ct, data)
}

func (s *cov1o7Store) PresignedPutURL(key string, ttl time.Duration) (string, error) {
	if s.presignErr != nil {
		return "", s.presignErr
	}
	return s.StubStorage.PresignedPutURL(key, ttl)
}

type cov1o7Queue struct {
	mu    sync.Mutex
	err   error
	tasks []*workers.Task
}

func (q *cov1o7Queue) Enqueue(_ context.Context, task *workers.Task, _ ...workers.Option) (*workers.TaskInfo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return nil, q.err
	}
	q.tasks = append(q.tasks, task)
	return &workers.TaskInfo{}, nil
}

func (q *cov1o7Queue) EnqueueIn(ctx context.Context, _ time.Duration, task *workers.Task, opts ...workers.Option) (*workers.TaskInfo, error) {
	return q.Enqueue(ctx, task, opts...)
}

// ─── Fixture ──────────────────────────────────────────────────────────────────

// UUID-shaped user ids so the same fixture works against Postgres columns.
const (
	cov1o7UAdmin = "00000000-0000-4000-8000-0000000000a1"
	cov1o7UCoach = "00000000-0000-4000-8000-0000000000a2"
	cov1o7UPA    = "00000000-0000-4000-8000-0000000000b1"
	cov1o7UPB    = "00000000-0000-4000-8000-0000000000b2"
	cov1o7USA    = "00000000-0000-4000-8000-0000000000b3"
	cov1o7UOut   = "00000000-0000-4000-8000-0000000000c1"
)

type cov1o7C struct {
	t                                *testing.T
	org                              string
	svc                              *comms.Service
	mem                              *members.Service
	grp                              *groups.Service
	aud                              *audit.Service
	store                            *cov1o7Store
	queue                            *cov1o7Queue
	h                                *CommsHandlers
	router                           http.Handler
	hhA, swA, hhB, swB, groupID, reg string
}

func cov1o7NewComms(t *testing.T, org string) *cov1o7C {
	t.Helper()
	return cov1o7NewCommsOn(t, org, nil)
}

// cov1o7NewCommsOn builds the fixture on in-memory services, or on Postgres
// when pool is non-nil (the org must then be a UUID).
func cov1o7NewCommsOn(t *testing.T, org string, pool *pgxpool.Pool) *cov1o7C {
	t.Helper()
	admin := &appmiddleware.Principal{UserID: cov1o7UAdmin, OrgID: org, Roles: []string{"admin"}}
	c := &cov1o7C{
		t: t, org: org,
		svc: comms.NewServiceWithPool(pool), mem: members.NewServiceWithPool(pool), grp: groups.NewServiceWithPool(pool), aud: audit.NewService(),
		store: &cov1o7Store{StubStorage: storage.NewStubStorage("http://stub.local")},
		queue: &cov1o7Queue{},
	}
	mk := func(name, email string) (string, string) {
		hh, err := c.mem.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: name, PrimaryEmail: email})
		if err != nil {
			t.Fatal(err)
		}
		sw, _, err := c.mem.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: name, LastName: "S", DateOfBirth: "2012-01-01", Gender: "female", RegistrationStatus: "active"})
		if err != nil {
			t.Fatal(err)
		}
		return hh.ID, sw.ID
	}
	c.hhA, c.swA = mk("Alpha", "A@Example.com")
	c.hhB, c.swB = mk("Beta", "b@example.com")
	// hhA also has a duplicate-of-primary contact and a second distinct contact.
	for _, email := range []string{" a@example.com ", "Second@Example.com", ""} {
		if _, err := c.mem.AddContact(admin, org, c.hhA, members.AddContactInput{Relationship: "guardian", FirstName: "G", LastName: "A", Email: email}); err != nil {
			t.Fatal(err)
		}
	}
	g, _, err := c.grp.CreateGroup(admin, org, groups.CreateGroupInput{Code: "SENIOR", Name: "Senior", Type: "competitive"})
	if err != nil {
		t.Fatal(err)
	}
	c.groupID = g.ID
	if _, err := c.mem.AssignSwimmerToGroup(admin, org, c.swA, members.AssignSwimmerGroupInput{GroupCode: "SENIOR", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if pool == nil {
		c.grp.SeedSwimmer(org, c.swA, c.hhA, "2012-01-01", "female", "active")
	}
	reg, err := c.grp.CreateRegistration(admin, org, groups.CreateRegistrationInput{HouseholdID: c.hhA, SwimmerID: c.swA, GroupID: g.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	c.reg = reg.ID

	c.h = NewCommsHandlersWithAudit(c.svc, c.aud, c.mem, c.grp)
	c.h.SetStorage(c.store)
	c.h.SetJobQueue(c.queue)
	c.router = c.routerFor(c.h)
	return c
}

func (c *cov1o7C) tokens() tokenResolver {
	return tokenResolver{
		"admin":    {UserID: cov1o7UAdmin, OrgID: c.org, Roles: []string{"admin"}},
		"coach":    {UserID: cov1o7UCoach, OrgID: c.org, Roles: []string{"coach"}},
		"parentA":  {UserID: cov1o7UPA, OrgID: c.org, HouseholdID: c.hhA, Roles: []string{"parent"}},
		"parentB":  {UserID: cov1o7UPB, OrgID: c.org, HouseholdID: c.hhB, Roles: []string{"parent"}},
		"swimmerA": {UserID: cov1o7USA, OrgID: c.org, HouseholdID: c.hhA, Roles: []string{"swimmer"}},
		"outsider": {UserID: cov1o7UOut, OrgID: c.org, HouseholdID: c.hhA, Roles: []string{"guest"}},
	}
}

func (c *cov1o7C) routerFor(h *CommsHandlers) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(c.tokens()))
		p.Use(appmiddleware.RequireOrgScope)
		h.RegisterRoutes(p)
	})
	h.RegisterPublicRoutes(r)
	return r
}

func (c *cov1o7C) doWith(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Org-Id", c.org)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func (c *cov1o7C) do(method, path, token, body string) *httptest.ResponseRecorder {
	return c.doWith(c.router, method, path, token, body)
}

func (c *cov1o7C) audited(action string) []audit.AuditEvent {
	c.t.Helper()
	evs, err := c.aud.Query(audit.Filter{Action: action})
	if err != nil {
		c.t.Fatal(err)
	}
	return evs
}

func (c *cov1o7C) sendMsg(token, hh, sw, body string) string {
	c.t.Helper()
	rec := c.do(http.MethodPost, "/messages", token, fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,"body":%q}`, hh, sw, body))
	cov1o7Expect(c.t, rec, 201, "")
	return cov1o7JSON(c.t, rec)["message"].(map[string]any)["id"].(string)
}

func (c *cov1o7C) attachmentKey(hh, sw, name string, data []byte) string {
	c.t.Helper()
	key := fmt.Sprintf("messages/%s/%s/%s/%s", c.org, hh, sw, name)
	if err := c.store.StubStorage.PutObject(context.Background(), key, "image/png", data); err != nil {
		c.t.Fatal(err)
	}
	return key
}

// ─── Constructors and webhook auth ───────────────────────────────────────────

func TestCov1o7CommsConstructors(t *testing.T) {
	aud := audit.NewService()
	h := NewCommsHandlersWithAudit(comms.NewService(), aud, nil, nil)
	if h.auditService != aud || h.validate == nil || h.allowInsecureWebhook {
		t.Fatalf("audit constructor misconfigured: %+v", h)
	}
	dev := NewCommsHandlersWithAuditAndWebhookAuth(comms.NewService(), aud, nil, nil, " X-Hook ", " s3cret ", " user ", " pass ", " Development ")
	if dev.auditService != aud || dev.postmarkHeaderName != "X-Hook" || dev.postmarkHeaderValue != "s3cret" ||
		dev.postmarkBasicUser != "user" || dev.postmarkBasicPass != "pass" || !dev.allowInsecureWebhook {
		t.Fatalf("webhook-auth constructor misconfigured: %+v", dev)
	}
	for _, env := range []string{"local", "TEST"} {
		if !NewCommsHandlersWithWebhookAuth(comms.NewService(), nil, nil, "", "", "", "", env).allowInsecureWebhook {
			t.Errorf("env %q must allow unauthenticated webhooks", env)
		}
	}
	if NewCommsHandlersWithWebhookAuth(comms.NewService(), nil, nil, "", "", "", "", "production").allowInsecureWebhook {
		t.Error("production must never allow unauthenticated webhooks")
	}
}

func TestCov1o7PostmarkWebhookAuthentication(t *testing.T) {
	const body = `{"RecordType":"Delivery","MessageID":"m-1"}`
	post := func(h *CommsHandlers, mutate func(*http.Request)) int {
		r := chi.NewRouter()
		h.RegisterPublicRoutes(r)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/postmark", strings.NewReader(body))
		if mutate != nil {
			mutate(req)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	mk := func(hn, hv, bu, bp, env string) *CommsHandlers {
		return NewCommsHandlersWithWebhookAuth(comms.NewService(), nil, nil, hn, hv, bu, bp, env)
	}
	basic := func(u, p string) func(*http.Request) { return func(r *http.Request) { r.SetBasicAuth(u, p) } }
	header := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("X-Hook", v) } }
	both := func(v, u, p string) func(*http.Request) {
		return func(r *http.Request) { header(v)(r); basic(u, p)(r) }
	}

	for _, tc := range []struct {
		name   string
		h      *CommsHandlers
		mutate func(*http.Request)
		want   int
	}{
		{"unconfigured in production is closed", mk("", "", "", "", "production"), nil, 401},
		{"unconfigured in test is open", mk("", "", "", "", "test"), nil, 200},
		{"header only: correct", mk("X-Hook", "tok", "", "", "production"), header("tok"), 200},
		{"header only: wrong", mk("X-Hook", "tok", "", "", "production"), header("nope"), 401},
		{"header only: missing", mk("X-Hook", "tok", "", "", "production"), nil, 401},
		{"header name without value counts as unconfigured", mk("X-Hook", "", "", "", "production"), header("x"), 401},
		{"basic only: correct", mk("", "", "u", "p", "production"), basic("u", "p"), 200},
		{"basic only: wrong password", mk("", "", "u", "p", "production"), basic("u", "x"), 401},
		{"basic only: wrong user", mk("", "", "u", "p", "production"), basic("x", "p"), 401},
		{"basic only: absent", mk("", "", "u", "p", "production"), nil, 401},
		{"both: correct", mk("X-Hook", "tok", "u", "p", "production"), both("tok", "u", "p"), 200},
		{"both: header wrong", mk("X-Hook", "tok", "u", "p", "production"), both("bad", "u", "p"), 401},
		{"both: basic wrong", mk("X-Hook", "tok", "u", "p", "production"), both("tok", "u", "bad"), 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := post(tc.h, tc.mutate); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCov1o7ParseWebhookTime(t *testing.T) {
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, raw := range []string{"2026-03-04T05:06:07Z", "2026-03-04T07:06:07+02:00"} {
		if got := parseWebhookTime(raw); !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("parseWebhookTime(%q) = %v", raw, got)
		}
	}
	if got := parseWebhookTime("2026-03-04T05:06:07.123456789Z"); got.Nanosecond() != 123456789 {
		t.Errorf("nano precision lost: %v", got)
	}
	for _, raw := range []string{"", "   ", "yesterday"} {
		if !parseWebhookTime(raw).IsZero() {
			t.Errorf("parseWebhookTime(%q) must be zero", raw)
		}
	}
	if metadataString(nil, "k") != "" || metadataString(map[string]any{"k": 5}, "k") != "" || metadataString(map[string]any{"k": "v"}, "k") != "v" {
		t.Error("metadataString mismatch")
	}
}

func TestCov1o7PostmarkWebhookAppliesEvents(t *testing.T) {
	c := cov1o7NewComms(t, "org-wh")
	pub := NewCommsHandlersWithWebhookAuth(c.svc, c.mem, c.grp, "", "", "", "", "test")
	router := c.routerFor(pub)
	hook := func(body string) *httptest.ResponseRecorder {
		return c.doWith(router, http.MethodPost, "/webhooks/postmark", "", body)
	}

	cov1o7Expect(t, hook(`{`), 400, "invalid_body")
	rec := hook(`{"RecordType":"Delivery","MessageID":"unknown","Recipient":"x@example.com","DeliveredAt":"2026-01-02T03:04:05Z"}`)
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["status"] != "ignored_missing_delivery" {
		t.Fatalf("body = %s", rec.Body.String())
	}

	// A bounce with no delivery still suppresses the recipient (Email field, BouncedAt time).
	rec = hook(`{"RecordType":"Bounce","Email":" Bad@Example.com ","BouncedAt":"2026-01-02T03:04:05.5Z","Metadata":{"org_id":"org-wh"},"UnknownField":1}`)
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["status"] != "processed" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	list := c.do(http.MethodGet, "/suppression", "admin", "")
	cov1o7Expect(t, list, 200, "")
	if !strings.Contains(list.Body.String(), "bad@example.com") || !strings.Contains(list.Body.String(), `"bounce"`) {
		t.Fatalf("suppression = %s", list.Body.String())
	}

	// SubscriptionChange (ChangedAt) suppresses, then un-suppresses.
	cov1o7Expect(t, hook(`{"RecordType":"SubscriptionChange","Recipient":"sub@example.com","ChangedAt":"2026-01-02T03:04:05Z","SuppressSending":true,"SuppressionReason":"ManualSuppression","Metadata":{"org_id":"org-wh"}}`), 200, "")
	if !strings.Contains(c.do(http.MethodGet, "/suppression", "admin", "").Body.String(), "sub@example.com") {
		t.Fatal("subscription change must add a suppression")
	}
	cov1o7Expect(t, hook(`{"RecordType":"SubscriptionChange","Recipient":"sub@example.com","SuppressSending":false,"Metadata":{"org_id":"org-wh"}}`), 200, "")
	if strings.Contains(c.do(http.MethodGet, "/suppression", "admin", "").Body.String(), "sub@example.com") {
		t.Fatal("subscription change must remove the suppression")
	}
}

// ─── Notification preferences ────────────────────────────────────────────────

func TestCov1o7NotificationPreferences(t *testing.T) {
	c := cov1o7NewComms(t, "org-np")
	rec := c.do(http.MethodGet, "/communication-preferences", "parentA", "")
	cov1o7Expect(t, rec, 200, "")
	prefs := cov1o7JSON(t, rec)["preferences"].([]any)
	if len(prefs) < 5 {
		t.Fatalf("expected the full event catalog, got %d", len(prefs))
	}

	cov1o7Expect(t, c.do(http.MethodPut, "/communication-preferences", "parentA", `{`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPut, "/communication-preferences", "parentA", `{"preferences":[]}`), 422, "validation_error")
	cov1o7Expect(t, c.do(http.MethodPut, "/communication-preferences", "parentA", `{"preferences":[{"event_id":"nope","email_enabled":true}]}`), 400, "update_failed")
	cov1o7Expect(t, c.do(http.MethodPut, "/communication-preferences", "parentA", `{"preferences":[{"event_id":"announcement"},{"event_id":"announcement"}]}`), 400, "update_failed")

	rec = c.do(http.MethodPut, "/communication-preferences", "parentA", `{"preferences":[{"event_id":"announcement","email_enabled":false,"push_enabled":true}]}`)
	cov1o7Expect(t, rec, 200, "")
	found := false
	for _, p := range cov1o7JSON(t, rec)["preferences"].([]any) {
		m := p.(map[string]any)
		if m["event_id"] == "announcement" {
			found = true
			if m["email_enabled"] != false || m["push_enabled"] != true {
				t.Fatalf("preference not saved: %v", m)
			}
		}
	}
	if !found {
		t.Fatal("announcement preference missing from response")
	}
	if evs := c.audited(audit.ActionCommsNotificationPreferencesUpdate); len(evs) != 1 || evs[0].Metadata["household_id"] != c.hhA {
		t.Fatalf("audit = %+v", evs)
	}
	// Preferences are per-user: another user still sees the default.
	rec = c.do(http.MethodGet, "/communication-preferences", "parentB", "")
	for _, p := range cov1o7JSON(t, rec)["preferences"].([]any) {
		if m := p.(map[string]any); m["event_id"] == "announcement" && m["email_enabled"] != true {
			t.Fatalf("preference leaked across users: %v", m)
		}
	}
}

// ─── Recipient resolution ────────────────────────────────────────────────────

func TestCov1o7ResolveCampaignRecipients(t *testing.T) {
	c := cov1o7NewComms(t, "org-rr")
	ctx := context.Background()
	emails := func(rs []comms.Recipient) string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.Email
		}
		return strings.Join(out, ",")
	}

	if _, err := c.h.resolveCampaignRecipients(ctx, c.org, nil); !errors.Is(err, comms.ErrCampaignNotFound) {
		t.Fatalf("nil campaign: %v", err)
	}
	if _, err := NewCommsHandlers(c.svc, nil, nil).resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: "all"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing members service: %v", err)
	}
	if _, err := c.h.resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: "everyone"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported audience: %v", err)
	}
	if _, err := NewCommsHandlers(c.svc, c.mem, nil).resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: "group", GroupID: c.groupID}); err == nil || !strings.Contains(err.Error(), "group recipient resolution is unavailable") {
		t.Fatalf("group audience without groups service: %v", err)
	}

	all, err := c.h.resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: " ALL "})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range all {
		if got[r.Email] {
			t.Fatalf("duplicate recipient %q in %v", r.Email, emails(all))
		}
		got[r.Email] = true
	}
	for _, want := range []string{"a@example.com", "second@example.com", "b@example.com"} {
		if !got[want] {
			t.Fatalf("missing %s in %v", want, emails(all))
		}
	}
	if len(all) != 3 {
		t.Fatalf("blank/duplicate contacts must be dropped, got %v", emails(all))
	}

	grp, err := c.h.resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: "group", GroupID: c.groupID})
	if err != nil {
		t.Fatal(err)
	}
	if emails(grp) != "a@example.com,second@example.com" && emails(grp) != "second@example.com,a@example.com" {
		t.Fatalf("group recipients = %v, want only household A", emails(grp))
	}
	none, err := c.h.resolveCampaignRecipients(ctx, c.org, &comms.EmailCampaign{OrgID: c.org, Audience: "group", GroupID: "other-group"})
	if err != nil || len(none) != 0 {
		t.Fatalf("other group = %v, %v", none, err)
	}
}

// ─── Messages ────────────────────────────────────────────────────────────────

func TestCov1o7AuthorizeMessageThread(t *testing.T) {
	c := cov1o7NewComms(t, "org-az")
	staff := &appmiddleware.Principal{UserID: "s", Roles: []string{"coach"}}
	parentA := &appmiddleware.Principal{UserID: "p", HouseholdID: c.hhA, Roles: []string{"parent"}}
	cases := []struct {
		name string
		h    *CommsHandlers
		p    *appmiddleware.Principal
		hh   string
		sw   string
		want bool
	}{
		{"staff real thread", c.h, staff, c.hhA, c.swA, true},
		{"staff blank household", c.h, staff, " ", c.swA, false},
		{"staff mismatched pair", c.h, staff, c.hhA, c.swB, false},
		{"staff without roster service", NewCommsHandlers(c.svc, nil, nil), staff, "x", "y", true},
		{"parent own thread", c.h, parentA, c.hhA, c.swA, true},
		{"parent other household", c.h, parentA, c.hhB, c.swB, false},
		{"parent swimmer from another household", c.h, parentA, c.hhA, c.swB, false},
		{"parent without roster service", NewCommsHandlers(c.svc, nil, nil), parentA, c.hhA, c.swA, false},
		{"unknown role", c.h, &appmiddleware.Principal{HouseholdID: c.hhA, Roles: []string{"guest"}}, c.hhA, c.swA, false},
		{"nil principal", c.h, nil, c.hhA, c.swA, false},
	}
	for _, tc := range cases {
		if got := tc.h.authorizeMessageThread(tc.p, c.org, tc.hh, tc.sw); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if primaryRole(nil) != "" || primaryRole(&appmiddleware.Principal{}) != "" || primaryRole(parentA) != "parent" {
		t.Error("primaryRole mismatch")
	}
}

func TestCov1o7SendListMessagesAndThreads(t *testing.T) {
	c := cov1o7NewComms(t, "org-msg")
	body := func(hh, sw, text string) string {
		return fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,"body":%q}`, hh, sw, text)
	}

	cov1o7Expect(t, c.do(http.MethodPost, "/messages", "parentA", `{`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages", "parentA", `{"household_id":"h"}`), 422, "validation_error")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages", "parentA", body(c.hhA, c.swA, strings.Repeat("x", 4001))), 422, "validation_error")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages", "parentB", body(c.hhA, c.swA, "hi")), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages", "outsider", body(c.hhA, c.swA, "hi")), 403, "forbidden")

	id := c.sendMsg("parentA", c.hhA, c.swA, "secret plans")
	c.sendMsg("coach", c.hhA, c.swA, "reply from coach")
	c.sendMsg("parentB", c.hhB, c.swB, "other family")

	evs := c.audited(audit.ActionCommsMessageSend)
	if len(evs) != 3 {
		t.Fatalf("send audit events = %d", len(evs))
	}
	for _, ev := range evs {
		if strings.Contains(string(ev.AfterState), "secret plans") {
			t.Fatalf("message body leaked into the append-only audit log: %s", ev.AfterState)
		}
	}

	// Listing is thread scoped.
	cov1o7Expect(t, c.do(http.MethodGet, "/messages", "parentA", ""), 400, "invalid_request")
	cov1o7Expect(t, c.do(http.MethodGet, "/messages?household_id="+c.hhB+"&swimmer_id="+c.swB, "parentA", ""), 403, "forbidden")
	rec := c.do(http.MethodGet, "/messages?household_id="+c.hhA+"&swimmer_id="+c.swA, "swimmerA", "")
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["count"].(float64) != 2 || !strings.Contains(rec.Body.String(), id) {
		t.Fatalf("thread = %s", rec.Body.String())
	}

	// The inbox is staff only and counts unread messages per thread.
	cov1o7Expect(t, c.do(http.MethodGet, "/message-threads", "parentA", ""), 403, "forbidden")
	rec = c.do(http.MethodGet, "/message-threads", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	if cov1o7JSON(t, rec)["count"].(float64) != 2 {
		t.Fatalf("threads = %s", rec.Body.String())
	}

	// Mark read.
	cov1o7Expect(t, c.do(http.MethodPost, "/messages/read", "admin", ""), 400, "invalid_request")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages/read?household_id="+c.hhB+"&swimmer_id="+c.swB, "parentA", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages/read?household_id="+c.hhA+"&swimmer_id="+c.swA, "admin", ""), 200, "")
	rec = c.do(http.MethodGet, "/message-threads", "admin", "")
	if strings.Contains(rec.Body.String(), `"unread_count":2`) {
		t.Fatalf("thread A should be read: %s", rec.Body.String())
	}
}

func TestCov1o7MessageAttachments(t *testing.T) {
	send := func(c *cov1o7C, token string, att string) *httptest.ResponseRecorder {
		return c.do(http.MethodPost, "/messages", token,
			fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,"body":"pic","attachments":[%s]}`, c.hhA, c.swA, att))
	}
	attJSON := func(key, name, mime string) string {
		return fmt.Sprintf(`{"object_key":%q,"file_name":%q,"file_mime":%q}`, key, name, mime)
	}
	png := []byte("\x89PNG-data")

	t.Run("request validation", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-att1")
		key := c.attachmentKey(c.hhA, c.swA, "a.png", png)
		cov1o7Expect(t, send(c, "parentA", attJSON(key, "../evil.png", "image/png")), 400, "invalid_filename")
		cov1o7Expect(t, send(c, "parentA", attJSON("messages/other/x.png", "a.png", "image/png")), 400, "invalid_object_key")
		cov1o7Expect(t, send(c, "parentA", attJSON(key, "a.png", "application/pdf")), 422, "invalid_mime_type")
	})
	t.Run("storage not configured", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-att2")
		key := c.attachmentKey(c.hhA, c.swA, "a.png", png)
		nostore := NewCommsHandlersWithAudit(c.svc, c.aud, c.mem, c.grp)
		rec := c.doWith(c.routerFor(nostore), http.MethodPost, "/messages", "parentA",
			fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,"body":"pic","attachments":[%s]}`, c.hhA, c.swA, attJSON(key, "a.png", "image/png")))
		cov1o7Expect(t, rec, 503, "storage_unavailable")
	})
	t.Run("storage failures", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-att3")
		key := c.attachmentKey(c.hhA, c.swA, "a.png", png)
		good := attJSON(key, "a.png", "image/png")

		c.store.existsErr = errors.New("s3 down")
		cov1o7Expect(t, send(c, "parentA", good), 500, "internal_error")
		c.store.existsErr = nil

		missing := attJSON(fmt.Sprintf("messages/%s/%s/%s/ghost.png", c.org, c.hhA, c.swA), "ghost.png", "image/png")
		cov1o7Expect(t, send(c, "parentA", missing), 409, "missing_uploaded_file")

		c.store.sizeErr = errors.New("s3 down")
		cov1o7Expect(t, send(c, "parentA", good), 500, "internal_error")
		c.store.sizeErr = nil

		huge := maxMessageAttachmentBytes + 1
		c.store.sizeOverride = &huge
		cov1o7Expect(t, send(c, "parentA", good), 422, "file_too_large")
		c.store.sizeOverride = nil

		c.store.getErr = errors.New("s3 down")
		cov1o7Expect(t, send(c, "parentA", good), 500, "internal_error")
		c.store.getErr = nil

		c.store.putErr = errors.New("s3 down")
		cov1o7Expect(t, send(c, "parentA", good), 500, "internal_error")
		c.store.putErr = nil

		// The reported size looks fine but the stored object is larger than allowed.
		bigKey := c.attachmentKey(c.hhA, c.swA, "big.png", bytes.Repeat([]byte{1}, int(maxMessageAttachmentBytes)+1))
		small := int64(10)
		c.store.sizeOverride = &small
		cov1o7Expect(t, send(c, "parentA", attJSON(bigKey, "big.png", "image/png")), 422, "file_too_large")
		c.store.sizeOverride = nil
		if ok, _ := c.store.StubStorage.Exists(context.Background(), bigKey); ok {
			t.Fatal("an oversized upload must be deleted")
		}
	})
	t.Run("success seals the upload under the thread prefix", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-att4")
		key := c.attachmentKey(c.hhA, c.swA, "a.png", png)
		rec := send(c, "parentA", attJSON(key, "a.png", " IMAGE/PNG "))
		cov1o7Expect(t, rec, 201, "")
		out := c.do(http.MethodGet, "/messages?household_id="+c.hhA+"&swimmer_id="+c.swA, "parentA", "").Body.String()
		if !strings.Contains(out, `"filename":"a.png"`) || !strings.Contains(out, `"mime_type":"image/png"`) || !strings.Contains(out, fmt.Sprintf(`"size_bytes":%d`, len(png))) {
			t.Fatalf("attachment metadata missing from the thread view: %s", out)
		}
		sealed, err := c.store.StubStorage.ListPrefix(context.Background(), fmt.Sprintf("messages/%s/%s/%s/sealed/", c.org, c.hhA, c.swA))
		if err != nil || len(sealed) != 1 || !strings.HasSuffix(sealed[0], "-a.png") {
			t.Fatalf("sealed objects = %v, err %v", sealed, err)
		}
		if ok, _ := c.store.StubStorage.Exists(context.Background(), key); ok {
			t.Fatal("the unsealed upload must be removed")
		}
	})
}

func TestCov1o7MessageUploadURL(t *testing.T) {
	c := cov1o7NewComms(t, "org-up")
	req := func(token, fields string) *httptest.ResponseRecorder {
		return c.do(http.MethodPost, "/messages/upload-url", token,
			fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,%s}`, c.hhA, c.swA, fields))
	}
	good := `"file_name":"photo.png","file_mime":"image/png","file_size_bytes":1024`

	nostore := NewCommsHandlers(c.svc, c.mem, c.grp)
	cov1o7Expect(t, c.doWith(c.routerFor(nostore), http.MethodPost, "/messages/upload-url", "parentA", `{}`), 503, "storage_unavailable")
	cov1o7Expect(t, c.do(http.MethodPost, "/messages/upload-url", "parentA", `{`), 400, "bad_request")
	cov1o7Expect(t, req("parentA", `"file_name":"p.png","file_mime":"image/png","file_size_bytes":0`), 422, "validation_error")
	cov1o7Expect(t, req("parentB", good), 403, "forbidden")
	cov1o7Expect(t, req("parentA", `"file_name":"a/b.png","file_mime":"image/png","file_size_bytes":10`), 400, "invalid_filename")
	cov1o7Expect(t, req("parentA", `"file_name":"p.exe","file_mime":"application/x-msdownload","file_size_bytes":10`), 422, "invalid_mime_type")
	cov1o7Expect(t, req("parentA", fmt.Sprintf(`"file_name":"p.png","file_mime":"image/png","file_size_bytes":%d`, maxMessageAttachmentBytes+1)), 422, "file_too_large")

	c.store.presignErr = errors.New("sign failed")
	cov1o7Expect(t, req("parentA", good), 500, "internal_error")
	c.store.presignErr = nil

	rec := req("parentA", good)
	cov1o7Expect(t, rec, 201, "")
	out := cov1o7JSON(t, rec)
	key, _ := out["object_key"].(string)
	if !strings.HasPrefix(key, fmt.Sprintf("messages/%s/%s/%s/", c.org, c.hhA, c.swA)) || !strings.HasSuffix(key, "-photo.png") ||
		out["expected_file_name"] != "photo.png" || !strings.HasPrefix(out["upload_url"].(string), "http") {
		t.Fatalf("response = %v", out)
	}
	if err := validateMessageAttachmentObjectKey(c.org, c.hhA, c.swA, key); err != nil {
		t.Fatalf("issued key must satisfy the attach validation: %v", err)
	}
}

func TestCov1o7EditMessageAndReactions(t *testing.T) {
	c := cov1o7NewComms(t, "org-edit")
	id := c.sendMsg("parentA", c.hhA, c.swA, "original")

	cov1o7Expect(t, c.do(http.MethodPatch, "/messages/"+id, "parentA", `{`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPatch, "/messages/"+id, "parentA", `{"body":""}`), 422, "validation_error")
	cov1o7Expect(t, c.do(http.MethodPatch, "/messages/missing", "parentA", `{"body":"x"}`), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodPatch, "/messages/"+id, "coach", `{"body":"x"}`), 403, "forbidden")
	rec := c.do(http.MethodPatch, "/messages/"+id, "parentA", `{"body":"edited"}`)
	cov1o7Expect(t, rec, 200, "")
	msg := cov1o7JSON(t, rec)["message"].(map[string]any)
	if msg["body"] != "edited" || msg["edited_at"] == nil {
		t.Fatalf("message = %v", msg)
	}
	if evs := c.audited(audit.ActionCommsMessageEdit); len(evs) != 1 || strings.Contains(string(evs[0].AfterState), `"body":`) || !strings.Contains(string(evs[0].AfterState), `"body_length":6`) {
		t.Fatalf("edit audit = %+v", evs)
	}

	// Reactions are authorized against the message's thread.
	react := "/messages/" + id + "/reactions"
	cov1o7Expect(t, c.do(http.MethodPost, "/messages/missing/reactions", "parentA", `{"emoji":"👍"}`), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodPost, react, "parentB", `{"emoji":"👍"}`), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodPost, react, "parentA", `{`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPost, react, "parentA", `{"emoji":"🦖"}`), 422, "invalid_reaction")
	cov1o7Expect(t, c.do(http.MethodPost, react, "coach", `{"emoji":"👍"}`), 201, "")
	cov1o7Expect(t, c.do(http.MethodPost, react, "coach", `{"emoji":"👍"}`), 201, "") // idempotent
	rec = c.do(http.MethodGet, "/messages?household_id="+c.hhA+"&swimmer_id="+c.swA, "parentA", "")
	if strings.Count(rec.Body.String(), "👍") != 1 {
		t.Fatalf("reaction must be recorded exactly once: %s", rec.Body.String())
	}

	del := "/messages/" + id + "/reactions/" + "%F0%9F%91%8D"
	cov1o7Expect(t, c.do(http.MethodDelete, "/messages/missing/reactions/x", "coach", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodDelete, del, "parentB", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodDelete, del, "coach", ""), 200, "")
	rec = c.do(http.MethodGet, "/messages?household_id="+c.hhA+"&swimmer_id="+c.swA, "parentA", "")
	if strings.Contains(rec.Body.String(), "👍") {
		t.Fatalf("reaction must be removed: %s", rec.Body.String())
	}
}

// ─── Announcements ───────────────────────────────────────────────────────────

func TestCov1o7Announcements(t *testing.T) {
	c := cov1o7NewComms(t, "org-ann")
	create := func(token, body string) *httptest.ResponseRecorder {
		return c.do(http.MethodPost, "/announcements", token, body)
	}
	cov1o7Expect(t, create("parentA", `{"title":"t","body":"b","audience":"all"}`), 403, "forbidden")
	cov1o7Expect(t, create("coach", `{`), 400, "invalid_body")
	cov1o7Expect(t, create("coach", `{"title":"t","body":"b","audience":"nobody"}`), 422, "validation_error")
	cov1o7Expect(t, create("coach", `{"title":"t","body":"b","audience":"group"}`), 400, "create_failed")

	rec := create("coach", `{"title":"Welcome","body":"hello","audience":"all","public":true}`)
	cov1o7Expect(t, rec, 201, "")
	allID := cov1o7JSON(t, rec)["id"].(string)
	rec = create("admin", fmt.Sprintf(`{"title":"Seniors","body":"b","audience":"group","group_id":%q}`, c.groupID))
	cov1o7Expect(t, rec, 201, "")
	groupAnn := cov1o7JSON(t, rec)["id"].(string)
	rec = create("admin", `{"title":"Draft","body":"b","audience":"all"}`)
	draftID := cov1o7JSON(t, rec)["id"].(string)
	if len(c.audited(audit.ActionCommsAnnouncementCreate)) != 3 {
		t.Fatal("each create is audited")
	}

	// Publish transitions.
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/publish", "parentA", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/missing/publish", "coach", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/publish", "coach", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/publish", "coach", ""), 409, "invalid_transition")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+groupAnn+"/publish", "admin", ""), 200, "")

	// Visibility toggle.
	patch := func(token, id, body string) *httptest.ResponseRecorder {
		return c.do(http.MethodPatch, "/announcements/"+id, token, body)
	}
	cov1o7Expect(t, patch("parentA", allID, `{"public":false}`), 403, "forbidden")
	cov1o7Expect(t, patch("coach", allID, `{`), 400, "invalid_body")
	cov1o7Expect(t, patch("coach", allID, `{}`), 422, "validation_error")
	cov1o7Expect(t, patch("coach", "missing", `{"public":true}`), 404, "not_found")
	cov1o7Expect(t, patch("coach", groupAnn, `{"public":true}`), 400, "update_failed")
	cov1o7Expect(t, patch("coach", allID, `{"public":false}`), 200, "")
	if evs := c.audited(audit.ActionCommsAnnouncementVisibility); len(evs) != 1 || evs[0].Metadata["public"] != "false" {
		t.Fatalf("visibility audit = %+v", evs)
	}

	// Listing: staff see drafts and can filter; parents see published, scoped by group.
	rec = c.do(http.MethodGet, "/announcements?status=draft", "coach", "")
	if !strings.Contains(rec.Body.String(), draftID) || strings.Contains(rec.Body.String(), allID) {
		t.Fatalf("draft filter = %s", rec.Body.String())
	}
	rec = c.do(http.MethodGet, "/announcements?status=draft", "parentA", "")
	if strings.Contains(rec.Body.String(), draftID) || !strings.Contains(rec.Body.String(), groupAnn) || !strings.Contains(rec.Body.String(), allID) {
		t.Fatalf("household in the group sees published only: %s", rec.Body.String())
	}
	rec = c.do(http.MethodGet, "/announcements", "parentB", "")
	if strings.Contains(rec.Body.String(), groupAnn) || !strings.Contains(rec.Body.String(), allID) {
		t.Fatalf("household outside the group must not see the group announcement: %s", rec.Body.String())
	}
	noGroups := NewCommsHandlers(c.svc, nil, nil)
	rec = c.doWith(c.routerFor(noGroups), http.MethodGet, "/announcements", "parentA", "")
	if strings.Contains(rec.Body.String(), groupAnn) || !strings.Contains(rec.Body.String(), allID) {
		t.Fatalf("without roster services group announcements are hidden: %s", rec.Body.String())
	}
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements", "outsider", ""), 500, "list_failed")

	// Single reads.
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/"+draftID, "coach", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/"+draftID, "parentA", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/missing", "coach", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/"+groupAnn, "parentB", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/"+groupAnn, "parentA", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodGet, "/announcements/"+allID, "outsider", ""), 500, "get_failed")

	// Archive is admin only and one-way.
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/archive", "coach", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/missing/archive", "admin", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/archive", "admin", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodPost, "/announcements/"+allID+"/archive", "admin", ""), 409, "invalid_transition")
	if len(c.audited(audit.ActionCommsAnnouncementArchive)) != 1 || len(c.audited(audit.ActionCommsAnnouncementPublish)) != 2 {
		t.Fatal("publish/archive must be audited")
	}
}

// ─── Campaigns and suppression ───────────────────────────────────────────────

const cov1o7CampaignBody = `{"name":"Spring","subject":"Hi","body_html":"<p>x</p>","from_name":"Club","from_email":"club@example.com","audience":"all"}`

func TestCov1o7CampaignLifecycle(t *testing.T) {
	c := cov1o7NewComms(t, "org-camp")
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns", "parentA", cov1o7CampaignBody), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns", "coach", `{`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns", "coach", strings.Replace(cov1o7CampaignBody, "club@example.com", "not-email", 1)), 422, "validation_error")
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns", "coach", strings.Replace(cov1o7CampaignBody, `"all"`, `"group"`, 1)), 400, "create_failed")

	c.svc.SetSenderPolicy(func(string) (string, string, bool) { return "", "", false })
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns", "coach", cov1o7CampaignBody), 422, "sender_not_configured")
	c.svc.SetSenderPolicy(nil)

	rec := c.do(http.MethodPost, "/email-campaigns", "coach", cov1o7CampaignBody)
	cov1o7Expect(t, rec, 201, "")
	id := cov1o7JSON(t, rec)["id"].(string)
	if len(c.audited(audit.ActionCommsCampaignCreate)) != 1 {
		t.Fatal("campaign creation is audited")
	}

	// Reads are staff only.
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns", "parentA", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/"+id, "parentA", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/"+id+"/deliveries", "parentA", ""), 403, "forbidden")
	rec = c.do(http.MethodGet, "/email-campaigns", "coach", "")
	cov1o7Expect(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), id) {
		t.Fatalf("list = %s", rec.Body.String())
	}
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/"+id, "coach", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/missing", "coach", ""), 404, "not_found")
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/"+id+"/deliveries", "coach", ""), 200, "")
	cov1o7Expect(t, c.do(http.MethodGet, "/email-campaigns/missing/deliveries", "coach", ""), 404, "not_found")

	// Send guards.
	send := "/email-campaigns/" + id + "/send"
	cov1o7Expect(t, c.do(http.MethodPost, send, "parentA", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodPost, send, "coach", `{"recipients":["x@example.com"]}`), 400, "invalid_body")
	cov1o7Expect(t, c.do(http.MethodPost, "/email-campaigns/missing/send", "coach", ""), 404, "not_found")

	// Recipient resolution failure and an empty audience are refused before claiming.
	cov1o7Expect(t, c.doWith(c.routerFor(NewCommsHandlers(c.svc, nil, nil)), http.MethodPost, send, "coach", ""), 500, "recipient_resolution_failed")
	empty := cov1o7NewComms(t, "org-camp-empty")
	rec = empty.do(http.MethodPost, "/email-campaigns", "coach", cov1o7CampaignBody)
	emptyID := cov1o7JSON(t, rec)["id"].(string)
	emptyMembers := NewCommsHandlers(empty.svc, members.NewService(), nil)
	cov1o7Expect(t, empty.doWith(empty.routerFor(emptyMembers), http.MethodPost, "/email-campaigns/"+emptyID+"/send", "coach", ""), 422, "no_eligible_recipients")

	// Enqueue failure releases the claim so the campaign can be retried.
	c.queue.err = errors.New("redis down")
	cov1o7Expect(t, c.do(http.MethodPost, send, "coach", ""), 500, "send_failed")
	c.queue.err = nil
	rec = c.do(http.MethodGet, "/email-campaigns/"+id, "coach", "")
	if cov1o7JSON(t, rec)["status"] != "draft" {
		t.Fatalf("claim must be released after an enqueue failure: %s", rec.Body.String())
	}

	// Success claims the campaign and queues exactly one durable job.
	rec = c.do(http.MethodPost, send, "coach", "")
	cov1o7Expect(t, rec, 202, "")
	if cov1o7JSON(t, rec)["status"] != "sending" || len(c.queue.tasks) != 1 || c.queue.tasks[0].Type != TaskTypeCampaignSend {
		t.Fatalf("body %s tasks %+v", rec.Body.String(), c.queue.tasks)
	}
	if evs := c.audited(audit.ActionCommsCampaignSend); len(evs) != 1 || evs[0].Metadata["recipient_count"] != "3" {
		t.Fatalf("send audit = %+v", evs)
	}
	cov1o7Expect(t, c.do(http.MethodPost, send, "coach", ""), 409, "campaign_not_draft")
}

func TestCov1o7Suppression(t *testing.T) {
	c := cov1o7NewComms(t, "org-sup")
	add := func(token, body string) *httptest.ResponseRecorder {
		return c.do(http.MethodPost, "/suppression", token, body)
	}
	cov1o7Expect(t, add("coach", `{"email":"x@example.com","reason":"manual"}`), 403, "forbidden")
	cov1o7Expect(t, add("admin", `{`), 400, "invalid_body")
	cov1o7Expect(t, add("admin", `{"email":"bad","reason":"manual"}`), 422, "validation_error")
	cov1o7Expect(t, add("admin", `{"email":"x@example.com","reason":"because"}`), 422, "validation_error")
	rec := add("admin", `{"email":"X@Example.com","reason":"manual"}`)
	cov1o7Expect(t, rec, 201, "")
	id := cov1o7JSON(t, rec)["id"].(string)
	cov1o7Expect(t, add("admin", `{"email":"x@example.com","reason":"complaint"}`), 409, "duplicate_suppression")
	if len(c.audited(audit.ActionCommsSuppressionAdd)) != 1 {
		t.Fatal("suppression add is audited")
	}

	cov1o7Expect(t, c.do(http.MethodGet, "/suppression", "coach", ""), 403, "forbidden")
	rec = c.do(http.MethodGet, "/suppression", "admin", "")
	cov1o7Expect(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), "x@example.com") {
		t.Fatalf("list = %s", rec.Body.String())
	}

	cov1o7Expect(t, c.do(http.MethodDelete, "/suppression/"+id, "coach", ""), 403, "forbidden")
	cov1o7Expect(t, c.do(http.MethodDelete, "/suppression/missing", "admin", ""), 404, "not_found")
	rec = c.do(http.MethodDelete, "/suppression/"+id, "admin", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("remove = %d %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(c.do(http.MethodGet, "/suppression", "admin", "").Body.String(), "x@example.com") {
		t.Fatal("entry must be gone")
	}
	if len(c.audited(audit.ActionCommsSuppressionRemove)) != 1 {
		t.Fatal("suppression removal is audited")
	}
}

func TestCov1o7CommsHandlersRequireAuthContext(t *testing.T) {
	c := cov1o7NewComms(t, "org-noauth")
	h := c.h
	for name, fn := range map[string]http.HandlerFunc{
		"listNotificationPreferences": h.listNotificationPreferences, "updateNotificationPreferences": h.updateNotificationPreferences,
		"sendMessage": h.sendMessage, "requestMessageUploadURL": h.requestMessageUploadURL, "listMessages": h.listMessages,
		"listMessageThreads": h.listMessageThreads, "editMessage": h.editMessage, "addReaction": h.addReaction,
		"removeReaction": h.removeReaction, "markThreadRead": h.markThreadRead, "streamMessages": h.streamMessages,
		"createAnnouncement": h.createAnnouncement, "listAnnouncements": h.listAnnouncements, "getAnnouncement": h.getAnnouncement,
		"setAnnouncementPublic": h.setAnnouncementPublic, "publishAnnouncement": h.publishAnnouncement,
		"archiveAnnouncement": h.archiveAnnouncement, "createCampaign": h.createCampaign, "listCampaigns": h.listCampaigns,
		"getCampaign": h.getCampaign, "sendCampaign": h.sendCampaign, "listDeliveries": h.listDeliveries,
		"addSuppression": h.addSuppression, "listSuppression": h.listSuppression, "removeSuppression": h.removeSuppression,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			cov1o7Expect(t, rec, http.StatusUnauthorized, "unauthorized")
		})
	}
	// emitAudit is a no-op without a principal.
	h.emitAudit(nil, "x", "y", "z", nil, nil)
	if got := c.audited("x"); len(got) != 0 {
		t.Fatalf("audit = %+v", got)
	}
	if redactedMessageAudit(nil) != nil {
		t.Fatal("nil message redacts to nil")
	}
}

// ─── Realtime stream (needs Redis, like the broadcaster itself) ──────────────

type cov1o7SSEWriter struct {
	mu     sync.Mutex
	header http.Header
	buf    bytes.Buffer
	code   int
	broken bool
}

func (w *cov1o7SSEWriter) Header() http.Header {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *cov1o7SSEWriter) WriteHeader(code int) {
	w.mu.Lock()
	w.code = code
	w.mu.Unlock()
}

func (w *cov1o7SSEWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.broken {
		return 0, errors.New("broken pipe")
	}
	return w.buf.Write(p)
}

func (w *cov1o7SSEWriter) Flush() {}

func (w *cov1o7SSEWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *cov1o7SSEWriter) status() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.code
}

func (w *cov1o7SSEWriter) setBroken() {
	w.mu.Lock()
	w.broken = true
	w.mu.Unlock()
}

func cov1o7Broadcaster(t *testing.T) (*comms.MessageBroadcaster, *redis.Client) {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL is not configured")
	}
	b, err := comms.NewMessageBroadcaster(url, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatalf("redis broadcaster: %v", err)
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	raw := redis.NewClient(opt)
	t.Cleanup(func() { _ = raw.Close() })
	return b, raw
}

// cov1o7Stream starts the SSE handler for token and returns the writer, a
// cancel func and a channel closed when the handler returns.
func cov1o7Stream(c *cov1o7C, token string, w *cov1o7SSEWriter) (cancel func(), done chan struct{}) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/messages/stream", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", c.org)
	done = make(chan struct{})
	go func() {
		defer close(done)
		c.router.ServeHTTP(w, req)
	}()
	return cancelCtx, done
}

func cov1o7Eventually(t *testing.T, what string, cond func() bool, tick func()) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		if tick != nil {
			tick()
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestCov1o7StreamMessagesUnavailableWithoutRealtime(t *testing.T) {
	c := cov1o7NewComms(t, "org-nostream")
	cov1o7Expect(t, c.do(http.MethodGet, "/messages/stream", "parentA", ""), 503, "realtime_unavailable")
}

func TestCov1o7StreamMessagesRequiresFlusher(t *testing.T) {
	b, _ := cov1o7Broadcaster(t)
	c := cov1o7NewComms(t, "org-noflush-"+uuid.NewString())
	c.svc.SetBroadcaster(b)
	req := httptest.NewRequest(http.MethodGet, "/messages/stream", nil)
	req.Header.Set("Authorization", "Bearer parentA")
	req.Header.Set("X-Org-Id", c.org)
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(struct{ http.ResponseWriter }{rec}, req)
	cov1o7Expect(t, rec, 500, "streaming_unsupported")
}

func TestCov1o7StreamMessagesFiltersEventsByThread(t *testing.T) {
	b, raw := cov1o7Broadcaster(t)
	for _, tc := range []struct {
		token       string
		seesForeign bool
	}{{"parentA", false}, {"admin", true}} {
		t.Run(tc.token, func(t *testing.T) {
			c := cov1o7NewComms(t, "org-stream-"+uuid.NewString())
			c.svc.SetBroadcaster(b)
			w := &cov1o7SSEWriter{}
			cancel, done := cov1o7Stream(c, tc.token, w)
			defer cancel()

			publish := func(kind, hh, sw string) {
				b.Publish(context.Background(), c.org, comms.MessageEvent{Type: kind, HouseholdID: hh, SwimmerID: sw})
			}
			// Keep publishing the same-thread event until the subscription is live.
			cov1o7Eventually(t, "own-thread event", func() bool { return strings.Contains(w.String(), "own.thread") },
				func() { publish("own.thread", c.hhA, c.swA) })

			// Foreign-thread and garbage payloads must be skipped; then a sentinel
			// proves everything before it has been processed (same publisher order).
			publish("foreign.thread", c.hhB, c.swB)
			if err := raw.Publish(context.Background(), "msg:"+c.org, "not json").Err(); err != nil {
				t.Fatal(err)
			}
			publish("sentinel", c.hhA, c.swA)
			cov1o7Eventually(t, "sentinel", func() bool { return strings.Contains(w.String(), "sentinel") }, nil)

			out := w.String()
			if got := strings.Contains(out, "foreign.thread"); got != tc.seesForeign {
				t.Fatalf("foreign-thread event delivered = %v, want %v\n%s", got, tc.seesForeign, out)
			}
			if strings.Contains(out, "not json") {
				t.Fatalf("garbage payload must never be forwarded: %s", out)
			}
			if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" || w.status() != http.StatusOK {
				t.Fatalf("headers: %q code %d", ct, w.status())
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("stream did not stop after the client disconnected")
			}
		})
	}
}

func TestCov1o7StreamMessagesHeartbeatAndBrokenClient(t *testing.T) {
	b, _ := cov1o7Broadcaster(t)
	old := messageStreamHeartbeat
	messageStreamHeartbeat = 10 * time.Millisecond
	t.Cleanup(func() { messageStreamHeartbeat = old })

	t.Run("heartbeat then write failure ends the stream", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-hb-"+uuid.NewString())
		c.svc.SetBroadcaster(b)
		w := &cov1o7SSEWriter{}
		cancel, done := cov1o7Stream(c, "admin", w)
		defer cancel()
		cov1o7Eventually(t, "heartbeat", func() bool { return strings.Contains(w.String(), ": heartbeat") }, nil)
		w.setBroken()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("stream must stop once heartbeat writes fail")
		}
	})
	t.Run("data write failure ends the stream", func(t *testing.T) {
		c := cov1o7NewComms(t, "org-brk-"+uuid.NewString())
		c.svc.SetBroadcaster(b)
		prev := messageStreamHeartbeat
		messageStreamHeartbeat = time.Hour // only data events write in this subtest
		t.Cleanup(func() { messageStreamHeartbeat = prev })
		w := &cov1o7SSEWriter{}
		cancel, done := cov1o7Stream(c, "admin", w)
		defer cancel()
		// Wait for the subscription to be live using a probe event, then break the client.
		cov1o7Eventually(t, "probe delivered", func() bool { return strings.Contains(w.String(), "probe") },
			func() {
				b.Publish(context.Background(), c.org, comms.MessageEvent{Type: "probe", HouseholdID: c.hhA, SwimmerID: c.swA})
			})
		w.setBroken()
		cov1o7Eventually(t, "handler exit", func() bool {
			select {
			case <-done:
				return true
			default:
				return false
			}
		}, func() {
			b.Publish(context.Background(), c.org, comms.MessageEvent{Type: "more", HouseholdID: c.hhA, SwimmerID: c.swA})
		})
	})
}
