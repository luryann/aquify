package app

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/authn"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1xIdP is a minimal OpenID provider: it serves a JWKS and exchanges an
// authorization code for an RS256 ID token whose claims the test chooses.
type cov1xIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	claims map[string]map[string]any // by authorization code
}

const cov1xKID = "cov1x-kid"

func cov1xNewIdP(t *testing.T) *cov1xIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &cov1xIdP{key: key, claims: map[string]map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": cov1xKID, "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		idp.mu.Lock()
		claims := idp.claims[r.Form.Get("code")]
		idp.mu.Unlock()
		if claims == nil {
			http.Error(w, "unknown code", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "provider-access", "id_token": idp.sign(t, claims), "token_type": "Bearer", "expires_in": 3600,
		})
	})
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (p *cov1xIdP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": cov1xKID, "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// login performs a full authorize/callback round trip for subject and returns
// the principal the resulting session resolves to.
func (p *cov1xIdP) login(t *testing.T, svc *authn.Service, subject, orgID string, roles []string) (hh, swimmer string) {
	t.Helper()
	authURL, err := svc.BeginOIDCLogin("", orgID)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	state, nonce := u.Query().Get("state"), u.Query().Get("nonce")
	code := "code-" + subject
	now := time.Now().UTC()
	p.mu.Lock()
	p.claims[code] = map[string]any{
		"sub": subject, "email": subject + "@example.test", "iss": p.srv.URL, "aud": []string{"cov1x-client"},
		"exp": now.Add(10 * time.Minute).Unix(), "iat": now.Add(-time.Minute).Unix(),
		"nonce": nonce, "org_id": orgID, "roles": roles,
	}
	p.mu.Unlock()
	res, err := svc.OIDCCallback(code, state, "")
	if err != nil {
		t.Fatalf("oidc callback for %s: %v", subject, err)
	}
	principal, ok := svc.ResolvePrincipal(res.AccessToken)
	if !ok {
		t.Fatal("session did not resolve")
	}
	return principal.HouseholdID, principal.SwimmerID
}

func cov1xOIDCService(idp *cov1xIdP) *authn.Service {
	return authn.NewService(authn.Config{OIDC: authn.OIDCConfig{
		AuthorizeURL: idp.srv.URL + "/authorize", TokenURL: idp.srv.URL + "/token", JWKSURL: idp.srv.URL + "/jwks",
		RedirectURL: "http://localhost:3000/callback", ClientID: "cov1x-client", ClientSecret: "secret",
		ExpectedIssuer: idp.srv.URL, StateSigningKey: strings.Repeat("s", 32),
	}})
}

func TestCov1xCrossDomainHooksAttachLinkedAccountsAtLogin(t *testing.T) {
	const org = "org-cov1x-hooks"
	admin := cov1xAdmin(org)
	idp := cov1xNewIdP(t)
	logger, _ := cov1xLogger()

	membersSvc := members.NewService()
	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Linked Family", KeycloakUserID: "kc-parent"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Sam", LastName: "Linked", DateOfBirth: "2000-01-02", Gender: "male"})
	if err != nil {
		t.Fatal(err)
	}
	if err := membersSvc.LinkSwimmerToUser(org, sw.ID, "kc-swimmer"); err != nil {
		t.Fatal(err)
	}

	authSvc := cov1xOIDCService(idp)
	wireCrossDomainHooks(logger, authSvc, membersSvc, groups.NewService(), meets.NewService())
	if gotHH, gotSW := idp.login(t, authSvc, "kc-parent", org, []string{"parent"}); gotHH != hh.ID || gotSW != "" {
		t.Fatalf("parent login: household %q (want %q), swimmer %q (want none)", gotHH, hh.ID, gotSW)
	}
	if gotHH, gotSW := idp.login(t, authSvc, "kc-swimmer", org, []string{"swimmer"}); gotHH != "" || gotSW != sw.ID {
		t.Fatalf("swimmer login: household %q, swimmer %q (want %q)", gotHH, gotSW, sw.ID)
	}
	// An account nobody linked gets neither.
	if gotHH, gotSW := idp.login(t, authSvc, "kc-stranger", org, []string{"parent"}); gotHH != "" || gotSW != "" {
		t.Fatalf("stranger login: %q %q", gotHH, gotSW)
	}

	// Without a members service the lookups resolve nothing instead of failing the login.
	bare := cov1xOIDCService(idp)
	wireCrossDomainHooks(logger, bare, nil, nil, nil)
	if gotHH, gotSW := idp.login(t, bare, "kc-parent", org, []string{"parent"}); gotHH != "" || gotSW != "" {
		t.Fatalf("members-less login: %q %q", gotHH, gotSW)
	}
}

func TestCov1xSwimmerCreatedHookLogsInviteFailureWithoutFailingCreate(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	admin := cov1xAdmin(org)
	logger, logs := cov1xLogger()

	membersSvc := members.NewServiceWithPool(pool)
	// Meets is wired to a database that is already gone, so inviting the new
	// swimmer to open meets fails; creating the swimmer must still succeed.
	meetsSvc := meets.NewServiceWithPool(cov1xClosedCopy(t, pool))
	wireCrossDomainHooks(logger, authn.NewService(authn.Config{}), membersSvc, nil, meetsSvc)

	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Hook Family"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Hook", LastName: "Swimmer", DateOfBirth: "2012-01-02", Gender: "female"})
	if err != nil {
		t.Fatalf("swimmer creation must not depend on the invite hook: %v", err)
	}
	if !strings.Contains(logs.String(), "invite new swimmer to open meets failed") || !strings.Contains(logs.String(), sw.ID) {
		t.Fatalf("expected the failed invite to be logged with the swimmer id, got %q", logs.String())
	}
	if _, ok := membersSvc.SwimmerHouseholdID(org, sw.ID); !ok {
		t.Fatal("swimmer was not persisted")
	}
}
