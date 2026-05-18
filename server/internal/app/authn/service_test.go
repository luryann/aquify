package authn

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func TestResolvePrincipalDisablesStubTokensWhenNotConfigured(t *testing.T) {
	t.Parallel()

	service := NewService(Config{EnableDevStubTokens: false})
	if _, ok := service.ResolvePrincipal("stub-admin"); ok {
		t.Fatal("expected stub token to be rejected when dev stubs are disabled")
	}
}

func TestResolvePrincipalAllowsStubTokensWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService(Config{EnableDevStubTokens: true})
	principal, ok := service.ResolvePrincipal("stub-admin")
	if !ok || principal == nil {
		t.Fatal("expected stub token to resolve when dev stubs are enabled")
	}
	if principal.UserID != LocalDevAdminUserID {
		t.Fatalf("expected %s, got %q", LocalDevAdminUserID, principal.UserID)
	}
}

func TestIssueSessionNormalizesLegacyLocalDevOrgID(t *testing.T) {
	t.Parallel()

	service := NewService(Config{})
	session, err := service.IssueSession(SessionInput{
		UserID: LocalDevAdminUserID,
		Email:  "admin@dare.local",
		Name:   "Dev Admin",
		OrgID:  "org-demo",
		Roles:  []string{"admin"},
	})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	principal, ok := service.ResolvePrincipal(session.AccessToken)
	if !ok || principal == nil {
		t.Fatal("expected issued session to resolve")
	}
	if principal.OrgID != LocalDevOrgID {
		t.Fatalf("expected normalized org_id %q, got %q", LocalDevOrgID, principal.OrgID)
	}
}

func TestIssueSessionPreservesLegacyOrgIDForNonLocalUsers(t *testing.T) {
	t.Parallel()

	service := NewService(Config{})
	session, err := service.IssueSession(SessionInput{
		UserID: "user-123",
		Email:  "user@example.com",
		OrgID:  "org-demo",
		Roles:  []string{"admin"},
	})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	principal, ok := service.ResolvePrincipal(session.AccessToken)
	if !ok || principal == nil {
		t.Fatal("expected issued session to resolve")
	}
	if principal.OrgID != "org-demo" {
		t.Fatalf("expected non-local org_id to stay %q, got %q", "org-demo", principal.OrgID)
	}
}

func TestChallengeMFARejectsStaticCodeAndAcceptsTOTP(t *testing.T) {
	t.Parallel()

	service := NewService(Config{EnableDevStubTokens: true})
	factor, err := service.EnrollMFA(LocalDevAdminUserID, "admin@dare.local", LocalDevOrgID, "totp", "Primary authenticator")
	if err != nil {
		t.Fatalf("failed to enroll mfa factor: %v", err)
	}
	if factor.ChallengeHint == "" {
		t.Fatal("expected challenge hint for local/dev enrollment")
	}

	principal := &appmiddleware.Principal{
		UserID: LocalDevAdminUserID,
		OrgID:  LocalDevOrgID,
		Roles:  []string{"admin"},
	}

	if _, err := service.ChallengeMFA("", principal, factor.ID, "123456"); err == nil {
		t.Fatal("expected static fallback code to be rejected")
	}

	validCode := generateTOTP(factor.ChallengeHint, time.Now().UTC().Unix())
	if validCode == "" {
		t.Fatal("expected generated totp code")
	}

	session, err := service.ChallengeMFA("", principal, factor.ID, validCode)
	if err != nil {
		t.Fatalf("expected valid totp code to pass, got error: %v", err)
	}
	if session.AccessToken == "" {
		t.Fatal("expected challenge flow to issue step-up access token")
	}
}

func TestChallengeMFASupportsAcceptedFactorsOnly(t *testing.T) {
	t.Parallel()

	service := NewService(Config{EnableDevStubTokens: true})
	principal := &appmiddleware.Principal{
		UserID: LocalDevAdminUserID,
		OrgID:  LocalDevOrgID,
		Roles:  []string{"admin"},
	}

	totpFactor, err := service.EnrollMFA(principal.UserID, "", principal.OrgID, "totp", "Authenticator")
	if err != nil {
		t.Fatalf("totp enrollment should succeed: %v", err)
	}
	totpCode := generateTOTP(totpFactor.ChallengeHint, time.Now().UTC().Unix())
	if _, err := service.ChallengeMFA("", principal, totpFactor.ID, totpCode); err != nil {
		t.Fatalf("totp challenge should succeed: %v", err)
	}

	backupFactor, err := service.EnrollMFA(principal.UserID, "", principal.OrgID, "backup_code", "Backup code")
	if err != nil {
		t.Fatalf("backup code enrollment should succeed: %v", err)
	}
	if len(backupFactor.Codes) == 0 {
		t.Fatal("backup code enrollment should return plaintext codes")
	}
	firstCode := backupFactor.Codes[0]
	if _, err := service.ChallengeMFA("", principal, backupFactor.ID, firstCode); err != nil {
		t.Fatalf("backup code challenge should succeed: %v", err)
	}
	if _, err := service.ChallengeMFA("", principal, backupFactor.ID, firstCode); err == nil {
		t.Fatal("backup code should be one-time use")
	}

	if _, err := service.EnrollMFA(principal.UserID, "", principal.OrgID, "webauthn", "Not supported"); err == nil {
		t.Fatal("unsupported factor should be rejected")
	}
}

func TestOIDCCallbackRejectsTamperedIDTokenPayload(t *testing.T) {
	t.Parallel()

	privateKey, jwk := generateJWK(t, "kid-1")
	now := time.Now().UTC()
	tokenClaims := map[string]any{
		"sub":     "user-123",
		"iss":     "https://issuer.example.com",
		"aud":     []string{"dare-client"},
		"exp":     now.Add(10 * time.Minute).Unix(),
		"iat":     now.Add(-1 * time.Minute).Unix(),
		"nonce":   "nonce-123",
		"org_id":  "org-secure",
		"roles":   []string{"parent"},
		"typ":     "JWT",
		"ignored": "value",
	}
	validToken := signedIDToken(t, privateKey, "kid-1", tokenClaims)
	parts := strings.Split(validToken, ".")
	tamperedPayload := map[string]any{}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if err := json.Unmarshal(payloadBytes, &tamperedPayload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	tamperedPayload["org_id"] = "org-attacker"
	tamperedPayloadBytes, err := json.Marshal(tamperedPayload)
	if err != nil {
		t.Fatalf("failed to marshal tampered payload: %v", err)
	}
	tamperedToken := parts[0] + "." + base64.RawURLEncoding.EncodeToString(tamperedPayloadBytes) + "." + parts[2]

	service := NewService(Config{
		OIDC: OIDCConfig{
			TokenURL:        "https://oidc.test/token",
			JWKSURL:         "https://oidc.test/jwks",
			RedirectURL:     "http://localhost:3000/callback",
			ClientID:        "dare-client",
			ClientSecret:    "secret",
			ExpectedIssuer:  "https://issuer.example.com",
			StateSigningKey: "state-signing-key",
		},
	})
	service.httpClient = oidcTestClient(t, tamperedToken, []oidcJWK{jwk})
	state := signedState(t, "state-signing-key", oidcStatePayload{
		Nonce: "nonce-123",
		Exp:   now.Add(5 * time.Minute).Unix(),
	})

	if _, err := service.OIDCCallback("auth-code", state, "org-untrusted"); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification failure, got: %v", err)
	}
}

func TestOIDCCallbackRejectsUntrustedBodyOrgFallback(t *testing.T) {
	t.Parallel()

	privateKey, jwk := generateJWK(t, "kid-2")
	now := time.Now().UTC()
	idToken := signedIDToken(t, privateKey, "kid-2", map[string]any{
		"sub":   "user-123",
		"iss":   "https://issuer.example.com",
		"aud":   []string{"dare-client"},
		"exp":   now.Add(10 * time.Minute).Unix(),
		"iat":   now.Add(-1 * time.Minute).Unix(),
		"nonce": "nonce-abc",
		"roles": []string{"parent"},
	})

	service := NewService(Config{
		OIDC: OIDCConfig{
			TokenURL:        "https://oidc.test/token",
			JWKSURL:         "https://oidc.test/jwks",
			RedirectURL:     "http://localhost:3000/callback",
			ClientID:        "dare-client",
			ClientSecret:    "secret",
			ExpectedIssuer:  "https://issuer.example.com",
			StateSigningKey: "state-signing-key",
		},
	})
	service.httpClient = oidcTestClient(t, idToken, []oidcJWK{jwk})
	state := signedState(t, "state-signing-key", oidcStatePayload{
		Nonce: "nonce-abc",
		Exp:   now.Add(5 * time.Minute).Unix(),
	})

	if _, err := service.OIDCCallback("auth-code", state, "org-body-value"); err == nil || !strings.Contains(err.Error(), "trusted oidc claims/state") {
		t.Fatalf("expected trusted org resolution error, got: %v", err)
	}
}

func TestOIDCCallbackNormalizesLegacyLocalDevOrgClaim(t *testing.T) {
	t.Parallel()

	privateKey, jwk := generateJWK(t, "kid-localdev")
	now := time.Now().UTC()
	idToken := signedIDToken(t, privateKey, "kid-localdev", map[string]any{
		"sub":    LocalDevAdminUserID,
		"email":  "admin@dare.local",
		"iss":    "https://issuer.example.com",
		"aud":    []string{"dare-client"},
		"exp":    now.Add(10 * time.Minute).Unix(),
		"iat":    now.Add(-1 * time.Minute).Unix(),
		"nonce":  "nonce-localdev",
		"org_id": "org-demo",
		"roles":  []string{"admin"},
	})

	service := NewService(Config{
		OIDC: OIDCConfig{
			TokenURL:        "https://oidc.test/token",
			JWKSURL:         "https://oidc.test/jwks",
			RedirectURL:     "http://localhost:3000/callback",
			ClientID:        "dare-client",
			ClientSecret:    "secret",
			ExpectedIssuer:  "https://issuer.example.com",
			StateSigningKey: "state-signing-key",
		},
	})
	service.httpClient = oidcTestClient(t, idToken, []oidcJWK{jwk})
	state := signedState(t, "state-signing-key", oidcStatePayload{
		Nonce: "nonce-localdev",
		Exp:   now.Add(5 * time.Minute).Unix(),
		OrgID: LocalDevOrgID,
	})

	result, err := service.OIDCCallback("auth-code", state, "")
	if err != nil {
		t.Fatalf("oidc callback: %v", err)
	}
	if result.OrgID != LocalDevOrgID {
		t.Fatalf("expected normalized org_id %q, got %q", LocalDevOrgID, result.OrgID)
	}

	principal, ok := service.ResolvePrincipal(result.AccessToken)
	if !ok || principal == nil {
		t.Fatal("expected oidc-issued session to resolve")
	}
	if principal.OrgID != LocalDevOrgID {
		t.Fatalf("expected normalized principal org_id %q, got %q", LocalDevOrgID, principal.OrgID)
	}
}

func TestLogoutSessionReturnsOIDCLogoutURL(t *testing.T) {
	t.Parallel()

	service := NewService(Config{
		OIDC: OIDCConfig{
			EndSessionURL:         "https://issuer.example.com/logout",
			PostLogoutRedirectURL: "http://localhost:3000/login",
		},
	})
	session, err := service.IssueSession(SessionInput{
		UserID:      "user-123",
		OrgID:       "org-demo",
		Roles:       []string{"parent"},
		OIDCIDToken: "id-token-value",
	})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	result := service.LogoutSession(session.RefreshToken)
	if result.RedirectURL != "/login" {
		t.Fatalf("expected /login fallback redirect, got %q", result.RedirectURL)
	}
	if result.LogoutURL == "" {
		t.Fatal("expected logout_url to be populated")
	}

	logoutURL, err := url.Parse(result.LogoutURL)
	if err != nil {
		t.Fatalf("parse logout url: %v", err)
	}
	if logoutURL.Scheme != "https" || logoutURL.Host != "issuer.example.com" || logoutURL.Path != "/logout" {
		t.Fatalf("unexpected logout url: %s", result.LogoutURL)
	}
	if got := logoutURL.Query().Get("id_token_hint"); got != "id-token-value" {
		t.Fatalf("expected id_token_hint=id-token-value, got %q", got)
	}
	if got := logoutURL.Query().Get("post_logout_redirect_uri"); got != "http://localhost:3000/login" {
		t.Fatalf("expected post_logout_redirect_uri, got %q", got)
	}
	if _, ok := service.ResolvePrincipal(session.AccessToken); ok {
		t.Fatal("expected local session to be revoked during logout")
	}
}

func TestLogoutSessionFallsBackWithoutOIDCIDToken(t *testing.T) {
	t.Parallel()

	service := NewService(Config{
		OIDC: OIDCConfig{
			EndSessionURL:         "https://issuer.example.com/logout",
			PostLogoutRedirectURL: "http://localhost:3000/login",
		},
	})
	session, err := service.IssueSession(SessionInput{
		UserID: "user-123",
		OrgID:  "org-demo",
		Roles:  []string{"parent"},
	})
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	result := service.LogoutSession(session.RefreshToken)
	if result.LogoutURL != "" {
		t.Fatalf("expected no logout_url fallback, got %q", result.LogoutURL)
	}
	if result.RedirectURL != "/login" {
		t.Fatalf("expected /login fallback redirect, got %q", result.RedirectURL)
	}
	if _, ok := service.ResolvePrincipal(session.AccessToken); ok {
		t.Fatal("expected local session to be revoked during logout fallback")
	}
}

func TestOIDCCallbackLogoutAllowsDifferentSubsequentUser(t *testing.T) {
	t.Parallel()

	privateKey, jwk := generateJWK(t, "kid-3")
	now := time.Now().UTC()
	idTokenByCode := map[string]string{
		"code-admin": signedIDToken(t, privateKey, "kid-3", map[string]any{
			"sub":    LocalDevAdminUserID,
			"iss":    "https://issuer.example.com",
			"aud":    []string{"dare-client"},
			"exp":    now.Add(10 * time.Minute).Unix(),
			"iat":    now.Add(-1 * time.Minute).Unix(),
			"nonce":  "nonce-admin",
			"org_id": LocalDevOrgID,
			"roles":  []string{"admin"},
		}),
		"code-swimmer": signedIDToken(t, privateKey, "kid-3", map[string]any{
			"sub":    LocalDevSwimmerUserID,
			"iss":    "https://issuer.example.com",
			"aud":    []string{"dare-client"},
			"exp":    now.Add(10 * time.Minute).Unix(),
			"iat":    now.Add(-1 * time.Minute).Unix(),
			"nonce":  "nonce-swimmer",
			"org_id": LocalDevOrgID,
			"roles":  []string{"swimmer"},
		}),
	}

	service := NewService(Config{
		OIDC: OIDCConfig{
			TokenURL:              "https://oidc.test/token",
			JWKSURL:               "https://oidc.test/jwks",
			RedirectURL:           "http://localhost:3000/callback",
			EndSessionURL:         "https://issuer.example.com/logout",
			PostLogoutRedirectURL: "http://localhost:3000/login",
			ClientID:              "dare-client",
			ClientSecret:          "secret",
			ExpectedIssuer:        "https://issuer.example.com",
			StateSigningKey:       "state-signing-key",
		},
	})
	service.httpClient = oidcTestClientForCodes(t, idTokenByCode, []oidcJWK{jwk})

	adminState := signedState(t, "state-signing-key", oidcStatePayload{
		Nonce: "nonce-admin",
		Exp:   now.Add(5 * time.Minute).Unix(),
	})
	adminSession, err := service.OIDCCallback("code-admin", adminState, "")
	if err != nil {
		t.Fatalf("oidc callback admin: %v", err)
	}

	logoutResult := service.LogoutSession(adminSession.RefreshToken)
	if logoutResult.LogoutURL == "" {
		t.Fatal("expected logout_url for oidc-backed session")
	}
	if _, ok := service.ResolvePrincipal(adminSession.AccessToken); ok {
		t.Fatal("expected old admin session to be revoked")
	}

	swimmerState := signedState(t, "state-signing-key", oidcStatePayload{
		Nonce: "nonce-swimmer",
		Exp:   now.Add(5 * time.Minute).Unix(),
	})
	swimmerSession, err := service.OIDCCallback("code-swimmer", swimmerState, "")
	if err != nil {
		t.Fatalf("oidc callback swimmer: %v", err)
	}

	principal, ok := service.ResolvePrincipal(swimmerSession.AccessToken)
	if !ok || principal == nil {
		t.Fatal("expected swimmer session to resolve after second login")
	}
	if principal.UserID != LocalDevSwimmerUserID {
		t.Fatalf("expected %s, got %q", LocalDevSwimmerUserID, principal.UserID)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != "swimmer" {
		t.Fatalf("expected swimmer role, got %#v", principal.Roles)
	}
}

func generateJWK(t *testing.T, kid string) (*rsa.PrivateKey, oidcJWK) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	eBytes := bigEndianBytes(privateKey.PublicKey.E)
	jwk := oidcJWK{
		Kid: kid,
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}
	return privateKey, jwk
}

func signedIDToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("failed to marshal token header: %v", err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal token payload: %v", err)
	}

	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := header + "." + payload

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("failed to sign id token: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func oidcTestClient(t *testing.T, idToken string, keys []oidcJWK) *http.Client {
	t.Helper()

	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/token":
				body, err := json.Marshal(map[string]any{
					"access_token": "provider-access-token",
					"id_token":     idToken,
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				if err != nil {
					t.Fatalf("marshal token response: %v", err)
				}
				return jsonHTTPResponse(req, body), nil
			case "/jwks":
				body, err := json.Marshal(oidcJWKSet{Keys: keys})
				if err != nil {
					t.Fatalf("marshal jwks response: %v", err)
				}
				return jsonHTTPResponse(req, body), nil
			default:
				t.Fatalf("unexpected oidc test request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}
}

func oidcTestClientForCodes(t *testing.T, idTokens map[string]string, keys []oidcJWK) *http.Client {
	t.Helper()

	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/token":
				if err := req.ParseForm(); err != nil {
					t.Fatalf("parse oidc token request: %v", err)
				}
				code := req.Form.Get("code")
				idToken, ok := idTokens[code]
				if !ok {
					t.Fatalf("unexpected oidc auth code: %s", code)
				}
				body, err := json.Marshal(map[string]any{
					"access_token": "provider-access-token-" + code,
					"id_token":     idToken,
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				if err != nil {
					t.Fatalf("marshal token response: %v", err)
				}
				return jsonHTTPResponse(req, body), nil
			case "/jwks":
				body, err := json.Marshal(oidcJWKSet{Keys: keys})
				if err != nil {
					t.Fatalf("marshal jwks response: %v", err)
				}
				return jsonHTTPResponse(req, body), nil
			default:
				t.Fatalf("unexpected oidc test request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonHTTPResponse(req *http.Request, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    req,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

func signedState(t *testing.T, signingKey string, payload oidcStatePayload) string {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal state payload: %v", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	h := hmac.New(sha256.New, []byte(signingKey))
	_, _ = h.Write([]byte(encodedPayload))
	signature := h.Sum(nil)
	return fmt.Sprintf("%s.%s", encodedPayload, base64.RawURLEncoding.EncodeToString(signature))
}

func bigEndianBytes(value int) []byte {
	if value <= 0 {
		return []byte{0}
	}
	out := make([]byte, 0, 4)
	for value > 0 {
		out = append([]byte{byte(value & 0xff)}, out...)
		value >>= 8
	}
	return out
}
