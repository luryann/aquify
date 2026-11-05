package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/keycloak"
)

// Tenancy tracker items 3.2 / 3.6 end to end: an org admin issues an invite, a
// visitor registers through the club slug with it, the invite's role wins, a
// code from another club or a spent code is refused, and only admins manage invites.
func TestInviteDrivenSelfRegistration(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	mk := func(slug string) *orgs.Org {
		o, _ := reg.Create(ctx, orgs.CreateInput{Slug: slug, Name: slug})
		_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
		return o
	}
	a, b := mk("club-a"), mk("club-b")
	kc := keycloak.NewStubClient()
	router := NewRouterWithServices(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: reg, Keycloak: kc})
	adminA := issueSession(t, router, sessionSpec{UserID: "adm-a", OrgID: a.ID, Roles: []string{"admin"}, MFA: true})
	adminB := issueSession(t, router, sessionSpec{UserID: "adm-b", OrgID: b.ID, Roles: []string{"admin"}, MFA: true})
	parentA := issueSession(t, router, sessionSpec{UserID: "par-a", OrgID: a.ID, Roles: []string{"parent"}, MFA: false})

	if code, _ := platformRequest(t, router, parentA, a.ID, "POST", "/api/v1/invites", `{"role":"parent"}`); code != http.StatusForbidden {
		t.Fatalf("a parent must not create invites: %d", code)
	}
	if code, _ := platformRequest(t, router, adminA, a.ID, "POST", "/api/v1/invites", `{"role":"admin"}`); code != http.StatusBadRequest {
		t.Fatalf("admin invites are impossible: %d", code)
	}
	code, res := platformRequest(t, router, adminA, a.ID, "POST", "/api/v1/invites", `{"role":"coach","max_uses":1,"label":"new coach"}`)
	if code != http.StatusCreated {
		t.Fatalf("create invite: %d %v", code, res)
	}
	inviteCode := res["code"].(string)
	inviteID := res["invite"].(map[string]any)["id"].(string)

	reqBody := func(email, slug, code string) string {
		return `{"first_name":"N","last_name":"M","email":"` + email + `","club_slug":"` + slug + `","club_code":"` + code + `","requested_role":"parent"}`
	}
	post := func(body string) (int, map[string]any) {
		return platformRequest(t, router, "", "", "POST", "/api/v1/register", body)
	}
	if c, _ := post(reqBody("x1@example.com", "club-b", inviteCode)); c != http.StatusBadRequest {
		t.Fatalf("club A's code must not work for club B: %d", c)
	}
	c, out := post(reqBody("x2@example.com", "club-a", inviteCode))
	if c != http.StatusCreated {
		t.Fatalf("valid invite: %d %v", c, out)
	}
	u, err := kc.GetUser(ctx, out["user_id"].(string))
	if err != nil || u.Attributes["requested_role"][0] != "coach" || u.Attributes["org_id"][0] != a.ID || u.Attributes["registration_invite_id"][0] != inviteID {
		t.Fatalf("invite must set role/org/invite id: %+v %v", u, err)
	}
	if c, _ := post(reqBody("x3@example.com", "club-a", inviteCode)); c != http.StatusBadRequest {
		t.Fatalf("a spent code must be refused: %d", c)
	}
	if c, _ := post(reqBody("x4@example.com", "club-a", "")); c != http.StatusCreated {
		t.Fatalf("registering without a code stays possible (admin approval queue): %d", c)
	}
	if c, _ := platformRequest(t, router, adminB, b.ID, "DELETE", "/api/v1/invites/"+inviteID, ""); c != http.StatusNotFound {
		t.Fatalf("another org's admin must not revoke it: %d", c)
	}
	if c, _ := platformRequest(t, router, adminA, a.ID, "DELETE", "/api/v1/invites/"+inviteID, ""); c != http.StatusNoContent {
		t.Fatalf("revoke: %d", c)
	}
}

func TestRegisterIsRateLimitedPerEmail(t *testing.T) {
	ctx := context.Background()
	reg := orgs.NewService()
	o, _ := reg.Create(ctx, orgs.CreateInput{Slug: "club-a", Name: "A"})
	_, _ = reg.SetStatus(ctx, o.ID, orgs.StatusActive)
	router := NewRouterWithServices(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), RouterDeps{Orgs: reg, Keycloak: keycloak.NewStubClient()})
	body := `{"first_name":"N","last_name":"M","email":"same@example.com","club_slug":"club-a"}`
	codes := []int{}
	for i := 0; i < 5; i++ {
		c, _ := platformRequest(t, router, "", "", "POST", "/api/v1/register", body)
		codes = append(codes, c)
	}
	// 1st creates (201), 2nd is a duplicate (409) -- both count -- then 3rd, then throttled.
	if codes[0] != http.StatusCreated || codes[3] != http.StatusTooManyRequests || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("attempt statuses: %v", codes)
	}
}
