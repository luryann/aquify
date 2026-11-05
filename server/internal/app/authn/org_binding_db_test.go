package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Tenancy tracker items 2.1b/2.3: a user is bound to one org; a token claiming a
// different org, or an org that is unregistered/suspended, cannot sign in.
func TestSignInIsBoundToTheUsersOrgAndOrgStatus(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	orgA, orgB, orgGhost := uuid.NewString(), uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, pool, orgA, orgB)
	svc := NewService(Config{EnableDevStubTokens: true})
	svc.SetPool(pool)
	user := uuid.NewString()
	in := func(org string) SessionInput {
		return SessionInput{UserID: user, Email: "u@example.com", Name: "U", OrgID: org, Roles: []string{"admin"}}
	}

	if _, err := svc.IssueSession(in(orgA)); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if _, err := svc.IssueSession(in(orgB)); !errors.Is(err, ErrUserOrgMismatch) {
		t.Fatalf("sign-in claiming another org must be rejected, got %v", err)
	}
	var org string
	if err := pool.QueryRow(context.Background(), `SELECT org_id::text FROM users WHERE id = $1::uuid`, user).Scan(&org); err != nil || org != orgA {
		t.Fatalf("user was re-homed: org=%q err=%v", org, err)
	}

	if _, err := svc.IssueSession(SessionInput{UserID: uuid.NewString(), Email: "g@example.com", Name: "G", OrgID: orgGhost, Roles: []string{"admin"}}); !errors.Is(err, ErrOrgNotActive) {
		t.Fatalf("unregistered org must be rejected, got %v", err)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE orgs SET status = 'suspended' WHERE id = $1::uuid`, orgA); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IssueSession(in(orgA)); !errors.Is(err, ErrOrgNotActive) {
		t.Fatalf("suspended org must be rejected, got %v", err)
	}
}
