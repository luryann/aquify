package meets

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xOrg = "org-a"

type cov1xEnv struct {
	svc      *Service
	admin    *appmiddleware.Principal
	coach    *appmiddleware.Principal
	parent   *appmiddleware.Principal // household hh-1
	parent2  *appmiddleware.Principal // household hh-2
	swimmer  *appmiddleware.Principal // household hh-1
	outsider *appmiddleware.Principal // holds no role this domain knows
}

func cov1xNewEnv() *cov1xEnv {
	e := &cov1xEnv{
		svc:      NewService(),
		admin:    &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}},
		coach:    &appmiddleware.Principal{UserID: "coach-1", Roles: []string{"coach"}},
		parent:   &appmiddleware.Principal{UserID: "parent-1", Roles: []string{"parent"}},
		parent2:  &appmiddleware.Principal{UserID: "parent-2", HouseholdID: "hh-2", Roles: []string{"parent"}},
		swimmer:  &appmiddleware.Principal{UserID: "swimmer-1", HouseholdID: "hh-1", Roles: []string{"swimmer"}},
		outsider: &appmiddleware.Principal{UserID: "nobody", Roles: []string{"guest"}},
	}
	e.svc.SeedSwimmer(cov1xOrg, "sw-1", "hh-1")
	e.svc.SeedSwimmer(cov1xOrg, "sw-2", "hh-2")
	// parent-1 is linked to hh-1 only through the parent->household index.
	e.svc.SeedParentHousehold("parent-1", "hh-1")
	e.svc.SeedParentHousehold("parent-1", "hh-1")
	return e
}

func (e *cov1xEnv) createMeet(t *testing.T) Meet {
	t.Helper()
	meet, _, err := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{
		Name: "  Winter Open ", Course: "SCY", StartDate: "2026-12-05", EndDate: "2026-12-06",
		Sessions: []CreateMeetSessionInput{{SessionNumber: 1, Name: "AM", WarmupTime: "07:00", StartTime: "08:00:00"}},
		Events: []CreateMeetEventDraftInput{
			{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 50},
			{EventNumber: "2", Stroke: "backstroke", Distance: 50, Gender: "F", MinAge: cov1xInt(8), MaxAge: cov1xInt(12)},
			{EventNumber: "3", Stroke: "freestyle_relay", Distance: 200, IsRelay: true},
			{EventNumber: "4", Stroke: "butterfly", Distance: 50, Gender: "M"},
			{EventNumber: "5", Stroke: "breaststroke", Distance: 50},
		},
		SignupSettings: &CreateMeetSignupSettingsInput{SelfSignupEnabled: false, MaxEntriesPerSwimmer: 2},
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if meet.Name != "Winter Open" || meet.SelfSignupEnabled || meet.MaxEntriesPerSwimmer != 2 || meet.Status != "draft" {
		t.Fatalf("meet = %+v", meet)
	}
	return meet
}

func (e *cov1xEnv) publish(t *testing.T, meetID string) {
	t.Helper()
	if _, _, err := e.svc.TransitionMeetStatus(e.admin, cov1xOrg, meetID, "published"); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// openMeet returns a commitment_open meet, its events by event number and the
// commitments by swimmer.
func (e *cov1xEnv) openMeet(t *testing.T) (Meet, map[string]MeetEvent, map[string]Commitment) {
	t.Helper()
	meet := e.createMeet(t)
	e.publish(t, meet.ID)
	meet, created, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID)
	if err != nil || len(created) != 2 {
		t.Fatalf("PublishCommitmentRequest: %d commitments, %v", len(created), err)
	}
	return meet, e.eventsByNumber(t, meet.ID), e.commitmentsBySwimmer(t, meet.ID)
}

func (e *cov1xEnv) eventsByNumber(t *testing.T, meetID string) map[string]MeetEvent {
	t.Helper()
	evs, err := e.svc.ListMeetEvents(e.admin, cov1xOrg, meetID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]MeetEvent{}
	for _, ev := range evs {
		out[ev.EventNumber] = ev
	}
	return out
}

func (e *cov1xEnv) commitmentsBySwimmer(t *testing.T, meetID string) map[string]Commitment {
	t.Helper()
	cs, err := e.svc.ListCommitments(e.admin, cov1xOrg, meetID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Commitment{}
	for _, c := range cs {
		out[c.SwimmerID] = c
	}
	return out
}

func (e *cov1xEnv) respond(t *testing.T, p *appmiddleware.Principal, meetID, cmtID, resp string) {
	t.Helper()
	if _, _, err := e.svc.RespondToCommitment(p, cov1xOrg, meetID, cmtID, CommitmentRespondInput{Response: resp}); err != nil {
		t.Fatalf("respond %s: %v", resp, err)
	}
}

func cov1xInt(v int) *int { return &v }

func cov1xIs(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: err = %v, want %v", what, err, want)
	}
}

func cov1xErrContains(t *testing.T, what string, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Errorf("%s: err = %v, want message containing %q", what, err, sub)
	}
}

// closedMeetWithEntries drives a meet through lock so entries exist.
func (e *cov1xEnv) closedMeetWithEntries(t *testing.T) (Meet, map[string]MeetEvent, []Entry) {
	t.Helper()
	meet, evs, cmts := e.openMeet(t)
	e.respond(t, e.parent, meet.ID, cmts["sw-1"].ID, "committed_yes")
	for _, n := range []string{"1", "5"} {
		if _, err := e.svc.SelectCommitmentEvent(e.parent, cov1xOrg, meet.ID, cmts["sw-1"].ID, SelectEventInput{MeetEventID: evs[n].ID}); err != nil {
			t.Fatalf("select %s: %v", n, err)
		}
	}
	closed, _, err := e.svc.LockCommitments(e.admin, cov1xOrg, meet.ID)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := e.svc.ListEntries(e.admin, cov1xOrg, meet.ID)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %d, %v", len(entries), err)
	}
	return closed, evs, entries
}

func TestCov1xRoleGatesOnEveryOperation(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	meet := e.createMeet(t)
	s := e.svc
	for _, who := range []struct {
		name string
		p    *appmiddleware.Principal
	}{{"nil", nil}, {"outsider", e.outsider}} {
		p := who.p
		calls := map[string]func() error{
			"CreateMeet": func() error { _, _, err := s.CreateMeet(p, cov1xOrg, CreateMeetInput{Name: "x"}); return err },
			"Transition": func() error { _, _, err := s.TransitionMeetStatus(p, cov1xOrg, meet.ID, "published"); return err },
			"CreateSession": func() error {
				_, _, err := s.CreateMeetSession(p, cov1xOrg, meet.ID, CreateMeetSessionInput{})
				return err
			},
			"CreateEvent": func() error { _, _, err := s.CreateMeetEvent(p, cov1xOrg, meet.ID, CreateMeetEventInput{}); return err },
			"Publish":     func() error { _, _, _, err := s.PublishCommitmentRequest(p, cov1xOrg, meet.ID); return err },
			"Lock":        func() error { _, _, err := s.LockCommitments(p, cov1xOrg, meet.ID); return err },
			"Propose":     func() error { _, _, err := s.ProposeEntry(p, cov1xOrg, meet.ID, ProposeEntryInput{}); return err },
			"Override": func() error {
				_, _, err := s.OverrideEntry(p, cov1xOrg, meet.ID, "e", OverrideEntryInput{})
				return err
			},
			"Scratch":         func() error { _, _, err := s.ScratchEntry(p, cov1xOrg, meet.ID, "e", ""); return err },
			"Approve":         func() error { _, _, err := s.ApproveEntry(p, cov1xOrg, meet.ID, "e"); return err },
			"MarkExported":    func() error { _, _, err := s.MarkEntriesExported(p, cov1xOrg, meet.ID, nil); return err },
			"ListMeets":       func() error { _, err := s.ListMeets(p, cov1xOrg); return err },
			"GetMeet":         func() error { _, err := s.GetMeet(p, cov1xOrg, meet.ID); return err },
			"ListSessions":    func() error { _, err := s.ListMeetSessions(p, cov1xOrg, meet.ID); return err },
			"ListEvents":      func() error { _, err := s.ListMeetEvents(p, cov1xOrg, meet.ID); return err },
			"ListCommitments": func() error { _, err := s.ListCommitments(p, cov1xOrg, meet.ID); return err },
			"ListEntries":     func() error { _, err := s.ListEntries(p, cov1xOrg, meet.ID); return err },
			"Respond": func() error {
				_, _, err := s.RespondToCommitment(p, cov1xOrg, meet.ID, "c", CommitmentRespondInput{})
				return err
			},
			"Select": func() error {
				_, err := s.SelectCommitmentEvent(p, cov1xOrg, meet.ID, "c", SelectEventInput{})
				return err
			},
			"RemoveSelection": func() error { return s.RemoveCommitmentEventSelection(p, cov1xOrg, meet.ID, "c", "s") },
			"ListSelections":  func() error { _, err := s.ListCommitmentEventSelections(p, cov1xOrg, meet.ID, "c"); return err },
			"ParseMeetFile":   func() error { _, err := s.ParseMeetFile(p, cov1xOrg, ParseMeetFileInput{}); return err },
		}
		for name, call := range calls {
			cov1xIs(t, who.name+"/"+name, call(), ErrForbidden)
		}
	}
	// Parents and swimmers may read but never mutate meet structure.
	if _, _, err := s.CreateMeetSession(e.parent, cov1xOrg, meet.ID, CreateMeetSessionInput{Name: "x", SessionNumber: 9}); !errors.Is(err, ErrForbidden) {
		t.Errorf("parent CreateMeetSession: %v", err)
	}
	if _, err := s.ListEntries(e.swimmer, cov1xOrg, meet.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("swimmer ListEntries: %v", err)
	}
	// A coach may not answer commitments on a family's behalf.
	if _, _, err := s.RespondToCommitment(e.coach, cov1xOrg, meet.ID, "c", CommitmentRespondInput{Response: "committed_yes"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("coach respond: %v", err)
	}
}

func TestCov1xCreateMeetRejectsBadInput(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	for _, tc := range []struct {
		name string
		in   CreateMeetInput
		want string
	}{
		{"blank name", CreateMeetInput{Name: "  "}, "name is required"},
		{"bad meet type", CreateMeetInput{Name: "m", MeetType: "bbq"}, "meet_type must be one of"},
		{"bad course", CreateMeetInput{Name: "m", Course: "XYZ"}, "course must be one of"},
		{"bad end date", CreateMeetInput{Name: "m", EndDate: "soon"}, "end_date must be a YYYY-MM-DD"},
		{"bad commitment deadline", CreateMeetInput{Name: "m", CommitmentDeadline: "x"}, "commitment_deadline must be"},
		{"session without name", CreateMeetInput{Name: "m", Sessions: []CreateMeetSessionInput{{SessionNumber: 1}}}, "session name is required"},
		{"session number zero", CreateMeetInput{Name: "m", Sessions: []CreateMeetSessionInput{{Name: "s"}}}, "session_number must be a positive"},
		{"session bad warmup", CreateMeetInput{Name: "m", Sessions: []CreateMeetSessionInput{{Name: "s", SessionNumber: 1, WarmupTime: "early"}}}, "warmup_time must be an HH:MM"},
		{"session bad start", CreateMeetInput{Name: "m", Sessions: []CreateMeetSessionInput{{Name: "s", SessionNumber: 1, StartTime: "late"}}}, "start_time must be an HH:MM"},
		{"duplicate session number", CreateMeetInput{Name: "m", Sessions: []CreateMeetSessionInput{{Name: "a", SessionNumber: 1}, {Name: "b", SessionNumber: 1}}}, "session_number 1 already exists"},
		{"event without number", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{Stroke: "freestyle", Distance: 50}}}, "event_number is required"},
		{"event without stroke", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Distance: 50}}}, "stroke is required"},
		{"event unknown stroke", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "crawl", Distance: 50}}}, "stroke must be one of"},
		{"event distance", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle"}}}, "distance must be a positive"},
		{"event age order", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, MinAge: cov1xInt(12), MaxAge: cov1xInt(10)}}}, "min_age must not be greater"},
		{"event course", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, Course: "pool"}}}, "course must be one of"},
		{"event gender", CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50, Gender: "Q"}}}, "gender must be one of"},
	} {
		_, _, err := e.svc.CreateMeet(e.admin, cov1xOrg, tc.in)
		cov1xErrContains(t, tc.name, err, tc.want)
	}
	_, _, err := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{
		{EventNumber: "1", Stroke: "freestyle", Distance: 50}, {EventNumber: " 1 ", Stroke: "backstroke", Distance: 50}}})
	cov1xIs(t, "duplicate event number", err, ErrEventNumberConflict)
	_, _, err = e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "m", Events: []CreateMeetEventDraftInput{
		{SessionNumber: 7, EventNumber: "1", Stroke: "freestyle", Distance: 50}}})
	cov1xIs(t, "unknown session number", err, ErrSessionNotFound)
	if got := e.svc.SnapshotMeets(cov1xOrg); len(got) != 0 {
		t.Fatalf("failed creates must not leave meets behind: %+v", got)
	}
}

func TestCov1xCreateMeetAppliesDefaultsAndWiresEvents(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	// Defaults: invitational / SCY / open signup with a cap of three.
	meet, audit, err := e.svc.CreateMeet(e.coach, cov1xOrg, CreateMeetInput{Name: "Defaults"})
	if err != nil {
		t.Fatal(err)
	}
	if meet.MeetType != "invitational" || meet.Course != "SCY" || !meet.SelfSignupEnabled || meet.MaxEntriesPerSwimmer != 3 {
		t.Fatalf("defaults = %+v", meet)
	}
	if audit.Action != "meet.created" || audit.ActorUserID != "coach-1" || audit.Metadata["meet_id"] != meet.ID {
		t.Fatalf("audit = %+v", audit)
	}
	// A non-positive cap in the settings falls back to three but keeps the signup flag.
	meet, _, err = e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "Cap",
		SignupSettings: &CreateMeetSignupSettingsInput{SelfSignupEnabled: false, MaxEntriesPerSwimmer: 0}})
	if err != nil || meet.MaxEntriesPerSwimmer != 3 || meet.SelfSignupEnabled {
		t.Fatalf("cap = %+v err=%v", meet, err)
	}
	// Events inherit the meet's course and map to their session by number.
	full, _, err := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "Long", Course: "LCM",
		Sessions: []CreateMeetSessionInput{{SessionNumber: 4, Name: "Prelims"}},
		Events:   []CreateMeetEventDraftInput{{SessionNumber: 4, EventNumber: "1", Stroke: " Freestyle ", Distance: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	sessions, _ := e.svc.ListMeetSessions(e.admin, cov1xOrg, full.ID)
	events, _ := e.svc.ListMeetEvents(e.admin, cov1xOrg, full.ID)
	if len(sessions) != 1 || len(events) != 1 {
		t.Fatalf("sessions=%d events=%d", len(sessions), len(events))
	}
	if events[0].SessionID != sessions[0].ID || events[0].Course != "LCM" || events[0].Gender != "O" || events[0].Stroke != "freestyle" {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestCov1xMeetVisibilityAndOrdering(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	draft := e.createMeet(t)
	later, _, err := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "Later", StartDate: "2027-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	e.publish(t, later.ID)
	// A meet of another club must never leak.
	if _, _, err := e.svc.CreateMeet(e.admin, "org-b", CreateMeetInput{Name: "Other club"}); err != nil {
		t.Fatal(err)
	}

	staff, err := e.svc.ListMeets(e.coach, cov1xOrg)
	if err != nil || len(staff) != 2 || staff[0].ID != later.ID || staff[1].ID != draft.ID {
		t.Fatalf("staff list (newest start first) = %+v err=%v", staff, err)
	}
	parent, err := e.svc.ListMeets(e.parent, cov1xOrg)
	if err != nil || len(parent) != 1 || parent[0].ID != later.ID {
		t.Fatalf("drafts must be hidden from parents: %+v err=%v", parent, err)
	}
	// Equal start dates fall back to ID order.
	a, _, _ := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "A", StartDate: "2027-02-01"})
	b, _, _ := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "B", StartDate: "2027-02-01"})
	staff, _ = e.svc.ListMeets(e.admin, cov1xOrg)
	wantFirst, wantSecond := a.ID, b.ID
	if wantFirst > wantSecond {
		wantFirst, wantSecond = wantSecond, wantFirst
	}
	if staff[0].ID != wantFirst || staff[1].ID != wantSecond {
		t.Fatalf("tie order = %s,%s want %s,%s", staff[0].ID, staff[1].ID, wantFirst, wantSecond)
	}

	if _, err := e.svc.GetMeet(e.parent, cov1xOrg, draft.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("parent GetMeet(draft): %v", err)
	}
	if _, err := e.svc.GetMeet(e.admin, "org-b", draft.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("cross-org GetMeet: %v", err)
	}
	if got, err := e.svc.GetMeet(e.parent, cov1xOrg, later.ID); err != nil || got.ID != later.ID {
		t.Errorf("parent GetMeet(published) = %+v, %v", got, err)
	}
	if _, err := e.svc.ListMeetSessions(e.parent, cov1xOrg, draft.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("parent ListMeetSessions(draft): %v", err)
	}
	if _, err := e.svc.ListMeetEvents(e.parent, cov1xOrg, draft.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("parent ListMeetEvents(draft): %v", err)
	}
	if _, err := e.svc.ListMeetSessions(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("ListMeetSessions(missing): %v", err)
	}
	if _, err := e.svc.ListMeetEvents(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("ListMeetEvents(missing): %v", err)
	}
}

func TestCov1xTransitionMeetStatus(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	meet := e.createMeet(t)
	tr := func(to string) (Meet, MeetAuditEvent, error) {
		return e.svc.TransitionMeetStatus(e.admin, cov1xOrg, meet.ID, to)
	}
	for _, to := range []string{"commitment_open", " COMMITMENT_CLOSED "} {
		if _, _, err := tr(to); !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), "/commitment-request") {
			t.Errorf("%q must be managed by its dedicated endpoint: %v", to, err)
		}
	}
	if _, _, err := e.svc.TransitionMeetStatus(e.admin, cov1xOrg, "missing", "published"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, _, err := tr("completed"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("draft->completed: %v", err)
	}
	got, audit, err := tr(" Published ")
	if err != nil || got.Status != "published" || got.UpdatedBy != "admin-1" {
		t.Fatalf("publish = %+v err=%v", got, err)
	}
	if audit.Action != "meet.status_transitioned" || audit.Before["status"] != "draft" || audit.After["status"] != "published" {
		t.Fatalf("audit = %+v", audit)
	}
	if got, _, err := tr("draft"); err != nil || got.Status != "draft" {
		t.Fatalf("unpublish = %+v err=%v", got, err)
	}
	e.publish(t, meet.ID)
	// Walk the rest of the lifecycle; finalizing stamps EntryFinalizedAt.
	if _, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.LockCommitments(e.admin, cov1xOrg, meet.ID); err != nil {
		t.Fatal(err)
	}
	fin, _, err := tr("entry_finalized")
	if err != nil || fin.EntryFinalizedAt == nil {
		t.Fatalf("finalize = %+v err=%v", fin, err)
	}
	for _, to := range []string{"completed", "archived"} {
		if got, _, err := tr(to); err != nil || got.Status != to {
			t.Fatalf("%s = %+v err=%v", to, got, err)
		}
	}
	if _, _, err := tr("draft"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("archived is terminal: %v", err)
	}
}

func TestCov1xCreateMeetSession(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	meet := e.createMeet(t)
	for _, tc := range []struct {
		name string
		in   CreateMeetSessionInput
		want string
	}{
		{"blank name", CreateMeetSessionInput{SessionNumber: 2}, "name is required"},
		{"number", CreateMeetSessionInput{Name: "s"}, "session_number must be a positive"},
		{"date", CreateMeetSessionInput{Name: "s", SessionNumber: 2, SessionDate: "tomorrow"}, "session_date must be"},
		{"warmup", CreateMeetSessionInput{Name: "s", SessionNumber: 2, WarmupTime: "x"}, "warmup_time must be an HH:MM"},
		{"start", CreateMeetSessionInput{Name: "s", SessionNumber: 2, StartTime: "x"}, "start_time must be an HH:MM"},
		{"duplicate", CreateMeetSessionInput{Name: "s", SessionNumber: 1}, "session_number 1 already exists"},
	} {
		_, _, err := e.svc.CreateMeetSession(e.admin, cov1xOrg, meet.ID, tc.in)
		cov1xErrContains(t, tc.name, err, tc.want)
	}
	if _, _, err := e.svc.CreateMeetSession(e.admin, cov1xOrg, "missing", CreateMeetSessionInput{Name: "s", SessionNumber: 2}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	sess, audit, err := e.svc.CreateMeetSession(e.coach, cov1xOrg, meet.ID, CreateMeetSessionInput{
		Name: " PM ", SessionNumber: 2, SessionDate: "2026-12-05", WarmupTime: "12:00", StartTime: "13:00", Notes: " bring towels "})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Name != "PM" || sess.Notes != "bring towels" || sess.CreatedBy != "coach-1" || audit.Action != "meet.session_created" {
		t.Fatalf("session = %+v audit=%+v", sess, audit)
	}
	list, err := e.svc.ListMeetSessions(e.admin, cov1xOrg, meet.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("sessions = %d err=%v", len(list), err)
	}
	// Once commitments are closed the structure is frozen.
	e.publish(t, meet.ID)
	e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID)
	e.svc.LockCommitments(e.admin, cov1xOrg, meet.ID)
	if _, _, err := e.svc.CreateMeetSession(e.admin, cov1xOrg, meet.ID, CreateMeetSessionInput{Name: "late", SessionNumber: 9}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("closed meet: %v", err)
	}
	if _, _, err := e.svc.CreateMeetEvent(e.admin, cov1xOrg, meet.ID, CreateMeetEventInput{EventNumber: "99", Stroke: "freestyle", Distance: 50}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("closed meet event: %v", err)
	}
}

func TestCov1xCreateMeetEvent(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	meet := e.createMeet(t)
	sessions, _ := e.svc.ListMeetSessions(e.admin, cov1xOrg, meet.ID)
	valid := CreateMeetEventInput{EventNumber: "20", Stroke: "backstroke", Distance: 100}
	for _, tc := range []struct {
		name   string
		mutate func(*CreateMeetEventInput)
		want   string
	}{
		{"number", func(i *CreateMeetEventInput) { i.EventNumber = " " }, "event_number is required"},
		{"stroke blank", func(i *CreateMeetEventInput) { i.Stroke = "" }, "stroke is required"},
		{"stroke unknown", func(i *CreateMeetEventInput) { i.Stroke = "crawl" }, "stroke must be one of"},
		{"distance", func(i *CreateMeetEventInput) { i.Distance = 0 }, "distance must be a positive"},
		{"fee", func(i *CreateMeetEventInput) { i.EntryFeeCents = -1 }, "entry_fee_cents must not be negative"},
		{"gender", func(i *CreateMeetEventInput) { i.Gender = "Z" }, "gender must be one of"},
		{"course", func(i *CreateMeetEventInput) { i.Course = "pool" }, "course must be one of"},
		{"session", func(i *CreateMeetEventInput) { i.SessionID = "sess_nope" }, ErrSessionNotFound.Error()},
		{"duplicate number", func(i *CreateMeetEventInput) { i.EventNumber = "1" }, ErrEventNumberConflict.Error()},
	} {
		in := valid
		tc.mutate(&in)
		_, _, err := e.svc.CreateMeetEvent(e.admin, cov1xOrg, meet.ID, in)
		cov1xErrContains(t, tc.name, err, tc.want)
	}
	if _, _, err := e.svc.CreateMeetEvent(e.admin, cov1xOrg, "missing", valid); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	// A session of a different meet is not acceptable.
	other, _, _ := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "Other", Sessions: []CreateMeetSessionInput{{SessionNumber: 1, Name: "x"}}})
	otherSessions, _ := e.svc.ListMeetSessions(e.admin, cov1xOrg, other.ID)
	in := valid
	in.SessionID = otherSessions[0].ID
	if _, _, err := e.svc.CreateMeetEvent(e.admin, cov1xOrg, meet.ID, in); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("foreign session: %v", err)
	}
	in = valid
	in.SessionID = " " + sessions[0].ID + " "
	in.Gender = "F"
	ev, audit, err := e.svc.CreateMeetEvent(e.coach, cov1xOrg, meet.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev.SessionID != sessions[0].ID || ev.Course != "SCY" || ev.Gender != "F" || ev.Stroke != "backstroke" || audit.Action != "meet.event_created" {
		t.Fatalf("event = %+v audit=%+v", ev, audit)
	}
	// ListMeetEvents orders by program order, then ID.
	evs, _ := e.svc.ListMeetEvents(e.admin, cov1xOrg, meet.ID)
	var order []string
	for _, v := range evs {
		order = append(order, v.EventNumber)
	}
	if strings.Join(order, ",") != "1,2,3,4,5,20" {
		t.Fatalf("event order = %v", order)
	}
}

func TestCov1xPublishCommitmentRequest(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	e.svc.SeedSwimmer("org-b", "sw-b", "hh-b") // another club's swimmer is never invited
	meet := e.createMeet(t)
	if _, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("draft meet must be published first: %v", err)
	}
	e.publish(t, meet.ID)
	// A swimmer who already has a commitment is skipped.
	e.svc.mu.Lock()
	e.svc.commitments["pre"] = Commitment{ID: "pre", OrgID: cov1xOrg, MeetID: meet.ID, SwimmerID: "sw-1", HouseholdID: "hh-1", Status: "invited"}
	e.svc.mu.Unlock()
	got, created, audit, err := e.svc.PublishCommitmentRequest(e.coach, cov1xOrg, meet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "commitment_open" || got.CommitmentOpenAt == nil || len(created) != 1 || created[0].SwimmerID != "sw-2" {
		t.Fatalf("meet=%+v created=%+v", got, created)
	}
	if created[0].Status != "invited" || created[0].HouseholdID != "hh-2" || created[0].CreatedBy != "coach-1" {
		t.Fatalf("commitment = %+v", created[0])
	}
	if audit.Action != "meet.commitment_request_published" || audit.After["commitments_created"] != 1 {
		t.Fatalf("audit = %+v", audit)
	}
	if _, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("already open: %v", err)
	}
}

func TestCov1xInviteSwimmerToOpenMeetsEdgeCases(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	if _, err := e.svc.InviteSwimmerToOpenMeets(cov1xOrg, "  ", "hh-9", "admin-1"); err == nil {
		t.Fatal("blank swimmer id must be rejected")
	}
	// Draft/published meets never get invitations; an open one does, even without a household.
	draft := e.createMeet(t)
	created, err := e.svc.InviteSwimmerToOpenMeets(cov1xOrg, "sw-late", "", "admin-1")
	if err != nil || len(created) != 0 {
		t.Fatalf("draft meet invited: %+v err=%v", created, err)
	}
	e.publish(t, draft.ID)
	created, err = e.svc.InviteSwimmerToOpenMeets(cov1xOrg, "sw-late", "", "admin-1")
	if err != nil || len(created) != 0 {
		t.Fatalf("published meet invited: %+v err=%v", created, err)
	}
	if _, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, draft.ID); err != nil {
		t.Fatal(err)
	}
	created, err = e.svc.InviteSwimmerToOpenMeets(cov1xOrg, "sw-late-2", "", "admin-1")
	if err != nil || len(created) != 1 || created[0].HouseholdID != "" || created[0].CreatedBy != "admin-1" {
		t.Fatalf("late invite = %+v err=%v", created, err)
	}
	if _, ok := e.svc.households[""]; ok {
		t.Fatal("a blank household must not be indexed")
	}
}

func TestCov1xRespondToCommitment(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	meet := e.createMeet(t)
	respond := func(p *appmiddleware.Principal, meetID, cmt, resp string) (Commitment, MeetAuditEvent, error) {
		return e.svc.RespondToCommitment(p, cov1xOrg, meetID, cmt, CommitmentRespondInput{Response: resp, Notes: " note "})
	}
	if _, _, err := respond(e.parent, meet.ID, "c", "maybe"); err == nil || !strings.Contains(err.Error(), "committed_yes") {
		t.Errorf("bad response: %v", err)
	}
	if _, _, err := respond(e.parent, "missing", "c", "committed_yes"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, _, err := respond(e.parent, meet.ID, "c", "committed_yes"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("commitments not open: %v", err)
	}

	open, evs, cmts := e.openMeet(t)
	if _, _, err := respond(e.parent, open.ID, "missing", "committed_yes"); !errors.Is(err, ErrCommitmentNotFound) {
		t.Errorf("missing commitment: %v", err)
	}
	// parent-1 owns hh-1 (via the index), so hh-2's commitment is off limits.
	if _, _, err := respond(e.parent, open.ID, cmts["sw-2"].ID, "committed_yes"); !errors.Is(err, ErrForbidden) {
		t.Errorf("foreign household: %v", err)
	}
	// A commitment that has moved past the response states cannot be answered.
	e.svc.mu.Lock()
	c := e.svc.commitments[cmts["sw-1"].ID]
	c.Status = "locked"
	e.svc.commitments[c.ID] = c
	e.svc.mu.Unlock()
	if _, _, err := respond(e.parent, open.ID, c.ID, "committed_yes"); !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), "locked") {
		t.Errorf("locked commitment: %v", err)
	}
	e.svc.mu.Lock()
	c.Status = "invited"
	e.svc.commitments[c.ID] = c
	e.svc.mu.Unlock()

	yes, audit, err := respond(e.parent, open.ID, c.ID, " COMMITTED_YES ")
	if err != nil || yes.Status != "committed_yes" || yes.Notes != "note" || yes.CommittedAt == nil {
		t.Fatalf("yes = %+v err=%v", yes, err)
	}
	if audit.Before["status"] != "invited" || audit.After["status"] != "committed_yes" {
		t.Fatalf("audit = %+v", audit)
	}
	sel, err := e.svc.SelectCommitmentEvent(e.parent, cov1xOrg, open.ID, c.ID, SelectEventInput{MeetEventID: evs["1"].ID})
	if err != nil {
		t.Fatal(err)
	}
	// Switching to "no" drops the selections the family had made.
	if no, _, err := respond(e.parent, open.ID, c.ID, "committed_no"); err != nil || no.Status != "committed_no" {
		t.Fatalf("no = %+v err=%v", no, err)
	}
	left, err := e.svc.ListCommitmentEventSelections(e.admin, cov1xOrg, open.ID, c.ID)
	if err != nil || len(left) != 0 {
		t.Fatalf("selections after switching to no: %+v err=%v (had %s)", left, err, sel.ID)
	}
	// An unrelated commitment's selections survive.
	if again, _, err := respond(e.parent, open.ID, c.ID, "committed_yes"); err != nil || again.Status != "committed_yes" {
		t.Fatalf("back to yes = %+v err=%v", again, err)
	}
}

func TestCov1xListCommitmentsScoping(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	open, _, _ := e.openMeet(t)
	all, err := e.svc.ListCommitments(e.coach, cov1xOrg, open.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("staff sees %d, %v", len(all), err)
	}
	for _, tc := range []struct {
		name string
		p    *appmiddleware.Principal
		want []string
	}{
		{"parent via index", e.parent, []string{"sw-1"}},
		{"parent via principal household", e.parent2, []string{"sw-2"}},
		{"swimmer of hh-1", e.swimmer, []string{"sw-1"}},
		{"swimmer without household", &appmiddleware.Principal{UserID: "s", Roles: []string{"swimmer"}}, nil},
	} {
		got, err := e.svc.ListCommitments(tc.p, cov1xOrg, open.ID)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var ids []string
		for _, c := range got {
			ids = append(ids, c.SwimmerID)
		}
		if strings.Join(ids, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s sees %v, want %v", tc.name, ids, tc.want)
		}
	}
	if _, err := e.svc.ListCommitments(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
}

func TestCov1xEventSelectionRulesAndScoping(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	e.svc.SetSwimmerEligibilityLookup(func(org, id string) (SwimmerEligibilityView, bool) {
		if org == cov1xOrg && id == "sw-1" {
			return SwimmerEligibilityView{Gender: "female", DateOfBirth: "2016-03-01"}, true // 10 on meet day
		}
		return SwimmerEligibilityView{}, false
	})
	open, evs, cmts := e.openMeet(t)
	c1, c2 := cmts["sw-1"], cmts["sw-2"]
	sel := func(p *appmiddleware.Principal, meetID, cmt string, in SelectEventInput) (EventSelection, error) {
		return e.svc.SelectCommitmentEvent(p, cov1xOrg, meetID, cmt, in)
	}

	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: " "}); err == nil || !strings.Contains(err.Error(), "meet_event_id is required") {
		t.Errorf("blank event: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID, SeedTimeMs: cov1xInt(0)}); !errors.Is(err, ErrValidation) {
		t.Errorf("bad seed: %v", err)
	}
	if _, err := sel(e.parent, "missing", c1.ID, SelectEventInput{MeetEventID: evs["1"].ID}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, err := sel(e.parent, open.ID, "missing", SelectEventInput{MeetEventID: evs["1"].ID}); !errors.Is(err, ErrCommitmentNotFound) {
		t.Errorf("missing commitment: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c2.ID, SelectEventInput{MeetEventID: evs["1"].ID}); !errors.Is(err, ErrForbidden) {
		t.Errorf("foreign household: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID}); !errors.Is(err, ErrNotCommittedYes) {
		t.Errorf("not yet committed: %v", err)
	}
	e.respond(t, e.parent, open.ID, c1.ID, "committed_yes")
	e.respond(t, e.parent2, open.ID, c2.ID, "committed_yes")

	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: "nope"}); !errors.Is(err, ErrEventNotFound) {
		t.Errorf("unknown event: %v", err)
	}
	foreign, _, _ := e.svc.CreateMeet(e.admin, cov1xOrg, CreateMeetInput{Name: "F", Events: []CreateMeetEventDraftInput{{EventNumber: "1", Stroke: "freestyle", Distance: 50}}})
	fev, _ := e.svc.ListMeetEvents(e.admin, cov1xOrg, foreign.ID)
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: fev[0].ID}); !errors.Is(err, ErrEventNotFound) {
		t.Errorf("event of another meet: %v", err)
	}
	// Eligibility is enforced up front: sw-1 is a girl, event 4 is boys-only.
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["4"].ID}); !errors.Is(err, ErrIneligibleEntry) {
		t.Errorf("ineligible: %v", err)
	}
	// A swimmer the lookup cannot resolve is not blocked.
	if _, err := sel(e.parent2, open.ID, c2.ID, SelectEventInput{MeetEventID: evs["4"].ID}); err != nil {
		t.Errorf("unresolvable swimmer must pass: %v", err)
	}

	first, err := sel(e.admin, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID, SeedTimeMs: cov1xInt(30000)})
	if err != nil || first.SeedTimeMs == nil || *first.SeedTimeMs != 30000 || first.CreatedBy != "admin-1" {
		t.Fatalf("select = %+v err=%v", first, err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["1"].ID}); !errors.Is(err, ErrDuplicateSelection) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["2"].ID}); err != nil {
		t.Fatal(err)
	}
	// The cap is two individual events; a relay does not count against it.
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["5"].ID}); !errors.Is(err, ErrMaxEntriesExceeded) {
		t.Errorf("third individual event: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["3"].ID}); err != nil {
		t.Errorf("relay over the cap: %v", err)
	}

	// Reading selections: scoping.
	list := func(p *appmiddleware.Principal, cmt string) ([]EventSelection, error) {
		return e.svc.ListCommitmentEventSelections(p, cov1xOrg, open.ID, cmt)
	}
	if got, err := list(e.parent, c1.ID); err != nil || len(got) != 3 {
		t.Errorf("parent list = %d err=%v", len(got), err)
	}
	if got, err := list(e.swimmer, c1.ID); err != nil || len(got) != 3 {
		t.Errorf("swimmer own household = %d err=%v", len(got), err)
	}
	if _, err := list(e.swimmer, c2.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("swimmer foreign household: %v", err)
	}
	if _, err := list(&appmiddleware.Principal{UserID: "s", Roles: []string{"swimmer"}}, c1.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("swimmer without household: %v", err)
	}
	if _, err := list(e.parent, c2.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("parent foreign household: %v", err)
	}
	if _, err := list(e.admin, "missing"); !errors.Is(err, ErrCommitmentNotFound) {
		t.Errorf("missing commitment: %v", err)
	}
	if _, err := e.svc.ListCommitmentEventSelections(e.admin, cov1xOrg, "missing", c1.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}

	// Removing a selection.
	rm := func(p *appmiddleware.Principal, meetID, cmt, selID string) error {
		return e.svc.RemoveCommitmentEventSelection(p, cov1xOrg, meetID, cmt, selID)
	}
	if err := rm(e.parent, "missing", c1.ID, first.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("rm missing meet: %v", err)
	}
	if err := rm(e.parent, open.ID, "missing", first.ID); !errors.Is(err, ErrCommitmentNotFound) {
		t.Errorf("rm missing commitment: %v", err)
	}
	if err := rm(e.parent, open.ID, c2.ID, first.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("rm foreign household: %v", err)
	}
	if err := rm(e.parent2, open.ID, c2.ID, first.ID); !errors.Is(err, ErrSelectionNotFound) {
		t.Errorf("rm selection of another commitment: %v", err)
	}
	if err := rm(e.parent, open.ID, c1.ID, "nope"); !errors.Is(err, ErrSelectionNotFound) {
		t.Errorf("rm unknown selection: %v", err)
	}
	if err := rm(e.parent, open.ID, c1.ID, " "+first.ID+" "); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if got, _ := list(e.admin, c1.ID); len(got) != 2 {
		t.Errorf("selections after removal = %d", len(got))
	}
	// Once the commitment window closes, selections are frozen.
	if _, _, err := e.svc.LockCommitments(e.admin, cov1xOrg, open.ID); err != nil {
		t.Fatal(err)
	}
	if err := rm(e.parent, open.ID, c1.ID, first.ID); !errors.Is(err, ErrCommitmentLocked) {
		t.Errorf("rm after lock: %v", err)
	}
	if _, err := sel(e.parent, open.ID, c1.ID, SelectEventInput{MeetEventID: evs["5"].ID}); !errors.Is(err, ErrCommitmentLocked) {
		t.Errorf("select after lock: %v", err)
	}
}

func TestCov1xLockCommitmentsRules(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	e.svc.SeedSwimmer(cov1xOrg, "sw-3", "hh-3")
	meet := e.createMeet(t)
	if _, _, err := e.svc.LockCommitments(e.admin, cov1xOrg, meet.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("not open: %v", err)
	}
	if _, _, err := e.svc.LockCommitments(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	e.publish(t, meet.ID)
	open, _, _, err := e.svc.PublishCommitmentRequest(e.admin, cov1xOrg, meet.ID)
	if err != nil {
		t.Fatal(err)
	}
	evs := e.eventsByNumber(t, open.ID)
	cmts := e.commitmentsBySwimmer(t, open.ID)
	e.respond(t, e.parent, open.ID, cmts["sw-1"].ID, "committed_yes")
	for _, n := range []string{"1", "2"} {
		if _, err := e.svc.SelectCommitmentEvent(e.parent, cov1xOrg, open.ID, cmts["sw-1"].ID, SelectEventInput{MeetEventID: evs[n].ID}); err != nil {
			t.Fatal(err)
		}
	}
	// A coach-reviewed commitment is locked too; its selection becomes an entry.
	e.svc.mu.Lock()
	sw3 := e.svc.commitments[cmts["sw-3"].ID]
	sw3.Status = "coach_reviewed"
	e.svc.commitments[sw3.ID] = sw3
	e.svc.selections["sel-sw3"] = EventSelection{ID: "sel-sw3", OrgID: cov1xOrg, CommitmentID: sw3.ID, MeetEventID: evs["5"].ID}
	// Selections that point at a missing event or one of another meet yield no entry.
	e.svc.selections["sel-ghost"] = EventSelection{ID: "sel-ghost", OrgID: cov1xOrg, CommitmentID: cmts["sw-1"].ID, MeetEventID: "ghost"}
	e.svc.events["foreign-ev"] = MeetEvent{ID: "foreign-ev", OrgID: cov1xOrg, MeetID: "other-meet", EventNumber: "1"}
	e.svc.selections["sel-foreign"] = EventSelection{ID: "sel-foreign", OrgID: cov1xOrg, CommitmentID: cmts["sw-1"].ID, MeetEventID: "foreign-ev"}
	// An entry that already exists is not duplicated.
	e.svc.entries["pre"] = Entry{ID: "pre", OrgID: cov1xOrg, MeetID: open.ID, SwimmerID: "sw-1", MeetEventID: evs["1"].ID, Status: "submitted"}
	e.svc.mu.Unlock()

	closed, audit, err := e.svc.LockCommitments(e.coach, cov1xOrg, open.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "commitment_closed" || closed.CommitmentClosedAt == nil {
		t.Fatalf("meet = %+v", closed)
	}
	if audit.After["locked_count"] != 2 || audit.After["entries_created"] != 2 {
		t.Fatalf("audit = %+v", audit.After)
	}
	entries, _ := e.svc.ListEntries(e.admin, cov1xOrg, open.ID)
	got := map[string]int{}
	for _, en := range entries {
		got[en.SwimmerID+"/"+en.MeetEventID]++
	}
	want := map[string]int{"sw-1/" + evs["1"].ID: 1, "sw-1/" + evs["2"].ID: 1, "sw-3/" + evs["5"].ID: 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	cmts = e.commitmentsBySwimmer(t, open.ID)
	if cmts["sw-1"].Status != "locked" || cmts["sw-3"].Status != "locked" || cmts["sw-2"].Status != "invited" {
		t.Fatalf("statuses = %v %v %v", cmts["sw-1"].Status, cmts["sw-3"].Status, cmts["sw-2"].Status)
	}
	if cmts["sw-1"].LockedAt == nil {
		t.Fatal("locked commitments carry a LockedAt")
	}
}

func TestCov1xEntryLifecycle(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	e.svc.SetSwimmerEligibilityLookup(func(org, id string) (SwimmerEligibilityView, bool) {
		return SwimmerEligibilityView{Gender: "female", DateOfBirth: "2016-03-01"}, id == "sw-1"
	})
	closed, evs, entries := e.closedMeetWithEntries(t)
	byEvent := map[string]Entry{}
	for _, en := range entries {
		byEvent[en.MeetEventID] = en
	}
	e1, e5 := byEvent[evs["1"].ID], byEvent[evs["5"].ID]

	// ListEntries scoping and errors.
	if got, err := e.svc.ListEntries(e.parent, cov1xOrg, closed.ID); err != nil || len(got) != 2 {
		t.Errorf("parent of hh-1 sees %d entries, %v", len(got), err)
	}
	if got, err := e.svc.ListEntries(e.parent2, cov1xOrg, closed.ID); err != nil || len(got) != 0 {
		t.Errorf("parent of hh-2 sees %d entries, %v", len(got), err)
	}
	if _, err := e.svc.ListEntries(e.admin, cov1xOrg, "missing"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}

	// ProposeEntry validation.
	propose := func(in ProposeEntryInput) (Entry, MeetAuditEvent, error) {
		return e.svc.ProposeEntry(e.coach, cov1xOrg, closed.ID, in)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["2"].ID, SeedTimeMs: cov1xInt(-1)}); !errors.Is(err, ErrValidation) {
		t.Errorf("seed: %v", err)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: " "}); err == nil || !strings.Contains(err.Error(), "swimmer_id and meet_event_id are required") {
		t.Errorf("blank ids: %v", err)
	}
	if _, _, err := e.svc.ProposeEntry(e.coach, cov1xOrg, "missing", ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: "x"}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: "x"}); !errors.Is(err, ErrEventNotFound) {
		t.Errorf("unknown event: %v", err)
	}
	// Eligibility: event 4 is for boys. Override needs a reason and is audited.
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["4"].ID}); !errors.Is(err, ErrIneligibleEntry) {
		t.Errorf("ineligible: %v", err)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["4"].ID, OverrideEligibility: true, OverrideReason: "  "}); !errors.Is(err, ErrIneligibleEntry) || !strings.Contains(err.Error(), "reason is required") {
		t.Errorf("override without reason: %v", err)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["2"].ID, CommitmentID: "nope"}); !errors.Is(err, ErrCommitmentNotFound) {
		t.Errorf("unknown commitment: %v", err)
	}
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["1"].ID}); !errors.Is(err, ErrDuplicateEntry) {
		t.Errorf("duplicate: %v", err)
	}
	// The cap (2) counts active individual entries only.
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["2"].ID}); !errors.Is(err, ErrMaxEntriesExceeded) {
		t.Errorf("cap: %v", err)
	}
	// Relays skip both the cap and the eligibility check.
	relay, audit, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["4"].ID, IsRelay: true, SeedTimeMs: cov1xInt(5000)})
	if err != nil || !relay.IsRelay || relay.Status != "submitted" || relay.ProposedBy != "coach-1" || audit.Action != "entry.proposed" {
		t.Fatalf("relay = %+v audit=%+v err=%v", relay, audit, err)
	}

	// Scratching frees a slot and the scratched row is revived in place.
	scratched, sAudit, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, e5.ID, " sick ")
	if err != nil || scratched.Status != "scratched" || scratched.ScratchReason != "sick" || sAudit.Action != "entry.scratched" {
		t.Fatalf("scratch = %+v err=%v", scratched, err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, e5.ID, ""); !errors.Is(err, ErrEntryAlreadyScratched) {
		t.Errorf("double scratch: %v", err)
	}
	revived, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["5"].ID})
	if err != nil || revived.ID != e5.ID || revived.Status != "submitted" {
		t.Fatalf("revive = %+v err=%v", revived, err)
	}
	// Freed slot: the duplicate check still applies to an event the swimmer already has.
	if _, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["4"].ID, OverrideEligibility: true, OverrideReason: " bona fide exception ", CommitmentID: "  "}); !errors.Is(err, ErrDuplicateEntry) {
		t.Fatalf("duplicate after relay: %v", err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, e5.ID, "swap"); err != nil {
		t.Fatal(err)
	}
	filled, _, err := propose(ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["2"].ID})
	if err != nil || filled.MeetEventID != evs["2"].ID {
		t.Fatalf("slot freed by scratch: %+v err=%v", filled, err)
	}
	// A swimmer the lookup cannot resolve is not eligibility-checked.
	unknown, _, err := e.svc.ProposeEntry(e.coach, cov1xOrg, closed.ID, ProposeEntryInput{SwimmerID: "sw-9", MeetEventID: evs["4"].ID})
	if err != nil || unknown.SwimmerID != "sw-9" {
		t.Fatalf("unresolvable swimmer: %+v err=%v", unknown, err)
	}
	// The override branch: a resolvable girl entered in a boys' event.
	e.svc.SetSwimmerEligibilityLookup(func(org, id string) (SwimmerEligibilityView, bool) {
		return SwimmerEligibilityView{Gender: "female", DateOfBirth: "2016-03-01"}, true
	})
	ov, ovAudit, err := e.svc.ProposeEntry(e.coach, cov1xOrg, closed.ID, ProposeEntryInput{SwimmerID: "sw-8", MeetEventID: evs["4"].ID, OverrideEligibility: true, OverrideReason: " bona fide "})
	if err != nil {
		t.Fatal(err)
	}
	if ovAudit.Metadata["eligibility_override"] != true || ovAudit.Metadata["eligibility_override_reason"] != "bona fide" || ov.ID == "" {
		t.Fatalf("override audit = %+v", ovAudit.Metadata)
	}
	// Approving.
	if _, _, err := e.svc.ApproveEntry(e.admin, cov1xOrg, "missing", e1.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("approve missing meet: %v", err)
	}
	if _, _, err := e.svc.ApproveEntry(e.admin, cov1xOrg, closed.ID, "nope"); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("approve missing entry: %v", err)
	}
	if _, _, err := e.svc.ApproveEntry(e.admin, cov1xOrg, closed.ID, scratched.ID); !errors.Is(err, ErrEntryNotEditable) {
		// scratched.ID was revived then scratched again above, so it is scratched now
		t.Errorf("approve scratched: %v", err)
	}
	appr, aAudit, err := e.svc.ApproveEntry(e.coach, cov1xOrg, closed.ID, e1.ID)
	if err != nil || appr.Status != "approved" || appr.ApprovedBy != "coach-1" || aAudit.Action != "entry.approved" {
		t.Fatalf("approve = %+v err=%v", appr, err)
	}
	if _, _, err := e.svc.ApproveEntry(e.coach, cov1xOrg, closed.ID, e1.ID); !errors.Is(err, ErrEntryNotEditable) {
		t.Errorf("double approve: %v", err)
	}
	// Exported entries can no longer be scratched.
	if n, _, err := e.svc.MarkEntriesExported(e.admin, cov1xOrg, closed.ID, []string{e1.ID}); err != nil || n != 1 {
		t.Fatalf("export n=%d err=%v", n, err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, e1.ID, "x"); !errors.Is(err, ErrEntryNotEditable) {
		t.Errorf("scratch exported: %v", err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, "missing", e1.ID, "x"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("scratch missing meet: %v", err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, "nope", "x"); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("scratch missing entry: %v", err)
	}
}

func TestCov1xOverrideEntry(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	closed, evs, entries := e.closedMeetWithEntries(t)
	var target Entry
	for _, en := range entries {
		if en.MeetEventID == evs["1"].ID {
			target = en
		}
	}
	ov := func(entryID string, in OverrideEntryInput) (Entry, MeetAuditEvent, error) {
		return e.svc.OverrideEntry(e.coach, cov1xOrg, closed.ID, entryID, in)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "r", SeedTimeMs: cov1xInt(0)}); !errors.Is(err, ErrValidation) {
		t.Errorf("seed: %v", err)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{Reason: "r"}); err == nil || !strings.Contains(err.Error(), "new_meet_event_id is required") {
		t.Errorf("blank event: %v", err)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "  "}); err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Errorf("blank reason: %v", err)
	}
	if _, _, err := e.svc.OverrideEntry(e.coach, cov1xOrg, "missing", target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "r"}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("missing meet: %v", err)
	}
	if _, _, err := ov("nope", OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "r"}); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("missing entry: %v", err)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: "nope", Reason: "r"}); !errors.Is(err, ErrEventNotFound) {
		t.Errorf("unknown event: %v", err)
	}
	// Moving onto an event the swimmer already has is a duplicate.
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["5"].ID, Reason: "r"}); !errors.Is(err, ErrDuplicateEntry) {
		t.Errorf("duplicate: %v", err)
	}
	// Eligibility is re-checked against the new event, except for relays.
	e.svc.SetSwimmerEligibilityLookup(func(org, id string) (SwimmerEligibilityView, bool) {
		return SwimmerEligibilityView{Gender: "female", DateOfBirth: "2016-03-01"}, true
	})
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["4"].ID, Reason: "r"}); !errors.Is(err, ErrIneligibleEntry) {
		t.Errorf("ineligible move: %v", err)
	}
	moved, audit, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: " fits better ", SeedTimeMs: cov1xInt(41000)})
	if err != nil {
		t.Fatal(err)
	}
	if moved.MeetEventID != evs["2"].ID || moved.OverrideReason != "fits better" || *moved.SeedTimeMs != 41000 || moved.UpdatedBy != "coach-1" {
		t.Fatalf("moved = %+v", moved)
	}
	if audit.Action != "entry.coach_override" || audit.Before["meet_event_id"] != evs["1"].ID || audit.After["meet_event_id"] != evs["2"].ID ||
		audit.Metadata["old_meet_event_id"] != evs["1"].ID || audit.Metadata["reason"] != "fits better" {
		t.Fatalf("audit = %+v", audit)
	}
	// Without a new seed time the existing one is kept.
	kept, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["1"].ID, Reason: "back"})
	if err != nil || *kept.SeedTimeMs != 41000 {
		t.Fatalf("kept = %+v err=%v", kept, err)
	}
	// An approved entry is no longer editable.
	if _, _, err := e.svc.ApproveEntry(e.admin, cov1xOrg, closed.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "r"}); !errors.Is(err, ErrEntryNotEditable) {
		t.Errorf("approved: %v", err)
	}
	// Overrides and scratches stop once entries are finalized.
	if _, _, err := e.svc.TransitionMeetStatus(e.admin, cov1xOrg, closed.ID, "entry_finalized"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ov(target.ID, OverrideEntryInput{NewMeetEventID: evs["2"].ID, Reason: "r"}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("finalized override: %v", err)
	}
	if _, _, err := e.svc.ScratchEntry(e.coach, cov1xOrg, closed.ID, target.ID, "x"); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("finalized scratch: %v", err)
	}
}

func TestCov1xOverrideRelayEntrySkipsEligibility(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	closed, evs, _ := e.closedMeetWithEntries(t)
	e.svc.SetSwimmerEligibilityLookup(func(org, id string) (SwimmerEligibilityView, bool) {
		return SwimmerEligibilityView{Gender: "female"}, true
	})
	relay, _, err := e.svc.ProposeEntry(e.admin, cov1xOrg, closed.ID, ProposeEntryInput{SwimmerID: "sw-1", MeetEventID: evs["3"].ID, IsRelay: true})
	if err != nil {
		t.Fatal(err)
	}
	moved, _, err := e.svc.OverrideEntry(e.admin, cov1xOrg, closed.ID, relay.ID, OverrideEntryInput{NewMeetEventID: evs["4"].ID, Reason: "relay leg"})
	if err != nil || moved.MeetEventID != evs["4"].ID {
		t.Fatalf("relay override = %+v err=%v", moved, err)
	}
}

func TestCov1xMarkEntriesExportedSkipsForeignAndUnapproved(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	closed, _, entries := e.closedMeetWithEntries(t)
	other := e.createMeet(t)
	e.svc.mu.Lock()
	e.svc.entries["foreign"] = Entry{ID: "foreign", OrgID: cov1xOrg, MeetID: other.ID, Status: "approved"}
	e.svc.mu.Unlock()
	if _, _, err := e.svc.ApproveEntry(e.admin, cov1xOrg, closed.ID, entries[0].ID); err != nil {
		t.Fatal(err)
	}
	n, audit, err := e.svc.MarkEntriesExported(e.admin, cov1xOrg, closed.ID, []string{" " + entries[0].ID + " ", entries[1].ID, "foreign", "ghost"})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if audit.After["exported_count"] != 1 || audit.After["entry_count"] != 4 || audit.Metadata["format"] != "entry_bundle" {
		t.Fatalf("audit = %+v", audit)
	}
	if e.svc.entries["foreign"].Status != "approved" {
		t.Fatal("entries of another meet must not be exported")
	}
}

func TestCov1xSeedTimeBackfill(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	s := e.svc
	id := s.SeedEntry(cov1xOrg, "sw-1", "m-1", "freestyle", "SCY", 100, cov1xInt(70000))
	// Entries that differ by distance, stroke, course, status, swimmer or org are untouched.
	skipDist := s.SeedEntry(cov1xOrg, "sw-1", "m-1", "freestyle", "SCY", 200, cov1xInt(1))
	skipStroke := s.SeedEntry(cov1xOrg, "sw-1", "m-1", "backstroke", "SCY", 100, cov1xInt(2))
	skipCourse := s.SeedEntry(cov1xOrg, "sw-1", "m-1", "freestyle", "LCM", 100, cov1xInt(3))
	skipStatus := s.SeedEntry(cov1xOrg, "sw-1", "m-1", "freestyle", "SCY", 100, cov1xInt(4))
	s.SetEntryStatusForTest(skipStatus, "approved")
	skipSwimmer := s.SeedEntry(cov1xOrg, "sw-2", "m-1", "freestyle", "SCY", 100, cov1xInt(5))
	skipOrg := s.SeedEntry("org-b", "sw-1", "m-2", "freestyle", "SCY", 100, cov1xInt(6))
	s.SetEntryStatusForTest("missing-entry", "approved") // no-op
	s.mu.Lock()
	s.entries["orphan"] = Entry{ID: "orphan", OrgID: cov1xOrg, SwimmerID: "sw-1", MeetEventID: "ghost", Status: "draft"}
	s.mu.Unlock()

	if got := s.UpdateSeedTimesForSwimmer(cov1xOrg, "sw-1", 100, "free", "SCY", cov1xInt(65000)); got != 1 {
		t.Fatalf("updated = %d, want 1", got)
	}
	if v, ok := s.GetEntrySeedTime(id); !ok || *v != 65000 {
		t.Fatalf("seed = %v ok=%v", v, ok)
	}
	for name, eid := range map[string]string{"dist": skipDist, "stroke": skipStroke, "course": skipCourse, "status": skipStatus, "swimmer": skipSwimmer, "org": skipOrg} {
		if v, _ := s.GetEntrySeedTime(eid); v == nil || *v == 65000 {
			t.Errorf("%s entry was back-filled: %v", name, v)
		}
	}
	if v, ok := s.GetEntrySeedTime("nope"); ok || v != nil {
		t.Fatalf("unknown entry = %v, %v", v, ok)
	}
	// A nil best time clears the seed.
	if got := s.UpdateSeedTimesForSwimmer(cov1xOrg, "sw-1", 100, "free", "SCY", nil); got != 1 {
		t.Fatalf("clear updated = %d", got)
	}
	if v, _ := s.GetEntrySeedTime(id); v != nil {
		t.Fatalf("seed should be cleared, got %v", *v)
	}
	if snap := s.SnapshotMeets(cov1xOrg); len(snap) != 1 || snap[0].ID != "m-1" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestCov1xHouseholdOwnershipChecks(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	s := e.svc
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.principalOwnsHouseholdLocked(nil, "hh-1") {
		t.Error("nil principal owns nothing")
	}
	if !s.principalOwnsHouseholdLocked(&appmiddleware.Principal{HouseholdID: " hh-1 "}, "hh-1") {
		t.Error("principal household (trimmed) should match")
	}
	if !s.principalOwnsHouseholdLocked(e.parent, "hh-1") || s.principalOwnsHouseholdLocked(e.parent, "hh-2") {
		t.Error("index-linked parent owns hh-1 only")
	}
	if s.parentOwnsHouseholdLocked("stranger", "hh-1") {
		t.Error("unknown parent owns nothing")
	}
}

func TestCov1xAuditTrailIsBounded(t *testing.T) {
	t.Parallel()
	e := cov1xNewEnv()
	s := e.svc
	s.mu.Lock()
	var last MeetAuditEvent
	for i := 0; i < maxInMemoryAuditEvents+25; i++ {
		last = s.recordAuditEventLocked(MeetAuditEvent{OrgID: cov1xOrg, Action: fmt.Sprintf("a%d", i)})
	}
	n, first := len(s.auditEvents), s.auditEvents[0].Action
	s.mu.Unlock()
	if n != maxInMemoryAuditEvents || first != "a25" || !strings.HasPrefix(last.ID, "aud_") || last.CreatedAt.IsZero() {
		t.Fatalf("len=%d first=%s last=%+v", n, first, last)
	}
}
