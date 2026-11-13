package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/performance"
	"github.com/luryann/aquify/server/internal/app/usaswimming"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov2xUSABase = "/integrations/usa-swimming"

// cov2xVendor is a fake of the external USA Swimming API (a true system
// boundary). It records what the handlers send it.
type cov2xVendor struct {
	mu sync.Mutex

	pingErr  error
	times    map[string][]usaswimming.OfficialTime
	timesErr map[string]error

	clubs        []usaswimming.Club
	clubsErr     error
	roster       []usaswimming.RosterMember
	rosterErr    error
	link         string
	linkErr      error
	checks       []usaswimming.MemberCheck
	checksErr    error
	timeValid    bool
	timeErr      error
	submitRes    usaswimming.RegistrationResult
	submitErr    error
	statusValue  string
	statusErr    error
	gotClubID    string
	gotMemberIDs []string
	gotMember    string
	gotEvent     string
	gotTime      string
	gotReg       usaswimming.Registration
	gotRequestID string

	// entered/release let a test hold OfficialTimes mid-flight.
	entered chan struct{}
	release chan struct{}
}

func (v *cov2xVendor) TestConnection(context.Context) error { return v.pingErr }

func (v *cov2xVendor) OfficialTimes(_ context.Context, id string) ([]usaswimming.OfficialTime, error) {
	if v.entered != nil {
		v.entered <- struct{}{}
		<-v.release
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if err, ok := v.timesErr[id]; ok {
		return nil, err
	}
	t, ok := v.times[id]
	if !ok {
		return nil, usaswimming.ErrSwimmerNotFound
	}
	return t, nil
}

func (v *cov2xVendor) Clubs(context.Context) ([]usaswimming.Club, error) { return v.clubs, v.clubsErr }

func (v *cov2xVendor) Roster(_ context.Context, clubID string, _ *time.Time) ([]usaswimming.RosterMember, error) {
	v.mu.Lock()
	v.gotClubID = clubID
	v.mu.Unlock()
	return v.roster, v.rosterErr
}

func (v *cov2xVendor) RegistrationLink(_ context.Context, clubID string) (string, error) {
	v.mu.Lock()
	v.gotClubID = clubID
	v.mu.Unlock()
	return v.link, v.linkErr
}

func (v *cov2xVendor) VerifyMembers(_ context.Context, clubID string, ids []string) ([]usaswimming.MemberCheck, error) {
	v.mu.Lock()
	v.gotClubID, v.gotMemberIDs = clubID, append([]string(nil), ids...)
	v.mu.Unlock()
	return v.checks, v.checksErr
}

func (v *cov2xVendor) VerifyTime(_ context.Context, memberID, event, swimTime string) (bool, error) {
	v.mu.Lock()
	v.gotMember, v.gotEvent, v.gotTime = memberID, event, swimTime
	v.mu.Unlock()
	return v.timeValid, v.timeErr
}

func (v *cov2xVendor) SubmitRegistration(_ context.Context, clubID string, reg usaswimming.Registration) (usaswimming.RegistrationResult, error) {
	v.mu.Lock()
	v.gotClubID, v.gotReg = clubID, reg
	v.mu.Unlock()
	return v.submitRes, v.submitErr
}

func (v *cov2xVendor) RegistrationStatus(_ context.Context, requestID string) (string, error) {
	v.mu.Lock()
	v.gotRequestID = requestID
	v.mu.Unlock()
	return v.statusValue, v.statusErr
}

// cov2xPerf is a performance recorder whose writes can be made to fail.
type cov2xPerf struct {
	mu       sync.Mutex
	has      map[string]bool
	upsert   func(performance.UpsertResultInput) error
	recorded []performance.UpsertResultInput
}

func (p *cov2xPerf) HasResultTime(_, swimmerID string, distance int, stroke, course string, ms int) bool {
	return p.has[fmt.Sprintf("%s|%d|%s|%s|%d", swimmerID, distance, stroke, course, ms)]
}

func (p *cov2xPerf) UpsertResultWithRecords(_, _ string, _ *appmiddleware.Principal, in performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.upsert != nil {
		if err := p.upsert(in); err != nil {
			return nil, nil, err
		}
	}
	p.recorded = append(p.recorded, in)
	return &performance.Result{}, nil, nil
}

type cov2xUSAFixture struct {
	t       *testing.T
	h       *USASwimmingHandlers
	svc     *usaswimming.Service
	members *members.Service
	audit   *audit.Service
	vendor  *cov2xVendor
	router  http.Handler
	house   string
}

func cov2xUSARouter(h *USASwimmingHandlers, mws ...func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		for _, mw := range mws {
			p.Use(mw)
		}
		h.RegisterRoutes(p)
	})
	return r
}

func cov2xNewUSAFixture(t *testing.T) *cov2xUSAFixture {
	t.Helper()
	svc := usaswimming.NewService(usaswimming.NewCipher("cov2x-integration-key"), nil)
	fake := &cov2xVendor{times: map[string][]usaswimming.OfficialTime{}, timesErr: map[string]error{}}
	svc.NewHTTP = func(usaswimming.ClientConfig) (usaswimming.Client, error) { return fake, nil }
	svc.NewSwims = func(usaswimming.SwimsConfig) (usaswimming.Client, error) { return fake, nil }
	ms := members.NewService()
	a := audit.NewService()
	h := NewUSASwimmingHandlers(svc, performance.NewService(), ms, a)
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cov2xFinOrg, Roles: []string{"admin"}}
	hh, err := ms.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov2xFinOrg, HouseholdName: "Lee"})
	if err != nil {
		t.Fatal(err)
	}
	return &cov2xUSAFixture{
		t: t, h: h, svc: svc, members: ms, audit: a, vendor: fake, house: hh.ID,
		router: cov2xUSARouter(h, appmiddleware.Auth(cov2xFinTokens), appmiddleware.RequireOrgScope),
	}
}

// configure stores working settings; vendor=true selects the OAuth vendor API.
func (f *cov2xUSAFixture) configure(vendor bool) {
	f.t.Helper()
	base, key, enabled := "https://swims.example.test", "super-secret-1234", true
	u := usaswimming.Updates{BaseURL: &base, APIKey: &key, Enabled: &enabled}
	if vendor {
		tok, cid, club := "https://auth.example.test/token", "client-1", "CLUB1"
		u.TokenURL, u.ClientID, u.ClubID = &tok, &cid, &club
	}
	if _, err := f.svc.Update(cov2xFinOrg, u, "setup"); err != nil {
		f.t.Fatal(err)
	}
}

func (f *cov2xUSAFixture) addSwimmer(first, last, dob, gender, ussID string) members.Swimmer {
	f.t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cov2xFinOrg, Roles: []string{"admin"}}
	sw, _, err := f.members.CreateSwimmer(admin, cov2xFinOrg, members.CreateSwimmerInput{
		HouseholdID: f.house, FirstName: first, LastName: last, DateOfBirth: dob, Gender: gender, USASwimmingID: ussID,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return sw
}

func (f *cov2xUSAFixture) do(method, path, token, body string) *httptest.ResponseRecorder {
	return cov2xReq(f.router, method, "/"+strings.TrimPrefix(path, "/"), token, body)
}

func (f *cov2xUSAFixture) must(want int, method, path, token, body string) map[string]any {
	f.t.Helper()
	rr := f.do(method, path, token, body)
	if rr.Code != want {
		f.t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, rr.Code, rr.Body.String())
	}
	return cov2xDecode(f.t, rr)
}

func (f *cov2xUSAFixture) auditMeta(action string) []map[string]string {
	f.t.Helper()
	evs, err := f.audit.Query(audit.Filter{OrgID: cov2xFinOrg, Action: action, Limit: 100})
	if err != nil {
		f.t.Fatal(err)
	}
	out := []map[string]string{}
	for _, ev := range evs {
		out = append(out, ev.Metadata)
	}
	return out
}

func TestCov2xUSASwimmingRoutesAreAdminOnlyAndScoped(t *testing.T) {
	t.Parallel()
	routes := []cov2xRoute{
		{http.MethodGet, cov2xUSABase, false},
		{http.MethodPut, cov2xUSABase, false},
		{http.MethodPost, cov2xUSABase + "/test", false},
		{http.MethodPost, cov2xUSABase + "/sync", false},
		{http.MethodGet, cov2xUSABase + "/clubs", false},
		{http.MethodGet, cov2xUSABase + "/roster-compare", false},
		{http.MethodGet, cov2xUSABase + "/registration-link", false},
		{http.MethodPost, cov2xUSABase + "/verify-members", false},
		{http.MethodPost, cov2xUSABase + "/verify-time", false},
		{http.MethodPost, cov2xUSABase + "/registrations", false},
		{http.MethodGet, cov2xUSABase + "/registrations/r1/status", false},
	}
	f := cov2xNewUSAFixture(t)
	f.configure(true) // a working vendor must still be unreachable for non-admins
	noAuth := cov2xUSARouter(f.h)
	authOnly := cov2xUSARouter(f.h, appmiddleware.Auth(cov2xFinTokens))
	for _, rt := range routes {
		if rr := cov2xReq(noAuth, rt.method, rt.path, "", "{}"); rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without principal: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		if rr := cov2xReq(authOnly, rt.method, rt.path, "adminMFA", "{}"); rr.Code != http.StatusBadRequest || cov2xErrCode(cov2xDecode(t, rr)) != "missing_org_scope" {
			t.Fatalf("%s %s without org: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
		for _, tok := range []string{"coach", "parentA"} {
			if rr := f.do(rt.method, rt.path, tok, "{}"); rr.Code != http.StatusForbidden || cov2xErrCode(cov2xDecode(t, rr)) != "forbidden" {
				t.Fatalf("%s %s as %s: %d %s", rt.method, rt.path, tok, rr.Code, rr.Body.String())
			}
		}
	}
	if n := len(f.vendor.gotMemberIDs) + len(f.vendor.gotClubID) + len(f.vendor.gotRequestID); n != 0 {
		t.Fatal("a denied request reached the vendor API")
	}
}

func TestCov2xUSASwimmingSettingsLifecycle(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	v := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, cov2xUSABase, "admin", ""), "integration")
	if v["configured"] != false || v["enabled"] != false {
		t.Fatalf("fresh org must be unconfigured: %v", v)
	}
	if rr := f.do(http.MethodPut, cov2xUSABase, "admin", `{nope`); rr.Code != http.StatusBadRequest || cov2xErrCode(cov2xDecode(t, rr)) != "bad_request" {
		t.Fatalf("malformed body: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPut, cov2xUSABase, "admin", `{"base_url":"http://insecure.example.test"}`); rr.Code != http.StatusUnprocessableEntity ||
		cov2xErrCode(cov2xDecode(t, rr)) != "invalid_integration_settings" || !strings.Contains(rr.Body.String(), "https") {
		t.Fatalf("http base url: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(http.MethodPut, cov2xUSABase, "admin", `{"enabled":true}`); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("enable without credentials: %d %s", rr.Code, rr.Body.String())
	}
	body := `{"base_url":"https://swims.example.test","api_key":"super-secret-1234","enabled":true,"token_url":"https://auth.example.test/token","client_id":"client-1","club_id":"CLUB1"}`
	rr := f.do(http.MethodPut, cov2xUSABase, "admin", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "super-secret") {
		t.Fatalf("API key echoed: %s", rr.Body.String())
	}
	v = cov2xObj(t, cov2xDecode(t, rr), "integration")
	if v["configured"] != true || v["enabled"] != true || v["api_key_hint"] != "1234" || v["vendor_api"] != true || v["club_id"] != "CLUB1" {
		t.Fatalf("saved view: %v", v)
	}
	f.must(http.StatusOK, http.MethodPut, cov2xUSABase, "admin", `{"enabled":false}`)

	metas := f.auditMeta("integrations.usa_swimming.update")
	if len(metas) != 2 {
		t.Fatalf("want 2 update audit events, got %v", metas)
	}
	var withKey, withoutKey map[string]string
	for _, m := range metas {
		if m["api_key_changed"] == "true" {
			withKey = m
		} else {
			withoutKey = m
		}
	}
	if withKey["base_url"] != "https://swims.example.test" || withKey["enabled"] != "true" || withoutKey["enabled"] != "false" {
		t.Fatalf("audit metadata: %v", metas)
	}
	for _, m := range metas {
		for _, val := range m {
			if strings.Contains(val, "super-secret") {
				t.Fatalf("API key in audit log: %v", m)
			}
		}
	}
}

func TestCov2xUSASwimmingSavingKeyFailsClosedWithoutEncryptionKey(t *testing.T) {
	t.Parallel()
	svc := usaswimming.NewService(nil, nil)
	h := NewUSASwimmingHandlers(svc, performance.NewService(), members.NewService(), audit.NewService())
	router := cov2xUSARouter(h, appmiddleware.Auth(cov2xFinTokens), appmiddleware.RequireOrgScope)
	rr := cov2xReq(router, http.MethodPut, cov2xUSABase, "admin", `{"base_url":"https://swims.example.test","api_key":"abc12345"}`)
	if rr.Code != http.StatusServiceUnavailable || cov2xErrCode(cov2xDecode(t, rr)) != "encryption_not_configured" {
		t.Fatalf("want 503 encryption_not_configured, got %d %s", rr.Code, rr.Body.String())
	}
	if v, _ := svc.Get(cov2xFinOrg); v.HasAPIKey {
		t.Fatal("a key must not be stored when it cannot be encrypted")
	}
}

func TestCov2xUSASwimmingStoreFailuresAreInternalErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := usaswimming.NewServiceWithPool(pool, usaswimming.NewCipher("k"), nil)
	h := NewUSASwimmingHandlers(svc, performance.NewService(), members.NewService(), audit.NewService())
	adminID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1, $2, $3, $4)`, adminID, org, adminID+"@example.test", adminID); err != nil {
		t.Fatal(err)
	}
	tokens := tokenResolver{"admin": {UserID: adminID, OrgID: org, Roles: []string{"admin"}}}
	router := cov2xUSARouter(h, appmiddleware.Auth(tokens), appmiddleware.RequireOrgScope)
	call := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, cov2xUSABase, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin")
		req.Header.Set("X-Org-Id", org)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	// Healthy first: settings persist through the database.
	if rr := call(http.MethodPut, `{"club_id":"CLUB9"}`); rr.Code != http.StatusOK {
		t.Fatalf("healthy put: %d %s", rr.Code, rr.Body.String())
	}
	if rr := call(http.MethodGet, ""); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"club_id":"CLUB9"`) {
		t.Fatalf("healthy get: %d %s", rr.Code, rr.Body.String())
	}

	// Break the table's shape (a bare table rename would fall through to a shared schema).
	if _, err := pool.Exec(context.Background(), `ALTER TABLE usa_swimming_integrations RENAME COLUMN club_id TO club_id_cov2x_gone`); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rr := call(method, `{"club_id":"X"}`)
		if rr.Code != http.StatusInternalServerError || cov2xErrCode(cov2xDecode(t, rr)) != "internal_error" || strings.Contains(rr.Body.String(), "club_id") {
			t.Fatalf("%s: want clean 500, got %d %s", method, rr.Code, rr.Body.String())
		}
	}
}

func TestCov2xUSASwimmingConnectionTest(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	if code := cov2xErrCode(f.must(http.StatusConflict, http.MethodPost, cov2xUSABase+"/test", "admin", "")); code != "integration_not_ready" {
		t.Fatalf("unconfigured: %q", code)
	}
	f.configure(false)
	res := f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/test", "admin", "")
	if res["ok"] != true || res["message"] != "Connection OK" {
		t.Fatalf("healthy test: %v", res)
	}
	f.vendor.pingErr = errors.New("401 from upstream")
	res = f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/test", "admin", "")
	if res["ok"] != false || res["message"] != "401 from upstream" {
		t.Fatalf("failed test: %v", res)
	}
	v := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, cov2xUSABase, "admin", ""), "integration")
	if v["last_test_ok"] != false || v["last_test_message"] != "401 from upstream" {
		t.Fatalf("outcome must be recorded: %v", v)
	}
	metas := f.auditMeta("integrations.usa_swimming.test")
	if len(metas) != 2 {
		t.Fatalf("want two audited tests, got %v", metas)
	}

	// Client construction failures map through writeClientErr.
	f.svc.NewHTTP = func(usaswimming.ClientConfig) (usaswimming.Client, error) { return nil, usaswimming.ErrUnauthorized }
	if code := cov2xErrCode(f.must(http.StatusForbidden, http.MethodPost, cov2xUSABase+"/test", "admin", "")); code != "tier_not_authorized" {
		t.Fatalf("unauthorized client: %q", code)
	}
	f.svc.NewHTTP = func(usaswimming.ClientConfig) (usaswimming.Client, error) {
		return nil, errors.New("dial tcp: refused")
	}
	if code := cov2xErrCode(f.must(http.StatusBadGateway, http.MethodPost, cov2xUSABase+"/test", "admin", "")); code != "integration_unavailable" {
		t.Fatalf("unavailable client: %q", code)
	}
}

func TestCov2xUSAWriteClientErrContract(t *testing.T) {
	t.Parallel()
	h := &USASwimmingHandlers{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{usaswimming.ErrUnsupported, http.StatusNotImplemented, "integration_unsupported"},
		{fmt.Errorf("wrapped: %w", usaswimming.ErrUnauthorized), http.StatusForbidden, "tier_not_authorized"},
		{usaswimming.ErrDisabled, http.StatusConflict, "integration_not_ready"},
		{usaswimming.ErrNotConfigured, http.StatusConflict, "integration_not_ready"},
		{errors.New("upstream timeout"), http.StatusBadGateway, "integration_unavailable"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		h.writeClientErr(rr, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
		if rr.Code != tc.status || cov2xErrCode(cov2xDecode(t, rr)) != tc.code {
			t.Fatalf("%v: got %d %s, want %d %s", tc.err, rr.Code, rr.Body.String(), tc.status, tc.code)
		}
	}
}

func TestCov2xUSAStrokeAndCourseValidation(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"free", "back", "breast", "fly", "im"} {
		if !validIndividualStroke(s) {
			t.Fatalf("%q should be a valid individual stroke", s)
		}
	}
	for _, s := range []string{"", "FREE", "medley", "relay"} {
		if validIndividualStroke(s) {
			t.Fatalf("%q must not be a valid individual stroke", s)
		}
	}
	if !validCourse("SCY") || !validCourse("SCM") || !validCourse("LCM") || validCourse("scy") || validCourse("") {
		t.Fatal("course validation changed")
	}
}

func cov2xTime(distance int, stroke, course string, ms int, date string) usaswimming.OfficialTime {
	d, _ := time.Parse("2006-01-02", date)
	return usaswimming.OfficialTime{Distance: distance, Stroke: stroke, Course: course, TimeMs: ms, SwimDate: d}
}

func TestCov2xUSASyncPreconditions(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	if rr := f.do(http.MethodPost, cov2xUSABase+"/sync", "admin", `{bad`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d %s", rr.Code, rr.Body.String())
	}
	if code := cov2xErrCode(f.must(http.StatusConflict, http.MethodPost, cov2xUSABase+"/sync", "admin", "")); code != "integration_not_ready" {
		t.Fatalf("unconfigured sync: %q", code)
	}
	f.configure(true)
	if code := cov2xErrCode(f.must(http.StatusNotImplemented, http.MethodPost, cov2xUSABase+"/sync", "admin", "")); code != "integration_unsupported" {
		t.Fatalf("vendor API has no times listing: %q", code)
	}
}

func TestCov2xUSASyncImportsSkipsAndReportsPerSwimmer(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(false)
	ann := f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "usa-0001")
	f.addSwimmer("Bo", "Kim", "2011-01-01", "male", "UNKNOWN9")
	cy := f.addSwimmer("Cy", "Ng", "2010-02-02", "male", "ERR00001")
	f.addSwimmer("Di", "Wu", "2010-03-03", "female", "")
	f.vendor.times["USA0001"] = []usaswimming.OfficialTime{
		cov2xTime(100, "free", "SCY", 58120, "2026-01-17"),
		cov2xTime(100, "FLY", "scy", 61000, "2026-01-18"), // case is normalised
		cov2xTime(50, "free", "SCY", 0, "2026-01-19"),     // no time
		cov2xTime(0, "free", "SCY", 30000, "2026-01-19"),  // no distance
		cov2xTime(50, "relay", "SCY", 30000, "2026-01-19"),
		cov2xTime(50, "free", "XXX", 30000, "2026-01-19"),
		{Distance: 50, Stroke: "free", Course: "SCY", TimeMs: 30000}, // no swim date
	}
	f.vendor.timesErr["ERR00001"] = errors.New("vendor 500")
	perf := &cov2xPerf{has: map[string]bool{}}
	f.h.perf = perf

	res := f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/sync", "admin", "")
	if cov2xNum(res, "swimmer_count") != 3 || cov2xNum(res, "imported") != 2 || cov2xNum(res, "skipped") != 5 || cov2xNum(res, "duplicates") != 0 || res["partial"] != false {
		t.Fatalf("sync result: %v", res)
	}
	status := map[string]string{}
	for _, raw := range res["swimmers"].([]any) {
		row := raw.(map[string]any)
		status[row["swimmer_id"].(string)] = row["status"].(string)
		if row["status"] == "error" && row["error"] != "vendor 500" {
			t.Fatalf("error row: %v", row)
		}
	}
	if status[ann.ID] != "synced" || status[cy.ID] != "error" || len(status) != 3 {
		t.Fatalf("per-swimmer status: %v", status)
	}
	if len(perf.recorded) != 2 {
		t.Fatalf("recorded results: %+v", perf.recorded)
	}
	first := perf.recorded[0]
	if first.SwimmerID != ann.ID || first.Distance != 100 || first.Stroke != "free" || first.Course != "SCY" || first.Gender != "F" ||
		first.FinTimeMs == nil || *first.FinTimeMs != 58120 || !first.IsOfficial {
		t.Fatalf("recorded input: %+v", first)
	}
	if perf.recorded[1].Stroke != "fly" || perf.recorded[1].Course != "SCY" {
		t.Fatalf("stroke/course must be normalised: %+v", perf.recorded[1])
	}
	if v := cov2xObj(t, f.must(http.StatusOK, http.MethodGet, cov2xUSABase, "admin", ""), "integration"); !strings.Contains(fmt.Sprint(v["last_sync_message"]), "3 swimmers, 2 imported") {
		t.Fatalf("sync outcome not recorded: %v", v)
	}
	if metas := f.auditMeta("integrations.usa_swimming.sync"); len(metas) != 1 || metas[0]["imported"] != "2" || metas[0]["skipped"] != "5" {
		t.Fatalf("sync audit: %v", metas)
	}

	// Already-recorded times are duplicates, and swimmer_ids narrows the run.
	perf.has[fmt.Sprintf("%s|100|free|SCY|58120", ann.ID)] = true
	perf.has[fmt.Sprintf("%s|100|fly|SCY|61000", ann.ID)] = true
	res = f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/sync", "admin", fmt.Sprintf(`{"swimmer_ids":[%q]}`, ann.ID))
	if cov2xNum(res, "swimmer_count") != 1 || cov2xNum(res, "imported") != 0 || cov2xNum(res, "duplicates") != 2 {
		t.Fatalf("second sync: %v", res)
	}
	if len(perf.recorded) != 2 {
		t.Fatalf("duplicates must not be recorded again: %d", len(perf.recorded))
	}
}

func TestCov2xUSASyncRecordingFailures(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(false)
	ann := f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "USA0001")
	f.vendor.times["USA0001"] = []usaswimming.OfficialTime{
		cov2xTime(100, "free", "SCY", 58120, "2026-01-17"),
		cov2xTime(50, "fly", "SCY", 29000, "2026-01-18"),
	}

	// A hard write failure stops that swimmer and surfaces a generic error.
	f.h.perf = &cov2xPerf{upsert: func(performance.UpsertResultInput) error { return errors.New("pq: connection reset") }}
	res := f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/sync", "admin", "")
	row := res["swimmers"].([]any)[0].(map[string]any)
	if row["status"] != "error" || row["error"] != "could not record a time" || cov2xNum(res, "imported") != 0 {
		t.Fatalf("write failure row: %v", row)
	}
	if strings.Contains(fmt.Sprint(res), "connection reset") {
		t.Fatalf("storage error leaked to the client: %v", res)
	}

	// Record-detection problems are non-fatal: the result is stored and counted.
	f.h.perf = &cov2xPerf{upsert: func(performance.UpsertResultInput) error {
		return fmt.Errorf("%w: persist", performance.ErrRecordDetection)
	}}
	res = f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/sync", "admin", "")
	row = res["swimmers"].([]any)[0].(map[string]any)
	if row["status"] != "synced" || cov2xNum(res, "imported") != 2 || row["swimmer_id"] != ann.ID {
		t.Fatalf("record-detection failure must not fail the swimmer: %v", res)
	}
}

func TestCov2xUSASyncStopsWhenRequestIsCancelled(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(false)
	f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "USA0001")
	f.vendor.times["USA0001"] = []usaswimming.OfficialTime{cov2xTime(100, "free", "SCY", 58120, "2026-01-17")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, cov2xUSABase+"/sync", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("X-Org-Id", cov2xFinOrg)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	res := cov2xDecode(t, rr)
	if res["partial"] != true || cov2xNum(res, "swimmer_count") != 0 || !strings.Contains(fmt.Sprint(res["summary"]), "partial: stopped after 0 of 1") {
		t.Fatalf("cancelled sync must report a partial run: %v", res)
	}
}

func TestCov2xUSASyncRefusesOversizedRequests(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(false)
	for i := 0; i <= maxSyncSwimmers; i++ {
		f.addSwimmer("S", fmt.Sprintf("N%d", i), "2012-01-01", "female", fmt.Sprintf("ID%06d", i))
	}
	rr := f.do(http.MethodPost, cov2xUSABase+"/sync", "admin", "")
	if rr.Code != http.StatusUnprocessableEntity || cov2xErrCode(cov2xDecode(t, rr)) != "too_many_swimmers" {
		t.Fatalf("want 422 too_many_swimmers, got %d %s", rr.Code, rr.Body.String())
	}
	if len(f.vendor.gotMemberIDs) != 0 {
		t.Fatal("no vendor call may be made for an oversized request")
	}
}

func TestCov2xUSASyncRejectsConcurrentRunForSameClub(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(false)
	f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "USA0001")
	f.vendor.times["USA0001"] = []usaswimming.OfficialTime{cov2xTime(100, "free", "SCY", 58120, "2026-01-17")}
	f.vendor.entered = make(chan struct{})
	f.vendor.release = make(chan struct{})

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- f.do(http.MethodPost, cov2xUSABase+"/sync", "admin", "") }()
	<-f.vendor.entered // the first sync is now inside the vendor call

	rr := f.do(http.MethodPost, cov2xUSABase+"/sync", "admin", "")
	if rr.Code != http.StatusConflict || cov2xErrCode(cov2xDecode(t, rr)) != "sync_in_progress" {
		t.Fatalf("second sync: want 409 sync_in_progress, got %d %s", rr.Code, rr.Body.String())
	}
	close(f.vendor.release)
	done := <-first
	if done.Code != http.StatusOK || cov2xNum(cov2xDecode(t, done), "imported") != 1 {
		t.Fatalf("first sync should finish normally: %d %s", done.Code, done.Body.String())
	}
	// The guard is released afterwards.
	f.vendor.entered = nil
	f.must(http.StatusOK, http.MethodPost, cov2xUSABase+"/sync", "admin", "")
}

func TestCov2xUSAVendorRoutesNeedVendorSettings(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	paths := []cov2xRoute{
		{http.MethodGet, cov2xUSABase + "/clubs", false},
		{http.MethodGet, cov2xUSABase + "/roster-compare", false},
		{http.MethodGet, cov2xUSABase + "/registration-link", false},
		{http.MethodPost, cov2xUSABase + "/verify-members", false},
		{http.MethodPost, cov2xUSABase + "/verify-time", false},
		{http.MethodPost, cov2xUSABase + "/registrations", false},
		{http.MethodGet, cov2xUSABase + "/registrations/r1/status", false},
	}
	expect := func(status int, code string) {
		t.Helper()
		for _, rt := range paths {
			rr := f.do(rt.method, rt.path, "admin", "{}")
			if rr.Code != status || cov2xErrCode(cov2xDecode(t, rr)) != code {
				t.Fatalf("%s %s: want %d %s, got %d %s", rt.method, rt.path, status, code, rr.Code, rr.Body.String())
			}
		}
	}
	expect(http.StatusConflict, "integration_not_ready") // disabled
	f.configure(false)
	expect(http.StatusNotImplemented, "integration_unsupported") // legacy bearer-key client has no vendor API

	// Vendor API enabled but no club ID: only the routes that need one refuse.
	base, key, enabled, tok, cid, club := "https://swims.example.test", "k-12345678", true, "https://auth.example.test/token", "client-1", ""
	if _, err := f.svc.Update(cov2xFinOrg, usaswimming.Updates{BaseURL: &base, APIKey: &key, Enabled: &enabled, TokenURL: &tok, ClientID: &cid, ClubID: &club}, "t"); err != nil {
		t.Fatal(err)
	}
	for _, rt := range []cov2xRoute{
		{http.MethodGet, cov2xUSABase + "/roster-compare", false},
		{http.MethodGet, cov2xUSABase + "/registration-link", false},
		{http.MethodPost, cov2xUSABase + "/verify-members", false},
		{http.MethodPost, cov2xUSABase + "/registrations", false},
	} {
		rr := f.do(rt.method, rt.path, "admin", "{}")
		if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "club ID") {
			t.Fatalf("%s %s without club id: %d %s", rt.method, rt.path, rr.Code, rr.Body.String())
		}
	}
	f.vendor.clubs = []usaswimming.Club{{ClubID: "C1", Name: "Dolphins"}}
	if clubs := f.must(http.StatusOK, http.MethodGet, cov2xUSABase+"/clubs", "admin", "")["clubs"].([]any); len(clubs) != 1 {
		t.Fatalf("clubs list works without a club id: %v", clubs)
	}
}

func TestCov2xUSAVendorReadRoutes(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(true)

	f.vendor.clubs = []usaswimming.Club{{ClubID: "C1", Name: "Dolphins", Code: "DOLF"}}
	clubs := f.must(http.StatusOK, http.MethodGet, cov2xUSABase+"/clubs", "admin", "")["clubs"].([]any)
	if clubs[0].(map[string]any)["name"] != "Dolphins" {
		t.Fatalf("clubs: %v", clubs)
	}
	f.vendor.clubsErr = usaswimming.ErrUnauthorized
	if code := cov2xErrCode(f.must(http.StatusForbidden, http.MethodGet, cov2xUSABase+"/clubs", "admin", "")); code != "tier_not_authorized" {
		t.Fatalf("clubs tier: %q", code)
	}

	f.vendor.link = "https://register.example.test/join"
	res := f.must(http.StatusOK, http.MethodGet, cov2xUSABase+"/registration-link", "admin", "")
	if res["registration_link"] != f.vendor.link || f.vendor.gotClubID != "CLUB1" {
		t.Fatalf("link: %v (club %q)", res, f.vendor.gotClubID)
	}
	f.vendor.linkErr = errors.New("503 upstream")
	if code := cov2xErrCode(f.must(http.StatusBadGateway, http.MethodGet, cov2xUSABase+"/registration-link", "admin", "")); code != "integration_unavailable" {
		t.Fatalf("link error: %q", code)
	}

	// Roster comparison is read-only and audits counts, never names.
	f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "aaa-111")
	f.addSwimmer("Bo", "Park", "2011-01-01", "male", "BBB222")
	f.addSwimmer("Di", "Wu", "2010-01-01", "female", "DDD444")
	f.vendor.roster = []usaswimming.RosterMember{
		{MemberID: "AAA111", FirstName: "Ann", LastName: "Lee", DateOfBirth: "2012-04-06"},
		{MemberID: "BBB222", FirstName: "Bo", LastName: "Kim", DateOfBirth: "2011-01-01"},
		{MemberID: "CCC333", FirstName: "Cy", LastName: "Ng"},
	}
	cmp := f.must(http.StatusOK, http.MethodGet, cov2xUSABase+"/roster-compare", "admin", "")
	if cov2xNum(cmp, "swims_count") != 3 || cov2xNum(cmp, "matched") != 2 ||
		len(cmp["only_in_swims"].([]any)) != 1 || len(cmp["only_in_aquify"].([]any)) != 1 || len(cmp["mismatched"].([]any)) != 2 {
		t.Fatalf("compare: %v", cmp)
	}
	diffs := map[string]string{}
	for _, raw := range cmp["mismatched"].([]any) {
		row := raw.(map[string]any)
		diffs[row["name"].(string)] = fmt.Sprint(row["differences"])
	}
	if diffs["Ann Lee"] != "[date of birth differs]" || diffs["Bo Kim"] != "[last name differs]" {
		t.Fatalf("mismatch reasons: %v", diffs)
	}
	metas := f.auditMeta("integrations.usa_swimming.roster_compare")
	if len(metas) != 1 || metas[0]["matched"] != "2" || metas[0]["only_in_swims"] != "1" || metas[0]["mismatched"] != "2" {
		t.Fatalf("compare audit: %v", metas)
	}
	for _, v := range metas[0] {
		if strings.Contains(v, "Lee") || strings.Contains(v, "Kim") {
			t.Fatalf("roster contents leaked to the audit log: %v", metas[0])
		}
	}
	f.vendor.rosterErr = usaswimming.ErrUnauthorized
	f.must(http.StatusForbidden, http.MethodGet, cov2xUSABase+"/roster-compare", "admin", "")
}

func TestCov2xUSAVerifyMembers(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(true)
	path := cov2xUSABase + "/verify-members"
	if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, path, "admin", "")); code != "invalid_swimmer_count" {
		t.Fatalf("no swimmers with ids: %q", code)
	}
	if rr := f.do(http.MethodPost, path, "admin", `{bad`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", rr.Code)
	}
	ann := f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "aaa-111")
	f.addSwimmer("Bo", "Kim", "2011-01-01", "male", "BBB222")
	f.addSwimmer("Cy", "Ng", "2010-01-01", "male", "")
	f.vendor.checks = []usaswimming.MemberCheck{{MemberID: "AAA111", Valid: true}, {MemberID: "BBB222", Valid: false, Detail: "expired"}}

	res := f.must(http.StatusOK, http.MethodPost, path, "admin", "")
	if cov2xNum(res, "invalid") != 1 || len(res["results"].([]any)) != 2 {
		t.Fatalf("verify result: %v", res)
	}
	byID := map[string]string{}
	for _, raw := range res["results"].([]any) {
		row := raw.(map[string]any)
		byID[row["member_id"].(string)] = row["name"].(string)
	}
	if byID["AAA111"] != "Ann Lee" || byID["BBB222"] != "Bo Kim" {
		t.Fatalf("names must be joined from the roster: %v", byID)
	}
	if len(f.vendor.gotMemberIDs) != 2 || f.vendor.gotClubID != "CLUB1" {
		t.Fatalf("vendor input: %v club=%s", f.vendor.gotMemberIDs, f.vendor.gotClubID)
	}
	if metas := f.auditMeta("integrations.usa_swimming.verify_members"); len(metas) != 1 || metas[0]["checked"] != "2" || metas[0]["invalid"] != "1" {
		t.Fatalf("audit: %v", metas)
	}

	f.vendor.checks = []usaswimming.MemberCheck{{MemberID: "AAA111", Valid: true}}
	f.must(http.StatusOK, http.MethodPost, path, "admin", fmt.Sprintf(`{"swimmer_ids":[%q]}`, ann.ID))
	if len(f.vendor.gotMemberIDs) != 1 || f.vendor.gotMemberIDs[0] != "AAA111" {
		t.Fatalf("swimmer_ids must narrow the vendor call: %v", f.vendor.gotMemberIDs)
	}

	f.vendor.checksErr = usaswimming.ErrUnauthorized
	if code := cov2xErrCode(f.must(http.StatusForbidden, http.MethodPost, path, "admin", "")); code != "tier_not_authorized" {
		t.Fatalf("tier 3 required: %q", code)
	}
}

func TestCov2xUSAVerifyTime(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(true)
	path := cov2xUSABase + "/verify-time"
	ann := f.addSwimmer("Ann", "Lee", "2012-04-05", "female", "aaa-111")
	noID := f.addSwimmer("Cy", "Ng", "2010-01-01", "male", "")

	if rr := f.do(http.MethodPost, path, "admin", `{bad`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", rr.Code)
	}
	for _, body := range []string{
		fmt.Sprintf(`{"swimmer_id":%q,"event":"","time":"00:52.34"}`, ann.ID),
		fmt.Sprintf(`{"swimmer_id":%q,"event":"100 Free SCY","time":"52.34"}`, ann.ID),
		fmt.Sprintf(`{"swimmer_id":%q,"event":"100 Free SCY","time":"00:52.3"}`, ann.ID),
	} {
		if code := cov2xErrCode(f.must(http.StatusUnprocessableEntity, http.MethodPost, path, "admin", body)); code != "validation_failed" {
			t.Fatalf("%s: %q", body, code)
		}
	}
	for _, id := range []string{"ghost", noID.ID} {
		rr := f.do(http.MethodPost, path, "admin", fmt.Sprintf(`{"swimmer_id":%q,"event":"100 Free SCY","time":"00:52.34"}`, id))
		if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "no USA Swimming ID") {
			t.Fatalf("swimmer %q: %d %s", id, rr.Code, rr.Body.String())
		}
	}
	ok := fmt.Sprintf(`{"swimmer_id":%q,"event":" 100 Free SCY ","time":" 00:52.34 "}`, ann.ID)
	f.vendor.timeValid = true
	if res := f.must(http.StatusOK, http.MethodPost, path, "admin", ok); res["valid"] != true {
		t.Fatalf("valid: %v", res)
	}
	if f.vendor.gotMember != "AAA111" || f.vendor.gotEvent != "100 Free SCY" || f.vendor.gotTime != "00:52.34" {
		t.Fatalf("vendor input: %q %q %q", f.vendor.gotMember, f.vendor.gotEvent, f.vendor.gotTime)
	}
	f.vendor.timeValid = false
	if res := f.must(http.StatusOK, http.MethodPost, path, "admin", ok); res["valid"] != false {
		t.Fatalf("invalid: %v", res)
	}
	f.vendor.timeErr = usaswimming.ErrUnauthorized
	f.must(http.StatusForbidden, http.MethodPost, path, "admin", ok)
}

func TestCov2xUSARegistrations(t *testing.T) {
	t.Parallel()
	f := cov2xNewUSAFixture(t)
	f.configure(true)
	path := cov2xUSABase + "/registrations"
	if rr := f.do(http.MethodPost, path, "admin", `{bad`); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", rr.Code)
	}
	rr := f.do(http.MethodPost, path, "admin", `{"first_name":"A1","last_name":"","date_of_birth":"2012-13-01","competition_category":"OTHER","phone":"123"}`)
	if rr.Code != http.StatusUnprocessableEntity || cov2xErrCode(cov2xDecode(t, rr)) != "validation_failed" ||
		!strings.Contains(rr.Body.String(), "names must be") || !strings.Contains(rr.Body.String(), "date_of_birth") || !strings.Contains(rr.Body.String(), "competition_category") {
		t.Fatalf("local validation must list every problem: %d %s", rr.Code, rr.Body.String())
	}
	if f.vendor.gotReg.FirstName != "" {
		t.Fatal("an invalid registration must not reach the vendor")
	}

	good := `{"first_name":"Ann","last_name":"Lee","date_of_birth":"2012-04-05","competition_category":"female","postal_code":"12345"}`
	f.vendor.submitRes = usaswimming.RegistrationResult{Status: "Payment Pending", RequestID: "req-1", RegistrationOK: true}
	res := cov2xObj(t, f.must(http.StatusOK, http.MethodPost, path, "admin", good), "result")
	if res["request_id"] != "req-1" || res["status"] != "Payment Pending" || f.vendor.gotReg.LastName != "Lee" || f.vendor.gotClubID != "CLUB1" {
		t.Fatalf("submit: %v / %+v", res, f.vendor.gotReg)
	}
	if metas := f.auditMeta("integrations.usa_swimming.registration_submit"); len(metas) != 1 || metas[0]["request_id"] != "req-1" || metas[0]["ok"] != "true" {
		t.Fatalf("audit: %v", metas)
	}
	f.vendor.submitErr = errors.New("vendor down")
	if code := cov2xErrCode(f.must(http.StatusBadGateway, http.MethodPost, path, "admin", good)); code != "integration_unavailable" {
		t.Fatalf("submit failure: %q", code)
	}

	f.vendor.statusValue = "Processed"
	st := f.must(http.StatusOK, http.MethodGet, path+"/req-1/status", "admin", "")
	if st["status"] != "Processed" || f.vendor.gotRequestID != "req-1" {
		t.Fatalf("status: %v (asked for %q)", st, f.vendor.gotRequestID)
	}
	f.vendor.statusErr = usaswimming.ErrUnauthorized
	f.must(http.StatusForbidden, http.MethodGet, path+"/req-1/status", "admin", "")
}
