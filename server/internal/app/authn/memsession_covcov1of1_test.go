package authn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func cov1xMemSession(t *testing.T, svc *Service, in SessionInput) SessionTokens {
	t.Helper()
	if in.UserID == "" {
		in.UserID = "cov1x-user"
	}
	if in.OrgID == "" {
		in.OrgID = "cov1x-org"
	}
	if len(in.Roles) == 0 {
		in.Roles = []string{"parent"}
	}
	tok, err := svc.IssueSession(in)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	return tok
}

func TestCov1xIssueSessionValidatesInput(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	if _, err := svc.IssueSession(SessionInput{UserID: "  ", Roles: []string{"admin"}}); err == nil || err.Error() != "user_id is required" {
		t.Fatalf("blank user: %v", err)
	}
	if _, err := svc.IssueSession(SessionInput{UserID: "u"}); err == nil || err.Error() != "roles are required" {
		t.Fatalf("no roles: %v", err)
	}
}

func TestCov1xIssueSessionRecordDefaultsNonPositiveLifetime(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	before := time.Now().UTC()
	tok, err := svc.issueSessionRecord(context.Background(), appmiddleware.Principal{UserID: "u", OrgID: "o", Roles: []string{"parent"}}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	lifetime := tok.ExpiresAt.Sub(before)
	if lifetime < 24*time.Hour-time.Minute || lifetime > 24*time.Hour+time.Minute {
		t.Fatalf("a zero lifetime defaults to 24h, got %v", lifetime)
	}
	if _, ok := svc.ResolvePrincipal(tok.AccessToken); !ok {
		t.Fatal("the session must be usable")
	}
}

func TestCov1xRefreshSessionInMemoryRejections(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	if _, err := svc.RefreshSession("   "); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("blank token: %v", err)
	}
	if _, err := svc.RefreshSession("rtk_unknown"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown token: %v", err)
	}

	tok := cov1xMemSession(t, svc, SessionInput{})
	svc.mu.Lock()
	svc.byRefresh[tok.RefreshToken].ExpiresAt = time.Now().UTC().Add(-time.Second)
	svc.mu.Unlock()
	if _, err := svc.RefreshSession(tok.RefreshToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired session must not refresh: %v", err)
	}
	if _, ok := svc.ResolvePrincipal(tok.AccessToken); ok {
		t.Fatal("an expired access token must not resolve")
	}
}

func TestCov1xRevokeSessionInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	if err := svc.RevokeSession("  "); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("blank token: %v", err)
	}
	if err := svc.RevokeSession("rtk_missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown token: %v", err)
	}

	tok := cov1xMemSession(t, svc, SessionInput{})
	if err := svc.RevokeSession(" " + tok.RefreshToken + " "); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, ok := svc.ResolvePrincipal(tok.AccessToken); ok {
		t.Fatal("a revoked session must stop resolving")
	}
	if _, err := svc.RefreshSession(tok.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("a revoked session must not refresh: %v", err)
	}

	svc.mu.RLock()
	firstRevoke := *svc.byRefresh[tok.RefreshToken].RevokedAt
	svc.mu.RUnlock()
	if err := svc.RevokeSession(tok.RefreshToken); err != nil {
		t.Fatalf("revoking twice is idempotent: %v", err)
	}
	svc.mu.RLock()
	secondRevoke := *svc.byRefresh[tok.RefreshToken].RevokedAt
	svc.mu.RUnlock()
	if !firstRevoke.Equal(secondRevoke) {
		t.Fatal("a repeat revoke must not move the original revocation time")
	}
}

func TestCov1xRevokeUserSessionsRequiresUser(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	if n, err := svc.RevokeUserSessions(context.Background(), "o", "  "); err == nil || n != 0 {
		t.Fatalf("a blank user id must be refused: %d %v", n, err)
	}
}

func TestCov1xLogoutSessionWithAccessInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{OIDC: cov1xOIDCConfig()})

	if res := svc.LogoutSessionWithAccess(" ", " "); res.RedirectURL != "/login" || res.LogoutURL != "" || res.RevokeErr != nil {
		t.Fatalf("nothing to log out: %+v", res)
	}

	tok := cov1xMemSession(t, svc, SessionInput{OIDCIDToken: "id-token-1"})
	res := svc.LogoutSessionWithAccess("", tok.AccessToken)
	if !strings.Contains(res.LogoutURL, "id_token_hint=id-token-1") {
		t.Fatalf("logout by access token returns the provider logout URL: %+v", res)
	}
	if _, ok := svc.ResolvePrincipal(tok.AccessToken); ok {
		t.Fatal("logout must revoke the session")
	}
	// A second logout with the same access token is harmless and still redirects.
	if again := svc.LogoutSessionWithAccess("", tok.AccessToken); again.RedirectURL != "/login" {
		t.Fatalf("repeat logout: %+v", again)
	}
	// An unknown access token yields no provider URL.
	if res := svc.LogoutSessionWithAccess("", "atk_unknown"); res.LogoutURL != "" || res.RedirectURL != "/login" {
		t.Fatalf("unknown access token: %+v", res)
	}
	// An already revoked refresh session still reports the id token for the provider.
	tok2 := cov1xMemSession(t, svc, SessionInput{OIDCIDToken: "id-token-2"})
	if err := svc.RevokeSession(tok2.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if res := svc.LogoutSessionWithAccess(tok2.RefreshToken, ""); !strings.Contains(res.LogoutURL, "id-token-2") {
		t.Fatalf("revoked refresh session logout: %+v", res)
	}
}

func TestCov1xResolvePrincipalErrInMemory(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{EnableDevStubTokens: true})
	if p, err := svc.ResolvePrincipalErr("   "); p != nil || err != nil {
		t.Fatalf("blank token: %v %v", p, err)
	}
	if p, ok := svc.ResolvePrincipal("atk_unknown"); p != nil || ok {
		t.Fatalf("unknown token: %v %v", p, ok)
	}

	// The stub swimmer resolves its linked swimmer the same way a real login does.
	svc.SetSwimmerLookup(func(userID string) string {
		if userID == LocalDevSwimmerUserID {
			return "swimmer-from-lookup"
		}
		return ""
	})
	p, ok := svc.ResolvePrincipal("stub-swimmer")
	if !ok || p.SwimmerID != "swimmer-from-lookup" || p.HouseholdID != LocalDevHouseholdID {
		t.Fatalf("stub swimmer principal: %+v %v", p, ok)
	}
	// Non-swimmer stubs are not looked up.
	if p, _ := svc.ResolvePrincipal("stub-parent"); p.SwimmerID != "" {
		t.Fatalf("a parent has no swimmer link: %+v", p)
	}
	// Mutating the returned principal must not corrupt the stub table.
	p.Roles[0] = "admin"
	if again, _ := svc.ResolvePrincipal("stub-swimmer"); again.Roles[0] != "swimmer" {
		t.Fatalf("stub principals must be returned as copies: %+v", again)
	}
}

func TestCov1xChallengeMFAInputAndFactorValidation(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	principal := &appmiddleware.Principal{UserID: "u1", OrgID: "o1", Roles: []string{"admin"}}

	if _, err := svc.ChallengeMFA("", nil, "f", "123456"); !errors.Is(err, ErrInvalidMFAFactor) {
		t.Fatalf("nil principal: %v", err)
	}
	if _, err := svc.ChallengeMFA("", principal, "mfa_unknown", "123456"); !errors.Is(err, ErrInvalidMFAFactor) {
		t.Fatalf("unknown factor: %v", err)
	}

	// A factor of a type this build cannot challenge is refused, not silently passed.
	svc.mu.Lock()
	svc.mfaFactorsByUser["u1"] = []mfaFactorRecord{{Factor: MFAFactor{ID: "mfa_sms", UserID: "u1", OrgID: "o1", FactorType: "sms"}}}
	svc.mu.Unlock()
	if _, err := svc.ChallengeMFA("", principal, "mfa_sms", "123456"); !errors.Is(err, ErrUnsupportedMFAFactor) {
		t.Fatalf("unsupported factor type: %v", err)
	}
}

func TestCov1xChallengeMFAExpiredLockoutAndReplayWindowsReset(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{EnableDevStubTokens: true})
	principal := &appmiddleware.Principal{UserID: "u2", OrgID: "o2", Roles: []string{"admin"}}
	factor, err := svc.EnrollMFA("u2", "u2@example.com", "o2", "totp", "phone")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < mfaMaxFailures; i++ {
		if _, err := svc.ChallengeMFA("", principal, factor.ID, "000000"); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	code := generateTOTP(factor.ChallengeHint, time.Now().UTC().Unix())
	if _, err := svc.ChallengeMFA("", principal, factor.ID, code); !errors.Is(err, ErrMFATooManyAttempts) {
		t.Fatalf("locked: %v", err)
	}

	// Age the failure window and an old replay entry past their TTLs.
	now := time.Now().UTC()
	svc.mu.Lock()
	svc.mfaFailures[factor.ID] = mfaAttempts{count: mfaMaxFailures, start: now.Add(-mfaLockoutWindow - time.Second)}
	svc.usedTOTPCodes["stale-factor|111111"] = now.Add(-5 * time.Minute)
	svc.mu.Unlock()

	tok, err := svc.ChallengeMFA("", principal, factor.ID, code)
	if err != nil {
		t.Fatalf("an expired lockout window must let a valid code through: %v", err)
	}
	if p, ok := svc.ResolvePrincipal(tok.AccessToken); !ok || !p.MFAAuthenticated {
		t.Fatalf("challenge issues an MFA-elevated session: %+v %v", p, ok)
	}
	svc.mu.RLock()
	_, staleKept := svc.usedTOTPCodes["stale-factor|111111"]
	_, replayRecorded := svc.usedTOTPCodes[factor.ID+"|"+code]
	_, failuresKept := svc.mfaFailures[factor.ID]
	svc.mu.RUnlock()
	if staleKept {
		t.Error("expired replay entries must be purged")
	}
	if !replayRecorded {
		t.Error("the accepted code must be remembered to block replay")
	}
	if failuresKept {
		t.Error("success clears the failure counter")
	}
	if _, err := svc.ChallengeMFA("", principal, factor.ID, code); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replaying the accepted code must fail: %v", err)
	}
}

func TestCov1xChallengeMFAElevatedSessionCarriesIDTokenAndRetiresOldSession(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{EnableDevStubTokens: true, OIDC: cov1xOIDCConfig()})
	base := cov1xMemSession(t, svc, SessionInput{UserID: "u3", OrgID: "o3", Roles: []string{"admin"}, OIDCIDToken: "id-token-mfa"})
	factor, err := svc.EnrollMFA("u3", "u3@example.com", "o3", "totp", "phone")
	if err != nil {
		t.Fatal(err)
	}
	principal := &appmiddleware.Principal{UserID: "u3", OrgID: "o3", Roles: []string{"admin"}}
	code := generateTOTP(factor.ChallengeHint, time.Now().UTC().Unix())

	elevated, err := svc.ChallengeMFA(base.AccessToken, principal, factor.ID, code)
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if _, ok := svc.ResolvePrincipal(base.AccessToken); ok {
		t.Fatal("the pre-MFA access token must be retired")
	}
	if res := svc.LogoutSessionWithAccess("", elevated.AccessToken); !strings.Contains(res.LogoutURL, "id-token-mfa") {
		t.Fatalf("the elevated session keeps the provider id token: %+v", res)
	}
	factors := svc.ListMFAFactors("o3", "u3")
	if len(factors) != 1 || !factors[0].IsVerified || factors[0].LastChallengedAt == nil {
		t.Fatalf("the factor is marked verified: %+v", factors)
	}
}
