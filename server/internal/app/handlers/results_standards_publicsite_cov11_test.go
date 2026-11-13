package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/app/performance"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov11gOrg = "org-cov11g"

func cov11gTokens() tokenResolver {
	return tokenResolver{
		"admin":  {UserID: "u-admin", OrgID: cov11gOrg, Roles: []string{"admin"}},
		"coach":  {UserID: "u-coach", OrgID: cov11gOrg, Roles: []string{"coach"}},
		"parent": {UserID: "u-parent", OrgID: cov11gOrg, Roles: []string{"parent"}},
	}
}

func cov11gRouter(t *testing.T, register func(chi.Router)) http.Handler {
	t.Helper()
	return g2Router(t, cov11gTokens(), register)
}

// cov11gUpload posts a multipart request with an optional "file" part.
func cov11gUpload(h http.Handler, path, token, field, filename string, content []byte) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if field != "" {
		fw, _ := mw.CreateFormFile(field, filename)
		_, _ = fw.Write(content)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", cov11gOrg)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func cov11gErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rr.Body.String(), err)
	}
	return body.Error.Code
}

func cov11gWant(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rr.Code, status, rr.Body.String())
	}
	if code != "" {
		if got := cov11gErrCode(t, rr); got != code {
			t.Fatalf("error code = %q, want %q (body %s)", got, code, rr.Body.String())
		}
	}
}

// ─── results import: handler ────────────────────────────────────────────────

type cov11gResultsEnv struct {
	members *members.Service
	meets   *meets.Service
	audit   *audit.Service
	meetID  string
}

func cov11gNewResultsEnv(t *testing.T, withSwimmers bool) cov11gResultsEnv {
	t.Helper()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: cov11gOrg, Roles: []string{"admin"}}
	env := cov11gResultsEnv{members: members.NewService(), meets: meets.NewService(), audit: audit.NewService()}
	if withSwimmers {
		hh, err := env.members.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: cov11gOrg, HouseholdName: "Smith"})
		if err != nil {
			t.Fatal(err)
		}
		for _, sw := range []members.CreateSwimmerInput{
			{HouseholdID: hh.ID, FirstName: "John", LastName: "Smith", DateOfBirth: "1999-02-15", Gender: "male", RegistrationStatus: "active"},
			{HouseholdID: hh.ID, FirstName: "Jane", LastName: "Doe", DateOfBirth: "2008-05-10", Gender: "female", RegistrationStatus: "active"},
		} {
			if _, _, err := env.members.CreateSwimmer(admin, cov11gOrg, sw); err != nil {
				t.Fatal(err)
			}
		}
	}
	m, _, err := env.meets.CreateMeet(admin, cov11gOrg, meets.CreateMeetInput{Name: "Spring Invitational", Course: "SCY", StartDate: "2026-03-15", EndDate: "2026-03-16"})
	if err != nil {
		t.Fatal(err)
	}
	env.meetID = m.ID
	return env
}

func cov11gFixtureSD3(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../../internal/meetfiles/testdata/fixtures/valid.sd3")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestCov11gResultsImportAllSwimmersMatchAndAudit(t *testing.T) {
	env := cov11gNewResultsEnv(t, true)
	perf := performance.NewService()
	withAudit := NewPerformanceHandlersWithAuditAndContext(perf, env.members, nil, env.meets, nil, env.audit)
	router := cov11gRouter(t, withAudit.RegisterRoutes)
	path := "/meets/" + env.meetID + "/results/import"
	sd3 := cov11gFixtureSD3(t)

	cov11gWant(t, cov11gUpload(router, path, "parent", "file", "r.sd3", sd3), http.StatusForbidden, "forbidden")

	rr := cov11gUpload(router, path, "coach", "file", "r.sd3", sd3)
	if rr.Code != http.StatusOK {
		t.Fatalf("import = %d %s", rr.Code, rr.Body.String())
	}
	var first struct {
		Status    string `json:"status"`
		Imported  int    `json:"imported"`
		Unmatched []any  `json:"unmatched"`
		Warnings  []any  `json:"warnings"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	// Both swimmers are club swimmers, so nothing is unmatched and the lists are
	// empty arrays, never null.
	if first.Status != "imported" || first.Imported != 2 || first.Unmatched == nil || len(first.Unmatched) != 0 || first.Warnings == nil {
		t.Fatalf("unexpected summary: %s", rr.Body.String())
	}
	events, err := env.audit.Query(audit.Filter{OrgID: cov11gOrg, Action: "results.imported"})
	if err != nil || len(events) != 1 || events[0].ResourceID != env.meetID || events[0].Metadata["imported"] != "2" {
		t.Fatalf("expected one results.imported audit event for the meet, got %+v err=%v", events, err)
	}

	// A handler without an audit service still imports; everything is a duplicate.
	noAudit := NewPerformanceHandlersWithAuditAndContext(perf, env.members, nil, env.meets, nil, nil)
	router2 := cov11gRouter(t, noAudit.RegisterRoutes)
	rr = cov11gUpload(router2, path, "admin", "file", "r.sd3", sd3)
	var second struct {
		Imported   int `json:"imported"`
		Duplicates int `json:"duplicates"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &second)
	if rr.Code != http.StatusOK || second.Imported != 0 || second.Duplicates != 2 {
		t.Fatalf("re-import = %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov11gResultsImportRejections(t *testing.T) {
	env := cov11gNewResultsEnv(t, false)
	h := NewPerformanceHandlersWithAuditAndContext(performance.NewService(), env.members, nil, env.meets, nil, env.audit)
	router := cov11gRouter(t, h.RegisterRoutes)
	path := "/meets/" + env.meetID + "/results/import"
	sd3 := cov11gFixtureSD3(t)

	t.Run("services not configured", func(t *testing.T) {
		bare := cov11gRouter(t, NewPerformanceHandlers(performance.NewService()).RegisterRoutes)
		cov11gWant(t, cov11gUpload(bare, path, "admin", "file", "r.sd3", sd3), http.StatusServiceUnavailable, "service_unavailable")
	})
	t.Run("unknown meet", func(t *testing.T) {
		cov11gWant(t, cov11gUpload(router, "/meets/nope/results/import", "admin", "file", "r.sd3", sd3), http.StatusNotFound, "meet_not_found")
	})
	t.Run("missing file part", func(t *testing.T) {
		cov11gWant(t, cov11gUpload(router, path, "admin", "", "", nil), http.StatusBadRequest, "missing_file")
	})
	t.Run("unparseable file", func(t *testing.T) {
		rr := cov11gUpload(router, path, "admin", "file", "notes.txt", []byte("not a results file"))
		cov11gWant(t, rr, http.StatusBadRequest, "invalid_results_file")
	})
	t.Run("empty file", func(t *testing.T) {
		cov11gWant(t, cov11gUpload(router, path, "admin", "file", "r.sd3", nil), http.StatusBadRequest, "invalid_results_file")
	})
	t.Run("file over the cap", func(t *testing.T) {
		big := bytes.Repeat([]byte{'x'}, maxResultsFileBytes+1)
		cov11gWant(t, cov11gUpload(router, path, "admin", "file", "r.sd3", big), http.StatusRequestEntityTooLarge, "file_too_large")
	})
	t.Run("request body over the cap", func(t *testing.T) {
		big := bytes.Repeat([]byte{'x'}, maxResultsFileBytes+(2<<20))
		cov11gWant(t, cov11gUpload(router, path, "admin", "file", "r.sd3", big), http.StatusRequestEntityTooLarge, "file_too_large")
	})
}

func TestCov11gResultsImportReportsPartialFailureWhenStoreIsDown(t *testing.T) {
	env := cov11gNewResultsEnv(t, true)
	pool := testutil.IsolatedSchemaPool(t)
	down := performance.NewServiceWithPool(pool)
	pool.Close()
	h := NewPerformanceHandlersWithAuditAndContext(down, env.members, nil, env.meets, nil, env.audit)
	router := cov11gRouter(t, h.RegisterRoutes)

	rr := cov11gUpload(router, "/meets/"+env.meetID+"/results/import", "admin", "file", "r.sd3", cov11gFixtureSD3(t))
	cov11gWant(t, rr, http.StatusInternalServerError, "result_import_failed")
	body := rr.Body.String()
	for _, want := range []string{`"partial":true`, `"retry_safe":true`, `"imported":0`, "re-importing the same file is safe"} {
		if !strings.Contains(body, want) {
			t.Fatalf("failure body missing %q: %s", want, body)
		}
	}
	events, err := env.audit.Query(audit.Filter{OrgID: cov11gOrg, Action: "results.import_partial"})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one results.import_partial audit event, got %d err=%v", len(events), err)
	}
}

// ─── results import: importParsedResults edge rows ──────────────────────────

func TestCov11gImportParsedResultsSkipsAndCourseHandling(t *testing.T) {
	dob := time.Date(2012, 5, 1, 0, 0, 0, 0, time.UTC)
	matcher := newResultSwimmerMatcher([]members.Swimmer{{ID: "amy", FirstName: "Amy", LastName: "Ames", Gender: "female", DateOfBirth: "2012-05-01"}})
	amy := meetfiles.ParsedSwimmer{FirstName: "Amy", LastName: "Ames", DOB: &dob}
	valid := meetfiles.Time{Raw: "x", Valid: true, Seconds: 30}
	ev := func(distance int, stroke meetfiles.Stroke, course meetfiles.Course) meetfiles.ParsedEvent {
		return meetfiles.ParsedEvent{Distance: distance, Stroke: stroke, EventGender: "F", Course: course}
	}

	parsed := &meetfiles.ParseResult{IndividualResults: []meetfiles.ParsedIndividualResult{
		// relay and zero-distance rows are not individual swims
		{Swimmer: amy, Event: ev(200, meetfiles.StrokeFreestyleRelay, "SCY"), FinalsTime: valid},
		{Swimmer: amy, Event: ev(0, meetfiles.StrokeFreestyle, "SCY"), FinalsTime: valid},
		// NT / scratch: no finals, no prelim, not a DQ
		{Swimmer: amy, Event: ev(50, meetfiles.StrokeBackstroke, "SCY")},
		// no course anywhere (event, file, meet): cannot be recorded
		{Swimmer: amy, Event: ev(100, meetfiles.StrokeFreestyle, ""), FinalsTime: valid},
		// prelim-only swim whose prelim course differs from the event course
		{Swimmer: amy, Event: ev(200, meetfiles.StrokeFreestyle, "SCY"), PrelimTime: valid, PrelimCourse: meetfiles.Course("LCM")},
	}}
	rec := &cov11gCourseRecorder{fakeRecorder: fakeRecorder{recorded: map[string]bool{}}}
	prog, err := importParsedResults(rec, "org", importPrincipal, "meet", "", parsed, matcher)
	if err != nil {
		t.Fatal(err)
	}
	if prog.Skipped != 4 || prog.Imported != 1 || prog.Processed != 5 || prog.Total != 5 {
		t.Fatalf("progress = %+v, want skipped=4 imported=1 processed=5", prog)
	}
	if len(rec.courses) != 1 || rec.courses[0] != "LCM" {
		t.Fatalf("prelim course should win for a prelim-only swim, got %v", rec.courses)
	}
}

type cov11gCourseRecorder struct {
	fakeRecorder
	courses []string
}

func (c *cov11gCourseRecorder) UpsertResultWithRecords(orgID, actorID string, p *appmiddleware.Principal, in performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error) {
	c.courses = append(c.courses, in.Course)
	return c.fakeRecorder.UpsertResultWithRecords(orgID, actorID, p, in)
}

// ─── time standards import ──────────────────────────────────────────────────

const cov11gStandardsCSV = "standard_name,standard_code,season_year,course,gender,age_group,event_distance,stroke,cut_time\n" +
	"2026 Cuts,A,2026,SCY,F,11-12,100,free,58.12\n"

func TestCov11gTimeStandardsImportRequestValidation(t *testing.T) {
	h := NewPerformanceHandlersWithAudit(performance.NewService(), audit.NewService())
	router := cov11gRouter(t, h.RegisterRoutes)
	post := func(path string, field string, content []byte) *httptest.ResponseRecorder {
		return cov11gUpload(router, path, "admin", field, "s.csv", content)
	}

	cov11gWant(t, post("/time-standards/import?preview=maybe", "file", []byte(cov11gStandardsCSV)), http.StatusBadRequest, "invalid_query_parameter")
	cov11gWant(t, post("/time-standards/import", "", nil), http.StatusBadRequest, "missing_file")
	cov11gWant(t, post("/time-standards/import", "file", nil), http.StatusBadRequest, "invalid_standards_file")
	cov11gWant(t, post("/time-standards/import", "file", bytes.Repeat([]byte{'a'}, maxStandardsFileBytes+1)), http.StatusRequestEntityTooLarge, "file_too_large")

	// The valid file still goes through after the rejections: nothing was written.
	rr := post("/time-standards/import?preview=true", "file", []byte(cov11gStandardsCSV))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"preview":true`) {
		t.Fatalf("preview = %d %s", rr.Code, rr.Body.String())
	}
}

func TestCov11gTimeStandardsImportStoreDown(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	down := performance.NewServiceWithPool(pool)
	pool.Close()
	router := cov11gRouter(t, NewPerformanceHandlers(down).RegisterRoutes)

	for _, path := range []string{"/time-standards/import?preview=true", "/time-standards/import"} {
		rr := cov11gUpload(router, path, "admin", "file", "s.csv", []byte(cov11gStandardsCSV))
		cov11gWant(t, rr, http.StatusInternalServerError, "internal_error")
		if strings.Contains(rr.Body.String(), "pool") {
			t.Fatalf("driver error leaked to client: %s", rr.Body.String())
		}
	}
}

func TestCov11gWriteStandardsImportErr(t *testing.T) {
	rr := httptest.NewRecorder()
	writeStandardsImportErr(rr, httptest.NewRequest(http.MethodPost, "/time-standards/import", nil), performance.ErrForbidden)
	cov11gWant(t, rr, http.StatusForbidden, "forbidden")

	rr = httptest.NewRecorder()
	writeStandardsImportErr(rr, httptest.NewRequest(http.MethodPost, "/time-standards/import", nil), errors.New("disk exploded"))
	cov11gWant(t, rr, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rr.Body.String(), "disk exploded") {
		t.Fatalf("internal error text leaked: %s", rr.Body.String())
	}
}

// ─── public site ────────────────────────────────────────────────────────────

func cov11gSiteGet(h *PublicSiteHandlers, slug string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/orgs/"+slug+"/site", nil))
	return rec
}

func cov11gSettings(name string) func(string) (*orgsettings.OrgSettings, error) {
	return func(slug string) (*orgsettings.OrgSettings, error) {
		return &orgsettings.OrgSettings{OrgID: "org-1", TeamName: name, PublicSlug: slug, PublicSiteEnabled: true}, nil
	}
}

func TestCov11gPublicSiteSourceFailuresAndInactiveOrg(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }

	rec := cov11gSiteGet(NewPublicSiteHandlers(PublicSiteSources{Now: now,
		Settings: func(string) (*orgsettings.OrgSettings, error) { return nil, errors.New("settings db down") }}, nil), "aquify")
	cov11gWant(t, rec, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rec.Body.String(), "settings db down") {
		t.Fatalf("source error leaked: %s", rec.Body.String())
	}

	rec = cov11gSiteGet(NewPublicSiteHandlers(PublicSiteSources{Now: now, Settings: cov11gSettings("Dolphins"),
		OrgActive: func(context.Context, string) (bool, error) { return false, errors.New("registry down") }}, nil), "aquify")
	cov11gWant(t, rec, http.StatusInternalServerError, "internal_error")

	rec = cov11gSiteGet(NewPublicSiteHandlers(PublicSiteSources{Now: now, Settings: cov11gSettings("Dolphins"),
		OrgActive: func(context.Context, string) (bool, error) { return false, nil }}, nil), "aquify")
	cov11gWant(t, rec, http.StatusNotFound, "not_found")
}

func TestCov11gPublicSiteNameFallbackMeetOrderAndCaps(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var list []meets.Meet
	// 35 upcoming meets listed latest-first, plus a draft and a finished meet that must not appear.
	for i := 35; i >= 1; i-- {
		d := fmt.Sprintf("2026-07-%02d", i)
		list = append(list, meets.Meet{ID: fmt.Sprintf("m%02d", i), OrgID: "org-1", Name: "Meet", Status: "published", StartDate: d, EndDate: d})
	}
	list = append(list,
		meets.Meet{ID: "draft", OrgID: "org-1", Status: "draft", StartDate: "2026-07-01"},
		meets.Meet{ID: "past", OrgID: "org-1", Status: "published", StartDate: "2026-01-01", EndDate: "2026-01-02"},
	)
	published := now.Add(-time.Hour)
	anns := []comms.Announcement{
		{ID: "undated", OrgID: "org-1", Title: "u", Status: "published", Audience: "all", Public: true},
		{ID: "dated", OrgID: "org-1", Title: "d", Status: "published", Audience: "all", Public: true, PublishedAt: &published},
	}
	h := NewPublicSiteHandlers(PublicSiteSources{
		Now:           func() time.Time { return now },
		Settings:      cov11gSettings("   "),
		Meets:         func(string) []meets.Meet { return list },
		Announcements: func(string) []comms.Announcement { return anns },
	}, nil)
	rec := cov11gSiteGet(h, "Aquify")
	if rec.Code != http.StatusOK {
		t.Fatalf("site = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Club struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"club"`
		Meets []struct {
			ID string `json:"id"`
		} `json:"meets"`
		Announcements []struct {
			ID string `json:"id"`
		} `json:"announcements"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Club.Name != "aquify" || body.Club.Slug != "aquify" {
		t.Fatalf("blank team name should fall back to the slug, got %+v", body.Club)
	}
	if len(body.Meets) != publicMaxMeets || body.Meets[0].ID != "m01" || body.Meets[publicMaxMeets-1].ID != "m30" {
		t.Fatalf("want the 30 soonest meets ascending (m01..m30), got %d first=%v", len(body.Meets), body.Meets[:1])
	}
	for _, m := range body.Meets {
		if m.ID == "draft" || m.ID == "past" {
			t.Fatalf("meet %s must not be public", m.ID)
		}
	}
	if len(body.Announcements) != 2 || body.Announcements[0].ID != "dated" || body.Announcements[1].ID != "undated" {
		t.Fatalf("dated announcement should sort before the undated one, got %+v", body.Announcements)
	}
}

func TestCov11gAnnouncementTime(t *testing.T) {
	if got := announcementTime(comms.Announcement{}); !got.IsZero() {
		t.Fatalf("unpublished announcement time = %v, want zero", got)
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := announcementTime(comms.Announcement{PublishedAt: &ts}); !got.Equal(ts) {
		t.Fatalf("announcement time = %v, want %v", got, ts)
	}
}
