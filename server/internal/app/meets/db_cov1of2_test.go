package meets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov1xForced = "cov1x forced failure"

type cov1xDBOrg struct {
	id     string
	admin  *appmiddleware.Principal
	parent *appmiddleware.Principal
	hh     string
	sw     []string
}

// cov1xDBEnv runs the service as the application's RLS-subject role against a
// private schema; super is used only for seeding, fault injection and checks.
type cov1xDBEnv struct {
	t     *testing.T
	super *pgxpool.Pool
	app   *pgxpool.Pool
	svc   *Service
	seq   int
}

func cov1xDBNew(t *testing.T) *cov1xDBEnv {
	t.Helper()
	super, app := testutil.IsolatedSchemaPoolAsAppRole(t)
	e := &cov1xDBEnv{t: t, super: super, app: app, svc: NewServiceWithPool(app)}
	e.exec(`CREATE FUNCTION cov1x_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '` + cov1xForced + `'; END $$`)
	return e
}

func (e *cov1xDBEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.super.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *cov1xDBEnv) count(table string) int {
	e.t.Helper()
	var n int
	if err := e.super.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *cov1xDBEnv) newUser(org, role string) *appmiddleware.Principal {
	e.t.Helper()
	id := uuid.NewString()
	e.exec(`INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $1, 'Test User', 'active', now(), now())`, id, org, id+"@example.test")
	return &appmiddleware.Principal{UserID: id, OrgID: org, Roles: []string{role}}
}

func (e *cov1xDBEnv) addSwimmer(o *cov1xDBOrg) string {
	e.t.Helper()
	id := uuid.NewString()
	e.exec(`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Sam', 'Swimmer', '2014-05-05', 'female')`, id, o.id, o.hh)
	o.sw = append(o.sw, id)
	return id
}

func (e *cov1xDBEnv) newOrg(swimmers int) *cov1xDBOrg {
	e.t.Helper()
	o := &cov1xDBOrg{id: uuid.NewString(), hh: uuid.NewString()}
	testutil.SeedOrgs(e.t, e.super, o.id)
	e.exec(`INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Household', 'active')`, o.hh, o.id)
	o.admin = e.newUser(o.id, "admin")
	o.parent = e.newUser(o.id, "parent")
	o.parent.HouseholdID = o.hh
	for i := 0; i < swimmers; i++ {
		e.addSwimmer(o)
	}
	return o
}

// failOn makes every statement of the given kind (INSERT/UPDATE/DELETE) on table
// fail inside Postgres until the returned restore func is called.
func (e *cov1xDBEnv) failOn(table, event string) (restore func()) {
	e.t.Helper()
	e.seq++
	name := fmt.Sprintf("cov1x_fail_%d", e.seq)
	e.exec(`CREATE TRIGGER ` + name + ` BEFORE ` + event + ` ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	return func() { e.exec(`DROP TRIGGER ` + name + ` ON ` + table) }
}

// failAtCommit makes inserts into table succeed until the transaction commits.
func (e *cov1xDBEnv) failAtCommit(table string) (restore func()) {
	e.t.Helper()
	e.seq++
	name := fmt.Sprintf("cov1x_fail_%d", e.seq)
	e.exec(`CREATE CONSTRAINT TRIGGER ` + name + ` AFTER INSERT ON ` + table + ` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cov1x_fail()`)
	return func() { e.exec(`DROP TRIGGER ` + name + ` ON ` + table) }
}

// breakTable hides a table so reads and writes of it fail.
func (e *cov1xDBEnv) breakTable(table string) (restore func()) {
	e.t.Helper()
	e.exec(`ALTER TABLE ` + table + ` RENAME TO ` + table + `_cov1x_hidden`)
	return func() { e.exec(`ALTER TABLE ` + table + `_cov1x_hidden RENAME TO ` + table) }
}

func cov1xPersisted(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, ErrPersistence) || !strings.Contains(err.Error(), cov1xForced) {
		t.Errorf("%s: err = %v, want ErrPersistence caused by the injected failure", what, err)
	}
}

func cov1xForcedErr(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), cov1xForced) {
		t.Errorf("%s: err = %v, want the injected storage failure", what, err)
	}
}

// cov1xDump renders every record of one club through the public read API, so a
// freshly loaded service can be compared with the one that wrote the data.
func cov1xDump(t *testing.T, s *Service, o *cov1xDBOrg) string {
	t.Helper()
	var b strings.Builder
	meetsList, err := s.ListMeets(o.admin, o.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range meetsList {
		fmt.Fprintf(&b, "meet %s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%d|%s|open=%t|closed=%t|final=%t|%s|%s\n",
			m.ID, m.Name, m.MeetType, m.Course, m.StartDate, m.EndDate, m.EntryDeadline, m.CommitmentDeadline,
			m.HostOrg, m.Location, m.SelfSignupEnabled, m.MaxEntriesPerSwimmer, m.Status,
			m.CommitmentOpenAt != nil, m.CommitmentClosedAt != nil, m.EntryFinalizedAt != nil, m.CreatedBy, m.UpdatedBy)
		sessions, _ := s.ListMeetSessions(o.admin, o.id, m.ID)
		for _, x := range sessions {
			fmt.Fprintf(&b, " session %s|%d|%s|%s|%s|%s|%s\n", x.ID, x.SessionNumber, x.Name, x.SessionDate, x.WarmupTime, x.StartTime, x.Notes)
		}
		events, _ := s.ListMeetEvents(o.admin, o.id, m.ID)
		for _, x := range events {
			fmt.Fprintf(&b, " event %s|%s|%s|%s|%d|%s|%s|%v|%v|%d|%t|%s\n", x.ID, x.SessionID, x.EventNumber, x.Stroke, x.Distance, x.Course, x.Gender, cov1xPtr(x.MinAge), cov1xPtr(x.MaxAge), x.EntryFeeCents, x.IsRelay, x.CreatedBy)
		}
		cmts, _ := s.ListCommitments(o.admin, o.id, m.ID)
		for _, c := range cmts {
			fmt.Fprintf(&b, " commitment %s|%s|%s|%s|%s|committed=%t|locked=%t\n", c.ID, c.HouseholdID, c.SwimmerID, c.Status, c.Notes, c.CommittedAt != nil, c.LockedAt != nil)
			sels, _ := s.ListCommitmentEventSelections(o.admin, o.id, m.ID, c.ID)
			for _, x := range sels {
				fmt.Fprintf(&b, "  selection %s|%s|%v|%s\n", x.ID, x.MeetEventID, cov1xPtr(x.SeedTimeMs), x.CreatedBy)
			}
		}
		entries, _ := s.ListEntries(o.admin, o.id, m.ID)
		for _, x := range entries {
			fmt.Fprintf(&b, " entry %s|%s|%s|%s|%s|%v|%t|%s|%s|%s|%s\n", x.ID, x.SwimmerID, x.MeetEventID, x.CommitmentID, x.Status, cov1xPtr(x.SeedTimeMs), x.IsRelay, x.OverrideReason, x.ScratchReason, x.ProposedBy, x.ApprovedBy)
		}
	}
	return b.String()
}

func cov1xPtr(p *int) any {
	if p == nil {
		return "nil"
	}
	return *p
}

func TestCov1xDBLifecycleRoundTripsThroughPostgres(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(2)
	svc := e.svc
	ctx := context.Background()

	meet, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{
		Name: "DB Meet", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-06",
		EntryDeadline: "2026-11-20", CommitmentDeadline: "2026-11-15", HostOrg: "Host Club", Location: "City Pool",
		Sessions: []CreateMeetSessionInput{{SessionNumber: 1, Name: "AM", SessionDate: "2026-12-05", WarmupTime: "07:00:00", StartTime: "08:00:00", Notes: "doors 6:30"}},
		Events: []CreateMeetEventDraftInput{
			{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 50},
			{EventNumber: "2", Stroke: "backstroke", Distance: 50, Gender: "F", MinAge: cov1xInt(8), MaxAge: cov1xInt(12), EntryFeeCents: 500},
			{EventNumber: "3", Stroke: "freestyle_relay", Distance: 200, IsRelay: true},
		},
		SignupSettings: &CreateMeetSignupSettingsInput{SelfSignupEnabled: true, MaxEntriesPerSwimmer: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, perr := uuid.Parse(meet.ID); perr != nil {
		t.Fatalf("DB-mode IDs must be UUIDs, got %q", meet.ID)
	}
	if _, _, err := svc.TransitionMeetStatus(o.admin, o.id, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateMeetSession(o.admin, o.id, meet.ID, CreateMeetSessionInput{SessionNumber: 2, Name: "PM", StartTime: "13:00:00"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateMeetEvent(o.admin, o.id, meet.ID, CreateMeetEventInput{EventNumber: "4", Stroke: "butterfly", Distance: 100, Gender: "M"}); err != nil {
		t.Fatal(err)
	}
	if _, created, _, err := svc.PublishCommitmentRequest(o.admin, o.id, meet.ID); err != nil || len(created) != 2 {
		t.Fatalf("publish: %d commitments, %v", len(created), err)
	}
	evs := map[string]MeetEvent{}
	listed, _ := svc.ListMeetEvents(o.admin, o.id, meet.ID)
	for _, ev := range listed {
		evs[ev.EventNumber] = ev
	}
	cmts := map[string]Commitment{}
	cl, _ := svc.ListCommitments(o.admin, o.id, meet.ID)
	for _, c := range cl {
		cmts[c.SwimmerID] = c
	}
	c1, c2 := cmts[o.sw[0]], cmts[o.sw[1]]
	for _, c := range []Commitment{c1, c2} {
		if _, _, err := svc.RespondToCommitment(o.parent, o.id, meet.ID, c.ID, CommitmentRespondInput{Response: "committed_yes", Notes: "see you there"}); err != nil {
			t.Fatal(err)
		}
	}
	s1, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID, SeedTimeMs: cov1xInt(31250)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, c1.ID, SelectEventInput{MeetEventID: evs["2"].ID}); err != nil {
		t.Fatal(err)
	}
	tmp, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, c1.ID, SelectEventInput{MeetEventID: evs["3"].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveCommitmentEventSelection(o.parent, o.id, meet.ID, c1.ID, tmp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, c2.ID, SelectEventInput{MeetEventID: evs["1"].ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RespondToCommitment(o.parent, o.id, meet.ID, c2.ID, CommitmentRespondInput{Response: "committed_no", Notes: "away"}); err != nil {
		t.Fatal(err)
	}
	if e.count("commitment_event_selections") != 2 {
		t.Fatalf("selections in DB = %d, want 2 (declining removed the other swimmer's)", e.count("commitment_event_selections"))
	}
	closed, _, err := svc.LockCommitments(o.admin, o.id, meet.ID)
	if err != nil || closed.Status != "commitment_closed" {
		t.Fatalf("lock = %+v err=%v", closed, err)
	}
	entries, _ := svc.ListEntries(o.admin, o.id, meet.ID)
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	var ev1Entry, ev2Entry Entry
	for _, en := range entries {
		switch en.MeetEventID {
		case evs["1"].ID:
			ev1Entry = en
		case evs["2"].ID:
			ev2Entry = en
		}
	}
	if ev1Entry.SeedTimeMs == nil || *ev1Entry.SeedTimeMs != *s1.SeedTimeMs {
		t.Fatalf("seed time lost on lock: %v", ev1Entry.SeedTimeMs)
	}
	extra, _, err := svc.ProposeEntry(o.admin, o.id, meet.ID, ProposeEntryInput{SwimmerID: o.sw[1], MeetEventID: evs["4"].ID, CommitmentID: c2.ID, SeedTimeMs: cov1xInt(60000)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.OverrideEntry(o.admin, o.id, meet.ID, extra.ID, OverrideEntryInput{NewMeetEventID: evs["3"].ID, Reason: "relay lead-off", SeedTimeMs: cov1xInt(59000)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ApproveEntry(o.admin, o.id, meet.ID, ev1Entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ScratchEntry(o.admin, o.id, meet.ID, ev2Entry.ID, "injured"); err != nil {
		t.Fatal(err)
	}
	if n, _, err := svc.MarkEntriesExported(o.admin, o.id, meet.ID, []string{ev1Entry.ID}); err != nil || n != 1 {
		t.Fatalf("export n=%d err=%v", n, err)
	}
	// Back-fill a best time onto the open entry (event 3 is a relay: free relay, 200).
	if got := svc.UpdateSeedTimesForSwimmer(o.id, o.sw[1], 200, "free_relay", "SCY", cov1xInt(58000)); got != 1 {
		t.Fatalf("back-fill = %d", got)
	}
	// A second meet that is open while a swimmer joins late.
	m2, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "Open Window"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(o.admin, o.id, m2.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(o.admin, o.id, m2.ID); err != nil {
		t.Fatal(err)
	}
	late := e.addSwimmer(o)
	if created, err := svc.InviteSwimmerToOpenMeets(o.id, late, o.hh, o.admin.UserID); err != nil || len(created) != 1 {
		t.Fatalf("late invite = %+v err=%v", created, err)
	}

	want := cov1xDump(t, svc, o)
	if !strings.Contains(want, "injured") || !strings.Contains(want, "relay lead-off") || !strings.Contains(want, "exported") {
		t.Fatalf("dump lacks expected records:\n%s", want)
	}

	// A second instance sees exactly what the first persisted, via both reload paths.
	fresh := NewServiceWithPool(e.app)
	if err := fresh.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cov1xDump(t, fresh, o); got != want {
		t.Fatalf("full reload differs:\n--- got\n%s\n--- want\n%s", got, want)
	}
	scoped := NewServiceWithPool(e.app)
	if err := scoped.RefreshOrg(ctx, o.id); err != nil {
		t.Fatal(err)
	}
	if got := cov1xDump(t, scoped, o); got != want {
		t.Fatalf("scoped reload differs:\n--- got\n%s\n--- want\n%s", got, want)
	}
	if got := fresh.SnapshotMeets(o.id); len(got) != 2 {
		t.Fatalf("snapshot = %d meets", len(got))
	}
	var seedInDB int
	if err := e.super.QueryRow(ctx, `SELECT seed_time_ms FROM entries WHERE id = $1::uuid`, extra.ID).Scan(&seedInDB); err != nil || seedInDB != 58000 {
		t.Fatalf("back-filled seed in DB = %d err=%v", seedInDB, err)
	}
}

func TestCov1xDBWriteFailuresAreReportedAndLeaveStateRetryable(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(2)
	svc := e.svc
	adm := o.admin

	// CreateMeet: any failing write of the bundle rolls the whole bundle back.
	bundle := CreateMeetInput{Name: "P", StartDate: "2026-12-05", EndDate: "2026-12-05",
		Sessions: []CreateMeetSessionInput{{SessionNumber: 1, Name: "AM"}},
		Events:   []CreateMeetEventDraftInput{{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 50}}}
	for name, inject := range map[string]func() func(){
		"meet row":    func() func() { return e.failOn("meets", "INSERT") },
		"session row": func() func() { return e.failOn("meet_sessions", "INSERT") },
		"event row":   func() func() { return e.failOn("meet_events", "INSERT") },
		"commit":      func() func() { return e.failAtCommit("meets") },
	} {
		restore := inject()
		_, _, err := svc.CreateMeet(adm, o.id, bundle)
		restore()
		cov1xPersisted(t, "CreateMeet/"+name, err)
		if e.count("meets") != 0 || e.count("meet_sessions") != 0 || e.count("meet_events") != 0 {
			t.Fatalf("CreateMeet/%s left rows behind", name)
		}
		if got, _ := svc.ListMeets(adm, o.id); len(got) != 0 {
			t.Fatalf("CreateMeet/%s left a cached meet", name)
		}
	}

	in := bundle
	in.Events = append(in.Events, CreateMeetEventDraftInput{EventNumber: "2", Stroke: "backstroke", Distance: 50}, CreateMeetEventDraftInput{EventNumber: "3", Stroke: "butterfly", Distance: 50})
	in.SignupSettings = &CreateMeetSignupSettingsInput{SelfSignupEnabled: true, MaxEntriesPerSwimmer: 5}
	meet, _, err := svc.CreateMeet(adm, o.id, in)
	if err != nil {
		t.Fatal(err)
	}
	status := func() string {
		m, err := svc.GetMeet(adm, o.id, meet.ID)
		if err != nil {
			t.Fatal(err)
		}
		return m.Status
	}

	restore := e.failOn("meets", "UPDATE")
	_, _, err = svc.TransitionMeetStatus(adm, o.id, meet.ID, "published")
	restore()
	cov1xPersisted(t, "Transition", err)
	if status() != "draft" {
		t.Fatalf("failed transition changed the status to %s", status())
	}
	if _, _, err := svc.TransitionMeetStatus(adm, o.id, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}

	restore = e.failOn("meet_sessions", "INSERT")
	_, _, err = svc.CreateMeetSession(adm, o.id, meet.ID, CreateMeetSessionInput{SessionNumber: 2, Name: "PM"})
	restore()
	cov1xPersisted(t, "CreateMeetSession", err)
	restore = e.failOn("meet_events", "INSERT")
	_, _, err = svc.CreateMeetEvent(adm, o.id, meet.ID, CreateMeetEventInput{EventNumber: "9", Stroke: "freestyle", Distance: 50})
	restore()
	cov1xPersisted(t, "CreateMeetEvent", err)
	if e.count("meet_sessions") != 1 || e.count("meet_events") != 3 {
		t.Fatalf("rows after failed adds: sessions=%d events=%d", e.count("meet_sessions"), e.count("meet_events"))
	}

	// Opening commitments: a failure while inviting, then one while opening the meet.
	restore = e.failOn("commitments", "INSERT")
	_, _, _, err = svc.PublishCommitmentRequest(adm, o.id, meet.ID)
	restore()
	cov1xPersisted(t, "Publish/invite", err)
	restore = e.failOn("meets", "UPDATE")
	_, _, _, err = svc.PublishCommitmentRequest(adm, o.id, meet.ID)
	restore()
	cov1xPersisted(t, "Publish/open", err)
	if status() != "published" || e.count("commitments") != 2 {
		t.Fatalf("after failed open: status=%s commitments=%d", status(), e.count("commitments"))
	}
	// Retrying finishes the job without inviting anyone twice.
	opened, created, _, err := svc.PublishCommitmentRequest(adm, o.id, meet.ID)
	if err != nil || opened.Status != "commitment_open" || len(created) != 0 {
		t.Fatalf("retry = %+v created=%d err=%v", opened, len(created), err)
	}

	evs := map[string]MeetEvent{}
	listed, _ := svc.ListMeetEvents(adm, o.id, meet.ID)
	for _, ev := range listed {
		evs[ev.EventNumber] = ev
	}
	cl, _ := svc.ListCommitments(adm, o.id, meet.ID)
	cmts := map[string]Commitment{}
	for _, c := range cl {
		cmts[c.SwimmerID] = c
	}
	c1, c2 := cmts[o.sw[0]], cmts[o.sw[1]]

	restore = e.failOn("commitments", "UPDATE")
	_, _, err = svc.RespondToCommitment(o.parent, o.id, meet.ID, c1.ID, CommitmentRespondInput{Response: "committed_yes"})
	restore()
	cov1xPersisted(t, "Respond", err)
	for _, c := range []Commitment{c1, c2} {
		if _, _, err := svc.RespondToCommitment(o.parent, o.id, meet.ID, c.ID, CommitmentRespondInput{Response: "committed_yes"}); err != nil {
			t.Fatal(err)
		}
	}
	restore = e.failOn("commitment_event_selections", "INSERT")
	_, err = svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID})
	restore()
	cov1xForcedErr(t, "Select", err)
	if got, _ := svc.ListCommitmentEventSelections(adm, o.id, meet.ID, c1.ID); len(got) != 0 {
		t.Fatalf("failed select left a selection behind: %+v", got)
	}
	var sels []EventSelection
	for _, sp := range []struct {
		cmt Commitment
		ev  string
	}{{c1, "1"}, {c1, "2"}, {c1, "3"}, {c2, "1"}} {
		s, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, sp.cmt.ID, SelectEventInput{MeetEventID: evs[sp.ev].ID})
		if err != nil {
			t.Fatal(err)
		}
		sels = append(sels, s)
	}
	restore = e.failOn("commitment_event_selections", "DELETE")
	err = svc.RemoveCommitmentEventSelection(o.parent, o.id, meet.ID, c1.ID, sels[2].ID)
	cov1xForcedErr(t, "RemoveSelection", err)
	_, _, err = svc.RespondToCommitment(o.parent, o.id, meet.ID, c2.ID, CommitmentRespondInput{Response: "committed_no"})
	restore()
	cov1xPersisted(t, "Respond/no drops selections", err)
	if e.count("commitment_event_selections") != 4 {
		t.Fatalf("selections = %d, want 4 after failed deletes", e.count("commitment_event_selections"))
	}
	if err := svc.RemoveCommitmentEventSelection(o.parent, o.id, meet.ID, c1.ID, sels[2].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RespondToCommitment(o.parent, o.id, meet.ID, c2.ID, CommitmentRespondInput{Response: "committed_no"}); err != nil {
		t.Fatal(err)
	}

	// Locking: each failing stage is reported, and retries neither duplicate entries nor lose any.
	restore = e.failOn("entries", "INSERT")
	_, _, err = svc.LockCommitments(adm, o.id, meet.ID)
	restore()
	cov1xPersisted(t, "Lock/entry", err)
	restore = e.failOn("commitments", "UPDATE")
	_, _, err = svc.LockCommitments(adm, o.id, meet.ID)
	restore()
	cov1xPersisted(t, "Lock/commitment", err)
	restore = e.failOn("meets", "UPDATE")
	_, _, err = svc.LockCommitments(adm, o.id, meet.ID)
	restore()
	cov1xPersisted(t, "Lock/meet", err)
	if closed, _, err := svc.LockCommitments(adm, o.id, meet.ID); err != nil || closed.Status != "commitment_closed" {
		t.Fatalf("final lock = %+v err=%v", closed, err)
	}
	if e.count("entries") != 2 {
		t.Fatalf("entries after retried lock = %d, want 2", e.count("entries"))
	}
	entries, _ := svc.ListEntries(adm, o.id, meet.ID)
	byEvent := map[string]Entry{}
	for _, en := range entries {
		byEvent[en.MeetEventID] = en
	}

	restore = e.failOn("entries", "INSERT")
	_, _, err = svc.ProposeEntry(adm, o.id, meet.ID, ProposeEntryInput{SwimmerID: o.sw[1], MeetEventID: evs["3"].ID, CommitmentID: c2.ID})
	restore()
	cov1xPersisted(t, "Propose", err)
	extra, _, err := svc.ProposeEntry(adm, o.id, meet.ID, ProposeEntryInput{SwimmerID: o.sw[1], MeetEventID: evs["3"].ID, CommitmentID: c2.ID, SeedTimeMs: cov1xInt(60000)})
	if err != nil {
		t.Fatal(err)
	}

	restore = e.failOn("entries", "UPDATE")
	_, _, err = svc.OverrideEntry(adm, o.id, meet.ID, extra.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "swap"})
	cov1xPersisted(t, "Override", err)
	_, _, err = svc.ApproveEntry(adm, o.id, meet.ID, byEvent[evs["1"].ID].ID)
	cov1xPersisted(t, "Approve", err)
	_, _, err = svc.ScratchEntry(adm, o.id, meet.ID, byEvent[evs["2"].ID].ID, "sick")
	cov1xPersisted(t, "Scratch", err)
	// Seed back-fill skips an entry it cannot save rather than lying about it.
	if got := svc.UpdateSeedTimesForSwimmer(o.id, o.sw[0], 50, "free", "SCY", cov1xInt(1)); got != 0 {
		t.Errorf("back-fill under failing writes updated %d entries", got)
	}
	restore()
	if got := svc.UpdateSeedTimesForSwimmer(o.id, o.sw[0], 50, "free", "SCY", cov1xInt(29000)); got != 1 {
		t.Errorf("back-fill after recovery updated %d entries, want 1", got)
	}
	var seed int
	if err := e.super.QueryRow(context.Background(), `SELECT seed_time_ms FROM entries WHERE id = $1::uuid`, byEvent[evs["1"].ID].ID).Scan(&seed); err != nil || seed != 29000 {
		t.Errorf("seed in DB = %d err=%v, want 29000", seed, err)
	}
}

func TestCov1xDBExportFailureKeepsEntryApproved(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(1)
	svc := e.svc
	meet, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "X", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(o.admin, o.id, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(o.admin, o.id, meet.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LockCommitments(o.admin, o.id, meet.ID); err != nil {
		t.Fatal(err)
	}
	evs, _ := svc.ListMeetEvents(o.admin, o.id, meet.ID)
	en, _, err := svc.ProposeEntry(o.admin, o.id, meet.ID, ProposeEntryInput{SwimmerID: o.sw[0], MeetEventID: evs[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ApproveEntry(o.admin, o.id, meet.ID, en.ID); err != nil {
		t.Fatal(err)
	}
	restore := e.failOn("entries", "UPDATE")
	n, _, err := svc.MarkEntriesExported(o.admin, o.id, meet.ID, []string{en.ID})
	restore()
	cov1xPersisted(t, "Export", err)
	if n != 0 {
		t.Fatalf("marked %d despite the failure", n)
	}
	var status string
	if err := e.super.QueryRow(context.Background(), `SELECT status FROM entries WHERE id = $1::uuid`, en.ID).Scan(&status); err != nil || status != "approved" {
		t.Fatalf("status in DB = %q err=%v", status, err)
	}
}

func TestCov1xDBLateInviteFailureIsReported(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(0)
	svc := e.svc
	meet, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "Open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(o.admin, o.id, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(o.admin, o.id, meet.ID); err != nil {
		t.Fatal(err)
	}
	late := e.addSwimmer(o)
	restore := e.failOn("commitments", "INSERT")
	created, err := svc.InviteSwimmerToOpenMeets(o.id, late, o.hh, o.admin.UserID)
	restore()
	cov1xPersisted(t, "late invite", err)
	if len(created) != 0 || e.count("commitments") != 0 {
		t.Fatalf("created=%d rows=%d", len(created), e.count("commitments"))
	}
	if created, err = svc.InviteSwimmerToOpenMeets(o.id, late, o.hh, o.admin.UserID); err != nil || len(created) != 1 {
		t.Fatalf("retry = %+v err=%v", created, err)
	}
}

func TestCov1xDBEveryOperationSurfacesLoadFailures(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(1)
	svc := e.svc
	adm := o.admin
	restore := e.breakTable("meets")
	calls := map[string]func() error{
		"CreateMeet":      func() error { _, _, err := svc.CreateMeet(adm, o.id, CreateMeetInput{Name: "x"}); return err },
		"ListMeets":       func() error { _, err := svc.ListMeets(adm, o.id); return err },
		"GetMeet":         func() error { _, err := svc.GetMeet(adm, o.id, "m"); return err },
		"Transition":      func() error { _, _, err := svc.TransitionMeetStatus(adm, o.id, "m", "published"); return err },
		"CreateSession":   func() error { _, _, err := svc.CreateMeetSession(adm, o.id, "m", CreateMeetSessionInput{}); return err },
		"ListSessions":    func() error { _, err := svc.ListMeetSessions(adm, o.id, "m"); return err },
		"CreateEvent":     func() error { _, _, err := svc.CreateMeetEvent(adm, o.id, "m", CreateMeetEventInput{}); return err },
		"ListEvents":      func() error { _, err := svc.ListMeetEvents(adm, o.id, "m"); return err },
		"Publish":         func() error { _, _, _, err := svc.PublishCommitmentRequest(adm, o.id, "m"); return err },
		"Invite":          func() error { _, err := svc.InviteSwimmerToOpenMeets(o.id, "s", "h", "a"); return err },
		"ListCommitments": func() error { _, err := svc.ListCommitments(adm, o.id, "m"); return err },
		"Respond": func() error {
			_, _, err := svc.RespondToCommitment(o.parent, o.id, "m", "c", CommitmentRespondInput{Response: "committed_yes"})
			return err
		},
		"Select": func() error {
			_, err := svc.SelectCommitmentEvent(o.parent, o.id, "m", "c", SelectEventInput{MeetEventID: "e"})
			return err
		},
		"RemoveSelection": func() error { return svc.RemoveCommitmentEventSelection(o.parent, o.id, "m", "c", "s") },
		"ListSelections":  func() error { _, err := svc.ListCommitmentEventSelections(adm, o.id, "m", "c"); return err },
		"Lock":            func() error { _, _, err := svc.LockCommitments(adm, o.id, "m"); return err },
		"ListEntries":     func() error { _, err := svc.ListEntries(adm, o.id, "m"); return err },
		"Propose": func() error {
			_, _, err := svc.ProposeEntry(adm, o.id, "m", ProposeEntryInput{SwimmerID: "s", MeetEventID: "e"})
			return err
		},
		"Override": func() error {
			_, _, err := svc.OverrideEntry(adm, o.id, "m", "e", OverrideEntryInput{NewMeetEventID: "n", Reason: "r"})
			return err
		},
		"Scratch":      func() error { _, _, err := svc.ScratchEntry(adm, o.id, "m", "e", "r"); return err },
		"Approve":      func() error { _, _, err := svc.ApproveEntry(adm, o.id, "m", "e"); return err },
		"MarkExported": func() error { _, _, err := svc.MarkEntriesExported(adm, o.id, "m", nil); return err },
		"RefreshOrg":   func() error { return svc.RefreshOrg(context.Background(), o.id) },
		"Refresh":      func() error { return svc.Refresh(context.Background()) },
	}
	for name, call := range calls {
		err := call()
		if !errors.Is(err, ErrPersistence) {
			t.Errorf("%s: err = %v, want ErrPersistence", name, err)
		}
	}
	restore()
	if _, err := svc.ListMeets(adm, o.id); err != nil {
		t.Fatalf("service must recover once storage is back: %v", err)
	}
}

func TestCov1xDBReloadFallsBackToLockedLoadUnderWriterContention(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(0)
	svc := e.svc
	if _, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "Busy"}); err != nil {
		t.Fatal(err)
	}
	var calls int
	svc.SetSyncHookForTest(func() {
		calls++
		// A concurrent writer finishing inside the reload window invalidates the
		// lock-free snapshot. Once the loader holds the lock the hook just observes.
		if svc.mu.TryLock() {
			svc.mu.Unlock()
		}
	})
	got, err := svc.ListMeets(o.admin, o.id)
	if err != nil || len(got) != 1 || got[0].Name != "Busy" {
		t.Fatalf("meets = %+v err=%v", got, err)
	}
	if want := dbutil.MaxLockFreeReloads + 1; calls != want {
		t.Fatalf("hook ran %d times, want %d (lock-free attempts, then the locked load)", calls, want)
	}
	// The full reload also runs the hook under the lock.
	calls = 0
	if err := svc.Refresh(context.Background()); err != nil || calls != 1 {
		t.Fatalf("Refresh err=%v hook calls=%d", err, calls)
	}

	// If storage disappears while the lock-free attempts are losing, the locked load reports it.
	calls = 0
	var restore func()
	svc.SetSyncHookForTest(func() {
		calls++
		if calls == dbutil.MaxLockFreeReloads {
			restore = e.breakTable("meets")
		}
		if svc.mu.TryLock() {
			svc.mu.Unlock()
		}
	})
	_, err = svc.ListMeets(o.admin, o.id)
	if !errors.Is(err, ErrPersistence) {
		t.Fatalf("err = %v, want ErrPersistence", err)
	}
	restore()
}

func TestCov1xDBOrgCacheEvictsLeastRecentlyServedClub(t *testing.T) {
	e := cov1xDBNew(t)
	a, b := e.newOrg(0), e.newOrg(0)
	svc := e.svc
	svc.SetOrgCacheLimits(0, 1)
	for _, o := range []*cov1xDBOrg{a, b} {
		if _, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "Meet of " + o.id[:8]}); err != nil {
			t.Fatal(err)
		}
	}
	// Serving b pushed a out of memory (cap of one club); the database still has it.
	if n := len(svc.SnapshotMeets(a.id)); n != 0 {
		t.Fatalf("club a still cached: %d meets", n)
	}
	if e.count("meets") != 2 {
		t.Fatalf("meets in DB = %d", e.count("meets"))
	}
	got, err := svc.ListMeets(a.admin, a.id)
	if err != nil || len(got) != 1 || got[0].OrgID != a.id {
		t.Fatalf("club a reload = %+v err=%v", got, err)
	}
	if len(svc.SnapshotMeets(a.id)) != 1 || len(svc.SnapshotMeets(b.id)) != 0 {
		t.Fatalf("after serving a: a=%d b=%d cached meets", len(svc.SnapshotMeets(a.id)), len(svc.SnapshotMeets(b.id)))
	}
	if svc.orgCache.Len() != 1 {
		t.Fatalf("tracked clubs = %d, want 1", svc.orgCache.Len())
	}
	// An all-club refresh loads every club regardless of the per-request cap.
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(svc.SnapshotMeets(a.id)) != 1 || len(svc.SnapshotMeets(b.id)) != 1 {
		t.Fatalf("after Refresh: a=%d b=%d", len(svc.SnapshotMeets(a.id)), len(svc.SnapshotMeets(b.id)))
	}
}

func TestCov1xDBRepositoryReportsFailuresPerStage(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(1)
	ctx := context.Background()
	repo := newRepository(e.app)

	// Every table read by LoadAll, in the order it reads them.
	for _, table := range []string{"meets", "meet_sessions", "meet_events", "commitments", "entries", "swimmers", "households", "commitment_event_selections"} {
		restore := e.breakTable(table)
		_, _, _, _, _, _, _, _, err := repo.LoadAll(ctx, o.id)
		if err == nil || !strings.Contains(err.Error(), "meets: load") {
			t.Errorf("LoadAll with %s missing: err = %v", table, err)
		}
		// An all-club load fails the same way, for whichever club it reaches.
		if _, _, _, _, _, _, _, _, err := repo.LoadAll(ctx, ""); err == nil {
			t.Errorf("LoadAll(all clubs) with %s missing: want an error", table)
		}
		restore()
	}
	if _, _, _, _, _, _, _, _, err := repo.LoadAll(ctx, o.id); err != nil {
		t.Fatalf("LoadAll after restore: %v", err)
	}

	// A dead connection pool fails every entry point.
	cfg := e.app.Config().Copy()
	dead, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	broken := newRepository(dead)
	if _, err := broken.OrgIDs(ctx); err == nil {
		t.Error("OrgIDs on a closed pool must fail")
	}
	if _, _, _, _, _, _, _, _, err := broken.LoadAll(ctx, ""); err == nil {
		t.Error("LoadAll(all) on a closed pool must fail")
	}
	err = broken.CreateMeetBundle(ctx, Meet{ID: uuid.NewString(), OrgID: o.id}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "begin meet bundle transaction") {
		t.Errorf("CreateMeetBundle on a closed pool: %v", err)
	}
	if err := broken.UpsertMeet(ctx, Meet{ID: uuid.NewString(), OrgID: o.id}); err == nil {
		t.Error("UpsertMeet on a closed pool must fail")
	}
}

func TestCov1xDBRepositoryRejectsUnreadableRows(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(1)
	svc := e.svc
	meet, _, err := svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "S", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.TransitionMeetStatus(o.admin, o.id, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.PublishCommitmentRequest(o.admin, o.id, meet.ID); err != nil {
		t.Fatal(err)
	}
	cmt, _ := svc.ListCommitments(o.admin, o.id, meet.ID)
	evs, _ := svc.ListMeetEvents(o.admin, o.id, meet.ID)
	if _, _, err := svc.RespondToCommitment(o.parent, o.id, meet.ID, cmt[0].ID, CommitmentRespondInput{Response: "committed_yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectCommitmentEvent(o.parent, o.id, meet.ID, cmt[0].ID, SelectEventInput{MeetEventID: evs[0].ID, SeedTimeMs: cov1xInt(30000)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LockCommitments(o.admin, o.id, meet.ID); err != nil {
		t.Fatal(err)
	}
	// A column whose type no longer matches what the repository scans into is a
	// reported error, not a silently dropped row.
	e.exec(`ALTER TABLE commitment_event_selections ALTER COLUMN seed_time_ms TYPE text USING seed_time_ms::text`)
	e.exec(`ALTER TABLE entries ALTER COLUMN seed_time_ms TYPE text USING seed_time_ms::text`)
	// A new pool, because the old one holds prepared plans of the previous column type.
	fresh, err := pgxpool.NewWithConfig(context.Background(), e.app.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	repo := newRepository(fresh)
	if _, err := repo.LoadEventSelections(context.Background(), o.id); err == nil || !strings.Contains(err.Error(), "meets: scan event selection") {
		t.Errorf("LoadEventSelections: %v", err)
	}
	if _, err := repo.LoadEntries(context.Background(), o.id); err == nil || !strings.Contains(err.Error(), "meets: scan entry") {
		t.Errorf("LoadEntries: %v", err)
	}
}

func TestCov1xDBNewServiceWithNilPoolStaysInMemory(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithPool(nil)
	if svc.repo != nil {
		t.Fatal("nil pool must give an in-memory service")
	}
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh must be a no-op in memory: %v", err)
	}
	if err := svc.RefreshOrg(context.Background(), "org-a"); err != nil {
		t.Fatalf("RefreshOrg must be a no-op in memory: %v", err)
	}
}

// Deadlines and the age-up date must come back from Postgres in exactly the shape
// the in-memory service stored them, so a reload never changes what clients see.
func TestDBMeetDeadlinesAndAgeUpDateRoundTrip(t *testing.T) {
	e := cov1xDBNew(t)
	o := e.newOrg(1)
	e.svc.SetOrgTimezoneLookup(func(string) string { return "America/Los_Angeles" })
	ctx := context.Background()

	cases := map[string]struct{ in, want string }{
		"date only": {"2026-11-20", "2026-11-20"},
		"local":     {"2026-11-20T17:00", "2026-11-21T01:00:00Z"},
		"offset":    {"2026-11-20T17:00:30-08:00", "2026-11-21T01:00:30Z"},
		"midnight":  {"2026-11-20T00:00:00Z", "2026-11-20"},
		"blank":     {"", ""},
	}
	ids := map[string]string{}
	for name, tc := range cases {
		m, _, err := e.svc.CreateMeet(o.admin, o.id, CreateMeetInput{
			Name: name, StartDate: "2026-12-05", AgeUpDate: "2026-12-31",
			EntryDeadline: tc.in, CommitmentDeadline: tc.in,
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if m.CommitmentDeadline != tc.want {
			t.Fatalf("%s: in-memory = %q, want %q", name, m.CommitmentDeadline, tc.want)
		}
		ids[m.ID] = name
	}
	// A meet without an age-up date reloads with none.
	plain, _, err := e.svc.CreateMeet(o.admin, o.id, CreateMeetInput{Name: "plain", StartDate: "2026-12-05"})
	if err != nil {
		t.Fatal(err)
	}

	fresh := NewServiceWithPool(e.app)
	if err := fresh.RefreshOrg(ctx, o.id); err != nil {
		t.Fatal(err)
	}
	for _, m := range fresh.SnapshotMeets(o.id) {
		if m.ID == plain.ID {
			if m.AgeUpDate != "" || m.AgeReferenceDate() != "2026-12-05" {
				t.Fatalf("plain meet reloaded with age-up %q", m.AgeUpDate)
			}
			continue
		}
		tc := cases[ids[m.ID]]
		if m.EntryDeadline != tc.want || m.CommitmentDeadline != tc.want || m.AgeUpDate != "2026-12-31" {
			t.Fatalf("%s reloaded as entry=%q commitment=%q age-up=%q, want %q / 2026-12-31", ids[m.ID], m.EntryDeadline, m.CommitmentDeadline, m.AgeUpDate, tc.want)
		}
	}
}
