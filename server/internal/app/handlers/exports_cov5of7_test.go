package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/luryann/aquify/server/internal/testutil"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/meetfiles/entries"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/app/performance"
	"github.com/luryann/aquify/server/internal/storage"
)

const (
	cov5xExpParentUID = "5c0a0000-0000-4000-8000-0000000000a3"
	cov5xExpOtherHH   = "5c0a0000-0000-4000-8000-0000000000b2"
)

func cov5xExpTokens() tokenResolver {
	t := cov5xTokens()
	t["parent2"] = &appmiddleware.Principal{UserID: "5c0a0000-0000-4000-8000-0000000000a5", OrgID: cov5xOrg, HouseholdID: cov5xExpOtherHH, Roles: []string{"parent"}}
	t["guest"] = &appmiddleware.Principal{UserID: "5c0a0000-0000-4000-8000-0000000000a6", OrgID: cov5xOrg, Roles: []string{"guest"}}
	return t
}

func cov5xExpMount(h *ExportsHandlers) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xExpTokens()))
		p.Use(appmiddleware.RequireOrgScope)
		h.RegisterRoutes(p)
	})
	return r
}

// cov5xFailStore is an object store whose reads and writes can be made to fail.
type cov5xFailStore struct {
	*storage.StubStorage
	failPut bool
	failGet bool
}

func (f *cov5xFailStore) PutObject(ctx context.Context, key, contentType string, data []byte) error {
	if f.failPut {
		return errors.New("disk full")
	}
	return f.StubStorage.PutObject(ctx, key, contentType, data)
}

func (f *cov5xFailStore) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	if f.failGet {
		return nil, "", errors.New("store offline")
	}
	return f.StubStorage.GetObject(ctx, key)
}

type cov5xExpFixture struct {
	meets    *meets.Service
	members  *members.Service
	settings *orgsettings.Service
	exports  *exports.Service
	store    *cov5xFailStore
	audit    *audit.Service
	h        *ExportsHandlers
	router   http.Handler
	admin    *appmiddleware.Principal

	meetID    string
	eventIDs  map[string]string // event number -> id
	swimmers  map[string]string // first name -> swimmer id
	entryIDs  map[string]string // "<first>/<event number>" -> entry id
	orphanID  string
	submitted string // id of the entry left unapproved
}

func cov5xExpIntPtr(v int) *int { return &v }

// cov5xNewExpFixture builds a finalized meet with swimmers of every gender
// vocabulary, eligible and ineligible entries, a relay entry, an unapproved
// entry, and an entry for a swimmer unknown to the members directory.
func cov5xNewExpFixture(t *testing.T) *cov5xExpFixture {
	t.Helper()
	admin := &appmiddleware.Principal{UserID: cov5xAdmin, OrgID: cov5xOrg, Roles: []string{"admin"}}
	f := &cov5xExpFixture{
		meets:    meets.NewService(),
		members:  members.NewService(),
		settings: orgsettings.NewService(),
		exports:  nil,
		store:    &cov5xFailStore{StubStorage: storage.NewStubStorage("http://stub.test")},
		audit:    audit.NewService(),
		admin:    admin,
		eventIDs: map[string]string{},
		swimmers: map[string]string{},
		entryIDs: map[string]string{},
	}
	f.exports = exports.NewServiceWithStorage(f.store)
	f.h = NewExportsHandlersWithAudit(f.exports, f.meets, f.members, f.settings, f.audit)
	f.router = cov5xExpMount(f.h)

	hh, err := f.members.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov5xOrg, HouseholdName: "Rivera", PrimaryEmail: "rivera@example.test"})
	if err != nil {
		t.Fatalf("household: %v", err)
	}
	for _, sw := range []struct{ first, last, dob, gender, uss string }{
		{"Ana", "Rivera", "2012-04-02", "female", "040212ANARIVE"},
		{"Ben", "Rivera", "2013-01-01", "male", "010113BENRIVE"},
		{"Cam", "Rivera", "2011-02-02", "non_binary", ""},
		{"Dan", "Rivera", "2010-03-03", "male", "030310DANRIVE"},
	} {
		created, _, err := f.members.CreateSwimmer(admin, cov5xOrg, members.CreateSwimmerInput{
			HouseholdID: hh.ID, FirstName: sw.first, LastName: sw.last, DateOfBirth: sw.dob, Gender: sw.gender, USASwimmingID: sw.uss,
		})
		if err != nil {
			t.Fatalf("swimmer %s: %v", sw.first, err)
		}
		f.swimmers[sw.first] = created.ID
		f.meets.SeedSwimmer(cov5xOrg, created.ID, cov5xHH)
	}
	f.orphanID = "orphan-swimmer"
	f.meets.SeedSwimmer(cov5xOrg, f.orphanID, cov5xHH)
	f.meets.SeedParentHousehold(cov5xExpParentUID, cov5xHH)

	// Ana: a non-primary and a primary group. Cam: a single non-primary group.
	for _, link := range []struct {
		who, code string
		primary   bool
	}{{"Ana", "silver", false}, {"Ana", "gold", true}, {"Cam", "bronze", false}} {
		if _, err := f.members.AssignSwimmerToGroup(admin, cov5xOrg, f.swimmers[link.who], members.AssignSwimmerGroupInput{GroupCode: link.code, IsPrimary: link.primary}); err != nil {
			t.Fatalf("group link: %v", err)
		}
	}

	code, name, lsc := "AQFY", "Aquify Aquatics", "ne"
	if _, err := f.settings.Update(cov5xOrg, orgsettings.Updates{TeamCode: &code, TeamName: &name, LSCCode: &lsc}, cov5xAdmin); err != nil {
		t.Fatalf("org settings: %v", err)
	}

	meet, _, err := f.meets.CreateMeet(admin, cov5xOrg, meets.CreateMeetInput{
		Name: "Winter Open!", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-06",
		HostOrg: "Host Club", Location: "Pool A",
		Sessions: []meets.CreateMeetSessionInput{{SessionNumber: 1, Name: "Saturday AM", SessionDate: "2026-12-05"}},
		Events: []meets.CreateMeetEventDraftInput{
			{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 50, Gender: "O"},
			{SessionNumber: 1, EventNumber: "2", Stroke: "backstroke", Distance: 100, Gender: "F", MinAge: cov5xExpIntPtr(15)},
			{SessionNumber: 1, EventNumber: "3", Stroke: "butterfly", Distance: 100, Gender: "F", MaxAge: cov5xExpIntPtr(11)},
			{SessionNumber: 1, EventNumber: "4", Stroke: "breaststroke", Distance: 100, Gender: "M"},
			{SessionNumber: 1, EventNumber: "5", Stroke: "medley_relay", Distance: 200, Gender: "O", IsRelay: true},
			{SessionNumber: 1, EventNumber: "6", Stroke: "individual_medley", Distance: 200, Gender: "O"}, // no entries
		},
	})
	if err != nil {
		t.Fatalf("create meet: %v", err)
	}
	f.meetID = meet.ID
	events, err := f.meets.ListMeetEvents(admin, cov5xOrg, meet.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	for _, ev := range events {
		f.eventIDs[ev.EventNumber] = ev.ID
	}
	if len(f.eventIDs) != 6 {
		t.Fatalf("want 6 events, got %v", f.eventIDs)
	}

	if _, _, err := f.meets.TransitionMeetStatus(admin, cov5xOrg, meet.ID, "published"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, _, _, err := f.meets.PublishCommitmentRequest(admin, cov5xOrg, meet.ID); err != nil {
		t.Fatalf("commitment request: %v", err)
	}
	if _, _, err := f.meets.LockCommitments(admin, cov5xOrg, meet.ID); err != nil {
		t.Fatalf("lock: %v", err)
	}

	seed := 30500
	propose := func(who, evNum string, seedMs *int, relay, approve bool) {
		t.Helper()
		sid := f.orphanID
		if who != "orphan" {
			sid = f.swimmers[who]
		}
		e, _, err := f.meets.ProposeEntry(admin, cov5xOrg, meet.ID, meets.ProposeEntryInput{SwimmerID: sid, MeetEventID: f.eventIDs[evNum], SeedTimeMs: seedMs, IsRelay: relay})
		if err != nil {
			t.Fatalf("propose %s/%s: %v", who, evNum, err)
		}
		f.entryIDs[who+"/"+evNum] = e.ID
		if approve {
			if _, _, err := f.meets.ApproveEntry(admin, cov5xOrg, meet.ID, e.ID); err != nil {
				t.Fatalf("approve %s/%s: %v", who, evNum, err)
			}
		}
	}
	propose("Ana", "1", &seed, false, true)
	propose("Ana", "2", nil, false, true)
	propose("Ana", "3", nil, false, true)
	propose("Ana", "5", nil, true, true)
	propose("Cam", "1", nil, false, true)
	propose("Cam", "2", nil, false, true)
	propose("Dan", "2", nil, false, true)
	propose("Dan", "4", nil, false, true)
	propose("orphan", "1", nil, false, true)
	propose("Ben", "1", nil, false, false)
	f.submitted = f.entryIDs["Ben/1"]

	if _, _, err := f.meets.TransitionMeetStatus(admin, cov5xOrg, meet.ID, "entry_finalized"); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return f
}

func (f *cov5xExpFixture) entryStatus(t *testing.T, id string) string {
	t.Helper()
	entries, err := f.meets.ListEntries(f.admin, cov5xOrg, f.meetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == id {
			return e.Status
		}
	}
	t.Fatalf("entry %s not found", id)
	return ""
}

func (f *cov5xExpFixture) generate(t *testing.T, reportType string) string {
	t.Helper()
	rr := cov5xDo(f.router, http.MethodPost, "/meets/"+f.meetID+"/exports", "admin", `{"report_type":"`+reportType+`"}`)
	cov5xWant(t, "generate "+reportType, rr, http.StatusCreated, `"export"`)
	export := cov5xParse(t, rr)["export"].(map[string]any)
	if export["status"] != "ready" || export["export_type"] != reportType {
		t.Fatalf("export not ready: %v", export)
	}
	return export["id"].(string)
}

func TestCov5xExportsConstructorsWithoutDependencies(t *testing.T) {
	t.Parallel()
	h := NewExportsHandlers(exports.NewService())
	router := cov5xExpMount(h)

	rr := cov5xDo(router, http.MethodGet, "/meets/m1/exports", "admin", "")
	cov5xWant(t, "list", rr, http.StatusOK, `"count":0`)
	// Without the meets service no report can be assembled.
	rr = cov5xDo(router, http.MethodPost, "/meets/m1/exports", "admin", `{"report_type":"event_report"}`)
	cov5xWant(t, "generate", rr, http.StatusInternalServerError, "report_assembly_failed")
	if strings.Contains(rr.Body.String(), "not configured") {
		t.Errorf("internal cause leaked to client: %s", rr.Body.String())
	}
	// SD3 export is explicitly unavailable.
	rr = cov5xDo(router, http.MethodPost, "/meets/m1/entries/export", "admin", "")
	cov5xWant(t, "sd3", rr, http.StatusServiceUnavailable, "service_unavailable")
	// With no meets service wired, portal roles cannot be authorized for listings.
	rr = cov5xDo(router, http.MethodGet, "/meets/m1/exports", "parent", "")
	cov5xWant(t, "parent list", rr, http.StatusForbidden, "forbidden")
}

func TestCov5xExportsRoutesRequirePrincipalAndOrgScope(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	bare := chi.NewRouter()
	f.h.RegisterRoutes(bare)
	authOnly := chi.NewRouter()
	authOnly.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(cov5xExpTokens()))
		f.h.RegisterRoutes(p)
	})
	for _, rt := range []struct{ method, path, body string }{
		{"POST", "/meets/m1/exports", `{"report_type":"event_report"}`},
		{"GET", "/meets/m1/exports", ""},
		{"POST", "/meets/m1/entries/export", ""},
		{"GET", "/exports/x1", ""},
		{"GET", "/exports/x1/download", ""},
	} {
		rr := cov5xDo(bare, rt.method, rt.path, "", rt.body)
		cov5xWant(t, rt.method+" "+rt.path+" no principal", rr, http.StatusUnauthorized, "unauthorized")

		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		authOnly.ServeHTTP(rec, req)
		cov5xWant(t, rt.method+" "+rt.path+" no org", rec, http.StatusBadRequest, "missing_org_scope")
	}
}

func TestCov5xExportsGenerateReportAndAudit(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)

	id := f.generate(t, "entry_report")
	events, err := f.audit.Query(audit.Filter{OrgID: cov5xOrg, Action: "export.generated"})
	if err != nil || len(events) != 1 {
		t.Fatalf("want one export.generated audit event, got %d (%v)", len(events), err)
	}
	if events[0].ResourceID != id || events[0].Metadata["meet_id"] != f.meetID || events[0].Metadata["report_type"] != "entry_report" {
		t.Errorf("unexpected audit event: %+v", events[0])
	}

	// The artifact is downloadable as a real PDF with a matching length header.
	rr := cov5xDo(f.router, http.MethodGet, "/exports/"+id+"/download", "coach", "")
	cov5xWant(t, "download", rr, http.StatusOK, "%PDF")
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content type = %q", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="`) {
		t.Errorf("content disposition = %q", cd)
	}
	if got, want := rr.Header().Get("Content-Length"), len(rr.Body.Bytes()); got == "" || got != strconv.Itoa(want) {
		t.Errorf("content length %q does not match body length %d", got, want)
	}

	// Get returns the same job.
	rr = cov5xDo(f.router, http.MethodGet, "/exports/"+id, "admin", "")
	cov5xWant(t, "get", rr, http.StatusOK, id)
}

func TestCov5xExportsGenerateRejections(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	path := "/meets/" + f.meetID + "/exports"

	cov5xWant(t, "parent", cov5xDo(f.router, http.MethodPost, path, "parent", `{"report_type":"event_report"}`), http.StatusForbidden, "Insufficient permissions to generate reports.")
	cov5xWant(t, "guest", cov5xDo(f.router, http.MethodPost, path, "guest", `{"report_type":"event_report"}`), http.StatusForbidden, "forbidden")
	// A blank meet id is rejected before the body is read.
	cov5xWant(t, "blank meet", cov5xDo(f.router, http.MethodPost, "/meets/%20/exports", "admin", `{"report_type":"event_report"}`), http.StatusBadRequest, "missing_meet_id")
	cov5xWant(t, "bad type", cov5xDo(f.router, http.MethodPost, path, "admin", `{"report_type":"payroll"}`), http.StatusBadRequest, "")
	cov5xWant(t, "unknown field", cov5xDo(f.router, http.MethodPost, path, "admin", `{"report_type":"event_report","orgId":"x"}`), http.StatusBadRequest, "")
	cov5xWant(t, "bad grouping", cov5xDo(f.router, http.MethodPost, path, "admin", `{"report_type":"entry_report","grouping":"zodiac"}`), http.StatusBadRequest, "")
	cov5xWant(t, "unknown meet", cov5xDo(f.router, http.MethodPost, "/meets/missing/exports", "admin", `{"report_type":"event_report"}`), http.StatusNotFound, "meet_not_found")

	// Nothing was persisted by the rejected requests.
	if list := f.exports.ListExportsForAuthorizedMeet(cov5xOrg, ""); len(list) != 0 {
		t.Fatalf("rejected requests created exports: %+v", list)
	}
}

func TestCov5xExportsGenerateFailsWhenArtifactCannotBeStored(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	f.store.failPut = true
	rr := cov5xDo(f.router, http.MethodPost, "/meets/"+f.meetID+"/exports", "admin", `{"report_type":"event_report"}`)
	cov5xWant(t, "store failure", rr, http.StatusInternalServerError, "report_failed")
	if strings.Contains(rr.Body.String(), "disk full") {
		t.Errorf("storage error leaked: %s", rr.Body.String())
	}
	events, _ := f.audit.Query(audit.Filter{OrgID: cov5xOrg, Action: "export.generated"})
	if len(events) != 0 {
		t.Errorf("failed generation must not be audited as generated: %+v", events)
	}
}

func TestCov5xExportsAssembleReportInput(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)

	in, err := f.h.assembleReportInput(f.admin, cov5xOrg, f.meetID, "entry_report", "group")
	if err != nil {
		t.Fatal(err)
	}
	if in.ReportType != "entry_report" || in.Grouping != "group" {
		t.Errorf("report type/grouping not carried: %+v", in)
	}
	if in.Team.TeamCode != "AQFY" || in.Team.TeamName != "Aquify Aquatics" || in.Team.LSCCode != "NE" {
		t.Errorf("team identity: %+v", in.Team)
	}
	if in.Meet.Name != "Winter Open!" || in.Meet.StartDate != "2026-12-05" || in.Meet.Course != "SCY" || in.Meet.Location != "Pool A" || in.Meet.HostOrg != "Host Club" {
		t.Errorf("meet: %+v", in.Meet)
	}

	// 4 (Ana) + 2 (Cam) + 2 (Dan) + 1 (orphan); Ben's unapproved entry is left out.
	if len(in.Entries) != 9 {
		t.Fatalf("want 9 entries, got %d: %+v", len(in.Entries), in.Entries)
	}
	byKey := map[string]exports.ReportEntry{}
	for _, e := range in.Entries {
		if e.SwimmerFirst == "Ben" {
			t.Errorf("unapproved entry exported: %+v", e)
		}
		byKey[e.SwimmerFirst+"/"+e.EventNumber] = e
	}
	wantNotes := map[string]struct {
		eligible bool
		note     string
	}{
		"Ana/1": {true, ""},
		"Ana/2": {false, "age 14 below event minimum 15"},
		"Ana/3": {false, "age 14 above event maximum 11"},
		"Dan/4": {true, ""},
		"Ana/5": {true, ""},
		"Cam/1": {true, ""},
		"Cam/2": {false, "non-binary/undisclosed eligible for open/mixed events only"},
		"Dan/2": {false, "swimmer sex (M) does not match event"},
		"/1":    {true, ""}, // orphan: unknown swimmer is treated as undisclosed, which open events accept
	}
	for key, want := range wantNotes {
		got, ok := byKey[key]
		if !ok {
			t.Errorf("missing entry %s", key)
			continue
		}
		if got.Eligible != want.eligible || got.EligibilityNote != want.note {
			t.Errorf("%s: eligible=%v note=%q, want %v %q", key, got.Eligible, got.EligibilityNote, want.eligible, want.note)
		}
	}
	ana := byKey["Ana/1"]
	if ana.SwimmerLast != "Rivera" || ana.USASwimmingID != "040212ANARIVE" || ana.AgeAtMeet != 14 || ana.GroupCode != "GOLD" ||
		ana.SessionName != "Saturday AM" || ana.Stroke != "freestyle" || ana.Distance != 50 || ana.SeedTimeMs == nil || *ana.SeedTimeMs != 30500 {
		t.Errorf("Ana/1 fields: %+v", ana)
	}
	if cam := byKey["Cam/1"]; cam.GroupCode != "BRONZE" {
		t.Errorf("a swimmer with only non-primary groups falls back to that group, got %q", cam.GroupCode)
	}
	if len(in.Events) != 5 {
		t.Fatalf("only events with entries are listed; want 5, got %d", len(in.Events))
	}
	var relay *exports.ReportEvent
	for i := range in.Events {
		if in.Events[i].EventNumber == "6" {
			t.Errorf("an event nobody entered must not be listed: %+v", in.Events[i])
		}
		if in.Events[i].EventNumber == "5" {
			relay = &in.Events[i]
		}
	}
	if relay == nil || !relay.IsRelay || relay.SessionName != "Saturday AM" {
		t.Errorf("relay event: %+v", relay)
	}
	var minAge *int
	for _, ev := range in.Events {
		if ev.EventNumber == "2" {
			minAge = ev.MinAge
		}
	}
	if minAge == nil || *minAge != 15 {
		t.Errorf("event 2 min age not carried: %v", minAge)
	}

	// Errors from the meets service are returned unchanged.
	if _, err := f.h.assembleReportInput(f.admin, cov5xOrg, "missing", "event_report", ""); !errors.Is(err, meets.ErrMeetNotFound) {
		t.Errorf("unknown meet: %v", err)
	}
	// Without members/org settings the report is still assembled, just anonymous.
	bare := NewExportsHandlersWithMeetAccess(f.exports, f.meets, nil, nil)
	in, err = bare.assembleReportInput(f.admin, cov5xOrg, f.meetID, "event_report", "")
	if err != nil {
		t.Fatal(err)
	}
	if in.Team != (exports.ReportTeam{}) || len(in.Entries) != 9 || in.Entries[0].SwimmerFirst != "" {
		t.Errorf("anonymous report: team=%+v entries=%d", in.Team, len(in.Entries))
	}
}

func TestCov5xEvaluateReportEligibility(t *testing.T) {
	t.Parallel()
	ev := func(gender string, minAge, maxAge *int, relay bool) meets.MeetEvent {
		return meets.MeetEvent{Gender: gender, MinAge: minAge, MaxAge: maxAge, IsRelay: relay}
	}
	swimmer := func(gender, dob string) members.Swimmer { return members.Swimmer{Gender: gender, DateOfBirth: dob} }
	for _, tc := range []struct {
		name  string
		sw    members.Swimmer
		ev    meets.MeetEvent
		ok    bool
		fresh string
	}{
		{"relay skips all checks", swimmer("male", "2000-01-01"), ev("F", cov5xExpIntPtr(8), cov5xExpIntPtr(9), true), true, ""},
		{"female in girls", swimmer(" Female ", "2012-04-02"), ev("f", nil, nil, false), true, ""},
		{"female in mixed", swimmer("female", "2012-04-02"), ev("X", nil, nil, false), true, ""},
		{"female in open", swimmer("female", "2012-04-02"), ev("O", nil, nil, false), true, ""},
		{"female in boys", swimmer("female", "2012-04-02"), ev("M", nil, nil, false), false, "swimmer sex (F) does not match event"},
		{"male in boys", swimmer("male", "2012-04-02"), ev("M", nil, nil, false), true, ""},
		{"male in girls", swimmer("male", "2012-04-02"), ev("F", nil, nil, false), false, "swimmer sex (M) does not match event"},
		{"non-binary in open", swimmer("non_binary", "2012-04-02"), ev("O", nil, nil, false), true, ""},
		{"undisclosed in girls", swimmer("undisclosed", "2012-04-02"), ev("F", nil, nil, false), false, "non-binary/undisclosed eligible for open/mixed events only"},
		{"too young", swimmer("male", "2020-01-01"), ev("M", cov5xExpIntPtr(10), nil, false), false, "age 6 below event minimum 10"},
		{"too old", swimmer("male", "2000-01-01"), ev("M", nil, cov5xExpIntPtr(12), false), false, "age 26 above event maximum 12"},
		{"inside band", swimmer("male", "2015-06-01"), ev("M", cov5xExpIntPtr(9), cov5xExpIntPtr(12), false), true, ""},
	} {
		ok, note := evaluateReportEligibility(tc.sw, tc.ev, "2026-12-05")
		if ok != tc.ok || note != tc.fresh {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.name, ok, note, tc.ok, tc.fresh)
		}
	}
}

func TestCov5xExportsListScopingAndAudience(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	eventReport := f.generate(t, "event_report")
	eligibility := f.generate(t, "eligibility_report")
	path := "/meets/" + f.meetID + "/exports"

	// Staff see every report.
	rr := cov5xDo(f.router, http.MethodGet, path, "coach", "")
	cov5xWant(t, "coach list", rr, http.StatusOK, eligibility)
	if got := cov5xParse(t, rr)["count"]; got != float64(2) {
		t.Errorf("staff count = %v", got)
	}

	// A household with a commitment sees only the non-staff report, without internals.
	rr = cov5xDo(f.router, http.MethodGet, path, "parent", "")
	cov5xWant(t, "parent list", rr, http.StatusOK, eventReport)
	if strings.Contains(rr.Body.String(), eligibility) || strings.Contains(rr.Body.String(), "exports/"+cov5xOrg) {
		t.Errorf("parent saw staff-only or internal data: %s", rr.Body.String())
	}
	if got := cov5xParse(t, rr)["count"]; got != float64(1) {
		t.Errorf("parent count = %v", got)
	}

	// A household with no commitment on the meet, and unknown roles, are denied.
	cov5xWant(t, "other household", cov5xDo(f.router, http.MethodGet, path, "parent2", ""), http.StatusForbidden, "forbidden")
	cov5xWant(t, "guest", cov5xDo(f.router, http.MethodGet, path, "guest", ""), http.StatusForbidden, "forbidden")
	// Portal roles cannot probe meet existence.
	cov5xWant(t, "unknown meet", cov5xDo(f.router, http.MethodGet, "/meets/missing/exports", "parent", ""), http.StatusNotFound, "meet_not_found")
}

func TestCov5xExportsListAuthorizationOutageIsInternalError(t *testing.T) {
	pool := testutil.ClosedPool(t)
	h := NewExportsHandlersWithMeetAccess(exports.NewService(), meets.NewServiceWithPool(pool), nil, nil)
	rr := cov5xDo(cov5xExpMount(h), http.MethodGet, "/meets/m1/exports", "parent", "")
	cov5xWant(t, "outage", rr, http.StatusInternalServerError, "Failed to authorize meet file access.")
}

func TestCov5xExportsGetAndDownloadFailureModes(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)

	for _, token := range []string{"parent", "guest"} {
		cov5xWant(t, token+" get", cov5xDo(f.router, http.MethodGet, "/exports/x", token, ""), http.StatusForbidden, "forbidden")
		cov5xWant(t, token+" download", cov5xDo(f.router, http.MethodGet, "/exports/x/download", token, ""), http.StatusForbidden, "forbidden")
	}
	cov5xWant(t, "get unknown", cov5xDo(f.router, http.MethodGet, "/exports/nope", "admin", ""), http.StatusNotFound, "export_not_found")
	cov5xWant(t, "download unknown", cov5xDo(f.router, http.MethodGet, "/exports/nope/download", "admin", ""), http.StatusNotFound, "export_not_found")

	// A failed export is not downloadable.
	failedID := f.generate(t, "event_report")
	f.exports.FailExport(failedID, errors.New("renderer crashed"))
	rr := cov5xDo(f.router, http.MethodGet, "/exports/"+failedID+"/download", "admin", "")
	cov5xWant(t, "not ready", rr, http.StatusConflict, "export_not_ready")

	// The artifact object vanishing from storage reads as not found.
	goneID := f.generate(t, "entry_report")
	gone, err := f.exports.GetExport(f.admin, cov5xOrg, goneID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Delete(gone.StorageKey); err != nil {
		t.Fatal(err)
	}
	cov5xWant(t, "object missing", cov5xDo(f.router, http.MethodGet, "/exports/"+goneID+"/download", "admin", ""), http.StatusNotFound, "export_not_found")

	// A storage outage is an opaque 500.
	outageID := f.generate(t, "entry_report")
	f.store.failGet = true
	rr = cov5xDo(f.router, http.MethodGet, "/exports/"+outageID+"/download", "admin", "")
	cov5xWant(t, "storage outage", rr, http.StatusInternalServerError, "Failed to download export.")
	if strings.Contains(rr.Body.String(), "store offline") {
		t.Errorf("storage error leaked: %s", rr.Body.String())
	}
	f.store.failGet = false

	// Expired artifacts are gone (410) once swept.
	expiredID := f.generate(t, "event_report")
	if swept := f.exports.ExpireArtifacts(time.Now().Add(8 * 24 * time.Hour)); len(swept) == 0 {
		t.Fatal("expected the sweep to expire exports")
	}
	cov5xWant(t, "expired download", cov5xDo(f.router, http.MethodGet, "/exports/"+expiredID+"/download", "admin", ""), http.StatusGone, "export_expired")
}

func TestCov5xExportsGetAndDownloadStorageOutageIsInternalError(t *testing.T) {
	pool := testutil.ClosedPool(t)
	svc := exports.NewServiceWithPool(pool, storage.NewStubStorage("http://stub.test"))
	router := cov5xExpMount(NewExportsHandlers(svc))
	rr := cov5xDo(router, http.MethodGet, "/exports/x1", "admin", "")
	cov5xWant(t, "get outage", rr, http.StatusInternalServerError, "Failed to get export.")
	rr = cov5xDo(router, http.MethodGet, "/exports/x1/download", "admin", "")
	cov5xWant(t, "download outage", rr, http.StatusInternalServerError, "Failed to download export.")
}

func TestCov5xExportsGenerateOutageIsAssemblyFailure(t *testing.T) {
	pool := testutil.ClosedPool(t)
	h := NewExportsHandlersWithMeetAccess(exports.NewService(), meets.NewServiceWithPool(pool), nil, nil)
	rr := cov5xDo(cov5xExpMount(h), http.MethodPost, "/meets/m1/exports", "admin", `{"report_type":"event_report"}`)
	cov5xWant(t, "outage", rr, http.StatusInternalServerError, "report_assembly_failed")
}

func TestCov5xEntryBundleRejections(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	path := "/meets/" + f.meetID + "/entries/export"

	cov5xWant(t, "parent", cov5xDo(f.router, http.MethodPost, path, "parent", ""), http.StatusForbidden, "Only admin or coach can export entry files.")
	cov5xWant(t, "unknown meet", cov5xDo(f.router, http.MethodPost, "/meets/missing/entries/export", "admin", ""), http.StatusNotFound, "meet_not_found")
	if st := f.entryStatus(t, f.entryIDs["Ana/1"]); st != "approved" {
		t.Fatalf("rejected exports must not change entries; Ana/1 is %q", st)
	}

	// Blank team code in settings: the file would be unimportable.
	blank := ""
	if _, err := f.settings.Update(cov5xOrg, orgsettings.Updates{TeamCode: &blank}, cov5xAdmin); err != nil {
		t.Fatal(err)
	}
	cov5xWant(t, "no team code", cov5xDo(f.router, http.MethodPost, path, "admin", ""), http.StatusUnprocessableEntity, "team_code_required")
}

func TestCov5xEntryBundleRequiresFinalizedMeetAndExportableEntries(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	admin := f.admin

	// A meet that is not finalized yet.
	draft, _, err := f.meets.CreateMeet(admin, cov5xOrg, meets.CreateMeetInput{Name: "Draft Meet", MeetType: "invitational", Course: "SCY", StartDate: "2027-01-10", EndDate: "2027-01-10"})
	if err != nil {
		t.Fatal(err)
	}
	rr := cov5xDo(f.router, http.MethodPost, "/meets/"+draft.ID+"/entries/export", "admin", "")
	cov5xWant(t, "draft", rr, http.StatusUnprocessableEntity, "entries_not_finalized")

	// Finalized meet with no approved individual entries.
	empty, _, err := f.meets.CreateMeet(admin, cov5xOrg, meets.CreateMeetInput{Name: "Empty Meet", MeetType: "invitational", Course: "SCY", StartDate: "2027-02-10", EndDate: "2027-02-10"})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []func() error{
		func() error {
			_, _, e := f.meets.TransitionMeetStatus(admin, cov5xOrg, empty.ID, "published")
			return e
		},
		func() error { _, _, _, e := f.meets.PublishCommitmentRequest(admin, cov5xOrg, empty.ID); return e },
		func() error { _, _, e := f.meets.LockCommitments(admin, cov5xOrg, empty.ID); return e },
		func() error {
			_, _, e := f.meets.TransitionMeetStatus(admin, cov5xOrg, empty.ID, "entry_finalized")
			return e
		},
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	rr = cov5xDo(f.router, http.MethodPost, "/meets/"+empty.ID+"/entries/export", "admin", "")
	cov5xWant(t, "no entries", rr, http.StatusUnprocessableEntity, "no_exportable_entries")
}

func cov5xUnzip(t *testing.T, body []byte) (names []string, files map[string]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a zip: %v", err)
	}
	files = map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, f.Name)
		files[f.Name] = string(data)
	}
	return names, files
}

func TestAttachmentDisposition(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"DARE-Entries-Fall Classic; Day 1-14Nov2026-0900.zip", "DARE-Entries-Café Open.zip", "plain.zip"} {
		_, params, err := mime.ParseMediaType(attachmentDisposition(name))
		if err != nil || params["filename"] != name {
			t.Errorf("%q: header %q parsed to %v, %v", name, attachmentDisposition(name), params, err)
		}
	}
}

func TestCov5xEntryBundleDownload(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	path := "/meets/" + f.meetID + "/entries/export"
	zone, line1, city, state, postal := "America/Los_Angeles", "1 Pool Way", "Azusa", "ca", "91702"
	if _, err := f.settings.Update(cov5xOrg, orgsettings.Updates{Timezone: &zone, AddressLine1: &line1, AddressCity: &city, AddressState: &state, AddressPostalCode: &postal}, cov5xAdmin); err != nil {
		t.Fatal(err)
	}

	// Entry files need a male/female competition category for every swimmer;
	// the export says who is missing one instead of writing a broken record.
	rr := cov5xDo(f.router, http.MethodPost, path, "coach", "")
	cov5xWant(t, "category", rr, http.StatusUnprocessableEntity, "Cam Rivera needs a competition category of male or female")
	if st := f.entryStatus(t, f.entryIDs["Cam/1"]); st != "approved" {
		t.Fatalf("a refused export must not change entries; Cam/1 is %q", st)
	}
	male := "male"
	if _, _, err := f.members.UpdateSwimmer(f.admin, cov5xOrg, f.swimmers["Cam"], members.UpdateSwimmerInput{Gender: &male}); err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	rr = cov5xDo(f.router, http.MethodPost, path, "coach", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content type = %q", ct)
	}
	// Ana: 1,2,3; Cam: 1,2; Dan: 2,4. Relay entry skipped, orphan swimmer skipped, Ben unapproved.
	if got := rr.Header().Get("X-Entry-Count"); got != "7" {
		t.Errorf("X-Entry-Count = %q", got)
	}
	if got := rr.Header().Get("X-Athlete-Count"); got != "3" {
		t.Errorf("X-Athlete-Count = %q", got)
	}
	if got := rr.Header().Get("X-Skipped-Orphans"); got != "1" {
		t.Errorf("X-Skipped-Orphans = %q, want the orphan-swimmer entry counted", got)
	}
	if got := rr.Header().Get("X-Skipped-Relays"); got != "1" {
		t.Errorf("X-Skipped-Relays = %q", got)
	}

	// The bundle is named for the club, meet and export time in the club's zone.
	loc, _ := time.LoadLocation(zone)
	var base string
	for _, at := range []time.Time{before, time.Now()} {
		candidate := "AQFY-Entries-Winter Open!-" + at.In(loc).Format("02Jan2006-1504")
		if rr.Header().Get("Content-Disposition") == `attachment; filename="`+candidate+`.zip"` {
			base = candidate
		}
	}
	if base == "" {
		t.Fatalf("content disposition = %q", rr.Header().Get("Content-Disposition"))
	}
	names, files := cov5xUnzip(t, rr.Body.Bytes())
	wantNames := []string{base + ".sd3", base + ".hy3", base + "-EntryReport-ByEvent.pdf", base + "-EntryReport-BySwimmer.pdf", base + "-EntryLog.txt"}
	if strings.Join(names, "|") != strings.Join(wantNames, "|") {
		t.Fatalf("bundle files = %q", names)
	}

	sd3 := files[base+".sd3"]
	var d0, d3 int
	for _, line := range strings.Split(sd3, "\r\n") {
		if line != "" && len(line) != 160 {
			t.Fatalf("SD3 line is %d columns: %q", len(line), line)
		}
		switch {
		case strings.HasPrefix(line, "D0"):
			d0++
			if strings.Contains(line, "Rivera, Ben") {
				t.Errorf("unapproved swimmer exported: %q", line)
			}
		case strings.HasPrefix(line, "D3"):
			d3++
		}
	}
	if d0 != 7 || d3 != 3 || !strings.HasPrefix(sd3, "A01V3      01Meet Entries                  aquify") || !strings.Contains(sd3, "C11        NEAQFYAquify Aquatics") {
		t.Fatalf("SD3: %d D0 / %d D3\n%s", d0, d3, sd3)
	}

	hy3 := files[base+".hy3"]
	var d1, e1 int
	for _, line := range strings.Split(hy3, "\r\n") {
		if line == "" {
			continue
		}
		if len(line) != 130 || entries.HY3Checksum(line[:128]) != line[128:] {
			t.Fatalf("HY3 line has a bad width or checksum: %q", line)
		}
		switch line[:2] {
		case "D1":
			d1++
		case "E1":
			e1++
		}
	}
	if d1 != 3 || e1 != 7 || !strings.Contains(hy3, "C1AQFY Aquify Aquatics") || !strings.Contains(hy3, "C21 Pool Way") || !strings.Contains(hy3, "CA91702     USA") {
		t.Fatalf("HY3: %d D1 / %d E1\n%s", d1, e1, hy3)
	}

	log := files[base+"-EntryLog.txt"]
	for _, want := range []string{
		"Generated By:     \n", "Club Code:        AQFY", "Code (LSC-Club):  NE-AQFY", "Address:          1 Pool Way, Azusa, CA, 91702",
		"Meet Name:        Winter Open!", "Start Date:       December 5, 2026", "Athletes:         3", "Individual Entries: 7", "Relay Teams:      0",
		base + ".hy3\n" + base + "-EntryReport-ByEvent.pdf",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("entry log is missing %q:\n%s", want, log)
		}
	}
	for _, name := range wantNames[2:4] {
		if pdf := files[name]; !strings.HasPrefix(pdf, "%PDF-1.3") || !strings.Contains(pdf, "(Winter Open!) Tj") || !strings.Contains(pdf, "(Meet Summary) Tj") {
			t.Errorf("%s is not a complete entry report", name)
		}
	}

	// Approved entries were marked exported; the unapproved one was left alone.
	for _, key := range []string{"Ana/1", "Ana/2", "Ana/3", "Cam/1", "Cam/2", "Dan/2", "Dan/4"} {
		if st := f.entryStatus(t, f.entryIDs[key]); st != "exported" {
			t.Errorf("%s status = %q, want exported", key, st)
		}
	}
	if st := f.entryStatus(t, f.submitted); st != "submitted" {
		t.Errorf("unapproved entry status = %q", st)
	}
	if st := f.entryStatus(t, f.entryIDs["orphan/1"]); st != "approved" {
		t.Errorf("entry that could not be exported must stay approved, got %q", st)
	}

	events, _ := f.audit.Query(audit.Filter{OrgID: cov5xOrg, Action: "meet.entry_bundle_exported"})
	if len(events) != 1 || events[0].Metadata["bundle"] != base || events[0].Metadata["entry_count"] != "7" || events[0].Metadata["athlete_count"] != "3" ||
		events[0].Metadata["newly_exported"] != "7" || events[0].Metadata["skipped_relays"] != "1" {
		t.Fatalf("audit: %+v", events)
	}

	// Downloading again works from exported entries and exports nothing new.
	rr = cov5xDo(f.router, http.MethodPost, path, "admin", "")
	if rr.Code != http.StatusOK || rr.Header().Get("X-Entry-Count") != "7" {
		t.Fatalf("re-download: %d, X-Entry-Count %q", rr.Code, rr.Header().Get("X-Entry-Count"))
	}
	events, _ = f.audit.Query(audit.Filter{OrgID: cov5xOrg, Action: "meet.entry_bundle_exported"})
	if len(events) != 2 {
		t.Fatalf("want 2 audit events, got %d", len(events))
	}
	var newly []string
	for _, e := range events {
		newly = append(newly, e.Metadata["newly_exported"])
	}
	if !(newly[0] == "0" && newly[1] == "7") && !(newly[0] == "7" && newly[1] == "0") {
		t.Errorf("newly_exported across downloads = %v", newly)
	}
}

func TestCov5xEntryBundleMeetLookupOutage(t *testing.T) {
	pool := testutil.ClosedPool(t)
	h := NewExportsHandlersWithMeetAccess(exports.NewService(), meets.NewServiceWithPool(pool), nil, nil)
	rr := cov5xDo(cov5xExpMount(h), http.MethodPost, "/meets/m1/entries/export", "admin", "")
	cov5xWant(t, "outage", rr, http.StatusInternalServerError, "Failed to load meet data for export.")
}

func TestCov5xWriteEntryBundleMeetErrorMapping(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{meets.ErrMeetNotFound, http.StatusNotFound, "meet_not_found"},
		{meets.ErrForbidden, http.StatusForbidden, "forbidden"},
		{errors.New("db exploded"), http.StatusInternalServerError, "entry_export_failed"},
	} {
		rr := httptest.NewRecorder()
		writeEntryBundleMeetError(rr, httptest.NewRequest(http.MethodPost, "/", nil), tc.err)
		if rr.Code != tc.status || !strings.Contains(rr.Body.String(), tc.code) {
			t.Errorf("%v: got %d %s", tc.err, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "db exploded") {
			t.Errorf("internal error leaked: %s", rr.Body.String())
		}
	}
}

type cov5xBestTimes struct {
	sources []performance.BestTimeSource
	err     error
}

func (b cov5xBestTimes) ListBestTimeSources(string, *appmiddleware.Principal) ([]performance.BestTimeSource, error) {
	return b.sources, b.err
}

func TestCov5xAppendBundleEntries(t *testing.T) {
	t.Parallel()
	seed := 61230
	zero := 0
	at := func(minute int) time.Time { return time.Date(2026, 11, 1, 9, minute, 0, 0, time.UTC) }
	events := map[string]meets.MeetEvent{
		"e-free":  {ID: "e-free", EventNumber: "1", Stroke: "free", Distance: 100, Gender: "o", Course: "", EntryFeeCents: 600},
		"e-lcm":   {ID: "e-lcm", EventNumber: "2", Stroke: "fly", Distance: 50, Gender: "M", Course: "lcm", MinAge: cov5xExpIntPtr(11), MaxAge: cov5xExpIntPtr(12)},
		"e-relay": {ID: "e-relay", EventNumber: "3", Stroke: "free_relay", Distance: 200, Gender: "F", IsRelay: true},
		"e-odd":   {ID: "e-odd", EventNumber: "9", Stroke: "diving", Distance: 1, Gender: "F"},
	}
	swimmers := map[string]members.Swimmer{
		"s1": {ID: "s1", FirstName: "Ana", LastName: "Rivera", MiddleInitial: "Q", PreferredName: "Annie", USASwimmingID: "040212ANARIVE", DateOfBirth: "2012-04-02T00:00:00Z", Gender: "female"},
		"s2": {ID: "s2", FirstName: "Sky", LastName: "Lee", DateOfBirth: "2012-04-02", Gender: "non_binary"},
		"s3": {ID: "s3", FirstName: "Max", LastName: "Ng", DateOfBirth: "", Gender: "male"},
	}
	meetEntries := []meets.Entry{
		// Deliberately out of creation order: output is oldest first.
		{ID: "x8", SwimmerID: "s1", MeetEventID: "e-odd", Status: "approved", CreatedAt: at(8)},
		{ID: "x2", SwimmerID: "s1", MeetEventID: "e-lcm", Status: "exported", SeedTimeMs: &seed, CreatedAt: at(2)},
		{ID: "x1", SwimmerID: "s1", MeetEventID: "e-free", Status: "approved", SeedTimeMs: &zero, CreatedAt: at(1)},
		{ID: "x3", SwimmerID: "s1", MeetEventID: "e-free", Status: "submitted", CreatedAt: at(3)},               // not exportable
		{ID: "x4", SwimmerID: "s1", MeetEventID: "gone", Status: "approved", CreatedAt: at(4)},                  // unknown event
		{ID: "x5", SwimmerID: "s1", MeetEventID: "e-relay", Status: "approved", CreatedAt: at(5)},               // relay event
		{ID: "x6", SwimmerID: "s1", MeetEventID: "e-free", Status: "approved", IsRelay: true, CreatedAt: at(6)}, // relay flag
		{ID: "x7", SwimmerID: "nobody", MeetEventID: "e-free", Status: "approved", CreatedAt: at(7)},            // unknown swimmer
		{ID: "x9", SwimmerID: "s2", MeetEventID: "e-free", Status: "approved", CreatedAt: at(9)},
		{ID: "xa", SwimmerID: "s2", MeetEventID: "e-lcm", Status: "approved", CreatedAt: at(10)},
		{ID: "xb", SwimmerID: "s3", MeetEventID: "e-free", Status: "approved", CreatedAt: at(11)},
	}
	swum := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	sources := map[seedSourceKey]seedSource{
		{"s1", 50, meetfiles.StrokeButterfly, "LCM"}:  {timeMs: seed, meet: "Summer Champs", date: swum},
		{"s1", 100, meetfiles.StrokeFreestyle, "SCY"}: {timeMs: 59000, meet: "Never Used", date: swum},
	}
	input := entries.Input{Meet: entries.Meet{Course: meetfiles.CourseShortCourseYards}}
	ids, skipped, orphans, problems := appendBundleEntries(&input, meetEntries, events, swimmers, sources)
	if strings.Join(ids, ",") != "x1,x2" || skipped != 2 {
		t.Fatalf("ids=%v skipped=%d", ids, skipped)
	}
	// x4 (event gone) and x7 (swimmer gone) are counted so the export can warn.
	if orphans != 2 {
		t.Fatalf("orphans = %d, want 2", orphans)
	}
	// One line per swimmer or event, however many entries share the problem.
	if want := "event 9 has no distance or stroke|Sky Lee needs a competition category of male or female|Max Ng has no date of birth"; strings.Join(problems, "|") != want {
		t.Fatalf("problems = %q", problems)
	}
	got := input.Entries
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	first := got[0]
	if first.SeedCourse != meetfiles.CourseShortCourseYards || first.SeedTimeMs != 0 || first.SeedMeet != "" || first.Event.Stroke != meetfiles.StrokeFreestyle ||
		first.Event.Gender != "X" || first.Event.FeeCents != 600 || first.Swimmer.Gender != "F" || first.Swimmer.MiddleInitial != "Q" ||
		first.Swimmer.PreferredName != "Annie" || first.Swimmer.DOB.Format("2006-01-02") != "2012-04-02" || first.Swimmer.USSID != "040212ANARIVE" {
		t.Errorf("entry 0: %+v", first)
	}
	second := got[1]
	if second.SeedCourse != meetfiles.CourseLongCourseMeters || second.SeedTimeMs != 61230 || second.Event.Stroke != meetfiles.StrokeButterfly ||
		second.Event.Gender != "M" || *second.Event.MinAge != 11 || *second.Event.MaxAge != 12 || second.SeedMeet != "Summer Champs" || !second.SeedDate.Equal(swum) {
		t.Errorf("entry 1: %+v", second)
	}
}

func TestCov5xEntrySeedSources(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	h := NewExportsHandlersWithMeetAccess(exports.NewService(), f.meets, f.members, f.settings)
	if got := h.entrySeedSources(f.admin, cov5xOrg); len(got) != 0 {
		t.Fatalf("no lister wired: %v", got)
	}
	h.WithBestTimeSources(cov5xBestTimes{err: errors.New("down")})
	if got := h.entrySeedSources(f.admin, cov5xOrg); len(got) != 0 {
		t.Fatalf("lookup failure must degrade to no sources: %v", got)
	}
	h.WithBestTimeSources(cov5xBestTimes{sources: []performance.BestTimeSource{
		{SwimmerID: "s1", Distance: 50, Stroke: "Freestyle", Course: "scy", BestTimeMs: 31000, MeetID: f.meetID},
		{SwimmerID: "s1", Distance: 100, Stroke: "back", Course: "SCY", BestTimeMs: 70000, MeetID: "some-other-org-meet"},
	}})
	got := h.entrySeedSources(f.admin, cov5xOrg)
	src, ok := got[seedSourceKey{"s1", 50, meetfiles.StrokeFreestyle, "SCY"}]
	if len(got) != 1 || !ok || src.timeMs != 31000 || src.meet != "Winter Open!" || src.date.Format("2006-01-02") != "2026-12-05" {
		t.Fatalf("sources = %+v", got)
	}
}

func TestCov5xEntryBundleHelpers(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]meetfiles.Stroke{
		"Freestyle": meetfiles.StrokeFreestyle, "free": meetfiles.StrokeFreestyle,
		"backstroke": meetfiles.StrokeBackstroke, "BACK": meetfiles.StrokeBackstroke,
		"breaststroke": meetfiles.StrokeBreaststroke, "breast": meetfiles.StrokeBreaststroke,
		"butterfly": meetfiles.StrokeButterfly, "fly": meetfiles.StrokeButterfly,
		"individual_medley": meetfiles.StrokeIM, "im": meetfiles.StrokeIM,
		"freestyle_relay": meetfiles.StrokeUnknown, "medley_relay": meetfiles.StrokeUnknown, "diving": meetfiles.StrokeUnknown, "": meetfiles.StrokeUnknown,
	} {
		if got := bundleStroke(in); got != want {
			t.Errorf("bundleStroke(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]string{"female": "F", " MALE ": "M", "non_binary": "", "": ""} {
		if got := bundleSwimmerGender(in); got != want {
			t.Errorf("bundleSwimmerGender(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"m": "M", " f ": "F", "O": "X", "X": "X", "": "X"} {
		if got := bundleEventGender(in); got != want {
			t.Errorf("bundleEventGender(%q) = %q, want %q", in, got, want)
		}
	}
	if d := parseEntryBundleDate(" 2026-12-05T01:02:03Z "); d.Format("2006-01-02") != "2026-12-05" {
		t.Errorf("parseEntryBundleDate long form: %v", d)
	}
	if d := parseEntryBundleDate("12/05/2026"); !d.IsZero() {
		t.Errorf("unparseable date must be zero, got %v", d)
	}
	if loc := clubLocation("America/New_York"); loc.String() != "America/New_York" {
		t.Errorf("clubLocation = %v", loc)
	}
	for _, zone := range []string{"", "Mars/Olympus"} {
		if loc := clubLocation(zone); loc != time.UTC {
			t.Errorf("clubLocation(%q) = %v, want UTC", zone, loc)
		}
	}
	if got := entryBundleAuthor(&appmiddleware.Principal{Name: " Kim Lee ", Email: "k@example.com"}); got != "Kim Lee" {
		t.Errorf("author = %q", got)
	}
	if got := entryBundleAuthor(&appmiddleware.Principal{Email: "k@example.com"}); got != "k@example.com" {
		t.Errorf("author fallback = %q", got)
	}
}
