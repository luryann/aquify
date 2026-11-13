package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/authn"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/testutil"
)

// A factor of a type the server cannot verify (here an SMS factor written by
// another system) is refused with 501 rather than treated as a wrong code.
func TestCvg4gChallengeUnsupportedFactorType(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := authn.NewService(authn.Config{EnableDevStubTokens: true})
	svc.SetPool(pool)
	h := NewAuthHandlers(svc, false)
	org, user, factorID := uuid.NewString(), uuid.NewString(), "mfa_sms_"+uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO mfa_factors (id, user_id, org_id, factor_type, label, is_verified, phone_e164)
		 VALUES ($1, $2, $3, 'sms', 'phone', true, '+15555550100')`, factorID, user, org); err != nil {
		t.Fatalf("seed sms factor: %v", err)
	}
	router := cov4xRouter(tokenResolver{"u": {UserID: user, OrgID: org, Roles: []string{"parent"}}}, h.RegisterProtectedRoutes)

	req := httptest.NewRequest("POST", "/mfa/challenge", strings.NewReader(`{"factor_id":"`+factorID+`","challenge_code":"123456"}`))
	req.Header.Set("Authorization", "Bearer u")
	req.Header.Set("X-Org-Id", org)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented || cov4xErrCode(t, rr) != "unsupported_mfa_factor" {
		t.Fatalf("want 501 unsupported_mfa_factor, got %d %s", rr.Code, rr.Body.String())
	}
}

// Stripe retries deliveries; a repeated event id is acknowledged without being
// processed again.
func TestCvg4gDuplicateWebhookAcknowledgedOnce(t *testing.T) {
	t.Parallel()
	e := cvg4NewEnv(t, billing.NewService())
	payload := cvg4Event("evt_dupe", "customer.created", "", time.Now().Unix(), map[string]any{"id": "c"})

	rr := e.webhook(payload)
	expectStatus(t, rr, http.StatusOK, "first delivery")
	if got := cvg4Status(t, rr); got != "processed" {
		t.Fatalf("first delivery status = %q", got)
	}
	rr = e.webhook(payload)
	expectStatus(t, rr, http.StatusOK, "second delivery")
	if got := cvg4Status(t, rr); got != "already_processed" {
		t.Fatalf("duplicate delivery status = %q, want already_processed", got)
	}
}
