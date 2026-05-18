package app

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

func issuedAccessTokenFromResponse(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()

	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == appmiddleware.AccessTokenCookieName {
			value := strings.TrimSpace(cookie.Value)
			if value == "" {
				t.Fatal("expected non-empty access token cookie")
			}
			return value
		}
	}

	t.Fatalf("expected %s cookie to be issued", appmiddleware.AccessTokenCookieName)
	return ""
}

func assertJSONOmitsSessionSecrets(t *testing.T, body []byte) {
	t.Helper()

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if jsonContainsKey(payload, "access_token") {
		t.Fatalf("response body leaked access_token: %s", string(body))
	}
	if jsonContainsKey(payload, "refresh_token") {
		t.Fatalf("response body leaked refresh_token: %s", string(body))
	}
}

func jsonContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed[key]; ok {
			return true
		}
		for _, nested := range typed {
			if jsonContainsKey(nested, key) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if jsonContainsKey(nested, key) {
				return true
			}
		}
	}
	return false
}

func TestIssueSessionResponseOmitsTokenSecrets(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", encodeJSONBody(t, map[string]any{
		"user_id": "cookie-user",
		"org_id":  "org-demo",
		"roles":   []string{"admin"},
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	assertJSONOmitsSessionSecrets(t, rr.Body.Bytes())

	var response struct {
		Status    string `json:"status"`
		CSRFToken string `json:"csrf_token"`
		Session   struct {
			ExpiresAt string `json:"expires_at"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	if response.Status != "issued" {
		t.Fatalf("expected issued status, got %q", response.Status)
	}
	if response.CSRFToken == "" || response.Session.ExpiresAt == "" {
		t.Fatalf("expected csrf token and session expiry metadata, got %#v", response)
	}
}

func TestRefreshSessionResponseOmitsTokenSecrets(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{AppEnv: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/refresh", nil)
	req.AddCookie(session.refreshCookie)
	req.AddCookie(session.csrfCookie)
	req.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	assertJSONOmitsSessionSecrets(t, rr.Body.Bytes())

	var response struct {
		Status    string `json:"status"`
		CSRFToken string `json:"csrf_token"`
		Session   struct {
			ExpiresAt string `json:"expires_at"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if response.Status != "refreshed" {
		t.Fatalf("expected refreshed status, got %q", response.Status)
	}
	if response.CSRFToken == "" || response.Session.ExpiresAt == "" {
		t.Fatalf("expected csrf token and session expiry metadata, got %#v", response)
	}
}

func TestMFAChallengeResponseOmitsTokenSecrets(t *testing.T) {
	t.Parallel()

	router := NewRouter(Config{
		AppEnv: "test",
		Auth: AuthConfig{
			EnableDevStubTokens: true,
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	session := issueSessionWithCookies(t, router)

	enrollReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/enroll", encodeJSONBody(t, map[string]any{
		"factor_type": "backup_code",
		"label":       "Primary backup",
	}))
	enrollReq.Header.Set("Content-Type", "application/json")
	enrollReq.Header.Set("X-Org-Id", "org-demo")
	enrollReq.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	enrollReq.AddCookie(session.accessCookie)
	enrollReq.AddCookie(session.csrfCookie)
	enrollRR := httptest.NewRecorder()
	router.ServeHTTP(enrollRR, enrollReq)
	if enrollRR.Code != http.StatusCreated {
		t.Fatalf("enroll mfa: expected 201, got %d: %s", enrollRR.Code, enrollRR.Body.String())
	}

	var enrollResp struct {
		Factor struct {
			ID    string   `json:"id"`
			Codes []string `json:"codes"`
		} `json:"factor"`
	}
	if err := json.Unmarshal(enrollRR.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("decode enroll response: %v", err)
	}
	if enrollResp.Factor.ID == "" || len(enrollResp.Factor.Codes) == 0 {
		t.Fatalf("expected challengeable MFA factor, got %#v", enrollResp)
	}

	challengeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/challenge", encodeJSONBody(t, map[string]any{
		"factor_id":      enrollResp.Factor.ID,
		"challenge_code": enrollResp.Factor.Codes[0],
	}))
	challengeReq.Header.Set("Content-Type", "application/json")
	challengeReq.Header.Set("X-Org-Id", "org-demo")
	challengeReq.Header.Set("X-CSRF-Token", session.csrfCookie.Value)
	challengeReq.AddCookie(session.accessCookie)
	challengeReq.AddCookie(session.csrfCookie)
	challengeRR := httptest.NewRecorder()
	router.ServeHTTP(challengeRR, challengeReq)
	if challengeRR.Code != http.StatusOK {
		t.Fatalf("challenge mfa: expected 200, got %d: %s", challengeRR.Code, challengeRR.Body.String())
	}

	assertJSONOmitsSessionSecrets(t, challengeRR.Body.Bytes())
}

func TestOIDCCallbackResponseOmitsTokenSecrets(t *testing.T) {
	t.Parallel()

	privateKey, jwk := generateOIDCTestJWK(t, "kid-1")
	nonce := "oidc-nonce"
	now := time.Now().UTC()
	idToken := signedOIDCTestToken(t, privateKey, jwk.Kid, map[string]any{
		"sub":    "oidc-user",
		"email":  "oidc-user@example.com",
		"name":   "OIDC User",
		"iss":    "https://issuer.example.com",
		"nonce":  nonce,
		"org_id": "org-oidc",
		"roles":  []string{"admin"},
		"aud":    []string{"aquify-admin"},
		"azp":    "aquify-admin",
		"iat":    now.Unix(),
		"exp":    now.Add(time.Hour).Unix(),
	})

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "provider-access-token",
				"id_token":     idToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{{
					"kid": jwk.Kid,
					"kty": jwk.Kty,
					"use": jwk.Use,
					"alg": jwk.Alg,
					"n":   jwk.N,
					"e":   jwk.E,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	router := NewRouter(Config{
		AppEnv: "test",
		Auth: AuthConfig{
			EnableDevStubTokens: false,
			OIDC: OIDCConfig{
				TokenURL:        provider.URL + "/token",
				JWKSURL:         provider.URL + "/jwks",
				RedirectURL:     "http://localhost:3000/auth/callback",
				ClientID:        "aquify-admin",
				ClientSecret:    "secret-value",
				ExpectedIssuer:  "https://issuer.example.com",
				StateSigningKey: "oidc-signing-key",
			},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	state := signOIDCTestState(t, "oidc-signing-key", map[string]any{
		"nonce":  nonce,
		"exp":    now.Add(5 * time.Minute).Unix(),
		"org_id": "org-oidc",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/callback", encodeJSONBody(t, map[string]any{
		"code":  "oidc-auth-code",
		"state": state,
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	assertJSONOmitsSessionSecrets(t, rr.Body.Bytes())

	var response struct {
		Status    string `json:"status"`
		CSRFToken string `json:"csrf_token"`
		User      struct {
			UserID string   `json:"user_id"`
			OrgID  string   `json:"org_id"`
			Roles  []string `json:"roles"`
		} `json:"user"`
		Session struct {
			ExpiresAt string `json:"expires_at"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode oidc callback response: %v", err)
	}
	if response.Status != "ok" || response.CSRFToken == "" || response.Session.ExpiresAt == "" {
		t.Fatalf("expected non-sensitive oidc response metadata, got %#v", response)
	}
	if response.User.UserID != "oidc-user" || response.User.OrgID != "org-oidc" {
		t.Fatalf("unexpected oidc user metadata: %#v", response.User)
	}
}

type oidcTestJWK struct {
	Kid string
	Kty string
	Use string
	Alg string
	N   string
	E   string
}

func generateOIDCTestJWK(t *testing.T, kid string) (*rsa.PrivateKey, oidcTestJWK) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return privateKey, oidcTestJWK{
		Kid: kid,
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(bigEndianBytesForOIDCTest(privateKey.PublicKey.E)),
	}
}

func signedOIDCTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("marshal token header: %v", err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal token claims: %v", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(claimsBytes)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign id token: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func signOIDCTestState(t *testing.T, signingKey string, payload map[string]any) string {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal oidc state payload: %v", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(signingKey))
	_, _ = mac.Write([]byte(encodedPayload))
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func bigEndianBytesForOIDCTest(value int) []byte {
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
