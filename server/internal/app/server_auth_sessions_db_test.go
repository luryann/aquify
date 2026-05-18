package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDBRepoAuthSessions_CrossInstanceRefreshRotationAndResolve(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	const (
		orgID  = "10000000-0000-0000-0000-0000000000f1"
		userID = "20000000-0000-0000-0000-0000000000f1"
		email  = "step32f-cross-instance@example.com"
	)

	cleanupAuthFixtures(t, pool, orgID)

	svc1 := authn.NewService(authn.Config{})
	svc1.SetPool(pool)
	svc2 := authn.NewService(authn.Config{})
	svc2.SetPool(pool)

	session, err := svc1.IssueSession(authn.SessionInput{
		UserID:    userID,
		Email:     email,
		Name:      "Step 32F Cross Instance",
		OrgID:     orgID,
		Roles:     []string{"admin"},
		ExpiresIn: 6 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	principal, ok := svc2.ResolvePrincipal(session.AccessToken)
	if !ok || principal == nil {
		t.Fatal("expected second instance to resolve persisted access token")
	}
	if principal.UserID != userID {
		t.Fatalf("expected user %q, got %q", userID, principal.UserID)
	}

	refreshed, err := svc2.RefreshSession(session.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshSession from second instance: %v", err)
	}
	if refreshed.AccessToken == session.AccessToken || refreshed.RefreshToken == session.RefreshToken {
		t.Fatal("expected refresh rotation to replace both access and refresh tokens")
	}

	if _, ok := svc1.ResolvePrincipal(session.AccessToken); ok {
		t.Fatal("expected rotated access token to be invalid across instances")
	}
	if _, ok := svc1.ResolvePrincipal(refreshed.AccessToken); !ok {
		t.Fatal("expected refreshed access token to resolve on another instance")
	}
	if _, err := svc1.RefreshSession(session.RefreshToken); !errors.Is(err, authn.ErrSessionRevoked) {
		t.Fatalf("expected old refresh token to be revoked, got %v", err)
	}
}

func TestDBRepoAuthSessions_RevokeSurvivesRestart(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	const (
		orgID  = "10000000-0000-0000-0000-0000000000f2"
		userID = "20000000-0000-0000-0000-0000000000f2"
		email  = "step32f-revoke@example.com"
	)

	cleanupAuthFixtures(t, pool, orgID)

	issuer := authn.NewService(authn.Config{})
	issuer.SetPool(pool)

	session, err := issuer.IssueSession(authn.SessionInput{
		UserID:    userID,
		Email:     email,
		Name:      "Step 32F Revoke",
		OrgID:     orgID,
		Roles:     []string{"admin"},
		ExpiresIn: 6 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	revoker := authn.NewService(authn.Config{})
	revoker.SetPool(pool)
	if err := revoker.RevokeSession(session.RefreshToken); err != nil {
		t.Fatalf("RevokeSession from second instance: %v", err)
	}

	restarted := authn.NewService(authn.Config{})
	restarted.SetPool(pool)
	if _, ok := restarted.ResolvePrincipal(session.AccessToken); ok {
		t.Fatal("expected revoked access token to stay invalid after restart")
	}
	if _, err := restarted.RefreshSession(session.RefreshToken); !errors.Is(err, authn.ErrSessionRevoked) {
		t.Fatalf("expected revoked refresh token after restart, got %v", err)
	}
}

func TestDBRepoAuthSessions_ExpiryValidatedFromPersistentStore(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	const (
		orgID  = "10000000-0000-0000-0000-0000000000f3"
		userID = "20000000-0000-0000-0000-0000000000f3"
		email  = "step32f-expiry@example.com"
	)

	cleanupAuthFixtures(t, pool, orgID)

	issuer := authn.NewService(authn.Config{})
	issuer.SetPool(pool)

	session, err := issuer.IssueSession(authn.SessionInput{
		UserID:    userID,
		Email:     email,
		Name:      "Step 32F Expiry",
		OrgID:     orgID,
		Roles:     []string{"admin"},
		ExpiresIn: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	if _, err := pool.Exec(context.Background(), `
		UPDATE sessions
		SET expires_at = now() - interval '1 minute'
		WHERE access_token_hash = $1`,
		hashTokenForTest(session.AccessToken),
	); err != nil {
		t.Fatalf("expire session row: %v", err)
	}

	restarted := authn.NewService(authn.Config{})
	restarted.SetPool(pool)
	if _, ok := restarted.ResolvePrincipal(session.AccessToken); ok {
		t.Fatal("expected expired access token to be rejected from persistent store")
	}
	if _, err := restarted.RefreshSession(session.RefreshToken); !errors.Is(err, authn.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired on refresh, got %v", err)
	}
}

func cleanupAuthFixtures(t *testing.T, pool *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM sessions WHERE org_id = $1::uuid",
		"DELETE FROM mfa_factors WHERE org_id = $1::uuid",
		"DELETE FROM users WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

func hashTokenForTest(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
