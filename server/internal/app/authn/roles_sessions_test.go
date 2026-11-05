package authn

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestFilterAppRolesDropsKeycloakDefaults(t *testing.T) {
	got := filterAppRoles([]string{"parent", "default-roles-aquify", "offline_access", "uma_authorization", "Parent"})
	if !slices.Equal(got, []string{"parent"}) {
		t.Fatalf("got %v", got)
	}
	if got := filterAppRoles([]string{"default-roles-aquify", "offline_access"}); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestOIDCCallbackFiltersRolesAndRejectsNoAppRole(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		roles   []string
		want    []string
		wantErr error
	}{
		{"filtered", []string{"coach", "default-roles-aquify", "offline_access"}, []string{"coach"}, nil},
		{"no app role", []string{"default-roles-aquify", "offline_access", "uma_authorization"}, nil, ErrNoAppRole},
	} {
		privateKey, jwk := generateJWK(t, "kid-r")
		now := time.Now().UTC()
		idToken := signedIDToken(t, privateKey, "kid-r", map[string]any{
			"sub": "user-123", "iss": "https://issuer.example.com", "aud": []string{"aquify-client"},
			"exp": now.Add(10 * time.Minute).Unix(), "iat": now.Add(-time.Minute).Unix(),
			"nonce": "nonce-abc", "org_id": "org-1", "roles": tc.roles,
		})
		service := NewService(Config{OIDC: OIDCConfig{
			TokenURL: "https://oidc.test/token", JWKSURL: "https://oidc.test/jwks",
			RedirectURL: "http://localhost:3000/callback", ClientID: "aquify-client", ClientSecret: "secret",
			ExpectedIssuer: "https://issuer.example.com", StateSigningKey: "state-signing-key",
		}})
		service.httpClient = oidcTestClient(t, idToken, []oidcJWK{jwk})
		state := signedState(t, "state-signing-key", oidcStatePayload{Nonce: "nonce-abc", Exp: now.Add(5 * time.Minute).Unix()})
		res, err := service.OIDCCallback("code", state, "")
		if !errors.Is(err, tc.wantErr) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
		}
		if tc.wantErr == nil && !slices.Equal(res.Roles, tc.want) {
			t.Fatalf("%s: roles %v, want %v", tc.name, res.Roles, tc.want)
		}
	}
}

func TestOIDCCallbackProviderDownIsUnavailableWithoutLeakingURL(t *testing.T) {
	t.Parallel()
	service := NewService(Config{OIDC: OIDCConfig{
		TokenURL: "http://127.0.0.1:1/token", JWKSURL: "http://127.0.0.1:1/jwks",
		RedirectURL: "http://localhost:3000/callback", ClientID: "aquify-client", ClientSecret: "secret",
		StateSigningKey: "state-signing-key",
	}})
	now := time.Now().UTC()
	state := signedState(t, "state-signing-key", oidcStatePayload{Nonce: "n", Exp: now.Add(5 * time.Minute).Unix()})
	_, err := service.OIDCCallback("code", state, "")
	if !errors.Is(err, ErrOIDCProviderUnavailable) {
		t.Fatalf("want provider unavailable, got %v", err)
	}
}

func TestRevokeUserSessionsInvalidatesExistingSessions(t *testing.T) {
	t.Parallel()
	service := NewService(Config{})
	mk := func(uid string) SessionTokens {
		s, err := service.IssueSession(SessionInput{UserID: uid, OrgID: "org-1", Roles: []string{"parent"}, ExpiresIn: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a1, a2, b := mk("user-a"), mk("user-a"), mk("user-b")
	n, err := service.RevokeUserSessions(context.Background(), "", "user-a")
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, s := range []SessionTokens{a1, a2} {
		if _, ok := service.ResolvePrincipal(s.AccessToken); ok {
			t.Fatal("revoked session still resolves")
		}
		if _, err := service.RefreshSession(s.RefreshToken); err == nil {
			t.Fatal("revoked session still refreshes")
		}
	}
	if _, ok := service.ResolvePrincipal(b.AccessToken); !ok {
		t.Fatal("other user's session affected")
	}
}
