package authn

import (
	"errors"
	"testing"
	"time"
)

func TestPurgeUnverifiedMFAFactorsKeepsVerifiedAndOtherTypes(t *testing.T) {
	t.Parallel()

	svc := NewService(Config{EnableDevStubTokens: true})
	if _, err := svc.EnrollMFA(LocalDevAdminUserID, "a@aquify.local", LocalDevOrgID, "totp", "old"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnrollMFA(LocalDevAdminUserID, "a@aquify.local", LocalDevOrgID, "backup_code", "codes"); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeUnverifiedMFAFactors(LocalDevOrgID, LocalDevAdminUserID, "totp"); err != nil {
		t.Fatal(err)
	}
	factors := svc.ListMFAFactors(LocalDevOrgID, LocalDevAdminUserID)
	if len(factors) != 1 || factors[0].FactorType != "backup_code" {
		t.Fatalf("factors after purge = %+v, want only the backup_code factor", factors)
	}
}

func TestRefreshSessionEnforcesAbsoluteLifetime(t *testing.T) {
	t.Parallel()

	svc := NewService(Config{EnableDevStubTokens: true})
	sess, err := svc.IssueSession(SessionInput{UserID: "u-1", OrgID: "org-1", Roles: []string{"parent"}})
	if err != nil {
		t.Fatal(err)
	}
	// Age the chain beyond maxSessionAge.
	svc.mu.Lock()
	rec := svc.byRefresh[sess.RefreshToken]
	rec.Principal.SessionStartedAt = time.Now().Add(-maxSessionAge - time.Hour).Unix()
	svc.mu.Unlock()

	if _, err := svc.RefreshSession(sess.RefreshToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("refresh past absolute lifetime: err = %v, want ErrSessionExpired", err)
	}

	fresh, err := svc.IssueSession(SessionInput{UserID: "u-1", OrgID: "org-1", Roles: []string{"parent"}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := svc.RefreshSession(fresh.RefreshToken)
	if err != nil {
		t.Fatalf("refresh of a fresh session: %v", err)
	}
	if p, ok := svc.ResolvePrincipal(next.AccessToken); !ok || p.SessionStartedAt == 0 {
		t.Fatalf("rotated session must carry SessionStartedAt: %+v", p)
	}
}
