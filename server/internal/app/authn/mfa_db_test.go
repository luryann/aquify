package authn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	mfaTestOrg  = "10000000-0000-0000-0000-0000000000a1"
	mfaTestUser = "20000000-0000-0000-0000-0000000000a1"
)

func openAuthnTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping authn DB tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("DATABASE_URL is set but the pool cannot be opened: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("DATABASE_URL is set but the database is unreachable: %v", err)
	}
	cleanup := func(report func(format string, args ...any)) {
		for _, q := range []string{
			"DELETE FROM sessions WHERE org_id = $1::uuid",
			"DELETE FROM mfa_factors WHERE org_id = $1",
			"DELETE FROM users WHERE org_id = $1::uuid",
		} {
			if _, err := pool.Exec(context.Background(), q, mfaTestOrg); err != nil {
				report("fixture cleanup failed (%s): %v", q, err)
				return
			}
		}
	}
	cleanup(t.Fatalf)
	t.Cleanup(func() { cleanup(t.Errorf); pool.Close() })
	return pool
}

func newMFAInstance(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	svc := NewService(Config{EnableDevStubTokens: true})
	svc.SetPool(pool)
	return svc
}

func mfaPrincipal() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: mfaTestUser, OrgID: mfaTestOrg, Roles: []string{"admin"}}
}

func seedMFAUser(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.IssueSession(SessionInput{UserID: mfaTestUser, Email: "mfa-a1@example.com", Name: "MFA", OrgID: mfaTestOrg, Roles: []string{"admin"}}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// A TOTP code accepted by one instance must be rejected by another instance
// that loaded the factor before the first challenge (and after restart).
func TestChallengeMFATOTPReplayRejectedAcrossInstances(t *testing.T) {
	pool := openAuthnTestPool(t)
	svc1 := newMFAInstance(t, pool)
	seedMFAUser(t, svc1)
	factor, err := svc1.EnrollMFA(mfaTestUser, "mfa-a1@example.com", mfaTestOrg, "totp", "t")
	if err != nil {
		t.Fatal(err)
	}
	svc2 := newMFAInstance(t, pool) // loads the factor before svc1 consumes the code

	code := generateTOTP(factor.ChallengeHint, time.Now().UTC().Unix())
	if _, err := svc1.ChallengeMFA("", mfaPrincipal(), factor.ID, code); err != nil {
		t.Fatalf("first use should pass: %v", err)
	}
	if _, err := svc2.ChallengeMFA("", mfaPrincipal(), factor.ID, code); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replay on second instance must be rejected, got %v", err)
	}
	svc3 := newMFAInstance(t, pool) // "restart"
	if _, err := svc3.ChallengeMFA("", mfaPrincipal(), factor.ID, code); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replay after restart must be rejected, got %v", err)
	}
}

func TestChallengeMFABackupCodeReplayRejectedAcrossInstances(t *testing.T) {
	pool := openAuthnTestPool(t)
	svc1 := newMFAInstance(t, pool)
	seedMFAUser(t, svc1)
	factor, err := svc1.EnrollMFA(mfaTestUser, "mfa-a1@example.com", mfaTestOrg, "backup_code", "b")
	if err != nil || len(factor.Codes) == 0 {
		t.Fatalf("enroll: %v", err)
	}
	svc2 := newMFAInstance(t, pool)

	if _, err := svc1.ChallengeMFA("", mfaPrincipal(), factor.ID, factor.Codes[0]); err != nil {
		t.Fatalf("first use should pass: %v", err)
	}
	if _, err := svc2.ChallengeMFA("", mfaPrincipal(), factor.ID, factor.Codes[0]); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replayed backup code must be rejected, got %v", err)
	}
	// A different, unused code still works on the second instance.
	if _, err := svc2.ChallengeMFA("", mfaPrincipal(), factor.ID, factor.Codes[1]); err != nil {
		t.Fatalf("unused backup code should pass: %v", err)
	}
	// Post-challenge bookkeeping must not resurrect spent codes.
	time.Sleep(200 * time.Millisecond)
	svc3 := newMFAInstance(t, pool)
	for _, c := range factor.Codes[:2] {
		if _, err := svc3.ChallengeMFA("", mfaPrincipal(), factor.ID, c); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("spent backup code %q must stay spent, got %v", c, err)
		}
	}
}

func TestRefreshDoesNotExtendMFAElevatedSession(t *testing.T) {
	for _, withDB := range []bool{false, true} {
		var svc *Service
		if withDB {
			svc = newMFAInstance(t, openAuthnTestPool(t))
		} else {
			svc = NewService(Config{EnableDevStubTokens: true})
		}
		first, err := svc.IssueSession(SessionInput{UserID: mfaTestUser, Email: "mfa-a1@example.com", Name: "MFA", OrgID: mfaTestOrg, Roles: []string{"admin"}, MFAAuthenticated: true, ExpiresIn: 15 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		next, err := svc.RefreshSession(first.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		if next.ExpiresAt.After(first.ExpiresAt.Add(time.Second)) {
			t.Fatalf("db=%v: refresh extended elevated session from %v to %v", withDB, first.ExpiresAt, next.ExpiresAt)
		}
		plain, _ := svc.IssueSession(SessionInput{UserID: mfaTestUser, Email: "mfa-a1@example.com", Name: "MFA", OrgID: mfaTestOrg, Roles: []string{"admin"}, ExpiresIn: time.Hour})
		pn, err := svc.RefreshSession(plain.RefreshToken)
		if err != nil || !pn.ExpiresAt.After(plain.ExpiresAt.Add(time.Hour)) {
			t.Fatalf("db=%v: non-elevated refresh should still extend to ~24h: %v %v", withDB, pn.ExpiresAt, err)
		}
	}
}

func TestUserSyncNeverOverwritesRealEmailWithSurrogate(t *testing.T) {
	pool := openAuthnTestPool(t)
	svc := newMFAInstance(t, pool)
	const otherUser = "20000000-0000-0000-0000-0000000000a2"
	ctx := context.Background()
	if err := svc.upsertAuthenticatedUser(ctx, mfaTestUser, "real-a1@example.com", "A", mfaTestOrg, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.upsertAuthenticatedUser(ctx, otherUser, "other-a2@example.com", "B", mfaTestOrg, false); err != nil {
		t.Fatal(err)
	}
	// Identity conflict: user A now arrives claiming user B's email.
	if err := svc.upsertAuthenticatedUser(ctx, mfaTestUser, "other-a2@example.com", "A", mfaTestOrg, true); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1::uuid`, mfaTestUser).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "real-a1@example.com" {
		t.Fatalf("real email was overwritten: %q", email)
	}
	// A brand new conflicting user still gets a row (surrogate is only for inserts).
	const third = "20000000-0000-0000-0000-0000000000a3"
	if err := svc.upsertAuthenticatedUser(ctx, third, "other-a2@example.com", "C", mfaTestOrg, false); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1::uuid`, third).Scan(&email); err != nil || email != surrogateSyncedUserEmail(third) {
		t.Fatalf("new row should get surrogate, got %q err=%v", email, err)
	}
}
