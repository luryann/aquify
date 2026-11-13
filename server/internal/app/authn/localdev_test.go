package authn

import (
	"context"
	"testing"
)

func TestStubPlatformOperatorLivesInThePlatformOrgOnly(t *testing.T) {
	t.Parallel()
	p, ok := localDevStaticPrincipals()["stub-platform"]
	if !ok {
		t.Fatal("stub-platform principal is missing")
	}
	if p.OrgID != LocalDevPlatformOrgID || len(p.Roles) != 1 || p.Roles[0] != "platform_admin" || !p.MFAAuthenticated {
		t.Fatalf("unexpected operator principal: %+v", p)
	}
	// It must never be seeded into the demo club's user table.
	for _, u := range LocalDevUsers() {
		if u.Token == "stub-platform" || u.UserID == LocalDevPlatformUserID {
			t.Fatal("the operator stub must not be part of LocalDevUsers()")
		}
	}
}

func TestResetMFARemovesEveryFactorAndVerifiedUsersReflectsIt(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{EnableDevStubTokens: true})
	ctx := context.Background()
	const org, other = "org-a", "org-b"

	if _, err := svc.EnrollMFA("u1", "u1@example.com", org, "totp", "phone"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA("u1", "u1@example.com", org, "backup_code", "codes"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA("u2", "u2@example.com", other, "totp", "phone"); err != nil {
		t.Fatal(err)
	}
	// Enrolled but never verified does not count as MFA-enrolled.
	if got, err := svc.VerifiedMFAUsers(ctx, org); err != nil || got["u1"] {
		t.Fatalf("unverified factors must not count: %v %v", got, err)
	}

	n, err := svc.ResetMFA(ctx, org, "u1")
	if err != nil || n != 2 {
		t.Fatalf("reset: removed=%d err=%v", n, err)
	}
	if left := svc.ListMFAFactors(org, "u1"); len(left) != 0 {
		t.Fatalf("factors remain after reset: %v", left)
	}
	if left := svc.ListMFAFactors(other, "u2"); len(left) != 1 {
		t.Fatalf("another user's factors must be untouched: %v", left)
	}
	if _, err := svc.ResetMFA(ctx, org, " "); err == nil {
		t.Fatal("a blank user id must be refused")
	}
}
