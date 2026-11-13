package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func cov1xRawSignedState(signingKey string, payloadRaw []byte) string {
	encoded := base64.RawURLEncoding.EncodeToString(payloadRaw)
	h := hmac.New(sha256.New, []byte(signingKey))
	_, _ = h.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func TestCov1xValidateOIDCStateRejectsEveryMalformedShape(t *testing.T) {
	t.Parallel()
	const key = "cov1x-state-key"
	now := time.Unix(1_800_000_000, 0).UTC()
	goodPayload := []byte(`{"nonce":"n1","exp":` + "1800000600" + `}`)

	good, err := validateOIDCState(cov1xRawSignedState(key, goodPayload), key, now)
	if err != nil || good.Nonce != "n1" || good.Exp != 1_800_000_600 {
		t.Fatalf("valid state rejected: %+v %v", good, err)
	}

	cases := []struct {
		name    string
		state   string
		wantErr string
	}{
		{"no separator", "onlyonepart", "state format invalid"},
		{"too many parts", "a.b.c", "state format invalid"},
		{"payload not base64", "!!!." + base64.RawURLEncoding.EncodeToString([]byte("x")), "state payload decode failed"},
		{"signature not base64", base64.RawURLEncoding.EncodeToString(goodPayload) + ".!!!", "state signature decode failed"},
		{"signature mismatch", cov1xRawSignedState("another-key", goodPayload), "state signature mismatch"},
		{"payload not json", cov1xRawSignedState(key, []byte("not json")), "state payload invalid"},
		{"blank nonce", cov1xRawSignedState(key, []byte(`{"nonce":"  ","exp":1800000600}`)), "state nonce missing"},
		{"expired", cov1xRawSignedState(key, []byte(`{"nonce":"n1","exp":1800000000}`)), "state expired"},
	}
	for _, tc := range cases {
		_, err := validateOIDCState(tc.state, key, now)
		if err == nil || err.Error() != tc.wantErr {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCov1xSignOIDCStateAppliesDefaultsAndRoundTrips(t *testing.T) {
	t.Parallel()
	const key = "cov1x-sign-key"
	state, nonce, err := signOIDCState(oidcStatePayload{OrgID: "org-x"}, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if len(nonce) != 48 {
		t.Fatalf("a generated nonce is 24 random bytes hex encoded, got %q", nonce)
	}
	payload, err := validateOIDCState(state, key, time.Now().UTC())
	if err != nil {
		t.Fatalf("a freshly signed state must validate: %v", err)
	}
	if payload.Nonce != nonce || payload.OrgID != "org-x" {
		t.Fatalf("round trip lost fields: %+v", payload)
	}
	remaining := time.Until(time.Unix(payload.Exp, 0))
	if remaining < 9*time.Minute || remaining > 10*time.Minute+time.Second {
		t.Fatalf("default expiry should be 10 minutes out, got %v", remaining)
	}

	_, kept, err := signOIDCState(oidcStatePayload{Nonce: "  fixed  ", Exp: time.Now().Add(time.Hour).Unix()}, key)
	if err != nil || kept != "fixed" {
		t.Fatalf("a supplied nonce is trimmed and kept: %q %v", kept, err)
	}
}

func TestCov1xFindSigningKeyByIDRejectsUnusableKeys(t *testing.T) {
	t.Parallel()
	set := oidcJWKSet{Keys: []oidcJWK{
		{Kid: "other", Kty: "RSA"},
		{Kid: "ec", Kty: "EC"},
		{Kid: "enc", Kty: "RSA", Use: "enc"},
		{Kid: "hs", Kty: "rsa", Use: "SIG", Alg: "HS256"},
		{Kid: "ok", Kty: " rsa ", Use: " SIG ", Alg: "RS256"},
		{Kid: "bare", Kty: "RSA"},
	}}
	tests := []struct {
		kid     string
		wantErr string
	}{
		{"ec", "oidc jwk key type is unsupported"},
		{"enc", "oidc jwk key use is not signing"},
		{"hs", "oidc jwk algorithm does not match id_token"},
		{"missing", "oidc jwk for token kid was not found"},
	}
	for _, tc := range tests {
		if _, err := set.findSigningKeyByID(tc.kid, "RS256"); err == nil || err.Error() != tc.wantErr {
			t.Errorf("kid %q: got %v, want %q", tc.kid, err, tc.wantErr)
		}
	}
	for kid, want := range map[string]string{"ok": "ok", " bare ": "bare"} {
		key, err := set.findSigningKeyByID(kid, "RS256")
		if err != nil || key.Kid != want {
			t.Errorf("kid %q should resolve to %q, got %+v %v", kid, want, key, err)
		}
	}
}

func TestCov1xRSAPublicKeyValidatesModulusAndExponent(t *testing.T) {
	t.Parallel()
	enc := base64.RawURLEncoding.EncodeToString
	nOK := enc([]byte{0x01, 0x00, 0x01})
	tests := []struct {
		name    string
		jwk     oidcJWK
		wantErr string
	}{
		{"modulus not base64", oidcJWK{N: "***", E: enc([]byte{1, 0, 1})}, "oidc jwk modulus decode failed"},
		{"exponent not base64", oidcJWK{N: nOK, E: "***"}, "oidc jwk exponent decode failed"},
		{"empty modulus", oidcJWK{N: "", E: enc([]byte{1, 0, 1})}, "oidc jwk modulus/exponent missing"},
		{"empty exponent", oidcJWK{N: nOK, E: ""}, "oidc jwk modulus/exponent missing"},
		{"exponent beyond int64", oidcJWK{N: nOK, E: enc([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})}, "oidc jwk exponent too large"},
		{"zero exponent", oidcJWK{N: nOK, E: enc([]byte{0})}, "oidc jwk exponent invalid"},
	}
	for _, tc := range tests {
		if _, err := tc.jwk.rsaPublicKey(); err == nil || err.Error() != tc.wantErr {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.wantErr)
		}
	}
	key, err := oidcJWK{N: nOK, E: enc([]byte{1, 0, 1})}.rsaPublicKey()
	if err != nil || key.E != 65537 || key.N.Int64() != 65537 {
		t.Fatalf("valid jwk parsed wrongly: %+v %v", key, err)
	}
}

func TestCov1xClaimConvertersHandleEveryJSONShape(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   any
		want string
	}{
		{"abc", "abc"},
		{json.Number("42"), "42"},
		{float64(1234), "1234"},
		{true, ""},
		{nil, ""},
	} {
		if got := toString(tc.in); got != tc.want {
			t.Errorf("toString(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}

	for _, tc := range []struct {
		in   any
		want int64
	}{
		{float64(7), 7},
		{int64(8), 8},
		{int(9), 9},
		{json.Number("10"), 10},
		{json.Number("1.5"), 0},
		{"11", 11},
		{"eleven", 0},
		{true, 0},
		{nil, 0},
	} {
		if got := toInt64(tc.in); got != tc.want {
			t.Errorf("toInt64(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}

	for _, tc := range []struct {
		name string
		in   any
		want []string
	}{
		{"string", "  admin ", []string{"admin"}},
		{"blank string", "   ", nil},
		{"any slice", []any{" a ", "", float64(5), true}, []string{"a", "5"}},
		{"string slice", []string{" b ", " ", "c"}, []string{"b", "c"}},
		{"unsupported", 17, nil},
		{"nil", nil, nil},
	} {
		got := toStringSlice(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	}

	if got := firstNonEmptyString(nil, "  ", 3.0, "later"); got != "3" {
		t.Errorf("firstNonEmptyString should skip blanks and coerce numbers, got %q", got)
	}
	if got := firstNonEmptyString(nil, ""); got != "" {
		t.Errorf("firstNonEmptyString with nothing usable = %q", got)
	}
}

func TestCov1xAudienceContains(t *testing.T) {
	t.Parallel()
	if !audienceContains([]string{"x", " client "}, "client") {
		t.Error("audience entries are trimmed before comparison")
	}
	if audienceContains([]string{"x", "y"}, "client") {
		t.Error("an absent client id must not match")
	}
	if audienceContains([]string{""}, "  ") {
		t.Error("a blank expected audience must never match, even a blank entry")
	}
}

func TestCov1xIsUniqueConstraintViolation(t *testing.T) {
	t.Parallel()
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	if isUniqueConstraintViolation(nil, "users_email_key") {
		t.Error("nil is not a violation")
	}
	if isUniqueConstraintViolation(errors.New("plain"), "") {
		t.Error("a non-postgres error is not a violation")
	}
	if isUniqueConstraintViolation(&pgconn.PgError{Code: "23503", ConstraintName: "users_email_key"}, "users_email_key") {
		t.Error("a foreign-key violation is not a unique violation")
	}
	if !isUniqueConstraintViolation(unique, "") {
		t.Error("an empty constraint name matches any unique violation")
	}
	if !isUniqueConstraintViolation(errors.Join(errors.New("ctx"), unique), "users_email_key") {
		t.Error("a wrapped unique violation on the named constraint matches")
	}
	if isUniqueConstraintViolation(unique, "some_other_key") {
		t.Error("a different constraint must not match")
	}
}

func TestCov1xParseUUIDHelpers(t *testing.T) {
	t.Parallel()
	const id = "11111111-2222-3333-4444-555555555555"

	if got, err := parseRequiredUUID("  "+id+" ", "user_id"); err != nil || got != id {
		t.Fatalf("required uuid should be trimmed: %q %v", got, err)
	}
	if _, err := parseRequiredUUID(" ", "user_id"); err == nil || err.Error() != "user_id is required when persistent sessions are enabled" {
		t.Fatalf("blank required uuid: %v", err)
	}
	if _, err := parseRequiredUUID("nope", "user_id"); err == nil {
		t.Fatal("a non-uuid must be rejected")
	}

	if got, err := parseOptionalUUID("", "org_id"); err != nil || got != nil {
		t.Fatalf("blank optional uuid is nil: %v %v", got, err)
	}
	if got, err := parseOptionalUUID(id, "org_id"); err != nil || got == nil || *got != id {
		t.Fatalf("optional uuid: %v %v", got, err)
	}
	if _, err := parseOptionalUUID("nope", "org_id"); err == nil {
		t.Fatal("a non-uuid optional must be rejected")
	}

	if nullableString("  ") != nil {
		t.Error("blank string becomes NULL")
	}
	if p := nullableString(" tok "); p == nil || *p != "tok" {
		t.Errorf("nullableString should trim, got %v", p)
	}
}

func TestCov1xTOTPPrimitives(t *testing.T) {
	t.Parallel()
	secret := generateTOTPSecret()
	now := time.Unix(1_800_000_015, 0).UTC()
	code := generateTOTP(secret, now.Unix())
	if len(code) != 6 {
		t.Fatalf("code should be six digits: %q", code)
	}

	step, ok := verifyTOTPStep(secret, code, now)
	if !ok || step != now.Unix()/30 {
		t.Fatalf("current code should verify at the current step: %d %v", step, ok)
	}
	if _, ok := verifyTOTPStep("", code, now); ok {
		t.Error("an empty secret never verifies")
	}
	if _, ok := verifyTOTPStep(secret, "12345", now); ok {
		t.Error("a code that is not six characters never verifies")
	}
	if got := generateTOTP("not*base32!", now.Unix()); got != "" {
		t.Errorf("an undecodable secret yields no code, got %q", got)
	}
}

func TestCov1xMiscHelpers(t *testing.T) {
	t.Parallel()
	if clonePrincipal(nil) != nil {
		t.Error("cloning nil gives nil")
	}
	orig := &appmiddleware.Principal{UserID: "u", Roles: []string{"admin"}}
	cp := clonePrincipal(orig)
	cp.Roles[0] = "parent"
	if orig.Roles[0] != "admin" {
		t.Error("a cloned principal must not share its roles slice")
	}
	if randomAlphaNumeric(0) != "" || randomAlphaNumeric(-3) != "" {
		t.Error("non-positive length yields an empty string")
	}
	if got := randomAlphaNumeric(12); len(got) != 12 {
		t.Errorf("length mismatch: %q", got)
	}
	if got := hashToken("  secret "); got != hashToken("secret") {
		t.Error("tokens are trimmed before hashing")
	}
	if !hasRole([]string{"Admin"}, "admin") || hasRole([]string{"coach"}, "admin") {
		t.Error("hasRole is case-insensitive and exact")
	}
	if got := surrogateSyncedUserEmail(" ABC "); got != "abc@users.local.invalid" {
		t.Errorf("surrogate email = %q", got)
	}
	if marshalBackupCodes(nil) != nil {
		t.Error("no backup codes marshals to nil")
	}
}

func TestCov1xNormalizeLocalDevOrgID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, user, email, org, want string
	}{
		{"real org untouched", "u", "a@aquify.local", " 9f0b ", "9f0b"},
		{"legacy org + local dev user id", LocalDevAdminUserID, "x@example.com", "org-demo", LocalDevOrgID},
		{"legacy org + aquify.local email", "someone-else", " Person@AQUIFY.local ", "ORG-DEMO", LocalDevOrgID},
		{"legacy org + unrelated user stays legacy", "someone-else", "p@example.com", "org-demo", "org-demo"},
	}
	for _, tc := range tests {
		if got := normalizeLocalDevOrgID(tc.user, tc.email, tc.org); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCov1xAssignRoleValidatesAndListsNewestFirst(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{})
	valid := RoleAssignmentInput{OrgID: "org-1", TargetUserID: "u1", Role: " COACH ", Reason: "season start", ActorUserID: "admin-1"}

	for name, mutate := range map[string]func(*RoleAssignmentInput){
		"missing org":    func(in *RoleAssignmentInput) { in.OrgID = " " },
		"missing target": func(in *RoleAssignmentInput) { in.TargetUserID = "" },
		"missing role":   func(in *RoleAssignmentInput) { in.Role = "" },
		"missing reason": func(in *RoleAssignmentInput) { in.Reason = "" },
		"missing actor":  func(in *RoleAssignmentInput) { in.ActorUserID = "" },
	} {
		in := valid
		mutate(&in)
		if _, err := svc.AssignRole(in); err == nil || err.Error() != "org_id, target_user_id, role, reason, and actor_user_id are required" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	bad := valid
	bad.Role = "owner"
	if _, err := svc.AssignRole(bad); err == nil || err.Error() != "unknown role" {
		t.Fatalf("a removed legacy role must be refused: %v", err)
	}
	if got := svc.ListRoleAssignments(""); len(got) != 0 {
		t.Fatalf("rejected assignments must not be stored: %+v", got)
	}

	first, err := svc.AssignRole(valid)
	if err != nil || first.Role != "coach" || first.ScopeType != "org" {
		t.Fatalf("valid assignment should normalise role and default the scope: %+v %v", first, err)
	}
	other := valid
	other.OrgID = "ORG-2"
	other.ScopeType = " Group "
	other.ScopeID = " g1 "
	second, err := svc.AssignRole(other)
	if err != nil || second.ScopeType != "group" || second.ScopeID != "g1" {
		t.Fatalf("scoped assignment: %+v %v", second, err)
	}

	// Make the ordering explicit instead of relying on wall-clock resolution.
	svc.mu.Lock()
	svc.assignments[0].AssignedAt = time.Unix(1_000, 0).UTC()
	svc.assignments[1].AssignedAt = time.Unix(2_000, 0).UTC()
	svc.mu.Unlock()

	all := svc.ListRoleAssignments("")
	if len(all) != 2 || all[0].OrgID != "ORG-2" || all[1].OrgID != "org-1" {
		t.Fatalf("assignments must be newest first: %+v", all)
	}
	scoped := svc.ListRoleAssignments(" org-2 ")
	if len(scoped) != 1 || scoped[0].OrgID != "ORG-2" {
		t.Fatalf("org filter is case-insensitive and trimmed: %+v", scoped)
	}
}
