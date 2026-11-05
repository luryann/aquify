package app

import (
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/usaswimming"
)

// resolveUSASwimming builds the integration service. The API key cipher comes
// from INTEGRATIONS_ENCRYPTION_KEY; development/local/test fall back to a fixed
// dev key and also enable the in-memory stub client (base URL "stub").
// Elsewhere a missing key fails closed (saving an API key returns 503) and the
// stub is never available.
func resolveUSASwimming(pool *pgxpool.Pool, appEnv string) *usaswimming.Service {
	var stub usaswimming.Client
	if isExplicitDevelopmentEnv(appEnv) {
		stub = usaswimming.NewStubClient()
	}
	return usaswimming.NewServiceWithPool(pool, integrationsCipher(appEnv), stub)
}

// integrationsCipher is the at-rest cipher for stored secrets (integration API keys,
// MFA TOTP secrets), keyed by INTEGRATIONS_ENCRYPTION_KEY. Development/local/test fall
// back to a fixed dev key; elsewhere a missing key yields nil (fail closed; startup
// validation refuses to run without the key).
func integrationsCipher(appEnv string) *usaswimming.Cipher {
	secret := strings.TrimSpace(os.Getenv("INTEGRATIONS_ENCRYPTION_KEY"))
	if secret == "" && isExplicitDevelopmentEnv(appEnv) {
		secret = "aquify-dev-integrations-key"
	}
	return usaswimming.NewCipher(secret)
}
