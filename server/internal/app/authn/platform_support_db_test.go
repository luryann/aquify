package authn

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

// Operator support against real tables under the application's RLS role: MFA reset
// deletes every factor of that user in that club only, and the enrolled-users list
// counts verified factors only.
func TestResetMFAAndVerifiedUsersUnderRowLevelSecurity(t *testing.T) {
	admin, appPool := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	svc := NewService(Config{EnableDevStubTokens: true})
	svc.SetPool(appPool)
	u1, u2, other := uuid.NewString(), uuid.NewString(), uuid.NewString()

	f1, err := svc.EnrollMFA(u1, "u1@example.com", orgA, "totp", "phone")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA(u1, "u1@example.com", orgA, "backup_code", "codes"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA(u2, "u2@example.com", orgA, "totp", "phone"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA(other, "o@example.com", orgB, "totp", "phone"); err != nil {
		t.Fatal(err)
	}
	// Only a verified factor counts as enrolled.
	if _, err := admin.Exec(ctx, `UPDATE mfa_factors SET is_verified = true WHERE id::text = $1`, f1.ID); err != nil {
		t.Fatal(err)
	}
	got, err := svc.VerifiedMFAUsers(ctx, orgA)
	if err != nil || !got[u1] || got[u2] || got[other] {
		t.Fatalf("verified users for club A: %v %v", got, err)
	}

	// Another club cannot reset this club's user.
	if n, err := svc.ResetMFA(ctx, orgB, u1); err != nil || n != 0 {
		t.Fatalf("club B must not reset club A's user: %d %v", n, err)
	}
	if n, err := svc.ResetMFA(ctx, orgA, u1); err != nil || n != 2 {
		t.Fatalf("reset removes every factor of the user: %d %v", n, err)
	}
	restarted := NewService(Config{EnableDevStubTokens: true})
	restarted.SetPool(appPool)
	if left := restarted.ListMFAFactors(orgA, u1); len(left) != 0 {
		t.Fatalf("factors remain after reset: %+v", left)
	}
	if left := restarted.ListMFAFactors(orgA, u2); len(left) != 1 {
		t.Fatalf("a different user's factor must be untouched: %+v", left)
	}
	if got, _ := svc.VerifiedMFAUsers(ctx, orgA); got[u1] {
		t.Fatal("a reset user is no longer enrolled")
	}
}
