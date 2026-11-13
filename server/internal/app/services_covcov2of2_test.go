package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/events"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
	"github.com/luryann/aquify/server/internal/runtimepersist"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov2xOrg = "org-cov2x"

// ---- billing roster ----

func TestCov2xBillingRosterWithoutMembersIsUnavailable(t *testing.T) {
	t.Parallel()
	_, err := newBillingRosterSource(nil, groups.NewService())(cov2xOrg)
	if !errors.Is(err, billing.ErrRosterUnavailable) {
		t.Fatalf("want ErrRosterUnavailable, got %v", err)
	}
}

func TestCov2xBillingRosterProjectsMembersAndGroups(t *testing.T) {
	t.Parallel()
	admin := testutil.Principal(testutil.PrincipalOptions{UserID: "cov2x-admin", OrgID: cov2xOrg, Roles: []string{"admin"}})
	memberSvc := members.NewService()
	groupSvc := groups.NewService()

	hh, err := memberSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov2xOrg, HouseholdName: "Roster Family", PrimaryEmail: "roster@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := memberSvc.CreateSwimmer(admin, cov2xOrg, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Ada", LastName: "Roster", DateOfBirth: "2014-03-02", Gender: "female", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	grp, _, err := groupSvc.CreateGroup(admin, cov2xOrg, groups.CreateGroupInput{Code: "gold", Name: "Gold Squad"})
	if err != nil {
		t.Fatal(err)
	}
	groupSvc.SeedSwimmer(cov2xOrg, sw.ID, hh.ID, sw.DateOfBirth, sw.Gender, sw.RegistrationStatus)
	reg, err := groupSvc.CreateRegistration(admin, cov2xOrg, groups.CreateRegistrationInput{HouseholdID: hh.ID, SwimmerID: sw.ID, GroupID: grp.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := memberSvc.AssignSwimmerToGroup(admin, cov2xOrg, sw.ID, members.AssignSwimmerGroupInput{GroupCode: grp.Code, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}

	snap, err := newBillingRosterSource(memberSvc, groupSvc)(cov2xOrg)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Households) != 1 || snap.Households[0] != (billing.RosterHousehold{ID: hh.ID, Name: "Roster Family", Status: hh.Status}) {
		t.Fatalf("households: %+v", snap.Households)
	}
	if len(snap.Swimmers) != 1 || snap.Swimmers[0].ID != sw.ID || snap.Swimmers[0].HouseholdID != hh.ID ||
		snap.Swimmers[0].FirstName != "Ada" || snap.Swimmers[0].DateOfBirth != "2014-03-02" || snap.Swimmers[0].Status != "active" {
		t.Fatalf("swimmers: %+v", snap.Swimmers)
	}
	if len(snap.GroupLinks) != 1 || snap.GroupLinks[0].SwimmerID != sw.ID || snap.GroupLinks[0].GroupCode != link.GroupCode || !snap.GroupLinks[0].IsPrimary {
		t.Fatalf("links: %+v", snap.GroupLinks)
	}
	if len(snap.Groups) != 1 || snap.Groups[0].Code != "GOLD" || snap.Groups[0].Name != "Gold Squad" {
		t.Fatalf("groups: %+v", snap.Groups)
	}
	if len(snap.Registrations) != 1 || snap.Registrations[0].ID != reg.ID || snap.Registrations[0].GroupCode != "GOLD" || snap.Registrations[0].Season != "2026" {
		t.Fatalf("registrations: %+v", snap.Registrations)
	}

	// Other clubs see nothing, and a nil groups service leaves group data empty.
	other, err := newBillingRosterSource(memberSvc, nil)("org-someone-else")
	if err != nil || len(other.Households) != 0 || len(other.Groups) != 0 || len(other.Registrations) != 0 {
		t.Fatalf("other org: %+v %v", other, err)
	}
	noGroups, err := newBillingRosterSource(memberSvc, nil)(cov2xOrg)
	if err != nil || len(noGroups.Households) != 1 || len(noGroups.Groups) != 0 {
		t.Fatalf("nil groups: %+v %v", noGroups, err)
	}
}

func TestCov2xBillingRosterPropagatesDatabaseFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	memberSvc := members.NewServiceWithPool(pool)
	pool.Close()
	_, err := newBillingRosterSource(memberSvc, nil)(cov2xOrg)
	if err == nil {
		t.Fatal("expected the members load failure to surface, got nil")
	}
}

// ---- org versions ----

func TestCov2xPruneOrgDataVersionsNilPoolIsNoop(t *testing.T) {
	t.Parallel()
	pruneOrgDataVersions(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCov2xPruneOrgDataVersionsLogsFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	pool.Close()
	var buf bytes.Buffer
	pruneOrgDataVersionsNow(pool, slog.New(slog.NewTextHandler(&buf, nil)))
	if !strings.Contains(buf.String(), "prune org_data_versions failed") {
		t.Fatalf("expected failure to be logged, got %q", buf.String())
	}
}

func TestCov2xPruneOrgDataVersionsBackgroundRun(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	ctx := context.Background()
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	for _, w := range []string{"old-a", "old-b"} {
		if _, err := pool.Exec(ctx, `INSERT INTO org_data_versions (org_id, domain, writer, version, touched_at)
			VALUES ($1::uuid, 'billing', $2, 3, now() - interval '30 days')`, org, w); err != nil {
			t.Fatal(err)
		}
	}
	pruneOrgDataVersions(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// The prune runs in the background; poll with a deadline until the two stale rows fold.
	deadline := time.Now().Add(15 * time.Second)
	for {
		var rows, sum int
		if err := pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(version),0)::int FROM org_data_versions WHERE org_id=$1::uuid`, org).Scan(&rows, &sum); err != nil {
			t.Fatal(err)
		}
		if rows == 1 {
			if sum != 6 {
				t.Fatalf("folding must preserve the version sum, got %d", sum)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stale rows were not folded in time (rows=%d)", rows)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---- performance directory ----

func TestCov2xPerformanceDirectoryNilServices(t *testing.T) {
	t.Parallel()
	d := newPerformanceDirectory(nil, nil)
	if got := d.SwimmerProfiles(cov2xOrg); got != nil {
		t.Fatalf("nil members -> nil profiles, got %v", got)
	}
	if got := d.Meets(cov2xOrg); got != nil {
		t.Fatalf("nil meets -> nil meets, got %v", got)
	}
}

func TestCov2xPerformanceDirectoryMapsRecords(t *testing.T) {
	t.Parallel()
	admin := testutil.Principal(testutil.PrincipalOptions{UserID: "cov2x-admin", OrgID: cov2xOrg, Roles: []string{"admin"}})
	memberSvc := members.NewService()
	meetSvc := meets.NewService()

	hh, err := memberSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov2xOrg, HouseholdName: "Dir Family", PrimaryEmail: "dir@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := memberSvc.CreateSwimmer(admin, cov2xOrg, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Bo", LastName: "Dir", PreferredName: "Bobby", DateOfBirth: "2012-05-06", Gender: "male", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"GOLD", "SILVER"} {
		if _, err := memberSvc.AssignSwimmerToGroup(admin, cov2xOrg, sw.ID, members.AssignSwimmerGroupInput{GroupCode: code}); err != nil {
			t.Fatal(err)
		}
	}
	meet, _, err := meetSvc.CreateMeet(admin, cov2xOrg, meets.CreateMeetInput{Name: "Dir Invite", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-06"})
	if err != nil {
		t.Fatal(err)
	}

	d := newPerformanceDirectory(memberSvc, meetSvc)
	profiles := d.SwimmerProfiles(cov2xOrg)
	if len(profiles) != 1 {
		t.Fatalf("profiles: %+v", profiles)
	}
	p := profiles[0]
	if p.ID != sw.ID || p.HouseholdID != hh.ID || p.FirstName != "Bo" || p.LastName != "Dir" || p.PreferredName != "Bobby" ||
		p.DateOfBirth != "2012-05-06" || p.Gender != "male" || len(p.GroupCodes) != 2 {
		t.Fatalf("profile: %+v", p)
	}
	if got := d.SwimmerProfiles("org-other"); len(got) != 0 {
		t.Fatalf("other org leaked profiles: %+v", got)
	}

	ms := d.Meets(cov2xOrg)
	if len(ms) != 1 || ms[0].ID != meet.ID || ms[0].Name != "Dir Invite" || ms[0].StartDate != "2026-12-05" || ms[0].Course != "SCY" {
		t.Fatalf("meets: %+v", ms)
	}
}

// ---- public site wiring ----

type cov2xFailingStaffKC struct {
	*keycloak.StubClient
	err error
}

func (f cov2xFailingStaffKC) ListUsersWithRealmRole(context.Context, string, string) ([]keycloak.User, error) {
	return nil, f.err
}

func cov2xPublicSite(t *testing.T, h interface{ RegisterPublicRoutes(chi.Router) }) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/public/orgs/cov2x/site", nil))
	return rr
}

func cov2xEnabledSettings(t *testing.T) *orgsettings.Service {
	t.Helper()
	svc := orgsettings.NewService()
	slug, on, coaches, name := "cov2x", true, true, "Cov Aquatics"
	if _, err := svc.Update(cov2xOrg, orgsettings.Updates{PublicSlug: &slug, PublicSiteEnabled: &on, PublicShowCoaches: &coaches, TeamName: &name}, "cov2x-admin"); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestCov2xPublicSiteWithoutOptionalSources(t *testing.T) {
	t.Parallel()
	h := buildPublicSiteHandlers(Config{}, cov2xEnabledSettings(t), nil, nil, nil, nil, nil)
	rr := cov2xPublicSite(t, h)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Cov Aquatics") {
		t.Fatalf("want 200 with club name, got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"coaches":[]`) || !strings.Contains(rr.Body.String(), `"meets":[]`) {
		t.Fatalf("missing sources must yield empty lists: %s", rr.Body.String())
	}
}

func TestCov2xPublicSiteSourcesFailClosed(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	meetSvc := meets.NewServiceWithPool(pool)
	eventSvc := events.NewServiceWithPool(pool)
	kc := cov2xFailingStaffKC{StubClient: keycloak.NewStubClient(), err: errors.New("keycloak down")}
	pool.Close() // every DB-backed source now errors

	h := buildPublicSiteHandlers(Config{}, cov2xEnabledSettings(t), comms.NewService(), meetSvc, eventSvc, kc, nil)
	rr := cov2xPublicSite(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("source errors must degrade to empty lists, got %d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Meets   []any `json:"meets"`
		Events  []any `json:"events"`
		Coaches []any `json:"coaches"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Meets) != 0 || len(out.Events) != 0 || len(out.Coaches) != 0 {
		t.Fatalf("failed sources must expose nothing, got %s", rr.Body.String())
	}
}

func TestCov2xPublicSiteListsOnlyNamedStaff(t *testing.T) {
	t.Parallel()
	kc := keycloak.NewStubClient()
	add := func(id, first, last, role string) {
		kc.AddUser(keycloak.User{ID: id, Email: id + "@example.test", FirstName: first, LastName: last, Enabled: true, Attributes: map[string][]string{"org_id": {cov2xOrg}}})
		if err := kc.AssignRealmRole(context.Background(), id, role); err != nil {
			t.Fatal(err)
		}
	}
	add("u-coach", "Casey", "Coach", "coach")
	add("u-noname", "", "", "coach")
	h := buildPublicSiteHandlers(Config{}, cov2xEnabledSettings(t), nil, nil, nil, kc, func() time.Time { return time.Unix(1_700_000_000, 0) })
	rr := cov2xPublicSite(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Casey Coach") || !strings.Contains(body, `"role":"Coach"`) {
		t.Fatalf("named coach missing: %s", body)
	}
	if strings.Contains(body, "u-noname") || strings.Contains(body, "example.test") {
		t.Fatalf("unnamed staff / emails must never be public: %s", body)
	}
}

// ---- runtime persistence ----

type cov2xRecordingSnapshotter struct {
	exports int
	payload string
	exportE error
}

func (s *cov2xRecordingSnapshotter) ExportRuntimeSnapshot() ([]byte, error) {
	s.exports++
	return []byte(s.payload), s.exportE
}
func (s *cov2xRecordingSnapshotter) LoadRuntimeSnapshot([]byte) error { return nil }

func TestCov2xLoadRuntimeSnapshotsSkipsNilNullAndLoadErrors(t *testing.T) {
	// Not parallel: t.Setenv.
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	// Unavailable repository: nothing happens, nothing is logged.
	loadRuntimeSnapshots(logger, nil, []runtimeParticipant{{domain: "members", service: &cov2xRecordingSnapshotter{}}})
	if logs.Len() != 0 {
		t.Fatalf("unexpected logs: %s", logs.String())
	}

	// "null" payloads (hex of null) and nil services are skipped without error.
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{selectOutput: "members\t6e756c6c\ngroups\t7b7d"}))
	repo := runtimepersist.NewRepository("postgres://runtime-test")
	loadRuntimeSnapshots(logger, repo, []runtimeParticipant{
		{domain: "members", service: &cov2xRecordingSnapshotter{}},
		{domain: "groups", service: nil},
		{domain: "meets", service: &cov2xRecordingSnapshotter{}},
	})
	if logs.Len() != 0 {
		t.Fatalf("skipped payloads must not log, got %s", logs.String())
	}
}

func TestCov2xLoadRuntimeSnapshotsLogsLoadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell psql stand-in")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\necho 'connection refused' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "psql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := &cov2xRecordingSnapshotter{}
	loadRuntimeSnapshots(logger, runtimepersist.NewRepository("postgres://runtime-test"), []runtimeParticipant{{domain: "members", service: svc}})
	if !strings.Contains(logs.String(), "runtime snapshot load failed") || !strings.Contains(logs.String(), "connection refused") {
		t.Fatalf("expected the load failure to be logged with its cause, got %q", logs.String())
	}
}

func TestCov2xPersistRuntimeSnapshotDomains(t *testing.T) {
	// Not parallel: t.Setenv.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// No repository: a no-op that never exports.
	svc := &cov2xRecordingSnapshotter{payload: `{}`}
	if err := persistRuntimeSnapshotDomains(logger, nil, []runtimeParticipant{{domain: "members", service: svc}}, nil); err != nil || svc.exports != 0 {
		t.Fatalf("nil repo: err=%v exports=%d", err, svc.exports)
	}

	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{}))
	repo := runtimepersist.NewRepository("postgres://runtime-test")

	// Only the requested domains are exported; nil services are skipped.
	members1 := &cov2xRecordingSnapshotter{payload: `{"a":1}`}
	groups1 := &cov2xRecordingSnapshotter{payload: `{"b":2}`}
	participants := []runtimeParticipant{{domain: "members", service: members1}, {domain: "groups", service: groups1}, {domain: "meets", service: nil}}
	if err := persistRuntimeSnapshotDomains(logger, repo, participants, map[string]struct{}{"members": {}, "meets": {}}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if members1.exports != 1 || groups1.exports != 0 {
		t.Fatalf("exports members=%d groups=%d, want 1/0", members1.exports, groups1.exports)
	}

	// An export failure aborts before anything is saved and is returned verbatim.
	boom := errors.New("export boom")
	bad := &cov2xRecordingSnapshotter{exportE: boom}
	if err := persistRuntimeSnapshots(logger, repo, []runtimeParticipant{{domain: "members", service: bad}}); !errors.Is(err, boom) {
		t.Fatalf("want export error, got %v", err)
	}
}

func TestCov2xRuntimeSnapshotDomainsFor(t *testing.T) {
	t.Parallel()
	if got := runtimeSnapshotDomainsFor("/health"); got != nil {
		t.Fatalf("paths outside /api/v1 snapshot everything (nil), got %v", got)
	}
	if got := runtimeSnapshotDomainsFor("/api/v1/unknown-thing/1"); got != nil {
		t.Fatalf("unlisted segments snapshot everything (nil), got %v", got)
	}
	got := runtimeSnapshotDomainsFor("/api/v1/invoices/abc/void")
	if _, ok := got["billing"]; !ok || len(got) != 2 {
		t.Fatalf("invoices -> billing+comms, got %v", got)
	}
	if got := runtimeSnapshotDomainsFor("/api/v1/club-events"); got == nil || len(got) != 0 {
		t.Fatalf("db-only domains skip snapshots (empty non-nil), got %v", got)
	}
}

func TestCov2xRuntimePersistenceMiddlewareSkipsNonSnapshotRequests(t *testing.T) {
	// Not parallel: t.Setenv.
	t.Setenv("PATH", prependFakePSQL(t, fakePSQLBehavior{}))
	repo := runtimepersist.NewRepository("postgres://runtime-test")
	svc := &cov2xRecordingSnapshotter{payload: `{}`}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := runtimePersistenceMiddleware(logger, repo, []runtimeParticipant{{domain: "members", service: svc}})(next)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/members/households"},   // not a mutation
		{http.MethodPost, "/api/v1/auth/sessions/issue"}, // auth routes never snapshot
		{http.MethodPost, "/api/v1/club-events"},         // domain outside the bridge
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(c.method, c.path, nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("%s %s: got %d", c.method, c.path, rr.Code)
		}
	}
	if svc.exports != 0 {
		t.Fatalf("no snapshot should have been taken, got %d exports", svc.exports)
	}

	// A successful member mutation does snapshot, only its domains.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/households", nil))
	if svc.exports != 1 {
		t.Fatalf("expected exactly one snapshot after a successful household mutation, got %d", svc.exports)
	}
	// A handler that never writes a status is treated as 200 and still snapshots.
	silent := runtimePersistenceMiddleware(logger, repo, []runtimeParticipant{{domain: "members", service: svc}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	silent.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/households", nil))
	if svc.exports != 2 {
		t.Fatalf("an implicit 200 must snapshot, got %d exports", svc.exports)
	}
	// Failed mutations (>=400) are not snapshotted.
	fail := runtimePersistenceMiddleware(logger, repo, []runtimeParticipant{{domain: "members", service: svc}})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusConflict) }))
	fail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/households", nil))
	if svc.exports != 2 {
		t.Fatalf("a rejected mutation must not snapshot, got %d exports", svc.exports)
	}
}
