package meets

import (
	"encoding/json"
	"reflect"
	"testing"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

func cov2xSnapshotFixture(t *testing.T) (*Service, *appmiddleware.Principal, *appmiddleware.Principal, Meet) {
	t.Helper()
	svc := NewService()
	admin := &appmiddleware.Principal{UserID: "admin-1", Roles: []string{"admin"}}
	parent := &appmiddleware.Principal{UserID: "parent-1", HouseholdID: "household-1", Roles: []string{"parent"}}

	svc.SeedSwimmer("org-a", "swimmer-1", "household-1")
	svc.SeedParentHousehold(parent.UserID, "household-1")

	meet, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Snapshot Invite", MeetType: "invitational", Course: "SCY", StartDate: "2026-07-10", EndDate: "2026-07-10",
		Sessions: []CreateMeetSessionInput{{SessionNumber: 1, Name: "Morning"}},
		Events:   []CreateMeetEventDraftInput{{SessionNumber: 1, EventNumber: "1", Stroke: "freestyle", Distance: 100, Course: "SCY", Gender: "F"}},
	})
	if err != nil {
		t.Fatalf("CreateMeet() error = %v", err)
	}
	if _, _, err := svc.TransitionMeetStatus(admin, "org-a", meet.ID, "published"); err != nil {
		t.Fatalf("TransitionMeetStatus() error = %v", err)
	}
	meet, _, _, err = svc.PublishCommitmentRequest(admin, "org-a", meet.ID)
	if err != nil {
		t.Fatalf("PublishCommitmentRequest() error = %v", err)
	}
	return svc, admin, parent, meet
}

func TestCov2xSnapshotRoundTripPreservesState(t *testing.T) {
	t.Parallel()
	src, admin, parent, meet := cov2xSnapshotFixture(t)

	data, err := src.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot() error = %v", err)
	}
	var decoded runtimeSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}
	if len(decoded.Meets) != 1 || len(decoded.Commitments) != 1 || len(decoded.Events) != 1 || len(decoded.Sessions) != 1 {
		t.Fatalf("snapshot contents = %+v, want one meet/session/event/commitment", decoded)
	}
	if len(decoded.AuditEvents) == 0 {
		t.Fatalf("snapshot audit events empty, want recorded audit trail")
	}
	if _, ok := decoded.ParentHousehold["parent-1"]["household-1"]; !ok {
		t.Fatalf("snapshot parent_household = %+v, want parent-1 -> household-1", decoded.ParentHousehold)
	}
	if decoded.Swimmers["swimmer-1"].HouseholdID != "household-1" {
		t.Fatalf("snapshot swimmers = %+v, want swimmer-1 in household-1", decoded.Swimmers)
	}

	dst := NewService()
	if err := dst.LoadRuntimeSnapshot(data); err != nil {
		t.Fatalf("LoadRuntimeSnapshot() error = %v", err)
	}

	gotMeet, err := dst.GetMeet(admin, "org-a", meet.ID)
	if err != nil {
		t.Fatalf("GetMeet() after load error = %v", err)
	}
	if gotMeet.Name != "Snapshot Invite" || gotMeet.Status != "commitment_open" {
		t.Fatalf("loaded meet = %+v, want Snapshot Invite in commitment_open", gotMeet)
	}
	srcEvents, _ := src.ListMeetEvents(admin, "org-a", meet.ID)
	dstEvents, err := dst.ListMeetEvents(admin, "org-a", meet.ID)
	if err != nil || !reflect.DeepEqual(srcEvents, dstEvents) {
		t.Fatalf("loaded events = %+v err=%v, want %+v", dstEvents, err, srcEvents)
	}
	srcCommits, _ := src.ListCommitments(admin, "org-a", meet.ID)
	dstCommits, err := dst.ListCommitments(admin, "org-a", meet.ID)
	if err != nil || !reflect.DeepEqual(srcCommits, dstCommits) {
		t.Fatalf("loaded commitments = %+v err=%v, want %+v", dstCommits, err, srcCommits)
	}
	// Parent household scope survives the round trip.
	parentCommits, err := dst.ListCommitments(parent, "org-a", meet.ID)
	if err != nil || len(parentCommits) != 1 {
		t.Fatalf("parent ListCommitments() after load = %+v err=%v, want 1", parentCommits, err)
	}

	// Re-exporting the loaded service yields an equivalent snapshot.
	again, err := dst.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("second export error = %v", err)
	}
	var a, b runtimeSnapshot
	if err := json.Unmarshal(again, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("re-export differs from original snapshot")
	}
}

func TestCov2xSnapshotExportIsIsolatedFromLaterMutation(t *testing.T) {
	t.Parallel()
	svc, admin, _, meet := cov2xSnapshotFixture(t)

	data, err := svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot() error = %v", err)
	}
	if _, _, err := svc.CreateMeet(admin, "org-a", CreateMeetInput{
		Name: "Later Meet", MeetType: "invitational", Course: "SCY", StartDate: "2026-08-10", EndDate: "2026-08-10",
	}); err != nil {
		t.Fatalf("CreateMeet() error = %v", err)
	}
	var snap runtimeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Meets) != 1 || snap.Meets[meet.ID].Status != "commitment_open" {
		t.Fatalf("snapshot meets = %+v, want only the original meet (point-in-time)", snap.Meets)
	}
}

func TestCov2xLoadSnapshotEmptyDataIsNoop(t *testing.T) {
	t.Parallel()
	svc, admin, _, meet := cov2xSnapshotFixture(t)

	if err := svc.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("LoadRuntimeSnapshot(nil) error = %v, want nil", err)
	}
	if err := svc.LoadRuntimeSnapshot([]byte{}); err != nil {
		t.Fatalf("LoadRuntimeSnapshot(empty) error = %v, want nil", err)
	}
	if _, err := svc.GetMeet(admin, "org-a", meet.ID); err != nil {
		t.Fatalf("meet lost after empty load: %v", err)
	}
}

func TestCov2xLoadSnapshotInvalidJSONKeepsState(t *testing.T) {
	t.Parallel()
	svc, admin, _, meet := cov2xSnapshotFixture(t)

	if err := svc.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatalf("LoadRuntimeSnapshot(invalid) error = nil, want decode error")
	}
	if err := svc.LoadRuntimeSnapshot([]byte(`{"meets": 5}`)); err == nil {
		t.Fatalf("LoadRuntimeSnapshot(wrong type) error = nil, want decode error")
	}
	if _, err := svc.GetMeet(admin, "org-a", meet.ID); err != nil {
		t.Fatalf("state changed by failed load: %v", err)
	}
}

func TestCov2xLoadSnapshotReplacesExistingState(t *testing.T) {
	t.Parallel()
	svc, admin, _, meet := cov2xSnapshotFixture(t)

	// An empty-object snapshot replaces all state with empty maps.
	if err := svc.LoadRuntimeSnapshot([]byte(`{}`)); err != nil {
		t.Fatalf("LoadRuntimeSnapshot({}) error = %v", err)
	}
	if _, err := svc.GetMeet(admin, "org-a", meet.ID); err == nil {
		t.Fatalf("GetMeet() after replacing with empty snapshot succeeded, want not found")
	}
	metas, err := svc.ListMeets(admin, "org-a")
	if err != nil || len(metas) != 0 {
		t.Fatalf("ListMeets() = %+v err=%v, want empty", metas, err)
	}
}
