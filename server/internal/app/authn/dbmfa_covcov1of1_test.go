package authn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xFailSealer cannot seal anything; it opens values the way fakeSealer does.
type cov1xFailSealer struct{ fakeSealer }

func (cov1xFailSealer) EncryptFor(string, string) (string, error) {
	return "", errors.New("cov1x: key unavailable")
}

type cov1xMFAEnv struct {
	pool      *pgxpool.Pool
	svc       *Service
	org, user string
	principal *appmiddleware.Principal
	logs      *cov1xLogBuffer
}

func cov1xNewMFAEnv(t *testing.T) *cov1xMFAEnv {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	logger, logs := cov1xLogger()
	env := &cov1xMFAEnv{pool: pool, org: cov1xOrg(t, pool), user: uuid.NewString(), logs: logs}
	env.svc = cov1xServiceOn(pool, Config{EnableDevStubTokens: true, Logger: logger})
	cov1xSignIn(t, env.svc, env.org, env.user, "")
	env.principal = &appmiddleware.Principal{UserID: env.user, OrgID: env.org, Roles: []string{"admin"}}
	return env
}

func (e *cov1xMFAEnv) enroll(t *testing.T, factorType string) MFAFactor {
	t.Helper()
	f, err := e.svc.EnrollMFA(e.user, "mfa@example.com", e.org, factorType, "cov1x")
	if err != nil {
		t.Fatalf("enroll %s: %v", factorType, err)
	}
	return f
}

func cov1xTOTPNow(f MFAFactor) string {
	return generateTOTP(f.ChallengeHint, time.Now().UTC().Unix())
}

// restarted models an API restart: a new service on a fresh connection pool (so no
// prepared statements survive a schema change made by the test).
func (e *cov1xMFAEnv) restarted(t *testing.T, sealer SecretSealer) (*Service, *cov1xLogBuffer) {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(context.Background(), e.pool.Config())
	if err != nil {
		t.Fatalf("open second pool: %v", err)
	}
	t.Cleanup(pool.Close)
	logger, logs := cov1xLogger()
	svc := NewService(Config{EnableDevStubTokens: true, Logger: logger})
	if sealer != nil {
		svc.SetSecretSealer(sealer)
	}
	svc.SetPool(pool)
	return svc, logs
}

func TestCov1xDBMFAPersistenceFailures(t *testing.T) {
	env := cov1xNewMFAEnv(t)

	t.Run("a failed TOTP step write releases the claim so the code can be retried", func(t *testing.T) {
		f := env.enroll(t, "totp")
		code := cov1xTOTPNow(f)
		drop := cov1xTrigger(t, env.pool, "cov1x_totp_block", "mfa_factors", "BEFORE UPDATE",
			"NEW.totp_last_counter IS DISTINCT FROM OLD.totp_last_counter", cov1xRaise)

		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, code); err == nil || !strings.Contains(err.Error(), "consume totp step") {
			t.Fatalf("expected the consume failure, got %v", err)
		}
		drop()
		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, code); err != nil {
			t.Fatalf("the same code must work once persistence recovers: %v", err)
		}
	})

	t.Run("a failed backup code write releases the code", func(t *testing.T) {
		f := env.enroll(t, "backup_code")
		drop := cov1xTrigger(t, env.pool, "cov1x_backup_block", "mfa_factors", "BEFORE UPDATE",
			"NEW.backup_codes IS DISTINCT FROM OLD.backup_codes", cov1xRaise)

		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, f.Codes[0]); err == nil || !strings.Contains(err.Error(), "consume backup code") {
			t.Fatalf("expected the consume failure, got %v", err)
		}
		drop()
		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, f.Codes[0]); err != nil {
			t.Fatalf("the code was not spent, so it must still work: %v", err)
		}
		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, f.Codes[0]); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("a spent backup code must not work twice: %v", err)
		}
	})

	t.Run("a backup code spent by another instance mid-flight is rejected", func(t *testing.T) {
		f := env.enroll(t, "backup_code")
		// The conditional UPDATE matching no row is exactly what losing the race looks like.
		cov1xTrigger(t, env.pool, "cov1x_backup_lost", "mfa_factors", "BEFORE UPDATE",
			"NEW.backup_codes IS DISTINCT FROM OLD.backup_codes", cov1xSkipRow)

		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, f.Codes[0]); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("expected ErrInvalidMFACode, got %v", err)
		}
		if n := cov1xCount(t, env.pool, `SELECT count(*) FROM mfa_factors WHERE id::text = $1 AND backup_codes::text LIKE '%"Used": true%'`, f.ID); n != 0 {
			t.Fatal("the stored backup codes must be untouched")
		}
	})

	t.Run("a failed factor update after the code was spent keeps the code spent", func(t *testing.T) {
		f := env.enroll(t, "totp")
		code := cov1xTOTPNow(f)
		drop := cov1xTrigger(t, env.pool, "cov1x_touch_block", "mfa_factors", "BEFORE UPDATE",
			"NEW.last_challenged_at IS DISTINCT FROM OLD.last_challenged_at", cov1xRaise)

		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, code); err == nil || !strings.Contains(err.Error(), "update MFA factor") {
			t.Fatalf("expected the update failure, got %v", err)
		}
		drop()
		if _, err := env.svc.ChallengeMFA("", env.principal, f.ID, code); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("codes are single use even when the bookkeeping write failed: %v", err)
		}
	})

	t.Run("a sealing failure at enrollment leaves no factor behind", func(t *testing.T) {
		before := cov1xCount(t, env.pool, `SELECT count(*) FROM mfa_factors`)
		sealFail, _ := env.restarted(t, cov1xFailSealer{})
		if _, err := sealFail.EnrollMFA(env.user, "mfa@example.com", env.org, "totp", "x"); err == nil || !strings.Contains(err.Error(), "seal TOTP secret") {
			t.Fatalf("expected seal failure, got %v", err)
		}
		if n := cov1xCount(t, env.pool, `SELECT count(*) FROM mfa_factors`); n != before {
			t.Fatalf("no factor may be written: %d -> %d", before, n)
		}
	})
}

func TestCov1xDBLoadMFAFactorsSurvivesBadStoredData(t *testing.T) {
	env := cov1xNewMFAEnv(t)
	f := env.enroll(t, "totp")
	secret := f.ChallengeHint

	t.Run("an encrypted secret without a key is skipped, never served raw", func(t *testing.T) {
		cov1xExec(t, env.pool, `UPDATE mfa_factors SET totp_secret = 'v2:'||$2||'|SECRET' WHERE id::text = $1`, f.ID, env.org)
		t.Cleanup(func() {
			cov1xExec(t, env.pool, `UPDATE mfa_factors SET totp_secret = $2 WHERE id::text = $1`, f.ID, secret)
		})

		svc, logs := env.restarted(t, nil)
		if !strings.Contains(logs.String(), "TOTP secret unreadable") {
			t.Fatalf("expected an unreadable-secret log: %s", logs.String())
		}
		if _, ok := loadedSecret(svc, f.ID); ok {
			t.Fatal("the encrypted factor must not be loaded without a key")
		}
		if err := svc.reloadMFAFactorsForUser(context.Background(), env.org, env.user); err == nil || !strings.Contains(err.Error(), "no encryption key") {
			t.Fatalf("reload must refuse to use it: %v", err)
		}
		if _, err := svc.ChallengeMFA("", env.principal, f.ID, "123456"); err == nil {
			t.Fatal("a challenge against an unreadable factor must fail, not pass")
		}
	})

	t.Run("an unscannable row is skipped on load and reported on reload", func(t *testing.T) {
		cov1xExec(t, env.pool, `ALTER TABLE mfa_factors ALTER COLUMN last_challenged_at TYPE text USING last_challenged_at::text`)
		t.Cleanup(func() {
			cov1xExec(t, env.pool, `ALTER TABLE mfa_factors ALTER COLUMN last_challenged_at TYPE timestamptz USING NULL`)
		})
		cov1xExec(t, env.pool, `UPDATE mfa_factors SET last_challenged_at = 'not a timestamp'`)

		svc, logs := env.restarted(t, nil)
		if !strings.Contains(logs.String(), "scan MFA factor row") {
			t.Fatalf("the scan failure is logged on load: %s", logs.String())
		}
		if err := svc.reloadMFAFactorsForUser(context.Background(), env.org, env.user); err == nil || !strings.Contains(err.Error(), "scan MFA factor") {
			t.Fatalf("reload must report the scan failure: %v", err)
		}
	})

	t.Run("a query failure is logged, does not stop startup, and blocks enrollment", func(t *testing.T) {
		cov1xExec(t, env.pool, `ALTER TABLE mfa_factors RENAME COLUMN label TO label_x`)
		t.Cleanup(func() { cov1xExec(t, env.pool, `ALTER TABLE mfa_factors RENAME COLUMN label_x TO label`) })

		svc, logs := env.restarted(t, nil)
		if !strings.Contains(logs.String(), "failed to load MFA factors from DB") {
			t.Fatalf("expected a load failure log: %s", logs.String())
		}
		if got := svc.ListMFAFactors(env.org, env.user); len(got) != 0 {
			t.Fatalf("nothing could be loaded: %+v", got)
		}
		if !strings.Contains(logs.String(), "MFA factor reload failed") {
			t.Fatalf("the reload failure is reported too: %s", logs.String())
		}
		if _, err := svc.EnrollMFA(env.user, "mfa@example.com", env.org, "totp", "x"); err == nil || !strings.Contains(err.Error(), "save MFA factor") {
			t.Fatalf("enrollment must fail when the row cannot be saved: %v", err)
		}
	})
}

func TestCov1xDBReloadReportsRowErrorRaisedWhileStreaming(t *testing.T) {
	env := cov1xNewMFAEnv(t)
	cov1xExec(t, env.pool, `ALTER TABLE mfa_factors RENAME TO mfa_factors_real`)
	// Two rows: the first streams fine, the second raises while it is produced
	// (the volatile label expression is evaluated after the sort).
	cov1xExec(t, env.pool, `CREATE VIEW mfa_factors AS SELECT
		'mfa_view_'||n AS id, '`+env.user+`'::uuid AS user_id, '`+env.org+`'::uuid AS org_id,
		'totp'::text AS factor_type,
		CASE WHEN n = 2 THEN (1 / (random() * 0))::text ELSE 'ok' END AS label,
		true AS is_verified, ''::text AS totp_secret, '[]'::jsonb AS backup_codes,
		now() + (n || ' seconds')::interval AS created_at, now() AS last_challenged_at
		FROM generate_series(1, 2) AS n`)
	err := env.svc.reloadMFAFactorsForUser(context.Background(), env.org, env.user)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected the streamed row error, got %v", err)
	}
}

func TestCov1xDBLegacyPlaintextResealFailuresAreNonFatal(t *testing.T) {
	env := cov1xNewMFAEnv(t) // writes the secret in the clear: no sealer yet
	f := env.enroll(t, "totp")
	secret := f.ChallengeHint

	t.Run("a sealer that cannot encrypt leaves the row as is and the factor usable", func(t *testing.T) {
		svc, logs := env.restarted(t, cov1xFailSealer{})
		if !strings.Contains(logs.String(), "could not seal legacy TOTP secret") {
			t.Fatalf("expected a seal failure log: %s", logs.String())
		}
		if got, ok := loadedSecret(svc, f.ID); !ok || got != secret {
			t.Fatalf("the factor must stay usable: %q %v", got, ok)
		}
		if got := storedTOTP(t, env.pool, f.ID); got != secret {
			t.Fatalf("the stored value must be untouched: %q", got)
		}
	})

	t.Run("a failing database write leaves the row as is and the factor usable", func(t *testing.T) {
		cov1xTrigger(t, env.pool, "cov1x_reseal_block", "mfa_factors", "BEFORE UPDATE",
			"NEW.totp_secret IS DISTINCT FROM OLD.totp_secret", cov1xRaise)
		svc, logs := env.restarted(t, fakeSealer{})
		if !strings.Contains(logs.String(), "could not re-seal legacy TOTP secret") {
			t.Fatalf("expected a re-seal failure log: %s", logs.String())
		}
		if got, ok := loadedSecret(svc, f.ID); !ok || got != secret {
			t.Fatalf("the factor must stay usable: %q %v", got, ok)
		}
		if got := storedTOTP(t, env.pool, f.ID); got != secret {
			t.Fatalf("the stored value must be untouched: %q", got)
		}
	})
}
