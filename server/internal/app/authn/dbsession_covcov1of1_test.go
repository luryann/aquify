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

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func cov1xCount(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

// cov1xOrg registers a fresh tenant and returns its id.
func cov1xOrg(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	return org
}

func cov1xSignIn(t *testing.T, svc *Service, org, user, idToken string) SessionTokens {
	t.Helper()
	tok, err := svc.IssueSession(SessionInput{
		UserID: user, Email: user + "@example.com", Name: "Cov User", OrgID: org,
		Roles: []string{"admin"}, OIDCIDToken: idToken,
	})
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	return tok
}

func cov1xServiceOn(pool *pgxpool.Pool, cfg Config) *Service {
	svc := NewService(cfg)
	svc.SetPool(pool)
	return svc
}

// cov1xTrigger installs a row trigger on table in the isolated schema and
// returns a function that removes it again (also run on test cleanup).
func cov1xTrigger(t *testing.T, pool *pgxpool.Pool, name, table, event, when, body string) (drop func()) {
	t.Helper()
	cov1xExec(t, pool, "CREATE FUNCTION "+name+"_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$")
	whenClause := ""
	if when != "" {
		whenClause = " WHEN (" + when + ")"
	}
	cov1xExec(t, pool, "CREATE TRIGGER "+name+" "+event+" ON "+table+" FOR EACH ROW"+whenClause+" EXECUTE FUNCTION "+name+"_fn()")
	dropped := false
	drop = func() {
		if dropped {
			return
		}
		dropped = true
		cov1xExec(t, pool, "DROP TRIGGER IF EXISTS "+name+" ON "+table)
		cov1xExec(t, pool, "DROP FUNCTION IF EXISTS "+name+"_fn()")
	}
	t.Cleanup(drop)
	return drop
}

const (
	cov1xRaise   = "RAISE EXCEPTION 'cov1x: injected failure';"
	cov1xSkipRow = "RETURN NULL;"
)

func TestCov1xDBSignInRejectionsUserSyncAndRLSRefusal(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	logger, logs := cov1xLogger()
	svc := cov1xServiceOn(pool, Config{EnableDevStubTokens: true, Logger: logger})
	org := cov1xOrg(t, pool)
	user := uuid.NewString()

	// An org the registry has never heard of cannot sign anyone in.
	_, err := svc.IssueSession(SessionInput{UserID: user, OrgID: uuid.NewString(), Roles: []string{"admin"}})
	if !errors.Is(err, ErrOrgNotActive) {
		t.Fatalf("unknown org: %v", err)
	}
	// Neither can a suspended one.
	suspended := cov1xOrg(t, pool)
	cov1xExec(t, pool, `UPDATE orgs SET status = 'suspended' WHERE id = $1::uuid`, suspended)
	if _, err := svc.IssueSession(SessionInput{UserID: user, OrgID: suspended, Roles: []string{"admin"}}); !errors.Is(err, ErrOrgNotActive) {
		t.Fatalf("suspended org: %v", err)
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM sessions`); n != 0 {
		t.Fatalf("refused sign-ins must not leave sessions behind: %d", n)
	}

	// Legacy non-UUID ids never reach the database: the org check is skipped but the
	// session store refuses them with a clear message.
	if _, err := svc.IssueSession(SessionInput{UserID: user, OrgID: "legacy-org", Roles: []string{"admin"}}); err == nil || errors.Is(err, ErrOrgNotActive) || !strings.Contains(err.Error(), "org_id must be a UUID") {
		t.Fatalf("non-uuid org with persistent sessions: %v", err)
	}
	if _, err := svc.IssueSession(SessionInput{UserID: "not-a-uuid", OrgID: org, Roles: []string{"admin"}}); err == nil || !strings.Contains(err.Error(), "user_id must be a UUID") {
		t.Fatalf("non-uuid user with persistent sessions: %v", err)
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM users`); n != 0 {
		t.Fatalf("skipped syncs must not create users: %d", n)
	}
	for _, args := range [][3]string{{"", org, "empty user"}, {user, "  ", "empty org"}} {
		if err := svc.upsertAuthenticatedUser(context.Background(), args[0], "a@example.com", "A", args[1], false); err != nil {
			t.Fatalf("%s: a no-op sync returns nil, got %v", args[2], err)
		}
	}
	if !strings.Contains(logs.String(), "user_id is not a UUID") || !strings.Contains(logs.String(), "org_id is not a UUID") {
		t.Fatalf("skipped syncs are logged: %s", logs.String())
	}

	// Blank email and name get stable placeholders; real values are never invented.
	if _, err := svc.IssueSession(SessionInput{UserID: user, OrgID: org, Roles: []string{"admin"}}); err != nil {
		t.Fatalf("sign in with blank profile: %v", err)
	}
	var email, display string
	if err := pool.QueryRow(context.Background(), `SELECT email, display_name FROM users WHERE id = $1::uuid`, user).Scan(&email, &display); err != nil {
		t.Fatal(err)
	}
	if email != user+"@local.invalid" || display != email {
		t.Fatalf("placeholder profile: email=%q display=%q", email, display)
	}

	// Postgres refusing the write with an insufficient-privilege error is what
	// row-level security raises for a row owned by another club; it must surface
	// as ErrUserOrgMismatch.
	cov1xTrigger(t, pool, "cov1x_rls", "users", "BEFORE INSERT", "", "RAISE EXCEPTION 'denied' USING ERRCODE = '42501';")
	if _, err := svc.IssueSession(SessionInput{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}); !errors.Is(err, ErrUserOrgMismatch) {
		t.Fatalf("an RLS refusal must surface as ErrUserOrgMismatch, got %v", err)
	}
}

func TestCov1xDBSeedLocalUsersDoesNotRehomeExistingUsers(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	other := cov1xOrg(t, pool)
	plain := cov1xServiceOn(pool, Config{EnableDevStubTokens: true})
	if _, err := plain.IssueSession(SessionInput{UserID: LocalDevAdminUserID, Email: "admin@aquify.local", Name: "Pre-existing", OrgID: other, Roles: []string{"admin"}}); err != nil {
		t.Fatalf("pre-existing admin: %v", err)
	}

	logger, logs := cov1xLogger()
	cov1xServiceOn(pool, Config{EnableDevStubTokens: true, SeedLocalUsers: true, Logger: logger})

	if n := cov1xCount(t, pool, `SELECT count(*) FROM orgs WHERE id = $1::uuid AND status = 'active'`, LocalDevOrgID); n != 1 {
		t.Fatalf("the local dev org must be registered: %d", n)
	}
	var adminOrg string
	if err := pool.QueryRow(context.Background(), `SELECT org_id::text FROM users WHERE id = $1::uuid`, LocalDevAdminUserID).Scan(&adminOrg); err != nil || adminOrg != other {
		t.Fatalf("seeding must never re-home an existing user: org=%q err=%v", adminOrg, err)
	}
	if !strings.Contains(logs.String(), "failed to seed local dev user") || !strings.Contains(logs.String(), LocalDevAdminUserID) {
		t.Fatalf("the refused seed is reported: %s", logs.String())
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM users WHERE org_id = $1::uuid`, LocalDevOrgID); n != 3 {
		t.Fatalf("the other three dev users are seeded under the dev org, got %d", n)
	}
}

func TestCov1xDBSeedLocalUsersReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()
	logger, logs := cov1xLogger()
	svc := NewService(Config{EnableDevStubTokens: true, SeedLocalUsers: true, Logger: logger})
	svc.SetPool(testutil.ClosedPool(t))
	out := logs.String()
	for _, want := range []string{"failed to list orgs for MFA load", "failed to register local dev org", "failed to seed local dev user"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing log %q in: %s", want, out)
		}
	}
	if got := strings.Count(out, "failed to seed local dev user"); got != len(LocalDevUsers()) {
		t.Errorf("every dev user failure is reported once, got %d", got)
	}
}

func TestCov1xDBOIDCSessionPersistsIDTokenAndLogsOut(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := cov1xOrg(t, pool)
	user := uuid.NewString()

	f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{"sub": user, "org_id": org, "roles": []string{"admin"}}))
	f.svc.SetPool(pool)
	res, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM sessions WHERE id_token IS NOT NULL AND org_id = $1::uuid`, org); n != 1 {
		t.Fatalf("the provider id token is stored with the session: %d", n)
	}

	// Logout by refresh token revokes the row and returns the provider logout URL.
	out := f.svc.LogoutSessionWithAccess(res.RefreshToken, "")
	if out.RevokeErr != nil || !strings.Contains(out.LogoutURL, "id_token_hint=") || out.RedirectURL != "/login" {
		t.Fatalf("logout: %+v", out)
	}
	if p, err := f.svc.ResolvePrincipalErr(res.AccessToken); p != nil || err != nil {
		t.Fatalf("a logged-out token resolves to nothing: %v %v", p, err)
	}
	// Repeating the logout on an already-revoked session is harmless.
	if again := f.svc.LogoutSession(res.RefreshToken); again.RevokeErr != nil || !strings.Contains(again.LogoutURL, "id_token_hint=") {
		t.Fatalf("repeat logout: %+v", again)
	}
	// Unknown tokens are "not found", which is not an error worth reporting.
	if unknown := f.svc.LogoutSession("rtk_unknown"); unknown.RevokeErr != nil || unknown.LogoutURL != "" {
		t.Fatalf("unknown refresh token: %+v", unknown)
	}

	// Logout by access token alone takes the second revocation path.
	tok := cov1xSignIn(t, f.svc, org, user, "second-id-token")
	byAccess := f.svc.LogoutSessionWithAccess("", tok.AccessToken)
	if byAccess.RevokeErr != nil || !strings.Contains(byAccess.LogoutURL, "second-id-token") {
		t.Fatalf("logout by access token: %+v", byAccess)
	}
	if p, _ := f.svc.ResolvePrincipalErr(tok.AccessToken); p != nil {
		t.Fatal("the access-token logout must revoke the session")
	}

	// A callback for an org the registry does not know fails closed.
	bad := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{"sub": uuid.NewString(), "org_id": uuid.NewString()}))
	bad.svc.SetPool(pool)
	if _, err := bad.svc.OIDCCallback("code", cov1xState(t, ""), ""); !errors.Is(err, ErrOrgNotActive) {
		t.Fatalf("callback for an unregistered org must be ErrOrgNotActive, got %v", err)
	}
}

func TestCov1xDBSessionRefreshAndResolveRejections(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := cov1xServiceOn(pool, Config{})
	org := cov1xOrg(t, pool)
	user := uuid.NewString()
	ctx := context.Background()

	if _, err := svc.RefreshSession("rtk_unknown"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown refresh token: %v", err)
	}
	if p, err := svc.ResolvePrincipalErr("atk_unknown"); p != nil || err != nil {
		t.Fatalf("unknown access token is (nil, nil): %v %v", p, err)
	}
	if err := svc.RevokeSession("rtk_unknown"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoking an unknown token: %v", err)
	}

	// Expired.
	expired := cov1xSignIn(t, svc, org, user, "")
	cov1xExec(t, pool, `UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE refresh_token_hash = $1`, hashToken(expired.RefreshToken))
	if _, err := svc.RefreshSession(expired.RefreshToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired: %v", err)
	}
	if p, err := svc.ResolvePrincipalErr(expired.AccessToken); p != nil || err != nil {
		t.Fatalf("an expired access token resolves to nothing: %v %v", p, err)
	}

	// Revoked, and revoking again is a no-op that keeps the first timestamp.
	revoked := cov1xSignIn(t, svc, org, user, "")
	if err := svc.RevokeSession(revoked.RefreshToken); err != nil {
		t.Fatal(err)
	}
	var first time.Time
	q := `SELECT revoked_at FROM sessions WHERE refresh_token_hash = $1`
	if err := pool.QueryRow(ctx, q, hashToken(revoked.RefreshToken)).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeSession(revoked.RefreshToken); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	var second time.Time
	if err := pool.QueryRow(ctx, q, hashToken(revoked.RefreshToken)).Scan(&second); err != nil || !first.Equal(second) {
		t.Fatalf("a repeat revoke must not move revoked_at: %v %v %v", first, second, err)
	}
	if _, err := svc.RefreshSession(revoked.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("revoked: %v", err)
	}

	// A refresh chain older than the absolute lifetime must sign in again, and the
	// refused refresh must leave the old session untouched.
	old := cov1xSignIn(t, svc, org, user, "")
	started := time.Now().Add(-(maxSessionAge + time.Hour)).Unix()
	cov1xExec(t, pool, `UPDATE sessions SET principal = jsonb_set(principal, '{session_started_at}', to_jsonb($2::bigint)) WHERE refresh_token_hash = $1`, hashToken(old.RefreshToken), started)
	if _, err := svc.RefreshSession(old.RefreshToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("chain past maxSessionAge: %v", err)
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM sessions WHERE refresh_token_hash = $1 AND revoked_at IS NULL`, hashToken(old.RefreshToken)); n != 1 {
		t.Fatal("a refused rotation must not revoke the presented session")
	}

	// A principal column that is not an object is reported, never served.
	corrupt := cov1xSignIn(t, svc, org, user, "")
	cov1xExec(t, pool, `UPDATE sessions SET principal = '[]'::jsonb WHERE refresh_token_hash = $1`, hashToken(corrupt.RefreshToken))
	if p, err := svc.ResolvePrincipalErr(corrupt.AccessToken); p != nil || err != nil {
		t.Fatalf("a corrupt session does not authenticate: %v %v", p, err)
	}
	if _, err := svc.RefreshSession(corrupt.RefreshToken); err == nil || !strings.Contains(err.Error(), "unmarshal session principal") {
		t.Fatalf("a corrupt session cannot be refreshed: %v", err)
	}

	// A user id that is not a UUID can only have in-memory sessions to revoke.
	if n, err := svc.RevokeUserSessions(ctx, org, "legacy-user"); err != nil || n != 0 {
		t.Fatalf("non-uuid user: %d %v", n, err)
	}
}

func TestCov1xDBMFAFactorPurgeAndSessionElevation(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := cov1xServiceOn(pool, Config{EnableDevStubTokens: true, OIDC: cov1xOIDCConfig()})
	org := cov1xOrg(t, pool)
	t.Run("purge only touches unverified factors of that type", func(t *testing.T) {
		cov1xPurgeScenario(t, pool, svc, org)
	})
	t.Run("challenge retires the current persistent session", func(t *testing.T) {
		cov1xElevationScenario(t, svc, org)
	})
}

func cov1xPurgeScenario(t *testing.T, pool *pgxpool.Pool, svc *Service, org string) {
	t.Helper()
	user := uuid.NewString()
	cov1xSignIn(t, svc, org, user, "")
	principal := &appmiddleware.Principal{UserID: user, OrgID: org, Roles: []string{"admin"}}

	verified, err := svc.EnrollMFA(user, "u@example.com", org, "totp", "kept")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChallengeMFA("", principal, verified.ID, generateTOTP(verified.ChallengeHint, time.Now().UTC().Unix())); err != nil {
		t.Fatalf("verify factor: %v", err)
	}
	abandoned, err := svc.EnrollMFA(user, "u@example.com", org, "totp", "abandoned")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := svc.EnrollMFA(user, "u@example.com", org, "backup_code", "backup")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.PurgeUnverifiedMFAFactors(org, user, "totp"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	ids := map[string]bool{}
	for _, f := range svc.ListMFAFactors(org, user) {
		ids[f.ID] = true
	}
	if !ids[verified.ID] || !ids[backup.ID] || ids[abandoned.ID] {
		t.Fatalf("purge kept the wrong factors: %v", ids)
	}
	if n := cov1xCount(t, pool, `SELECT count(*) FROM mfa_factors WHERE user_id::text = $1`, user); n != 2 {
		t.Fatalf("the database agrees with memory, got %d rows", n)
	}
}

func cov1xElevationScenario(t *testing.T, svc *Service, org string) {
	t.Helper()
	user := uuid.NewString()
	base := cov1xSignIn(t, svc, org, user, "persisted-id-token")
	factor, err := svc.EnrollMFA(user, "u@example.com", org, "totp", "phone")
	if err != nil {
		t.Fatal(err)
	}
	principal := &appmiddleware.Principal{UserID: user, OrgID: org, Roles: []string{"admin"}}

	elevated, err := svc.ChallengeMFA(base.AccessToken, principal, factor.ID, generateTOTP(factor.ChallengeHint, time.Now().UTC().Unix()))
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if p, err := svc.ResolvePrincipalErr(base.AccessToken); p != nil || err != nil {
		t.Fatalf("the pre-MFA session must be revoked: %v %v", p, err)
	}
	p, err := svc.ResolvePrincipalErr(elevated.AccessToken)
	if err != nil || p == nil || !p.MFAAuthenticated {
		t.Fatalf("the elevated session works: %+v %v", p, err)
	}
	if out := svc.LogoutSessionWithAccess("", elevated.AccessToken); !strings.Contains(out.LogoutURL, "persisted-id-token") {
		t.Fatalf("the id token follows the session through MFA elevation: %+v", out)
	}
}

func TestCov1xDBFailuresOnAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	closed := testutil.ClosedPool(t)
	logger, logs := cov1xLogger()
	svc := NewService(Config{EnableDevStubTokens: true, Logger: logger})
	svc.SetPool(closed)
	ctx := context.Background()
	org, user := uuid.NewString(), uuid.NewString()

	if _, err := svc.IssueSession(SessionInput{UserID: user, OrgID: org, Roles: []string{"admin"}}); err == nil || !strings.Contains(err.Error(), "check org status") {
		t.Fatalf("sign-in needs the registry: %v", err)
	}
	// With the org check out of the way (legacy id), the session write itself fails.
	if _, err := svc.IssueSession(SessionInput{UserID: user, OrgID: "legacy", Roles: []string{"admin"}}); err == nil {
		t.Fatal("sign-in without a session store must fail")
	}
	rec := &sessionRecord{AccessToken: "a", RefreshToken: "r", Principal: appmiddleware.Principal{UserID: user, OrgID: org}, ExpiresAt: time.Now().Add(time.Hour)}
	if err := insertSessionRecord(ctx, closed, rec); err == nil || !strings.Contains(err.Error(), "persist session") {
		t.Fatalf("insertSessionRecord: %v", err)
	}
	if _, err := svc.issueSessionRecord(ctx, rec.Principal, "", time.Hour); err == nil {
		t.Fatal("issueSessionRecord must report the failed insert")
	}

	if p, err := svc.ResolvePrincipalErr("atk_x"); p != nil || !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("an outage is not a 401: %v %v", p, err)
	}
	if p, ok := svc.ResolvePrincipal("atk_x"); p != nil || ok {
		t.Fatalf("ResolvePrincipal treats an outage as unauthenticated: %v %v", p, ok)
	}
	if !strings.Contains(logs.String(), "session lookup failed") {
		t.Fatalf("the outage is logged: %s", logs.String())
	}
	if _, err := svc.RefreshSession("rtk_x"); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("refresh during an outage: %v", err)
	}
	if err := svc.RevokeSession("rtk_x"); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("revoke during an outage: %v", err)
	}
	out := svc.LogoutSessionWithAccess("rtk_x", "")
	if !errors.Is(out.RevokeErr, ErrSessionUnavailable) || out.RedirectURL != "/login" {
		t.Fatalf("logout reports the failed revocation but still redirects: %+v", out)
	}
	if n, err := svc.RevokeUserSessions(ctx, org, user); err == nil || n != 0 || !strings.Contains(err.Error(), "revoke user sessions") {
		t.Fatalf("revoke user sessions: %d %v", n, err)
	}
	if err := svc.PurgeUnverifiedMFAFactors(org, user, "totp"); err == nil || !strings.Contains(err.Error(), "purge unverified MFA factors") {
		t.Fatalf("purge: %v", err)
	}
	if _, err := svc.EnrollMFA(user, "u@example.com", org, "totp", "x"); err == nil || !strings.Contains(err.Error(), "save MFA factor") {
		t.Fatalf("enroll: %v", err)
	}
	if got := svc.ListMFAFactors(org, user); len(got) != 0 {
		t.Fatalf("a factor the database never saw must not exist in memory: %+v", got)
	}

	// The cached view is served when the reload fails, with a loud log.
	svc.mu.Lock()
	svc.mfaFactorsByUser[user] = []mfaFactorRecord{{Factor: MFAFactor{ID: "mfa_cached", UserID: user, OrgID: org, FactorType: "totp"}}}
	svc.mu.Unlock()
	if got := svc.ListMFAFactors(org, user); len(got) != 1 || got[0].ID != "mfa_cached" {
		t.Fatalf("cached factors are served: %+v", got)
	}
	if !strings.Contains(logs.String(), "MFA factor reload failed") {
		t.Fatalf("the failed reload is logged: %s", logs.String())
	}
	principal := &appmiddleware.Principal{UserID: user, OrgID: org, Roles: []string{"admin"}}
	if _, err := svc.ChallengeMFA("", principal, "mfa_cached", "123456"); err == nil || !strings.Contains(err.Error(), "reload MFA factors") {
		t.Fatalf("a challenge cannot proceed on stale factors: %v", err)
	}

	if _, err := svc.consumeTOTPStepInDB(ctx, org, "f", 1); err == nil || !strings.Contains(err.Error(), "consume totp step") {
		t.Fatalf("consumeTOTPStepInDB: %v", err)
	}
	if _, err := svc.consumeBackupCodeInDB(ctx, org, "f", "h"); err == nil || !strings.Contains(err.Error(), "consume backup code") {
		t.Fatalf("consumeBackupCodeInDB: %v", err)
	}
	if err := svc.updateMFAFactorInDB(mfaFactorRecord{Factor: MFAFactor{ID: "f", OrgID: org}}); err == nil || !strings.Contains(err.Error(), "update MFA factor") {
		t.Fatalf("updateMFAFactorInDB: %v", err)
	}
}

func TestCov1xDBSessionWritesFailAtomically(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := cov1xServiceOn(pool, Config{})
	org := cov1xOrg(t, pool)
	user := uuid.NewString()
	ctx := context.Background()

	t.Run("a failing session update surfaces on revoke, logout and rotation", func(t *testing.T) {
		tok := cov1xSignIn(t, svc, org, user, "")
		drop := cov1xTrigger(t, pool, "cov1x_sess_upd", "sessions", "BEFORE UPDATE", "", cov1xRaise)

		if n, err := svc.RevokeUserSessions(ctx, org, user); err == nil || !strings.Contains(err.Error(), "revoke user sessions") {
			t.Fatalf("revoke user sessions: %d %v", n, err)
		}
		if err := svc.RevokeSession(tok.RefreshToken); err == nil || !strings.Contains(err.Error(), "revoke session") {
			t.Fatalf("revoke session: %v", err)
		}
		if out := svc.LogoutSession(tok.RefreshToken); out.RevokeErr == nil {
			t.Fatalf("logout must report the failed revocation: %+v", out)
		}
		if _, err := svc.RefreshSession(tok.RefreshToken); err == nil || !strings.Contains(err.Error(), "revoke rotated session") {
			t.Fatalf("rotation: %v", err)
		}

		drop()
		if _, err := svc.RefreshSession(tok.RefreshToken); err != nil {
			t.Fatalf("the failed rotation was rolled back, so the token still works: %v", err)
		}
	})

	t.Run("an update that touches no row leaves the session alone", func(t *testing.T) {
		tok := cov1xSignIn(t, svc, org, user, "kept-token")
		cov1xTrigger(t, pool, "cov1x_sess_skip", "sessions", "BEFORE UPDATE", "", cov1xSkipRow)
		if err := svc.RevokeSession(tok.RefreshToken); err != nil {
			t.Fatalf("a concurrent revoke that wins the race is not an error: %v", err)
		}
		if p, err := svc.ResolvePrincipalErr(tok.AccessToken); err != nil || p == nil {
			t.Fatalf("nothing was actually revoked, so the session still resolves: %v %v", p, err)
		}
	})

	t.Run("a failing insert aborts rotation and sign-in", func(t *testing.T) {
		tok := cov1xSignIn(t, svc, org, user, "")
		drop := cov1xTrigger(t, pool, "cov1x_sess_ins", "sessions", "BEFORE INSERT", "", cov1xRaise)
		if _, err := svc.RefreshSession(tok.RefreshToken); err == nil || !strings.Contains(err.Error(), "persist session") {
			t.Fatalf("rotation insert: %v", err)
		}
		if _, err := svc.IssueSession(SessionInput{UserID: user, Email: user + "@example.com", OrgID: org, Roles: []string{"admin"}}); err == nil || !strings.Contains(err.Error(), "persist session") {
			t.Fatalf("sign-in insert: %v", err)
		}
		drop()
		if _, err := svc.RefreshSession(tok.RefreshToken); err != nil {
			t.Fatalf("the original session survives the aborted rotation: %v", err)
		}
	})

	t.Run("a commit failure is a retryable outage and keeps the old session", func(t *testing.T) {
		tok := cov1xSignIn(t, svc, org, user, "")
		cov1xExec(t, pool, `CREATE FUNCTION cov1x_commit_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x: commit refused'; END $$`)
		cov1xExec(t, pool, `CREATE CONSTRAINT TRIGGER cov1x_commit AFTER INSERT ON sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_commit_fn()`)
		drop := func() {
			cov1xExec(t, pool, `DROP TRIGGER IF EXISTS cov1x_commit ON sessions`)
			cov1xExec(t, pool, `DROP FUNCTION IF EXISTS cov1x_commit_fn()`)
		}
		t.Cleanup(drop)
		if _, err := svc.RefreshSession(tok.RefreshToken); !errors.Is(err, ErrSessionUnavailable) {
			t.Fatalf("commit failure: %v", err)
		}
		drop()
		if _, err := svc.RefreshSession(tok.RefreshToken); err != nil {
			t.Fatalf("the old session still refreshes after the failed commit: %v", err)
		}
	})
}
