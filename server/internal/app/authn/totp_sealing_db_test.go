package authn

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeSealer is a reversible, org-bound stand-in for the AES-GCM cipher.
type fakeSealer struct{}

func (fakeSealer) EncryptFor(orgID, plain string) (string, error) {
	return sealedSecretPrefix + orgID + "|" + plain, nil
}

func (fakeSealer) DecryptFor(orgID, enc string) (string, error) {
	rest := strings.TrimPrefix(enc, sealedSecretPrefix)
	o, p, ok := strings.Cut(rest, "|")
	if !ok || o != orgID {
		return "", errors.New("wrong organization")
	}
	return p, nil
}

func storedTOTP(t *testing.T, pool *pgxpool.Pool, factorID string) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(), `SELECT totp_secret FROM mfa_factors WHERE id = $1`, factorID).Scan(&v); err != nil {
		t.Fatalf("read stored secret: %v", err)
	}
	return v
}

func loadedSecret(svc *Service, factorID string) (string, bool) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	for _, recs := range svc.mfaFactorsByUser {
		for _, r := range recs {
			if r.Factor.ID == factorID {
				return r.TOTPSecret, true
			}
		}
	}
	return "", false
}

// TOTP secrets must not sit in the database in the clear, must still work after a
// restart, and legacy plaintext rows must be sealed the first time they are loaded.
func TestTOTPSecretsAreSealedAtRestAndLegacyRowsAreResealed(t *testing.T) {
	pool := openAuthnTestPool(t)
	svc1 := NewService(Config{EnableDevStubTokens: true})
	svc1.SetSecretSealer(fakeSealer{})
	svc1.SetPool(pool)
	seedMFAUser(t, svc1)
	factor, err := svc1.EnrollMFA(mfaTestUser, "mfa-a1@example.com", mfaTestOrg, "totp", "t")
	if err != nil {
		t.Fatal(err)
	}
	secret := factor.ChallengeHint
	if secret == "" {
		t.Fatal("enrolment returned no secret")
	}

	if got := storedTOTP(t, pool, factor.ID); !strings.HasPrefix(got, sealedSecretPrefix) || got == secret {
		t.Fatalf("stored secret is not sealed: %q", got)
	}

	svc2 := NewService(Config{EnableDevStubTokens: true})
	svc2.SetSecretSealer(fakeSealer{})
	svc2.SetPool(pool)
	if got, ok := loadedSecret(svc2, factor.ID); !ok || got != secret {
		t.Fatalf("restarted instance read %q (found=%v), want the original secret", got, ok)
	}

	// Legacy plaintext row (written before sealing existed).
	if _, err := pool.Exec(context.Background(), `UPDATE mfa_factors SET totp_secret = $1 WHERE id = $2`, secret, factor.ID); err != nil {
		t.Fatal(err)
	}
	svc3 := NewService(Config{EnableDevStubTokens: true})
	svc3.SetSecretSealer(fakeSealer{})
	svc3.SetPool(pool)
	if got, ok := loadedSecret(svc3, factor.ID); !ok || got != secret {
		t.Fatalf("legacy row read %q (found=%v), want the plaintext secret", got, ok)
	}
	if got := storedTOTP(t, pool, factor.ID); !strings.HasPrefix(got, sealedSecretPrefix) {
		t.Fatalf("legacy plaintext was not re-sealed on load: %q", got)
	}

	// An encrypted row without a key must not be read as if it were a secret.
	svc4 := NewService(Config{EnableDevStubTokens: true})
	svc4.SetPool(pool)
	if got, ok := loadedSecret(svc4, factor.ID); ok {
		t.Fatalf("sealed row loaded without a key: %q", got)
	}
}
