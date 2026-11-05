package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// the whole sign-in lifecycle works through the application's
// non-superuser role, so every statement carries the binding it needs, and the database
// itself refuses an unbound or cross-club read.
func TestSignInLifecycleUnderRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := NewService(Config{EnableDevStubTokens: true})
	svc.SetPool(appPool)
	user := uuid.NewString()
	in := func(org string) SessionInput {
		return SessionInput{UserID: user, Email: "u@example.com", Name: "U", OrgID: org, Roles: []string{"admin"}}
	}

	tok, err := svc.IssueSession(in(orgA))
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if _, err := svc.IssueSession(in(orgB)); !errors.Is(err, ErrUserOrgMismatch) {
		t.Fatalf("a sign-in claiming another club must be refused, got %v", err)
	}
	// The session is found by its token before any club is known.
	if p, ok := svc.ResolvePrincipal(tok.AccessToken); !ok || p.OrgID != orgA {
		t.Fatalf("the access token resolves to its club: %+v %v", p, ok)
	}
	// Rotation reads the old session by refresh hash and writes the new one under the club.
	next, err := svc.RefreshSession(tok.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := svc.RefreshSession(tok.RefreshToken); err == nil {
		t.Fatal("a rotated refresh token must not work twice")
	}
	if p, ok := svc.ResolvePrincipal(next.AccessToken); !ok || p.OrgID != orgA {
		t.Fatalf("the rotated token resolves: %+v %v", p, ok)
	}
	// Another club's revoke-by-user touches nothing; the owning club's does.
	if n, err := svc.RevokeUserSessions(ctx, orgB, user); err != nil || n != 0 {
		t.Fatalf("club B must not revoke club A's sessions: %d %v", n, err)
	}
	if n, err := svc.RevokeUserSessions(ctx, orgA, user); err != nil || n < 1 {
		t.Fatalf("club A revokes its user's sessions: %d %v", n, err)
	}
	if _, ok := svc.ResolvePrincipal(next.AccessToken); ok {
		t.Fatal("a revoked session must not resolve")
	}

	// MFA: enrol, restart (all-club load), and read it back per club.
	f, err := svc.EnrollMFA(user, "u@example.com", orgA, "totp", "phone")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	restarted := NewService(Config{EnableDevStubTokens: true})
	restarted.SetPool(appPool)
	if got := restarted.ListMFAFactors(orgA, user); len(got) != 1 || got[0].ID != f.ID {
		t.Fatalf("the factor survives a restart for its club: %+v", got)
	}
	if got := restarted.ListMFAFactors(orgB, user); len(got) != 0 {
		t.Fatalf("club B must not be served club A's factor: %+v", got)
	}

	for _, table := range []string{"users", "sessions", "mfa_factors"} {
		var n int
		if err := appPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE org_id IS NOT NULL`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound session must see nothing in %s: %d %v", table, n, err)
		}
	}
}
