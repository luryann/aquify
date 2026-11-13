package handlers

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/app/exports"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/testutil"
)

// An entry bundle export must (1) see the club's swimmers even when the members cache is
// cold, and (2) never see another club's swimmers. Both clubs live in one schema
// and the services connect as the non-superuser role, so row-level security is
// really enforced: only the per-request org binding keeps the clubs apart.
func TestEntryBundleColdRosterIsFilledForThatClubOnly(t *testing.T) {
	admin, app := testutil.IsolatedSchemaPoolAsAppRole(t)
	ctx := context.Background()

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	swimmerA, swimmerB := uuid.NewString(), uuid.NewString()
	testutil.SeedOrgs(t, admin, orgA, orgB)
	cov3xInsertUser(t, admin, orgA, userA)
	cov3xInsertUser(t, admin, orgB, userB)
	for _, club := range []struct{ org, swimmer, first, last string }{
		{orgA, swimmerA, "Kaiala", "Alphaclub"},
		{orgB, swimmerB, "Zorbax", "Betaclub"},
	} {
		hh := uuid.NewString()
		if _, err := admin.Exec(ctx, `INSERT INTO households (id, org_id, household_name, keycloak_user_id) VALUES ($1::uuid, $2::uuid, 'Family', $3::uuid)`, hh, club.org, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status, medical_flags)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, '2014-06-01', 'male', 'active', '[]'::jsonb)`, club.swimmer, club.org, hh, club.first, club.last); err != nil {
			t.Fatal(err)
		}
	}

	adminA := &appmiddleware.Principal{UserID: userA, OrgID: orgA, Roles: []string{"admin"}}
	adminB := &appmiddleware.Principal{UserID: userB, OrgID: orgB, Roles: []string{"admin"}}
	meetsSvc := meets.NewServiceWithPool(app)
	settings := orgsettings.NewServiceWithPool(app)
	code := "AQFY"
	if _, err := settings.Update(orgA, orgsettings.Updates{TeamCode: &code}, userA); err != nil {
		t.Fatalf("settings: %v", err)
	}
	meet, _, err := meetsSvc.CreateMeet(adminA, orgA, meets.CreateMeetInput{
		Name: "Isolation Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-05",
		Events: []meets.CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, Gender: "M"}},
	})
	if err != nil {
		t.Fatalf("meet: %v", err)
	}
	events, err := meetsSvc.ListMeetEvents(adminA, orgA, meet.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %v %v", events, err)
	}
	for _, step := range []func() error{
		func() error { _, _, e := meetsSvc.TransitionMeetStatus(adminA, orgA, meet.ID, "published"); return e },
		func() error { _, _, _, e := meetsSvc.PublishCommitmentRequest(adminA, orgA, meet.ID); return e },
		func() error { _, _, e := meetsSvc.LockCommitments(adminA, orgA, meet.ID); return e },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	entry, _, err := meetsSvc.ProposeEntry(adminA, orgA, meet.ID, meets.ProposeEntryInput{SwimmerID: swimmerA, MeetEventID: events[0].ID})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, _, err := meetsSvc.ApproveEntry(adminA, orgA, meet.ID, entry.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, _, err := meetsSvc.TransitionMeetStatus(adminA, orgA, meet.ID, "entry_finalized"); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// A brand-new members service: nothing has been loaded into its cache.
	membersSvc := members.NewServiceWithPool(app)
	if got := membersSvc.SnapshotSwimmers(orgA); len(got) != 0 {
		t.Fatalf("precondition: the members cache must start cold, has %d swimmers", len(got))
	}
	h := NewExportsHandlersWithMeetAccess(exports.NewService(), meetsSvc, membersSvc, settings)
	router := cov5xTokenRouter(tokenResolver{"admin-a": adminA, "admin-b": adminB}, h.RegisterRoutes)

	do := func(token, orgHeader string) *responseCapture {
		req, _ := http.NewRequest(http.MethodPost, "/meets/"+meet.ID+"/entries/export", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-Id", orgHeader)
		rr := serveRecorded(router, req)
		return &responseCapture{code: rr.Code, body: rr.Body.String()}
	}

	// Club B's admin cannot reach club A's meet, with or without claiming A's org.
	if got := do("admin-b", orgA); got.code != http.StatusForbidden {
		t.Fatalf("B's token claiming A's org: want 403, got %d %s", got.code, got.body)
	}
	got := do("admin-b", orgB)
	if got.code != http.StatusNotFound {
		t.Fatalf("B's admin on A's meet id: want 404, got %d %s", got.code, got.body)
	}
	for _, leaked := range []string{"ALPHACLUB", "Alphaclub", "KAIALA", "Kaiala"} {
		if strings.Contains(got.body, leaked) {
			t.Fatalf("club A's data leaked to club B's admin (%q): %s", leaked, got.body)
		}
	}
	if len(membersSvc.SnapshotSwimmers(orgA)) != 0 {
		t.Fatal("a rejected cross-club request must not even load club A's roster")
	}

	// Club A's admin gets a file naming A's swimmer and nobody from club B.
	got = do("admin-a", orgA)
	if got.code != http.StatusOK {
		t.Fatalf("A's export: want 200, got %d %s", got.code, got.body)
	}
	_, bundleFiles := cov5xUnzip(t, []byte(got.body))
	var bundleText strings.Builder
	for _, content := range bundleFiles {
		bundleText.WriteString(content)
	}
	upper := strings.ToUpper(bundleText.String())
	if !strings.Contains(upper, "ALPHACLUB") {
		t.Fatalf("a cold cache must be refilled from Postgres so A's swimmer is in the file")
	}
	if strings.Contains(upper, "BETACLUB") || strings.Contains(upper, "ZORBAX") {
		t.Fatalf("club B's swimmer must never appear in club A's file")
	}
	// The generated report has its own refresh. Use a second, still-cold members service:
	// the entry bundle export above already warmed the first one.
	coldReports := members.NewServiceWithPool(app)
	if len(coldReports.SnapshotSwimmers(orgA)) != 0 {
		t.Fatal("precondition: the second members cache must start cold")
	}
	reportRouter := cov5xTokenRouter(tokenResolver{"admin-a": adminA},
		NewExportsHandlersWithMeetAccess(exports.NewService(), meetsSvc, coldReports, settings).RegisterRoutes)
	repReq, _ := http.NewRequest(http.MethodPost, "/meets/"+meet.ID+"/exports", strings.NewReader(`{"report_type":"entry_report"}`))
	repReq.Header.Set("Authorization", "Bearer admin-a")
	repReq.Header.Set("X-Org-Id", orgA)
	repReq.Header.Set("Content-Type", "application/json")
	rep := serveRecorded(reportRouter, repReq)
	if rep.Code != http.StatusCreated {
		t.Fatalf("report: want 201, got %d %s", rep.Code, rep.Body.String())
	}
	var created struct {
		Export struct {
			ID string `json:"id"`
		} `json:"export"`
	}
	if err := json.Unmarshal(rep.Body.Bytes(), &created); err != nil || created.Export.ID == "" {
		t.Fatalf("report response: %v %s", err, rep.Body.String())
	}
	dlReq, _ := http.NewRequest(http.MethodGet, "/exports/"+created.Export.ID+"/download", nil)
	dlReq.Header.Set("Authorization", "Bearer admin-a")
	dlReq.Header.Set("X-Org-Id", orgA)
	dl := serveRecorded(reportRouter, dlReq)
	if dl.Code != http.StatusOK {
		t.Fatalf("report download: want 200, got %d", dl.Code)
	}
	pdfText := strings.ToUpper(inflatePDFStreams(t, dl.Body.Bytes()))
	if !strings.Contains(pdfText, "ALPHACLUB") {
		t.Fatalf("the report must name club A's swimmer after a cold-cache refresh; decoded text:\n%s", pdfText)
	}
	if strings.Contains(pdfText, "BETACLUB") {
		t.Fatal("club B's swimmer must never appear in club A's report")
	}

	// Only club A's roster was loaded into the cache by the refresh.
	if n := len(membersSvc.SnapshotSwimmers(orgA)); n != 1 {
		t.Fatalf("club A should have exactly its own swimmer cached, got %d", n)
	}
	if n := len(membersSvc.SnapshotSwimmers(orgB)); n != 0 {
		t.Fatalf("refreshing club A must not load club B's swimmers, got %d", n)
	}
}

type responseCapture struct {
	code int
	body string
}

// inflatePDFStreams returns the text of every Flate-compressed stream in a PDF, so tests
// can search the page content that fpdf compresses.
func inflatePDFStreams(t *testing.T, pdf []byte) string {
	t.Helper()
	var out strings.Builder
	rest := pdf
	for {
		start := bytes.Index(rest, []byte("stream\n"))
		if start < 0 {
			break
		}
		rest = rest[start+len("stream\n"):]
		end := bytes.Index(rest, []byte("endstream"))
		if end < 0 {
			break
		}
		if zr, err := zlib.NewReader(bytes.NewReader(rest[:end])); err == nil {
			if b, err := io.ReadAll(zr); err == nil {
				out.Write(b)
				out.WriteByte('\n')
			}
		}
		rest = rest[end+len("endstream"):]
	}
	return out.String()
}
