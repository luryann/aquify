package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/testutil"
)

// When Postgres refuses to record the exported state, the entry bundle must not be
// handed out (the host would hold entries the club still shows as approved).
func TestCov5xEntryBundleStateWriteFailureWithholdsFile(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org, user, hh, sw := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	cov3xInsertUser(t, pool, org, user)
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, keycloak_user_id) VALUES ($1::uuid, $2::uuid, 'Cov Family', $3::uuid)`, hh, org, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status, medical_flags)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Kai', 'Cov', '2014-06-01', 'male', 'active', '[]'::jsonb)`, sw, org, hh); err != nil {
		t.Fatal(err)
	}

	admin := &appmiddleware.Principal{UserID: user, OrgID: org, Roles: []string{"admin"}}
	meetsSvc := meets.NewServiceWithPool(pool)
	settings := orgsettings.NewServiceWithPool(pool)
	code := "AQFY"
	if _, err := settings.Update(org, orgsettings.Updates{TeamCode: &code}, user); err != nil {
		t.Fatalf("settings: %v", err)
	}
	meet, _, err := meetsSvc.CreateMeet(admin, org, meets.CreateMeetInput{
		Name: "DB Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-05",
		Events: []meets.CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, Gender: "M"}},
	})
	if err != nil {
		t.Fatalf("meet: %v", err)
	}
	events, err := meetsSvc.ListMeetEvents(admin, org, meet.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %v %v", events, err)
	}
	for _, step := range []func() error{
		func() error { _, _, e := meetsSvc.TransitionMeetStatus(admin, org, meet.ID, "published"); return e },
		func() error { _, _, _, e := meetsSvc.PublishCommitmentRequest(admin, org, meet.ID); return e },
		func() error { _, _, e := meetsSvc.LockCommitments(admin, org, meet.ID); return e },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	entry, _, err := meetsSvc.ProposeEntry(admin, org, meet.ID, meets.ProposeEntryInput{SwimmerID: sw, MeetEventID: events[0].ID})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, _, err := meetsSvc.ApproveEntry(admin, org, meet.ID, entry.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, _, err := meetsSvc.TransitionMeetStatus(admin, org, meet.ID, "entry_finalized"); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	for _, q := range []string{
		`CREATE FUNCTION cov5x_freeze_entries() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'entries are frozen'; END $$`,
		`CREATE TRIGGER cov5x_freeze BEFORE UPDATE ON entries FOR EACH ROW EXECUTE FUNCTION cov5x_freeze_entries()`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	h := NewExportsHandlersWithMeetAccess(exports.NewService(), meetsSvc, members.NewServiceWithPool(pool), settings)
	router := cov5xTokenRouter(tokenResolver{"admin": admin}, h.RegisterRoutes)
	req, _ := http.NewRequest(http.MethodPost, "/meets/"+meet.ID+"/entries/export", nil)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("X-Org-Id", org)
	rr := serveRecorded(router, req)
	cov5xWant(t, "export", rr, http.StatusInternalServerError, "entry_export_state_failed")
	if strings.Contains(rr.Body.String(), "frozen") || strings.HasPrefix(rr.Body.String(), "PK") {
		t.Errorf("file or database error leaked: %s", rr.Body.String())
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM entries WHERE id = $1`, entry.ID).Scan(&status); err != nil || status != "approved" {
		t.Fatalf("entry must stay approved: %q %v", status, err)
	}
}

func cov5xTokenRouter(tokens tokenResolver, register func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		register(p)
	})
	return r
}

func serveRecorded(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// The roster is re-synced before it is read; if that fails the export fails
// loudly instead of silently producing a report or entry bundle with no swimmers.
func TestCov5xExportsRosterSyncFailureFailsTheExport(t *testing.T) {
	t.Parallel()
	f := cov5xNewExpFixture(t)
	h := NewExportsHandlersWithAudit(f.exports, f.meets, members.NewServiceWithPool(testutil.ClosedPool(t)), f.settings, f.audit)
	router := cov5xExpMount(h)

	rr := cov5xDo(router, http.MethodPost, "/meets/"+f.meetID+"/exports", "admin", `{"report_type":"entry_report"}`)
	cov5xWant(t, "report", rr, http.StatusInternalServerError, "report_assembly_failed")
	rr = cov5xDo(router, http.MethodPost, "/meets/"+f.meetID+"/entries/export", "admin", "")
	cov5xWant(t, "entry bundle", rr, http.StatusInternalServerError, "entry_export_failed")
	if st := f.entryStatus(t, f.entryIDs["Ana/1"]); st != "approved" {
		t.Errorf("failed export must not change entries; Ana/1 is %q", st)
	}
}
