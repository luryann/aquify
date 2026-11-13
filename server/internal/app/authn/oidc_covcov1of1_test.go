package authn

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	cov1xClient = "aquify-client"
	cov1xIssuer = "https://issuer.example.com"
	cov1xKey    = "cov1x-state-signing-key"
)

func cov1xOIDCConfig() OIDCConfig {
	return OIDCConfig{
		AuthorizeURL:          "https://idp.test/auth",
		TokenURL:              "https://idp.test/token",
		JWKSURL:               "https://idp.test/jwks",
		EndSessionURL:         "https://idp.test/logout",
		RedirectURL:           "http://localhost:3000/callback",
		PostLogoutRedirectURL: "http://localhost:3000/login",
		ClientID:              cov1xClient,
		ClientSecret:          "secret",
		ExpectedIssuer:        cov1xIssuer,
		StateSigningKey:       cov1xKey,
	}
}

// cov1xLogBuffer is a concurrency-safe log sink so tests can assert on warnings.
type cov1xLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *cov1xLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *cov1xLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func cov1xLogger() (*slog.Logger, *cov1xLogBuffer) {
	buf := &cov1xLogBuffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func cov1xClaims(overrides map[string]any) map[string]any {
	now := time.Now().UTC()
	claims := map[string]any{
		"sub":    "cov1x-subject",
		"email":  "swimmer.parent@example.com",
		"name":   "Casey Parent",
		"iss":    cov1xIssuer,
		"aud":    []string{cov1xClient},
		"exp":    now.Add(10 * time.Minute).Unix(),
		"iat":    now.Add(-time.Minute).Unix(),
		"nonce":  "cov1x-nonce",
		"org_id": "cov1x-org",
		"roles":  []string{"parent"},
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	return claims
}

func cov1xState(t *testing.T, org string) string {
	t.Helper()
	return signedState(t, cov1xKey, oidcStatePayload{
		Nonce: "cov1x-nonce",
		Exp:   time.Now().UTC().Add(5 * time.Minute).Unix(),
		OrgID: org,
	})
}

func cov1xResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

type cov1xErrReader struct{}

func (cov1xErrReader) Read([]byte) (int, error) { return 0, errors.New("cov1x: connection reset") }

// cov1xCallbackFixture wires a service to a fake provider that serves one signed
// ID token and the matching JWKS.
type cov1xCallbackFixture struct {
	svc *Service
	key *rsa.PrivateKey
	jwk oidcJWK
}

func cov1xNewCallbackFixture(t *testing.T, cfg Config, claims map[string]any) *cov1xCallbackFixture {
	t.Helper()
	key, jwk := generateJWK(t, "cov1x-kid")
	if cfg.OIDC.TokenURL == "" {
		cfg.OIDC = cov1xOIDCConfig()
	}
	svc := NewService(cfg)
	svc.httpClient = oidcTestClient(t, signedIDToken(t, key, "cov1x-kid", claims), []oidcJWK{jwk})
	return &cov1xCallbackFixture{svc: svc, key: key, jwk: jwk}
}

func TestCov1xBeginOIDCLoginRejectsUnparseableAuthorizeURL(t *testing.T) {
	t.Parallel()
	cfg := cov1xOIDCConfig()
	cfg.AuthorizeURL = "http://[::1"
	svc := NewService(Config{OIDC: cfg})
	if got, err := svc.BeginOIDCLogin("", ""); err == nil || got != "" {
		t.Fatalf("an unparseable authorize URL must fail, got %q %v", got, err)
	}
}

func TestCov1xOIDCStateNonceAndBinding(t *testing.T) {
	t.Parallel()
	svc := NewService(Config{OIDC: cov1xOIDCConfig()})
	state := cov1xState(t, "")

	nonce, err := svc.OIDCStateNonce(state)
	if err != nil || nonce != "cov1x-nonce" {
		t.Fatalf("nonce: %q %v", nonce, err)
	}
	if _, err := svc.OIDCStateNonce("garbage"); !errors.Is(err, ErrOIDCStateInvalid) {
		t.Fatalf("a malformed state is ErrOIDCStateInvalid, got %v", err)
	}
	if err := svc.VerifyOIDCStateBinding("garbage", "cov1x-nonce"); !errors.Is(err, ErrOIDCStateInvalid) {
		t.Fatalf("binding with an invalid state: %v", err)
	}
	if err := svc.VerifyOIDCStateBinding(state, "   "); !errors.Is(err, ErrOIDCStateInvalid) {
		t.Fatalf("a blank cookie value never binds: %v", err)
	}
	if err := svc.VerifyOIDCStateBinding(state, "another-browser"); !errors.Is(err, ErrOIDCStateInvalid) {
		t.Fatalf("a different browser's nonce never binds: %v", err)
	}
	if err := svc.VerifyOIDCStateBinding(state, " cov1x-nonce "); err != nil {
		t.Fatalf("the initiating browser binds: %v", err)
	}
}

func TestCov1xOIDCCallbackRejectsBadInputsBeforeAnyNetworkCall(t *testing.T) {
	t.Parallel()
	configured := NewService(Config{OIDC: cov1xOIDCConfig()})
	configured.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("no provider call may happen for a rejected input")
		return nil, errors.New("unexpected")
	})}

	if _, err := configured.OIDCCallback("  ", "state", ""); err == nil || err.Error() != "code and state are required" {
		t.Fatalf("blank code: %v", err)
	}
	if _, err := configured.OIDCCallback("code", " ", ""); err == nil || err.Error() != "code and state are required" {
		t.Fatalf("blank state: %v", err)
	}
	if _, err := NewService(Config{}).OIDCCallback("code", "state", ""); !errors.Is(err, ErrOIDCNotConfigured) {
		t.Fatalf("unconfigured provider: %v", err)
	}
	if _, err := configured.OIDCCallback("code", "forged.state", ""); !errors.Is(err, ErrOIDCStateInvalid) {
		t.Fatalf("a forged state: %v", err)
	}
}

func TestCov1xOIDCCallbackValidatesTokenClaimsAgainstStateAndConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		overrides map[string]any
		wantErr   string
	}{
		{"nonce differs from state", map[string]any{"nonce": "someone-elses"}, "id_token nonce does not match state nonce"},
		{"issuer differs", map[string]any{"iss": "https://evil.example.com"}, "id_token issuer mismatch"},
		{"audience does not include the client", map[string]any{"aud": []string{"another-client"}}, "id_token audience mismatch"},
		{"multi audience with foreign azp", map[string]any{"aud": []string{cov1xClient, "other"}, "azp": "other"}, "id_token azp mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(tc.overrides))
			_, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("got %v, want %q", err, tc.wantErr)
			}
		})
	}

	t.Run("multi audience naming this client as azp is accepted", func(t *testing.T) {
		t.Parallel()
		f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{"aud": []string{cov1xClient, "other"}, "azp": cov1xClient}))
		res, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
		if err != nil || res.OrgID != "cov1x-org" || res.UserID != "cov1x-subject" {
			t.Fatalf("callback: %+v %v", res, err)
		}
	})
}

func TestCov1xOIDCCallbackFallsBackToStateOrgOnlyInDevAndWarns(t *testing.T) {
	t.Parallel()
	logger, logs := cov1xLogger()
	f := cov1xNewCallbackFixture(t, Config{EnableDevStubTokens: true, Logger: logger}, cov1xClaims(map[string]any{"org_id": nil}))

	res, err := f.svc.OIDCCallback("code", cov1xState(t, " org-from-state "), "")
	if err != nil {
		t.Fatalf("dev fallback: %v", err)
	}
	if res.OrgID != "org-from-state" {
		t.Fatalf("org should come from the signed state in dev, got %q", res.OrgID)
	}
	if !strings.Contains(logs.String(), "falling back to state parameter (dev-only)") {
		t.Fatalf("the fallback must be logged, got: %s", logs.String())
	}
	principal, ok := f.svc.ResolvePrincipal(res.AccessToken)
	if !ok || principal.OrgID != "org-from-state" {
		t.Fatalf("the issued session carries the resolved org: %+v %v", principal, ok)
	}

	// No org anywhere (claim and state both blank) is refused even in dev.
	f2 := cov1xNewCallbackFixture(t, Config{EnableDevStubTokens: true}, cov1xClaims(map[string]any{"org_id": nil}))
	if _, err := f2.svc.OIDCCallback("code", cov1xState(t, "  "), ""); err == nil || !strings.Contains(err.Error(), "org_id could not be resolved") {
		t.Fatalf("expected unresolved org error, got %v", err)
	}
}

func TestCov1xOIDCCallbackRoleHandling(t *testing.T) {
	t.Parallel()

	t.Run("a token with no roles at all is treated as a parent", func(t *testing.T) {
		t.Parallel()
		f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{"roles": nil}))
		res, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
		if err != nil || len(res.Roles) != 1 || res.Roles[0] != "parent" {
			t.Fatalf("legacy role-less token: %+v %v", res, err)
		}
	})

	t.Run("realm_access roles are filtered to app roles", func(t *testing.T) {
		t.Parallel()
		f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{
			"roles":        nil,
			"realm_access": map[string]any{"roles": []string{"offline_access", "COACH", "coach", "uma_authorization"}},
		}))
		res, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
		if err != nil || len(res.Roles) != 1 || res.Roles[0] != "coach" {
			t.Fatalf("realm_access roles: %+v %v", res, err)
		}
	})

	t.Run("linked household and swimmer are attached to the session", func(t *testing.T) {
		t.Parallel()
		f := cov1xNewCallbackFixture(t, Config{}, cov1xClaims(map[string]any{"roles": []string{"swimmer"}}))
		var looked []string
		f.svc.SetHouseholdLookup(func(id string) string { looked = append(looked, "h:"+id); return "household-9" })
		f.svc.SetSwimmerLookup(func(id string) string { looked = append(looked, "s:"+id); return "swimmer-9" })
		res, err := f.svc.OIDCCallback("code", cov1xState(t, ""), "")
		if err != nil {
			t.Fatalf("callback: %v", err)
		}
		p, ok := f.svc.ResolvePrincipal(res.AccessToken)
		if !ok || p.HouseholdID != "household-9" || p.SwimmerID != "swimmer-9" {
			t.Fatalf("lookups must be applied: %+v %v", p, ok)
		}
		if len(looked) != 2 || looked[0] != "h:cov1x-subject" || looked[1] != "s:cov1x-subject" {
			t.Fatalf("lookups keyed by the token subject: %v", looked)
		}
	})
}

func TestCov1xExchangeAuthorizationCodeFailureModes(t *testing.T) {
	t.Parallel()
	newSvc := func(rt func(*http.Request) (*http.Response, error)) *Service {
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = &http.Client{Transport: roundTripFunc(rt)}
		return svc
	}

	t.Run("request cannot be built", func(t *testing.T) {
		t.Parallel()
		cfg := cov1xOIDCConfig()
		cfg.TokenURL = "http://\x7f"
		if _, err := NewService(Config{OIDC: cfg}).exchangeOIDCAuthorizationCode("c"); err == nil {
			t.Fatal("a control character in the token URL must fail request construction")
		}
	})
	t.Run("transport failure is provider-unavailable", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(*http.Request) (*http.Response, error) { return nil, errors.New("dial refused") })
		if _, err := svc.exchangeOIDCAuthorizationCode("c"); !errors.Is(err, ErrOIDCProviderUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("body read failure", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(cov1xErrReader{}), Request: req}, nil
		})
		if _, err := svc.exchangeOIDCAuthorizationCode("c"); err == nil || !strings.Contains(err.Error(), "connection reset") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("5xx is provider-unavailable", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) { return cov1xResponse(req, 503, "down"), nil })
		if _, err := svc.exchangeOIDCAuthorizationCode("c"); !errors.Is(err, ErrOIDCProviderUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("4xx is a plain failure", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) {
			return cov1xResponse(req, 400, `{"error":"invalid_grant"}`), nil
		})
		_, err := svc.exchangeOIDCAuthorizationCode("c")
		if err == nil || errors.Is(err, ErrOIDCProviderUnavailable) || !strings.Contains(err.Error(), "status 400") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("non-json body", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) { return cov1xResponse(req, 200, "<html>"), nil })
		if _, err := svc.exchangeOIDCAuthorizationCode("c"); err == nil {
			t.Fatal("non-JSON token response must fail")
		}
	})
	t.Run("missing id_token", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) {
			return cov1xResponse(req, 200, `{"access_token":"a","id_token":"  "}`), nil
		})
		if _, err := svc.exchangeOIDCAuthorizationCode("c"); err == nil || err.Error() != "oidc token response did not include id_token" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a well formed response carries the form fields", func(t *testing.T) {
		t.Parallel()
		svc := newSvc(func(req *http.Request) (*http.Response, error) {
			if err := req.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if req.Form.Get("grant_type") != "authorization_code" || req.Form.Get("code") != "the-code" || req.Form.Get("client_id") != cov1xClient {
				t.Errorf("unexpected token request form: %v", req.Form)
			}
			return cov1xResponse(req, 200, `{"id_token":"x.y.z","token_type":"Bearer"}`), nil
		})
		got, err := svc.exchangeOIDCAuthorizationCode("  the-code ")
		if err != nil || got.IDToken != "x.y.z" {
			t.Fatalf("got %+v %v", got, err)
		}
	})
}

func TestCov1xBuildOIDCLogoutURL(t *testing.T) {
	t.Parallel()
	cfg := cov1xOIDCConfig()
	cfg.EndSessionURL = "http://[::1"
	if u, ok := NewService(Config{OIDC: cfg}).buildOIDCLogoutURL("idtok"); ok || u != "" {
		t.Fatalf("an unparseable end-session URL yields no logout URL: %q %v", u, ok)
	}

	cfg = cov1xOIDCConfig()
	cfg.PostLogoutRedirectURL = ""
	u, ok := NewService(Config{OIDC: cfg}).buildOIDCLogoutURL(" idtok ")
	if !ok || !strings.Contains(u, "id_token_hint=idtok") || strings.Contains(u, "post_logout_redirect_uri") {
		t.Fatalf("logout URL without a post-logout redirect: %q %v", u, ok)
	}
}

// cov1xRawToken assembles a three-part token from raw pieces without signing.
func cov1xRawToken(header, payload any, sig []byte) string {
	enc := func(v any) string {
		switch typed := v.(type) {
		case string:
			return typed
		default:
			raw, _ := json.Marshal(typed)
			return base64.RawURLEncoding.EncodeToString(raw)
		}
	}
	return enc(header) + "." + enc(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestCov1xVerifyIDTokenRejectsMalformedTokens(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	svc := NewService(Config{OIDC: cov1xOIDCConfig()})
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("structural rejections must happen before any JWKS fetch")
		return nil, errors.New("unexpected")
	})}
	hdr := map[string]any{"alg": "RS256", "kid": "k"}
	okClaims := func(over map[string]any) map[string]any { return cov1xClaims(over) }
	b64 := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

	tests := []struct {
		name    string
		token   string
		wantErr string
	}{
		{"two parts", "a.b", "id_token format invalid"},
		{"header not base64", cov1xRawToken("***", okClaims(nil), nil), "id_token header decode failed"},
		{"payload not base64", cov1xRawToken(hdr, "***", nil), "id_token payload decode failed"},
		{"signature not base64", cov1xRawToken(hdr, okClaims(nil), nil) + "!", "id_token signature decode failed"},
		{"header not json", cov1xRawToken(b64("nope"), okClaims(nil), nil), "id_token header invalid"},
		{"unsupported alg", cov1xRawToken(map[string]any{"alg": "HS256", "kid": "k"}, okClaims(nil), nil), "id_token uses unsupported signing algorithm"},
		{"alg none", cov1xRawToken(map[string]any{"alg": "none", "kid": "k"}, okClaims(nil), nil), "id_token uses unsupported signing algorithm"},
		{"missing kid", cov1xRawToken(map[string]any{"alg": "RS256", "kid": " "}, okClaims(nil), nil), "id_token header missing kid"},
		{"payload not json", cov1xRawToken(hdr, b64("nope"), nil), "id_token payload invalid"},
		{"missing sub", cov1xRawToken(hdr, okClaims(map[string]any{"sub": nil}), nil), "id_token missing sub"},
		{"missing iss", cov1xRawToken(hdr, okClaims(map[string]any{"iss": nil}), nil), "id_token missing iss"},
		{"missing exp", cov1xRawToken(hdr, okClaims(map[string]any{"exp": nil}), nil), "id_token missing exp"},
		{"missing iat", cov1xRawToken(hdr, okClaims(map[string]any{"iat": nil}), nil), "id_token missing iat"},
		{"expired", cov1xRawToken(hdr, okClaims(map[string]any{"exp": now.Add(-time.Minute).Unix()}), nil), "id_token is expired"},
		{"issued in the future", cov1xRawToken(hdr, okClaims(map[string]any{"iat": now.Add(10 * time.Minute).Unix()}), nil), "id_token iat is in the future"},
	}
	for _, tc := range tests {
		if _, err := svc.verifyAndParseIDToken(tc.token, now); err == nil || err.Error() != tc.wantErr {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}

// cov1xJWKSServer serves a sequence of JWKS responses and counts fetches.
func cov1xJWKSClient(t *testing.T, fetches *atomic.Int32, responses ...func(*http.Request) (*http.Response, error)) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		i := int(fetches.Add(1)) - 1
		if i >= len(responses) {
			i = len(responses) - 1
		}
		return responses[i](req)
	})}
}

func cov1xJWKSBody(keys ...oidcJWK) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		raw, _ := json.Marshal(oidcJWKSet{Keys: keys})
		return cov1xResponse(req, 200, string(raw)), nil
	}
}

func TestCov1xVerifyIDTokenKeyResolution(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	key, jwk := generateJWK(t, "rotated")
	_, staleJWK := generateJWK(t, "old")
	token := signedIDToken(t, key, "rotated", cov1xClaims(nil))

	t.Run("a rotated key is found by forcing one JWKS refresh", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(staleJWK), cov1xJWKSBody(staleJWK, jwk))
		claims, err := svc.verifyAndParseIDToken(token, now)
		if err != nil || claims.Subject != "cov1x-subject" {
			t.Fatalf("verify: %+v %v", claims, err)
		}
		if n.Load() != 2 {
			t.Fatalf("expected the cached miss plus one forced refresh, got %d fetches", n.Load())
		}
	})
	t.Run("a kid missing even after the refresh is rejected", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(staleJWK))
		if _, err := svc.verifyAndParseIDToken(token, now); err == nil || err.Error() != "oidc jwk for token kid was not found" {
			t.Fatalf("got %v", err)
		}
		if n.Load() != 2 {
			t.Fatalf("exactly one forced refresh, got %d fetches", n.Load())
		}
	})
	t.Run("the forced refresh failing surfaces the provider outage", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(staleJWK), func(req *http.Request) (*http.Response, error) {
			return cov1xResponse(req, 502, "bad gateway"), nil
		})
		if _, err := svc.verifyAndParseIDToken(token, now); !errors.Is(err, ErrOIDCProviderUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("the first JWKS fetch failing is returned", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, func(req *http.Request) (*http.Response, error) {
			return cov1xResponse(req, 500, "boom"), nil
		})
		if _, err := svc.verifyAndParseIDToken(token, now); !errors.Is(err, ErrOIDCProviderUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a key with an undecodable modulus is rejected", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		broken := jwk
		broken.N = "***"
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(broken))
		if _, err := svc.verifyAndParseIDToken(token, now); err == nil || err.Error() != "oidc jwk modulus decode failed" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a signature made by a different key is rejected", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		forged := signedIDToken(t, otherKey, "rotated", cov1xClaims(nil))
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(jwk))
		if _, err := svc.verifyAndParseIDToken(forged, now); err == nil || err.Error() != "id_token signature verification failed" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("string and realm_access claim encodings are understood", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		now := time.Now().UTC()
		tok := signedIDToken(t, key, "rotated", cov1xClaims(map[string]any{
			"roles":              nil,
			"exp":                "9999999999",
			"iat":                json.Number("1700000000"),
			"aud":                cov1xClient,
			"preferred_username": "fallback@example.com",
			"email":              nil,
			"name":               nil,
			"given_name":         "Given",
			"realm_access":       map[string]any{"roles": []any{"admin"}},
		}))
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(jwk))
		claims, err := svc.verifyAndParseIDToken(tok, now)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		if claims.Email != "fallback@example.com" || claims.Name != "Given" || claims.Exp != 9_999_999_999 || claims.Iat != 1_700_000_000 {
			t.Fatalf("claim fallbacks: %+v", claims)
		}
		if len(claims.Aud) != 1 || claims.Aud[0] != cov1xClient || len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
			t.Fatalf("aud/roles: %+v", claims)
		}
	})
}

func TestCov1xFetchJWKSFailureModesAndCaching(t *testing.T) {
	t.Parallel()

	t.Run("request cannot be built", func(t *testing.T) {
		t.Parallel()
		cfg := cov1xOIDCConfig()
		cfg.JWKSURL = "http://\x7f"
		if _, err := NewService(Config{OIDC: cfg}).fetchOIDCJWKS(); err == nil {
			t.Fatal("a control character in the JWKS URL must fail")
		}
	})
	t.Run("transport failure", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, func(*http.Request) (*http.Response, error) { return nil, errors.New("no route") })
		if _, err := svc.fetchOIDCJWKS(); !errors.Is(err, ErrOIDCProviderUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("body read failure", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(cov1xErrReader{}), Request: req}, nil
		})
		if _, err := svc.fetchOIDCJWKS(); err == nil || !strings.Contains(err.Error(), "connection reset") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("4xx is a plain failure", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, func(req *http.Request) (*http.Response, error) { return cov1xResponse(req, 404, "nf"), nil })
		_, err := svc.fetchOIDCJWKS()
		if err == nil || errors.Is(err, ErrOIDCProviderUnavailable) || !strings.Contains(err.Error(), "status 404") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, func(req *http.Request) (*http.Response, error) { return cov1xResponse(req, 200, "{"), nil })
		if _, err := svc.fetchOIDCJWKS(); err == nil || err.Error() != "oidc jwks payload invalid" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an empty key set is refused and not cached", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody())
		if _, err := svc.fetchOIDCJWKS(); err == nil || err.Error() != "oidc jwks has no keys" {
			t.Fatalf("got %v", err)
		}
		if svc.jwksCache != nil {
			t.Fatal("a rejected key set must not be cached")
		}
	})
	t.Run("keys are cached until the TTL passes or a refresh is forced", func(t *testing.T) {
		t.Parallel()
		_, jwk := generateJWK(t, "cached")
		var n atomic.Int32
		svc := NewService(Config{OIDC: cov1xOIDCConfig()})
		svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(jwk))
		for i := 0; i < 3; i++ {
			set, err := svc.fetchOIDCJWKS()
			if err != nil || len(set.Keys) != 1 || set.Keys[0].Kid != "cached" {
				t.Fatalf("fetch %d: %+v %v", i, set, err)
			}
		}
		if n.Load() != 1 {
			t.Fatalf("a warm cache must not hit the network again, got %d fetches", n.Load())
		}
		svc.jwksMu.Lock()
		svc.jwksCachedAt = time.Now().Add(-2 * svc.jwksCacheTTL)
		svc.jwksMu.Unlock()
		if _, err := svc.fetchOIDCJWKS(); err != nil || n.Load() != 2 {
			t.Fatalf("a stale cache refetches: fetches=%d err=%v", n.Load(), err)
		}
		if _, err := svc.fetchOIDCJWKSInternal(true); err != nil || n.Load() != 3 {
			t.Fatalf("forceRefresh bypasses a warm cache: fetches=%d err=%v", n.Load(), err)
		}
	})
}

func TestCov1xVerifiedSignatureUsesRS256PKCS1(t *testing.T) {
	t.Parallel()
	// Guards the contract that tokens are checked as PKCS#1 v1.5 over SHA-256 of
	// "header.payload": a token whose payload is altered after signing fails even
	// though header, kid and JWKS are all valid.
	key, jwk := generateJWK(t, "k1")
	good := signedIDToken(t, key, "k1", cov1xClaims(nil))
	parts := strings.Split(good, ".")
	altered := cov1xClaims(map[string]any{"sub": "intruder"})
	rawAltered, _ := json.Marshal(altered)
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString(rawAltered) + "." + parts[2]

	var n atomic.Int32
	svc := NewService(Config{OIDC: cov1xOIDCConfig()})
	svc.httpClient = cov1xJWKSClient(t, &n, cov1xJWKSBody(jwk))
	if _, err := svc.verifyAndParseIDToken(forged, time.Now().UTC()); err == nil || err.Error() != "id_token signature verification failed" {
		t.Fatalf("altered payload must fail verification: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], cov1xMustDecode(t, parts[2])); err != nil {
		t.Fatalf("the original token verifies under PKCS#1 v1.5: %v", err)
	}
}

func cov1xMustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
