package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1o7Exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// With Postgres behind the comms domain, a failing database must surface as the
// documented 4xx/5xx error codes and never as a silent success.
func TestCov1o7CommsReportsDatabaseFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	c := cov1o7NewCommsOn(t, org, pool)

	// Seed durable state while the database is healthy.
	msgID := c.sendMsg("parentA", c.hhA, c.swA, "hello")
	rec := c.do(http.MethodPost, "/announcements", "admin", `{"title":"T","body":"b","audience":"all"}`)
	cov1o7Expect(t, rec, 201, "")
	annID := cov1o7JSON(t, rec)["id"].(string)
	rec = c.do(http.MethodPost, "/email-campaigns", "admin", cov1o7CampaignBody)
	cov1o7Expect(t, rec, 201, "")
	campID := cov1o7JSON(t, rec)["id"].(string)
	rec = c.do(http.MethodPost, "/suppression", "admin", `{"email":"x@example.com","reason":"manual"}`)
	cov1o7Expect(t, rec, 201, "")
	supID := cov1o7JSON(t, rec)["id"].(string)

	// Claiming a campaign fails when the row cannot be updated.
	cov1o7Exec(t, pool, `
		CREATE FUNCTION cov1o7_block_campaign() RETURNS trigger AS $$
		BEGIN
			IF OLD.status = 'draft' AND NEW.status = 'sending' THEN
				RAISE EXCEPTION 'claim blocked';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER cov1o7_block_campaign BEFORE UPDATE ON email_campaigns
			FOR EACH ROW EXECUTE FUNCTION cov1o7_block_campaign();`)
	send := "/email-campaigns/" + campID + "/send"
	cov1o7Expect(t, c.do(http.MethodPost, send, "admin", ""), 500, "send_failed")
	if len(c.queue.tasks) != 0 {
		t.Fatal("nothing may be queued when the claim fails")
	}

	// Claim succeeds, enqueue fails, and the release itself fails: the release error wins.
	cov1o7Exec(t, pool, `
		CREATE OR REPLACE FUNCTION cov1o7_block_campaign() RETURNS trigger AS $$
		BEGIN
			IF OLD.status = 'sending' AND NEW.status = 'draft' THEN
				RAISE EXCEPTION 'release blocked';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;`)
	c.queue.err = fmt.Errorf("redis down")
	cov1o7Expect(t, c.do(http.MethodPost, send, "admin", ""), 500, "send_failed")
	c.queue.err = nil

	// Now take the whole database away.
	pool.Close()
	for _, tc := range []struct {
		name, method, path, token, body string
		status                          int
		code                            string
	}{
		{"send message", http.MethodPost, "/messages", "parentA", fmt.Sprintf(`{"household_id":%q,"swimmer_id":%q,"body":"x"}`, c.hhA, c.swA), 500, "send_failed"},
		{"edit message", http.MethodPatch, "/messages/" + msgID, "parentA", `{"body":"x"}`, 500, "edit_failed"},
		{"add reaction", http.MethodPost, "/messages/" + msgID + "/reactions", "parentA", `{"emoji":"👍"}`, 500, "reaction_failed"},
		{"remove reaction", http.MethodDelete, "/messages/" + msgID + "/reactions/x", "parentA", "", 500, "reaction_failed"},
		{"mark read", http.MethodPost, "/messages/read?household_id=" + c.hhA + "&swimmer_id=" + c.swA, "parentA", "", 500, "mark_read_failed"},
		{"update preferences", http.MethodPut, "/communication-preferences", "parentA", `{"preferences":[{"event_id":"announcement","email_enabled":true}]}`, 400, "update_failed"},
		{"create announcement", http.MethodPost, "/announcements", "admin", `{"title":"T","body":"b","audience":"all"}`, 400, "create_failed"},
		{"list announcements", http.MethodGet, "/announcements", "admin", "", 500, "list_failed"},
		{"get announcement", http.MethodGet, "/announcements/" + annID, "admin", "", 500, "get_failed"},
		{"announcement visibility", http.MethodPatch, "/announcements/" + annID, "admin", `{"public":true}`, 400, "update_failed"},
		{"publish announcement", http.MethodPost, "/announcements/" + annID + "/publish", "admin", "", 500, "publish_failed"},
		{"archive announcement", http.MethodPost, "/announcements/" + annID + "/archive", "admin", "", 500, "archive_failed"},
		{"create campaign", http.MethodPost, "/email-campaigns", "admin", cov1o7CampaignBody, 400, "create_failed"},
		{"list campaigns", http.MethodGet, "/email-campaigns", "admin", "", 500, "list_failed"},
		{"get campaign", http.MethodGet, "/email-campaigns/" + campID, "admin", "", 500, "get_failed"},
		{"send campaign", http.MethodPost, send, "admin", "", 500, "send_failed"},
		{"list deliveries", http.MethodGet, "/email-campaigns/" + campID + "/deliveries", "admin", "", 500, "list_failed"},
		{"add suppression", http.MethodPost, "/suppression", "admin", `{"email":"y@example.com","reason":"manual"}`, 400, "add_failed"},
		{"list suppression", http.MethodGet, "/suppression", "admin", "", 500, "list_failed"},
		{"remove suppression", http.MethodDelete, "/suppression/" + supID, "admin", "", 500, "remove_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cov1o7Expect(t, c.do(tc.method, tc.path, tc.token, tc.body), tc.status, tc.code)
		})
	}

	// A webhook whose delivery lookup fails is not acknowledged, so Postmark retries it.
	hook := NewCommsHandlersWithWebhookAuth(c.svc, c.mem, c.grp, "", "", "", "", "test")
	rec = c.doWith(c.routerFor(hook), http.MethodPost, "/webhooks/postmark", "", `{"RecordType":"Delivery","MessageID":"m-1","Metadata":{"org_id":"`+org+`"}}`)
	cov1o7Expect(t, rec, 500, "webhook_apply_failed")

	// Recipient resolution refuses to build a partial list from stale caches.
	_, err := c.h.resolveCampaignRecipients(context.Background(), org, &comms.EmailCampaign{OrgID: org, Audience: "all"})
	if err == nil || !strings.Contains(err.Error(), "refresh members") {
		t.Fatalf("members refresh failure must abort resolution, got %v", err)
	}
}

func TestCov1o7CommsGroupRefreshFailureAbortsResolution(t *testing.T) {
	healthy := testutil.IsolatedSchemaPool(t)
	broken := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	broken.Close()
	h := NewCommsHandlers(comms.NewService(), members.NewServiceWithPool(healthy), groups.NewServiceWithPool(broken))
	_, err := h.resolveCampaignRecipients(context.Background(), org, &comms.EmailCampaign{OrgID: org, Audience: "group", GroupID: "g"})
	if err == nil || !strings.Contains(err.Error(), "refresh groups") {
		t.Fatalf("groups refresh failure must abort resolution, got %v", err)
	}
}

// A parent's announcement view depends on roster lookups; if those fail the
// handler must answer 500 rather than hide or leak group announcements.
func TestCov1o7AnnouncementsFailClosedWhenRosterLookupFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		broken func(p *pgxpool.Pool) (*members.Service, *groups.Service)
	}{
		{"members unavailable", func(p *pgxpool.Pool) (*members.Service, *groups.Service) {
			return members.NewServiceWithPool(p), groups.NewService()
		}},
		{"groups unavailable", func(p *pgxpool.Pool) (*members.Service, *groups.Service) {
			return members.NewService(), groups.NewServiceWithPool(p)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			org := "org-fc-" + uuid.NewString()
			dead := testutil.IsolatedSchemaPool(t)
			dead.Close()
			mem, grp := tc.broken(dead)
			svc := comms.NewService()
			h := NewCommsHandlers(svc, mem, grp)
			c := &cov1o7C{t: t, org: org, hhA: "hh"}
			router := c.routerFor(h)
			a, err := svc.CreateAnnouncement(comms.CreateAnnouncementInput{OrgID: org, Title: "t", Body: "b", Audience: "all", CreatedBy: cov1o7UAdmin})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PublishAnnouncement(org, a.ID, cov1o7UAdmin); err != nil {
				t.Fatal(err)
			}
			cov1o7Expect(t, c.doWith(router, http.MethodGet, "/announcements", "parentA", ""), 500, "list_failed")
			cov1o7Expect(t, c.doWith(router, http.MethodGet, "/announcements/"+a.ID, "parentA", ""), 500, "get_failed")
			// Staff never depend on the roster lookup.
			cov1o7Expect(t, c.doWith(router, http.MethodGet, "/announcements", "coach", ""), 200, "")
		})
	}
}

// ─── users.go: link/unlink failures that need a real database ────────────────

type cov1o7DBUsers struct {
	kc      *cov1o7KC
	h       *UserHandlers
	members *members.Service
	router  http.Handler
	pool    *pgxpool.Pool
	org     string
	admin   *appmiddleware.Principal
}

func cov1o7NewDBUsers(t *testing.T) *cov1o7DBUsers {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
	kc := cov1o7NewKC()
	mem := members.NewServiceWithPool(pool)
	h := NewUserHandlersWithMembers(kc, audit.NewService(), mem)
	r := chi.NewRouter()
	r.Use(appmiddleware.Auth(tokenResolver{"admin": admin}), appmiddleware.RequireOrgScope)
	h.RegisterRoutes(r)
	return &cov1o7DBUsers{kc: kc, h: h, members: mem, router: r, pool: pool, org: org, admin: admin}
}

func (d *cov1o7DBUsers) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("X-Org-Id", d.org)
	rec := httptest.NewRecorder()
	d.router.ServeHTTP(rec, req)
	return rec
}

func (d *cov1o7DBUsers) addUser(t *testing.T, email string) string {
	t.Helper()
	id := uuid.NewString()
	d.kc.AddUser(keycloak.User{ID: id, Email: email, Enabled: true, Attributes: map[string][]string{"org_id": {d.org}}})
	cov1o7Exec(t, d.pool, `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1::uuid, $2::uuid, $3, $4)`, id, d.org, email, "sub-"+id)
	return id
}

func (d *cov1o7DBUsers) swimmer(t *testing.T, name string) (members.Household, members.Swimmer) {
	t.Helper()
	hh, err := d.members.CreateHousehold(d.admin, members.CreateHouseholdInput{OrgID: d.org, HouseholdName: name, PrimaryEmail: name + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := d.members.CreateSwimmer(d.admin, d.org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: name, LastName: "T", DateOfBirth: "2000-01-01", Gender: "female", RegistrationStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	return hh, sw
}

// A database that rejects clearing a link must yield a 500, never a false "unlinked".
func TestCov1o7UnlinkReportsDatabaseFailure(t *testing.T) {
	d := cov1o7NewDBUsers(t)
	userID := d.addUser(t, "unlink@example.com")
	hh, sw := d.swimmer(t, "Unl")

	cov1o7Expect(t, d.do(http.MethodPost, "/users/"+userID+"/household/link", `{"household_id":"`+hh.ID+`"}`), 200, "")
	cov1o7Expect(t, d.do(http.MethodPost, "/users/"+userID+"/swimmer/link", `{"swimmer_id":"`+sw.ID+`"}`), 200, "")
	cov1o7Exec(t, d.pool, `
		CREATE FUNCTION cov1o7_block_unlink() RETURNS trigger AS $$
		BEGIN
			IF NEW.keycloak_user_id IS NULL THEN RAISE EXCEPTION 'unlink blocked'; END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER cov1o7_block_unlink BEFORE UPDATE ON households FOR EACH ROW EXECUTE FUNCTION cov1o7_block_unlink();
		CREATE TRIGGER cov1o7_block_unlink BEFORE UPDATE ON swimmers FOR EACH ROW EXECUTE FUNCTION cov1o7_block_unlink();`)
	cov1o7Expect(t, d.do(http.MethodPost, "/users/"+userID+"/household/unlink", `{"household_id":"`+hh.ID+`"}`), 500, "service_error")
	cov1o7Expect(t, d.do(http.MethodPost, "/users/"+userID+"/swimmer/unlink", `{"swimmer_id":"`+sw.ID+`"}`), 500, "service_error")
	if got, ok := d.members.GetHouseholdByKeycloakUserID(userID); !ok || got.ID != hh.ID {
		t.Fatal("a failed unlink must leave the household linked")
	}
	if got, ok := d.members.GetSwimmerByKeycloakUserID(userID); !ok || got.ID != sw.ID {
		t.Fatal("a failed unlink must leave the swimmer linked")
	}
}

func TestCov1o7SwimmerLinkRequestDatabaseFailures(t *testing.T) {
	d := cov1o7NewDBUsers(t)
	userID := d.addUser(t, "lr@example.com")
	_, sw := d.swimmer(t, "Lee")
	req, err := d.members.CreateLinkRequest(d.org, userID, "Lee", "")
	if err != nil {
		t.Fatal(err)
	}
	resolve := "/users/swimmer-link-requests/" + req.ID + "/resolve"

	// The link is made but recording the resolution fails: reported, not swallowed.
	cov1o7Exec(t, d.pool, `
		CREATE FUNCTION cov1o7_block_resolve() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'resolve blocked'; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER cov1o7_block_resolve BEFORE UPDATE ON swimmer_link_requests FOR EACH ROW EXECUTE FUNCTION cov1o7_block_resolve();`)
	cov1o7Expect(t, d.do(http.MethodPost, resolve, `{"swimmer_id":"`+sw.ID+`"}`), 500, "internal_error")

	// With the database gone, every queue operation fails closed.
	d.pool.Close()
	cov1o7Expect(t, d.do(http.MethodPost, "/users/me/swimmer-link-requests", `{}`), 500, "internal_error")
	cov1o7Expect(t, d.do(http.MethodGet, "/users/swimmer-link-requests", ""), 500, "internal_error")
	cov1o7Expect(t, d.do(http.MethodPost, resolve, `{"swimmer_id":"`+sw.ID+`"}`), 500, "internal_error")
}
