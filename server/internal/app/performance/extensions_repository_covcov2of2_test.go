package performance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov2xIntPtr(v int) *int { return &v }

func TestCov2xNullableHelpers(t *testing.T) {
	if got := nullableDate(""); got != nil {
		t.Fatalf("empty date must be NULL, got %v", got)
	}
	if got := nullableDate("2026-03-01"); got != "2026-03-01" {
		t.Fatalf("date passthrough, got %v", got)
	}
	if got := nullableInt(nil); got != nil {
		t.Fatalf("nil int must be NULL, got %v", got)
	}
	if got := nullableInt(cov2xIntPtr(7)); got != 7 {
		t.Fatalf("int passthrough, got %v", got)
	}
}

type cov2xDB struct {
	pool    *pgxpool.Pool
	repo    *Repository
	org     string
	swimmer string
	meet    string
}

func cov2xNewDB(t *testing.T) cov2xDB {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	d := cov2xDB{pool: pool, repo: newRepository(pool), org: uuid.NewString(), swimmer: uuid.NewString(), meet: uuid.NewString()}
	hh := uuid.NewString()
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Cov2x')`, []any{hh, d.org}},
		{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'Cov', 'Swimmer', '2013-01-01', 'female')`, []any{d.swimmer, d.org, hh}},
		{`INSERT INTO meets (id, org_id, name, meet_type, course, status) VALUES ($1::uuid, $2::uuid, 'Cov2x Meet', 'invitational', 'SCY', 'draft')`, []any{d.meet, d.org}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return d
}

func TestCov2xRecordEntryRepositoryRoundTrip(t *testing.T) {
	d := cov2xNewDB(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	verifiedAt := now.Add(time.Hour)
	manual := &RecordEntry{
		ID: uuid.NewString(), OrgID: d.org, Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free",
		TimeMs: 50000, SwimmerName: "Legacy Swimmer", SwimDate: "2020-05-06", Source: "manual", Status: "accepted",
		CreatedAt: now, UpdatedAt: now, CreatedBy: "u1", UpdatedBy: "u1",
	}
	linked := &RecordEntry{
		ID: uuid.NewString(), OrgID: d.org, Course: "SCM", Gender: "F", AgeGroup: "11-12", Distance: 50, Stroke: "fly",
		TimeMs: 31000, SwimmerID: d.swimmer, SwimmerName: "Cov Swimmer", SwimmerAge: cov2xIntPtr(12), SwimDate: "2026-01-02",
		MeetID: d.meet, MeetName: "Cov2x Meet", Source: "manual", Status: "pending", Locked: true,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "u1", UpdatedBy: "u1",
	}
	for _, e := range []*RecordEntry{manual, linked} {
		if err := d.repo.UpsertRecordEntry(ctx, e); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	// Conflict path updates the mutable columns in place.
	linked.TimeMs = 30500
	linked.Status = "accepted"
	linked.Verified, linked.VerifiedBy, linked.VerifiedAt = true, "coach", &verifiedAt
	linked.Notes = "re-timed"
	linked.UpdatedAt = verifiedAt
	linked.UpdatedBy = "u2"
	if err := d.repo.UpsertRecordEntry(ctx, linked); err != nil {
		t.Fatal(err)
	}
	got, err := d.repo.LoadRecordEntries(ctx, d.org)
	if err != nil || len(got) != 2 {
		t.Fatalf("load: %v len=%d", err, len(got))
	}
	byID := map[string]*RecordEntry{}
	for _, e := range got {
		byID[e.ID] = e
	}
	m := byID[manual.ID]
	if m.SwimmerID != "" || m.MeetID != "" || m.ResultID != "" || m.SwimmerAge != nil || m.SwimDate != "2020-05-06" || m.Source != "manual" {
		t.Fatalf("manual entry NULL columns not normalised: %+v", m)
	}
	l := byID[linked.ID]
	if l.SwimmerID != d.swimmer || l.MeetID != d.meet || l.TimeMs != 30500 || l.Status != "accepted" || !l.Verified ||
		l.VerifiedBy != "coach" || l.VerifiedAt == nil || !l.VerifiedAt.Equal(verifiedAt) || l.Notes != "re-timed" ||
		!l.Locked || l.SwimmerAge == nil || *l.SwimmerAge != 12 || l.UpdatedBy != "u2" || l.CreatedBy != "u1" {
		t.Fatalf("linked entry not updated: %+v", l)
	}
	if err := d.repo.DeleteRecordEntry(ctx, d.org, manual.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.repo.LoadRecordEntries(ctx, d.org)
	if len(got) != 1 || got[0].ID != linked.ID {
		t.Fatalf("delete should leave only the linked entry: %+v", got)
	}
}

func TestCov2xGoalRepositoryRoundTrip(t *testing.T) {
	d := cov2xNewDB(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	withDeadline := &Goal{
		ID: uuid.NewString(), OrgID: d.org, SwimmerID: d.swimmer, Distance: 100, Stroke: "free", Course: "SCY",
		TargetTimeMs: 60000, BaselineTimeMs: cov2xIntPtr(65000), Deadline: "2026-03-01", Notes: "n", Status: "active",
		CreatedAt: now, UpdatedAt: now, CreatedBy: "u1", UpdatedBy: "u1",
	}
	open := &Goal{
		ID: uuid.NewString(), OrgID: d.org, SwimmerID: d.swimmer, Distance: 50, Stroke: "back", Course: "SCM",
		TargetTimeMs: 33000, Status: "active", CreatedAt: now, UpdatedAt: now, CreatedBy: "u1", UpdatedBy: "u1",
	}
	for _, g := range []*Goal{withDeadline, open} {
		if err := d.repo.UpsertGoal(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	withDeadline.TargetTimeMs, withDeadline.Status, withDeadline.Deadline, withDeadline.UpdatedBy = 59000, "archived", "", "u2"
	if err := d.repo.UpsertGoal(ctx, withDeadline); err != nil {
		t.Fatal(err)
	}
	got, err := d.repo.LoadGoals(ctx, d.org)
	if err != nil || len(got) != 2 {
		t.Fatalf("load: %v len=%d", err, len(got))
	}
	for _, g := range got {
		switch g.ID {
		case withDeadline.ID:
			if g.TargetTimeMs != 59000 || g.Status != "archived" || g.Deadline != "" || g.UpdatedBy != "u2" ||
				g.BaselineTimeMs == nil || *g.BaselineTimeMs != 65000 {
				t.Fatalf("goal update lost: %+v", g)
			}
		case open.ID:
			if g.BaselineTimeMs != nil || g.Deadline != "" {
				t.Fatalf("open goal should have NULL baseline/deadline: %+v", g)
			}
		default:
			t.Fatalf("unexpected goal %s", g.ID)
		}
	}
	// A deadline round-trips as YYYY-MM-DD.
	open.Deadline = "2026-09-30"
	if err := d.repo.UpsertGoal(ctx, open); err != nil {
		t.Fatal(err)
	}
	got, _ = d.repo.LoadGoals(ctx, d.org)
	for _, g := range got {
		if g.ID == open.ID && g.Deadline != "2026-09-30" {
			t.Fatalf("deadline = %q", g.Deadline)
		}
	}
	if err := d.repo.DeleteGoal(ctx, d.org, withDeadline.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.repo.LoadGoals(ctx, d.org)
	if len(got) != 1 || got[0].ID != open.ID {
		t.Fatalf("delete should leave only the open goal: %+v", got)
	}
}

func cov2xLineup(d cov2xDB) *RelayLineup {
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	return &RelayLineup{
		ID: uuid.NewString(), OrgID: d.org, MeetID: d.meet, Name: "A relay", Course: "SCY", Gender: "F", AgeGroup: "OPEN",
		RelayType: "medley", Distance: 200,
		Teams: []RelayTeam{{Label: "A", TotalMs: 120000, Legs: []RelayLeg{{Leg: 1, Stroke: "back", SwimmerID: d.swimmer, SwimmerName: "Cov Swimmer", TimeMs: 30000}}}},
		Notes: "x", CreatedAt: now, UpdatedAt: now, CreatedBy: "u1", UpdatedBy: "u1",
	}
}

func TestCov2xRelayLineupRepositoryRoundTrip(t *testing.T) {
	d := cov2xNewDB(t)
	ctx := context.Background()
	l := cov2xLineup(d)
	if err := d.repo.UpsertRelayLineup(ctx, l); err != nil {
		t.Fatal(err)
	}
	l.Name, l.Notes, l.UpdatedBy = "B relay", "changed", "u2"
	l.Teams = append(l.Teams, RelayTeam{Label: "B", TotalMs: 130000})
	if err := d.repo.UpsertRelayLineup(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := d.repo.LoadRelayLineups(ctx, d.org)
	if err != nil || len(got) != 1 {
		t.Fatalf("load: %v len=%d", err, len(got))
	}
	g := got[0]
	if g.Name != "B relay" || g.Notes != "changed" || g.UpdatedBy != "u2" || g.MeetID != d.meet || len(g.Teams) != 2 ||
		g.Teams[0].Legs[0].SwimmerID != d.swimmer || g.Teams[0].TotalMs != 120000 || g.Teams[1].Label != "B" {
		t.Fatalf("lineup round trip: %+v", g)
	}
	if err := d.repo.DeleteRelayLineup(ctx, d.org, l.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = d.repo.LoadRelayLineups(ctx, d.org); len(got) != 0 {
		t.Fatalf("lineup not deleted: %+v", got)
	}

	// Corrupt stored data is reported, not silently dropped.
	l = cov2xLineup(d)
	if err := d.repo.UpsertRelayLineup(ctx, l); err != nil {
		t.Fatal(err)
	}
	// Teams must be an array of teams; a JSON string decodes to a type error.
	if _, err := d.pool.Exec(ctx, `UPDATE relay_lineups SET teams = '"nope"'::jsonb WHERE id = $1`, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.repo.LoadRelayLineups(ctx, d.org); err == nil || !strings.Contains(err.Error(), "decode relay teams") {
		t.Fatalf("want decode error, got %v", err)
	}
	// A NULL notes value cannot scan into the string field.
	if _, err := d.pool.Exec(ctx, `ALTER TABLE relay_lineups ALTER COLUMN notes DROP NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, `UPDATE relay_lineups SET teams = '[]'::jsonb, notes = NULL WHERE id = $1`, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.repo.LoadRelayLineups(ctx, d.org); err == nil || !strings.Contains(err.Error(), "scan relay_lineup") {
		t.Fatalf("want scan error, got %v", err)
	}
}

func TestCov2xLoadFailuresAreWrappedAndStopExtensionsLoad(t *testing.T) {
	d := cov2xNewDB(t)
	ctx := context.Background()
	// Break the tables one at a time, last-loaded first, so loadExtensions
	// reaches (and reports) each failing table in turn.
	steps := []struct {
		table string
		load  func() error
	}{
		{"relay_lineups", func() error { _, err := d.repo.LoadRelayLineups(ctx, d.org); return err }},
		{"swimmer_goals", func() error { _, err := d.repo.LoadGoals(ctx, d.org); return err }},
		{"team_record_entries", func() error { _, err := d.repo.LoadRecordEntries(ctx, d.org); return err }},
	}
	if ext, err := d.repo.loadExtensions(ctx, d.org); err != nil || len(ext.entries)+len(ext.goals)+len(ext.lineups) != 0 {
		t.Fatalf("healthy empty club: %v %+v", err, ext)
	}
	for _, st := range steps {
		if _, err := d.pool.Exec(ctx, cov2xBreakTable(st.table)); err != nil {
			t.Fatal(err)
		}
		if err := st.load(); err == nil || !strings.Contains(err.Error(), "load "+st.table) {
			t.Fatalf("%s: want wrapped load error, got %v", st.table, err)
		}
		if _, err := d.repo.loadExtensions(ctx, d.org); err == nil || !strings.Contains(err.Error(), "load "+st.table) {
			t.Fatalf("%s: loadExtensions must stop at the failing table, got %v", st.table, err)
		}
	}
	// With the club unloadable, request paths surface the failure instead of serving empty data.
	svc := NewServiceWithPool(d.pool)
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: d.org, Roles: []string{"admin"}}
	if _, err := svc.ProgressReport(d.org, admin, cov2xIn()); err == nil {
		t.Fatal("ProgressReport must surface the club load failure")
	}
	if _, err := svc.TopTimes(d.org, admin, TopTimesQuery{}); err == nil {
		t.Fatal("TopTimes must surface the club load failure")
	}
}

// cov2xBreakTable shadows a table in the test's isolated schema with one that
// lacks every real column (the shared public copy stays untouched), so any
// query against it fails.
func cov2xBreakTable(table string) string {
	return `ALTER TABLE ` + table + ` RENAME TO ` + table + `_gone; CREATE TABLE ` + table + ` (unrelated INT)`
}
