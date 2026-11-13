package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/offboarding"
	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/app/provisioning"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov5xPlatform is the operator plane over a real Postgres-backed tenant registry,
// the real provisioner/offboarding services, a stub identity provider and stub
// object storage (the true external boundaries).
type cov5xPlatform struct {
	h          *PlatformHandlers
	router     http.Handler
	reg        *orgs.Service
	kc         *keycloak.StubClient
	store      *storage.StubStorage
	audit      *audit.Service
	platformID string
	// now is the offboarding clock; tests advance it to pass the cooling-off period.
	now time.Time
	dns map[string][]string
}

// cov5xWho selects the principal the test middleware injects.
const (
	cov5xOperator = "operator"
	cov5xNoMFA    = "nomfa"
	cov5xPlain    = "plain"
	cov5xForeign  = "foreign"
)

func cov5xInject(platformID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var p *appmiddleware.Principal
			switch req.Header.Get("X-Test-Who") {
			case cov5xOperator:
				p = &appmiddleware.Principal{UserID: "op-1", OrgID: platformID, Roles: []string{PlatformRole}, MFAAuthenticated: true}
			case cov5xNoMFA:
				p = &appmiddleware.Principal{UserID: "op-2", OrgID: platformID, Roles: []string{PlatformRole}}
			case cov5xPlain:
				p = &appmiddleware.Principal{UserID: "adm", OrgID: platformID, Roles: []string{"admin"}, MFAAuthenticated: true}
			case cov5xForeign:
				p = &appmiddleware.Principal{UserID: "op-3", OrgID: uuid.NewString(), Roles: []string{PlatformRole}, MFAAuthenticated: true}
			}
			if p != nil {
				req = req.WithContext(appmiddleware.ContextWithPrincipal(req.Context(), p))
			}
			next.ServeHTTP(w, req)
		})
	}
}

func cov5xMount(h *PlatformHandlers, platformID string) http.Handler {
	r := chi.NewRouter()
	r.Use(cov5xInject(platformID))
	h.RegisterRoutes(r)
	return r
}

func cov5xNewPlatform(t *testing.T, withOffboarding bool) *cov5xPlatform {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	f := &cov5xPlatform{
		reg:        orgs.NewServiceWithPool(pool),
		kc:         keycloak.NewStubClient(),
		store:      storage.NewStubStorage("http://stub.invalid"),
		audit:      audit.NewService(),
		platformID: uuid.NewString(),
		now:        time.Now(),
		dns:        map[string][]string{},
	}
	f.reg.SetStatusCacheTTL(0)
	if _, err := f.reg.Create(ctx, orgs.CreateInput{ID: f.platformID, Slug: "platform", Name: "Aquify Platform"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reg.SetStatus(ctx, f.platformID, orgs.StatusActive); err != nil {
		t.Fatal(err)
	}
	prov := &provisioning.Provisioner{Orgs: f.reg, Keycloak: f.kc, Settings: orgsettings.NewService()}
	f.h = NewPlatformHandlers(f.reg, prov, f.audit, f.platformID)
	f.h.LookupTXT = func(_ context.Context, name string) ([]string, error) {
		if v, ok := f.dns[name]; ok {
			return v, nil
		}
		return nil, errors.New("no such host")
	}
	if withOffboarding {
		f.h.SetOffboarding(&offboarding.Service{
			Pool: pool, Orgs: f.reg, Store: f.store, Keycloak: f.kc,
			CoolingOff: time.Hour, Now: func() time.Time { return f.now }, PlatformOrgID: f.platformID,
		})
	}
	f.router = cov5xMount(f.h, f.platformID)
	return f
}

func (f *cov5xPlatform) do(who, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("X-Test-Who", who)
	}
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func (f *cov5xPlatform) op(method, path, body string) *httptest.ResponseRecorder {
	return f.do(cov5xOperator, method, path, body)
}

func (f *cov5xPlatform) provision(t *testing.T, slug, email string) string {
	t.Helper()
	rr := f.op("POST", "/platform/orgs", `{"name":"`+slug+` Club","slug":"`+slug+`","admin_email":"`+email+`","admin_first_name":"A","admin_last_name":"B"}`)
	cov5xWant(t, "provision "+slug, rr, http.StatusCreated, `"active"`)
	return cov5xParse(t, rr)["org"].(map[string]any)["id"].(string)
}

func (f *cov5xPlatform) auditActions(t *testing.T, action string) []audit.AuditEvent {
	t.Helper()
	evs, err := f.audit.Query(audit.Filter{OrgID: f.platformID, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestCov5xPlatformHelpers(t *testing.T) {
	t.Parallel()
	if got := DomainChallengeName("swim.example.org"); got != "_aquify-verify.swim.example.org" {
		t.Fatalf("challenge name: %q", got)
	}
	if got := DomainChallengeValue("org-1"); got != "aquify-verify=org-1" {
		t.Fatalf("challenge value: %q", got)
	}
	h := NewPlatformHandlers(orgs.NewService(), nil, nil, "  platform-org  ")
	if h.platformOrgID != "platform-org" {
		t.Fatalf("platform org id must be trimmed: %q", h.platformOrgID)
	}
	// The default resolver goes to the network; a cancelled context fails without touching it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.LookupTXT(ctx, "_aquify-verify.example.invalid"); err == nil {
		t.Fatal("a cancelled lookup must fail")
	}
}

func TestCov5xPlatformOffboardingErrorMapping(t *testing.T) {
	t.Parallel()
	h := NewPlatformHandlers(orgs.NewService(), nil, nil, "p")
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{orgs.ErrNotFound, http.StatusNotFound, "not_found"},
		{offboarding.ErrNotOffboarding, http.StatusConflict, "offboarding_precondition"},
		{offboarding.ErrCoolingOff, http.StatusConflict, "offboarding_precondition"},
		{offboarding.ErrProtectedOrg, http.StatusConflict, "offboarding_precondition"},
		{orgs.ErrInvalidStatus, http.StatusConflict, "invalid_transition"},
		{offboarding.ErrConfirmMismatch, http.StatusBadRequest, "confirmation_mismatch"},
		{offboarding.ErrNoDatabase, http.StatusNotImplemented, "offboarding_unavailable"},
		{offboarding.ErrStorageUnsupport, http.StatusNotImplemented, "offboarding_unavailable"},
		{errors.New("disk on fire"), http.StatusInternalServerError, "internal_error"},
	} {
		rr := httptest.NewRecorder()
		h.offboardingError(rr, httptest.NewRequest("POST", "/x", nil), tc.err)
		if rr.Code != tc.status || !strings.Contains(rr.Body.String(), `"`+tc.code+`"`) {
			t.Errorf("%v: got %d %s, want %d %s", tc.err, rr.Code, rr.Body.String(), tc.status, tc.code)
		}
		if strings.Contains(rr.Body.String(), "disk on fire") {
			t.Errorf("internal cause leaked: %s", rr.Body.String())
		}
	}
}

func TestCov5xPlatformRequiresAnOperator(t *testing.T) {
	f := cov5xNewPlatform(t, false)
	cov5xWant(t, "no principal", f.do("", "GET", "/platform/orgs", ""), http.StatusUnauthorized, "unauthorized")
	cov5xWant(t, "org admin", f.do(cov5xPlain, "GET", "/platform/orgs", ""), http.StatusForbidden, `"forbidden"`)
	cov5xWant(t, "foreign operator", f.do(cov5xForeign, "GET", "/platform/orgs", ""), http.StatusForbidden, `"forbidden"`)
	cov5xWant(t, "operator without mfa", f.do(cov5xNoMFA, "GET", "/platform/orgs", ""), http.StatusForbidden, "mfa_required")
	cov5xWant(t, "operator", f.do(cov5xOperator, "GET", "/platform/orgs", ""), http.StatusOK, `"count":1`)

	// A deployment without a platform org grants nobody operator access.
	unset := cov5xMount(NewPlatformHandlers(f.reg, nil, nil, ""), "")
	req := httptest.NewRequest("GET", "/platform/orgs", nil)
	req.Header.Set("X-Test-Who", cov5xOperator)
	rr := httptest.NewRecorder()
	unset.ServeHTTP(rr, req)
	cov5xWant(t, "no platform org configured", rr, http.StatusForbidden, `"forbidden"`)
}

func TestCov5xPlatformProvisioningOutcomes(t *testing.T) {
	f := cov5xNewPlatform(t, false)
	good := `{"name":"Dolphins","slug":"dolphins","admin_email":"boss@example.com","admin_first_name":"B","admin_last_name":"O","team_code":"DOLP"}`

	cov5xWant(t, "bad json", f.op("POST", "/platform/orgs", `{`), http.StatusBadRequest, "invalid_request")
	cov5xWant(t, "unknown field", f.op("POST", "/platform/orgs", `{"bogus":1}`), http.StatusBadRequest, "invalid_request")
	cov5xWant(t, "missing name", f.op("POST", "/platform/orgs", `{"slug":"x-club","admin_email":"a@b.example"}`), http.StatusBadRequest, "name and admin_email are required")
	cov5xWant(t, "bad slug", f.op("POST", "/platform/orgs", `{"name":"N","slug":"No Spaces","admin_email":"a@b.example"}`), http.StatusBadRequest, "invalid_slug")

	rr := f.op("POST", "/platform/orgs", good)
	cov5xWant(t, "create", rr, http.StatusCreated, `"resumed":false`)
	created := cov5xParse(t, rr)
	orgID := created["org"].(map[string]any)["id"].(string)
	if created["admin_user_id"] == "" {
		t.Fatalf("an admin user must be provisioned: %v", created)
	}
	if evs := f.auditActions(t, audit.ActionPlatformOrgCreate); len(evs) != 1 || evs[0].ResourceID != orgID || evs[0].Metadata["target_org_id"] != orgID || evs[0].ActorUserID != "op-1" {
		t.Fatalf("create must be audited against the new org by the operator: %+v", evs)
	}

	cov5xWant(t, "repeat", f.op("POST", "/platform/orgs", good), http.StatusConflict, "slug_taken")

	// The same admin email in another club is refused without creating anything.
	cov5xWant(t, "email in use", f.op("POST", "/platform/orgs",
		`{"name":"Sharks","slug":"sharks","admin_email":"boss@example.com"}`), http.StatusConflict, "admin_email_in_use")

	// An identity-provider outage leaves a provisioning org that the same request resumes.
	f.kc.CreateUserErr = errors.New("keycloak unavailable")
	resume := `{"name":"Orcas","slug":"orcas","admin_email":"orca@example.com"}`
	rr = f.op("POST", "/platform/orgs", resume)
	cov5xWant(t, "partial", rr, http.StatusBadGateway, "provisioning_incomplete")
	if strings.Contains(rr.Body.String(), "keycloak unavailable") {
		t.Errorf("identity provider error leaked: %s", rr.Body.String())
	}
	f.kc.CreateUserErr = nil
	rr = f.op("POST", "/platform/orgs", resume)
	cov5xWant(t, "resumed", rr, http.StatusOK, `"resumed":true`)
}

func TestCov5xPlatformOrgReadsAndTransitions(t *testing.T) {
	f := cov5xNewPlatform(t, false)
	id := f.provision(t, "waves", "waves@example.com")

	rr := f.op("GET", "/platform/orgs/"+id, "")
	cov5xWant(t, "get", rr, http.StatusOK, `"slug":"waves"`)
	cov5xWant(t, "get unknown", f.op("GET", "/platform/orgs/"+uuid.NewString(), ""), http.StatusNotFound, "Organization not found.")

	cov5xWant(t, "list active", f.op("GET", "/platform/orgs?status=active", ""), http.StatusOK, `"count":2`)
	cov5xWant(t, "list suspended", f.op("GET", "/platform/orgs?status=%20suspended%20", ""), http.StatusOK, `"count":0`)

	cov5xWant(t, "resume active org", f.op("POST", "/platform/orgs/"+id+"/resume", ""), http.StatusConflict, "invalid_transition")
	cov5xWant(t, "suspend unknown", f.op("POST", "/platform/orgs/"+uuid.NewString()+"/suspend", ""), http.StatusNotFound, "not_found")
	cov5xWant(t, "suspend", f.op("POST", "/platform/orgs/"+id+"/suspend", ""), http.StatusOK, `"suspended"`)
	cov5xWant(t, "list suspended", f.op("GET", "/platform/orgs?status=suspended", ""), http.StatusOK, `"count":1`)
	cov5xWant(t, "platform org stays", f.op("POST", "/platform/orgs/"+strings.ToUpper(f.platformID)+"/suspend", ""), http.StatusConflict, "platform_org_protected")
	cov5xWant(t, "resume", f.op("POST", "/platform/orgs/"+id+"/resume", ""), http.StatusOK, `"active"`)

	// cancel-offboarding only applies to an org that is being offboarded.
	cov5xWant(t, "cancel when active", f.op("POST", "/platform/orgs/"+id+"/cancel-offboarding", ""), http.StatusConflict, "not being offboarded")
	cov5xWant(t, "cancel unknown", f.op("POST", "/platform/orgs/"+uuid.NewString()+"/cancel-offboarding", ""), http.StatusNotFound, "not_found")
	cov5xWant(t, "offboard", f.op("POST", "/platform/orgs/"+id+"/offboard", ""), http.StatusOK, `"offboarding"`)
	rr = f.op("POST", "/platform/orgs/"+id+"/cancel-offboarding", "")
	cov5xWant(t, "cancel", rr, http.StatusOK, `"suspended"`)
	if cov5xParse(t, rr)["offboarding_at"] != nil {
		t.Fatalf("cancelling offboarding clears the clock: %s", rr.Body.String())
	}

	for _, action := range []string{
		audit.ActionPlatformOrgSuspend, audit.ActionPlatformOrgResume,
		audit.ActionPlatformOrgOffboard, audit.ActionPlatformOrgOffboardCancel,
	} {
		// The refused attempt on the platform org is not audited.
		if got := len(f.auditActions(t, action)); got != 1 {
			t.Errorf("%s: %d audit events, want 1", action, got)
		}
	}
}

func TestCov5xPlatformRegistryOutagesAreInternalErrors(t *testing.T) {
	h := NewPlatformHandlers(orgs.NewServiceWithPool(testutil.ClosedPool(t)), nil, nil, uuid.NewString())
	id := uuid.NewString()
	h.LookupTXT = func(context.Context, string) ([]string, error) {
		return []string{DomainChallengeValue(id)}, nil
	}
	router := chi.NewRouter()
	router.Use(cov5xInject(h.platformOrgID))
	h.RegisterRoutes(router)
	for _, tc := range []struct{ method, path, body, msg string }{
		{"GET", "/platform/orgs", "", "Could not list organizations."},
		{"GET", "/platform/orgs/" + id, "", "Could not load organization."},
		{"POST", "/platform/orgs/" + id + "/offboard", "", "Could not update organization."},
		{"POST", "/platform/orgs/" + id + "/domains", `{"domain":"swim.example.org"}`, "Could not add domain."},
		{"POST", "/platform/orgs/" + id + "/domains/swim.example.org/verify", "", "Could not verify domain."},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("X-Test-Who", cov5xOperator)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		cov5xWant(t, tc.method+" "+tc.path, rr, http.StatusInternalServerError, tc.msg)
		if strings.Contains(rr.Body.String(), "closed pool") {
			t.Errorf("storage error leaked: %s", rr.Body.String())
		}
	}
}

func TestCov5xPlatformDomains(t *testing.T) {
	f := cov5xNewPlatform(t, false)
	a := f.provision(t, "alpha", "alpha@example.com")
	b := f.provision(t, "bravo", "bravo@example.com")

	cov5xWant(t, "bad json", f.op("POST", "/platform/orgs/"+a+"/domains", `{`), http.StatusBadRequest, "invalid_request")
	cov5xWant(t, "unknown org", f.op("POST", "/platform/orgs/"+uuid.NewString()+"/domains", `{"domain":"a.example.org"}`), http.StatusNotFound, "not_found")
	cov5xWant(t, "invalid domain", f.op("POST", "/platform/orgs/"+a+"/domains", `{"domain":"not a domain"}`), http.StatusBadRequest, "invalid_domain")

	rr := f.op("POST", "/platform/orgs/"+a+"/domains", `{"domain":"  Swim.Example.ORG "}`)
	cov5xWant(t, "add", rr, http.StatusCreated, `"verified":false`)
	challenge := cov5xParse(t, rr)["challenge"].(map[string]any)
	if challenge["name"] != "_aquify-verify.swim.example.org" || challenge["value"] != "aquify-verify="+a || challenge["type"] != "TXT" {
		t.Fatalf("challenge: %v", challenge)
	}
	cov5xWant(t, "domain taken", f.op("POST", "/platform/orgs/"+b+"/domains", `{"domain":"swim.example.org"}`), http.StatusConflict, "domain_taken")
	if evs := f.auditActions(t, audit.ActionPlatformOrgDomainAdd); len(evs) != 1 || evs[0].ResourceID != a {
		t.Fatalf("domain add audit: %+v", evs)
	}

	verify := "/platform/orgs/" + a + "/domains/swim.example.org/verify"
	cov5xWant(t, "no TXT", f.op("POST", verify, ""), http.StatusConflict, "challenge_not_found")
	f.dns["_aquify-verify.swim.example.org"] = []string{"v=spf1 -all", "aquify-verify=" + b}
	cov5xWant(t, "other org's TXT", f.op("POST", verify, ""), http.StatusConflict, "challenge_mismatch")
	if org, err := f.reg.ResolveDomain(context.Background(), "swim.example.org"); err == nil {
		t.Fatalf("an unverified domain must not resolve: %+v", org)
	}

	// The org must actually own the domain being verified.
	f.dns["_aquify-verify.other.example.org"] = []string{"aquify-verify=" + a}
	cov5xWant(t, "domain not registered", f.op("POST", "/platform/orgs/"+a+"/domains/other.example.org/verify", ""), http.StatusNotFound, "Domain not found for this organization.")

	f.dns["_aquify-verify.swim.example.org"] = []string{" aquify-verify=" + a + " "}
	cov5xWant(t, "verify", f.op("POST", verify, ""), http.StatusOK, `"verified":true`)
	if org, err := f.reg.ResolveDomain(context.Background(), "swim.example.org"); err != nil || org.ID != a {
		t.Fatalf("verified domain must resolve to its org: %+v %v", org, err)
	}
	if evs := f.auditActions(t, audit.ActionPlatformOrgDomainVerify); len(evs) != 1 {
		t.Fatalf("domain verify audit: %+v", evs)
	}

	// A resolver that returns a matching record together with an error still counts as proof.
	f.h.LookupTXT = func(context.Context, string) ([]string, error) {
		return []string{"aquify-verify=" + a}, errors.New("partial answer")
	}
	cov5xWant(t, "partial DNS answer", f.op("POST", verify, ""), http.StatusOK, `"verified":true`)
}

func TestCov5xPlatformOffboardingIsUnavailableWithoutADatabase(t *testing.T) {
	f := cov5xNewPlatform(t, false)
	id := f.provision(t, "nodb", "nodb@example.com")
	for _, path := range []string{"export", "purge", "restore"} {
		cov5xWant(t, path, f.op("POST", "/platform/orgs/"+id+"/"+path, `{}`), http.StatusNotImplemented, "offboarding_unavailable")
	}
}

func TestCov5xPlatformExportPurgeRestoreLifecycle(t *testing.T) {
	f := cov5xNewPlatform(t, true)
	id := f.provision(t, "gone", "gone@example.com")
	other := f.provision(t, "stays", "stays@example.com")

	cov5xWant(t, "export unknown org", f.op("POST", "/platform/orgs/"+uuid.NewString()+"/export", ""), http.StatusNotFound, "not_found")

	rr := f.op("POST", "/platform/orgs/"+id+"/export", "")
	cov5xWant(t, "export", rr, http.StatusOK, "download_url")
	body := cov5xParse(t, rr)
	key := body["export"].(map[string]any)["key"].(string)
	if !strings.HasPrefix(key, offboarding.ExportKeyPrefix(id)) || !strings.Contains(body["download_url"].(string), url.PathEscape(key)) {
		t.Fatalf("export key/url: %v", body)
	}
	if stored, _, err := f.store.GetObject(context.Background(), key); err != nil || len(stored) == 0 {
		t.Fatalf("the archive must be stored: %v", err)
	}
	if evs := f.auditActions(t, audit.ActionPlatformOrgExport); len(evs) != 1 || evs[0].ResourceID != id {
		t.Fatalf("export audit: %+v", evs)
	}

	// Purge guards: body, state, cooling-off, typed confirmation, protected org.
	cov5xWant(t, "purge bad json", f.op("POST", "/platform/orgs/"+id+"/purge", `{`), http.StatusBadRequest, "invalid_request")
	cov5xWant(t, "purge active org", f.op("POST", "/platform/orgs/"+id+"/purge", `{"confirm_slug":"gone"}`), http.StatusConflict, "offboarding_precondition")
	cov5xWant(t, "purge unknown org", f.op("POST", "/platform/orgs/"+uuid.NewString()+"/purge", `{"confirm_slug":"gone"}`), http.StatusNotFound, "not_found")
	cov5xWant(t, "purge platform org", f.op("POST", "/platform/orgs/"+f.platformID+"/purge", `{"confirm_slug":"platform"}`), http.StatusConflict, "offboarding_precondition")
	cov5xWant(t, "offboard", f.op("POST", "/platform/orgs/"+id+"/offboard", ""), http.StatusOK, `"offboarding"`)
	cov5xWant(t, "purge during cooling-off", f.op("POST", "/platform/orgs/"+id+"/purge", `{"confirm_slug":"gone"}`), http.StatusConflict, "cooling-off")
	f.now = time.Now().Add(2 * time.Hour)
	cov5xWant(t, "purge wrong slug", f.op("POST", "/platform/orgs/"+id+"/purge", `{"confirm_slug":"stays"}`), http.StatusBadRequest, "confirmation_mismatch")

	rr = f.op("POST", "/platform/orgs/"+id+"/purge", `{"confirm_slug":"gone"}`)
	cov5xWant(t, "purge", rr, http.StatusOK, "users_deleted")
	if got, err := f.reg.Get(context.Background(), id); err != nil || got.Status != orgs.StatusDeleted {
		t.Fatalf("purged org must be tombstoned: %+v %v", got, err)
	}
	if got, err := f.reg.Get(context.Background(), other); err != nil || got.Status != orgs.StatusActive {
		t.Fatalf("other org must be untouched: %+v %v", got, err)
	}
	if evs := f.auditActions(t, audit.ActionPlatformOrgPurge); len(evs) != 1 {
		t.Fatalf("purge audit: %+v", evs)
	}

	// Restore guards and recovery from the earlier export.
	restore := "/platform/orgs/" + id + "/restore"
	cov5xWant(t, "restore bad json", f.op("POST", restore, `{`), http.StatusBadRequest, "invalid_request")
	cov5xWant(t, "restore other org's key", f.op("POST", restore, `{"export_key":"offboarding/`+other+`/export-x.zip"}`), http.StatusBadRequest, "invalid_export_key")
	cov5xWant(t, "restore traversal", f.op("POST", restore, `{"export_key":"offboarding/`+id+`/export-/../../x"}`), http.StatusBadRequest, "invalid_export_key")
	cov5xWant(t, "restore missing archive", f.op("POST", restore, `{"export_key":"offboarding/`+id+`/export-missing.zip"}`), http.StatusNotFound, "export_not_found")

	junkKey := offboarding.ExportKeyPrefix(id) + "junk.zip"
	if err := f.store.PutObject(context.Background(), junkKey, "application/zip", []byte("not a zip")); err != nil {
		t.Fatal(err)
	}
	rr = f.op("POST", restore, `{"export_key":"`+junkKey+`"}`)
	cov5xWant(t, "restore corrupt archive", rr, http.StatusInternalServerError, "Offboarding step failed.")
	if strings.Contains(rr.Body.String(), "read archive") {
		t.Errorf("internal cause leaked: %s", rr.Body.String())
	}

	// An org id that has no registry row cannot be restored into.
	ghost := uuid.NewString()
	ghostKey := offboarding.ExportKeyPrefix(ghost) + "a.zip"
	if err := f.store.PutObject(context.Background(), ghostKey, "application/zip", []byte("x")); err != nil {
		t.Fatal(err)
	}
	cov5xWant(t, "restore into unknown org", f.op("POST", "/platform/orgs/"+ghost+"/restore", `{"export_key":"`+ghostKey+`"}`), http.StatusNotFound, "registry row must exist")

	rr = f.op("POST", restore, `{"export_key":"`+key+`"}`)
	cov5xWant(t, "restore", rr, http.StatusOK, "rows_restored")
	if got, err := f.reg.Get(context.Background(), id); err != nil || got.Status != orgs.StatusSuspended {
		t.Fatalf("a restored org comes back suspended, never active: %+v %v", got, err)
	}
	if evs := f.auditActions(t, audit.ActionPlatformOrgRestore); len(evs) != 1 {
		t.Fatalf("restore audit: %+v", evs)
	}
}
