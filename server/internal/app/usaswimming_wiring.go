package app

import (
	"os"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/usaswimming"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolveUSASwimming builds the integration service. The API key cipher comes
// from INTEGRATIONS_ENCRYPTION_KEY; development/local/test fall back to a fixed
// dev key and also enable the in-memory stub client (base URL "stub").
// Elsewhere a missing key fails closed (saving an API key returns 503) and the
// stub is never available.
func resolveUSASwimming(pool *pgxpool.Pool, appEnv string) *usaswimming.Service {
	secret := strings.TrimSpace(os.Getenv("INTEGRATIONS_ENCRYPTION_KEY"))
	var stub usaswimming.Client
	if isExplicitDevelopmentEnv(appEnv) {
		if secret == "" {
			secret = "aquify-dev-integrations-key"
		}
		stub = usaswimming.NewStubClient()
	}
	return usaswimming.NewServiceWithPool(pool, usaswimming.NewCipher(secret), stub)
}
