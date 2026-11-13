package training

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

const cov1xBoomMsg = "cov1x boom"

func cov1xExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// cov1xSwimmer inserts a household and swimmer and returns the swimmer id.
func cov1xSwimmer(t *testing.T, pool *pgxpool.Pool, org string) string {
	t.Helper()
	hh, sw := uuid.NewString(), uuid.NewString()
	cov1xExec(t, pool, `INSERT INTO households (id, org_id, household_name) VALUES ($1::uuid, $2::uuid, 'Cov Family')`, hh, org)
	cov1xExec(t, pool, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'Cov', 'Swimmer', '2012-01-01', 'female', 'active')`, sw, org, hh)
	return sw
}

func cov1xSeason(t *testing.T, pool *pgxpool.Pool, org string) string {
	t.Helper()
	id := uuid.NewString()
	cov1xExec(t, pool, `INSERT INTO seasons (id, org_id, name, start_date, end_date, status) VALUES ($1::uuid, $2, 'Cov Season', '2026-01-01', '2026-06-30', 'active')`, id, org)
	return id
}

// cov1xFailOn makes every INSERT/UPDATE/DELETE statement on table fail until the returned undo runs.
func cov1xFailOn(t *testing.T, pool *pgxpool.Pool, table, event string) (undo func()) {
	t.Helper()
	cov1xExec(t, pool, `CREATE OR REPLACE FUNCTION cov1x_boom() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x boom'; END $$`)
	name := fmt.Sprintf("cov1x_fail_%s_%s", table, strings.ToLower(event))
	cov1xExec(t, pool, fmt.Sprintf(`CREATE TRIGGER %s BEFORE %s ON %s FOR EACH STATEMENT EXECUTE FUNCTION cov1x_boom()`, name, event, table))
	undone := false
	undo = func() {
		if undone {
			return
		}
		undone = true
		cov1xExec(t, pool, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, name, table))
	}
	t.Cleanup(undo)
	return undo
}

func cov1xDeadPool(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	dead, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	return dead
}

func cov1xWantBoom(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), cov1xBoomMsg) {
		t.Fatalf("%s: err = %v, want the injected %q failure", what, err, cov1xBoomMsg)
	}
}

func cov1xWorkoutExists(t *testing.T, svc *Service, org, id string) bool {
	t.Helper()
	_, err := svc.GetWorkout(org, id)
	if err != nil && !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("GetWorkout(%s): %v", id, err)
	}
	return err == nil
}

func TestCov1xDBPracticeCRUDAndAttendance(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org, other := uuid.NewString(), uuid.NewString()

	empty, err := svc.List(org)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v err=%v, want non-nil empty", empty, err)
	}

	p, err := svc.Create(org, "2026-11-02", "G1", "water", "Warm", 3000, nil, PracticeStrokes{Free: 3000}, "n", "coach")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.List(org)
	if err != nil || len(listed) != 1 || listed[0].ID != p.ID || listed[0].Strokes.Free != 3000 {
		t.Fatalf("list = %+v err=%v", listed, err)
	}
	if _, err := svc.Get(org, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get err = %v", err)
	}
	if _, err := svc.Get(other, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if _, err := svc.Get(org, "not-a-uuid"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id err = %v, want a database error rather than not-found", err)
	}

	// Patch every field in one update.
	d, g, ty, desc, notes := "2026-11-09", "G2", "dryland", "Core", "bring bands"
	y, att := 1500, 12
	str := PracticeStrokes{Free: 1, Back: 2, Breast: 3, Fly: 4, IM: 5}
	upd, err := svc.Update(org, p.ID, PracticeUpdates{Date: &d, GroupID: &g, Type: &ty, Description: &desc,
		YardageTarget: &y, Attendance: &att, Strokes: &str, Notes: &notes}, "coach2")
	if err != nil {
		t.Fatal(err)
	}
	if upd.Date != d || upd.GroupID != g || upd.Type != ty || upd.Description != desc || upd.YardageTarget != y ||
		upd.Attendance == nil || *upd.Attendance != att || upd.Strokes != str || upd.Notes != notes ||
		upd.UpdatedBy != "coach2" || upd.CreatedBy != "coach" {
		t.Fatalf("updated practice = %+v", upd)
	}
	bad := "swim-meet"
	if _, err := svc.Update(org, p.ID, PracticeUpdates{Type: &bad}, "coach"); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("invalid type err = %v", err)
	}
	if got, _ := svc.Get(org, p.ID); got.Type != ty {
		t.Fatalf("type changed by rejected update: %q", got.Type)
	}
	if _, err := svc.Update(org, uuid.NewString(), PracticeUpdates{Notes: &notes}, "coach"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing err = %v", err)
	}

	// Attendance upserts per swimmer.
	sw := cov1xSwimmer(t, pool, org)
	first, err := svc.SetAttendance(org, p.ID, sw, "present", "coach")
	if err != nil || first.Status != "present" {
		t.Fatalf("first mark = %+v err=%v", first, err)
	}
	second, err := svc.SetAttendance(org, p.ID, sw, "absent", "coach2")
	if err != nil || second.ID != first.ID || second.Status != "absent" || second.MarkedBy != "coach2" {
		t.Fatalf("second mark = %+v err=%v", second, err)
	}
	recs, err := svc.ListAttendance(org, p.ID)
	if err != nil || len(recs) != 1 || recs[0].Status != "absent" {
		t.Fatalf("attendance = %+v err=%v", recs, err)
	}
	byPractice, err := svc.AttendanceForSwimmer(org, sw, []string{p.ID, uuid.NewString()})
	if err != nil || !reflect.DeepEqual(byPractice, map[string]string{p.ID: "absent"}) {
		t.Fatalf("attendance for swimmer = %v err=%v", byPractice, err)
	}
	if got, err := svc.AttendanceForSwimmer(org, sw, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no practices = %v err=%v", got, err)
	}
	if got, err := svc.attendanceForPractices(org, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no practice ids = %v err=%v", got, err)
	}
	if got, err := svc.attendanceForPractices(org, []string{p.ID}); err != nil || len(got) != 1 {
		t.Fatalf("attendanceForPractices = %v err=%v", got, err)
	}
	if _, err := svc.SetAttendance(org, p.ID, uuid.NewString(), "present", "coach"); err == nil {
		t.Fatal("marking an unknown swimmer must fail the foreign key")
	}
	if _, err := svc.SetAttendance(org, uuid.NewString(), sw, "present", "coach"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mark on missing practice err = %v", err)
	}
	if _, err := svc.SetAttendance(org, p.ID, sw, "late", "coach"); !errors.Is(err, ErrInvalidAttendance) {
		t.Fatalf("invalid status err = %v", err)
	}
	if _, err := svc.ListAttendance(org, uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list attendance on missing practice err = %v", err)
	}
	if _, err := svc.repo.ListAttendanceForPractice(context.Background(), org, "not-a-uuid"); err == nil {
		t.Fatal("malformed practice id must fail the query")
	}
	if _, err := svc.repo.AttendanceForSwimmer(context.Background(), org, "not-a-uuid", []string{p.ID}); err == nil {
		t.Fatal("malformed swimmer id must fail the query")
	}
	if _, err := svc.repo.AttendanceForPractices(context.Background(), org, []string{"not-a-uuid"}); err == nil {
		t.Fatal("malformed practice id must fail the query")
	}

	// Deleting a practice removes its own workout instance but keeps library templates.
	tmpl, err := svc.CreateWorkout(org, mainSetInput("Template", "SCY"), "coach")
	if err != nil {
		t.Fatal(err)
	}
	assigned, inst, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{Date: "2026-11-10"}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(other, assigned.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org delete err = %v", err)
	}
	if err := svc.Delete(org, assigned.ID); err != nil {
		t.Fatal(err)
	}
	if cov1xWorkoutExists(t, svc, org, inst.ID) {
		t.Fatal("instance should be deleted with its practice")
	}
	if !cov1xWorkoutExists(t, svc, org, tmpl.ID) {
		t.Fatal("template must survive practice deletion")
	}
	if err := svc.Delete(org, assigned.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
	if err := svc.Delete(org, "not-a-uuid"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id delete err = %v", err)
	}

	// The repository-level delete is org scoped and reports a miss.
	p3, err := svc.Create(org, "2026-11-11", "G1", "water", "", 0, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.repo.Delete(ctx, other, p3.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org repo delete err = %v", err)
	}
	if err := svc.repo.Delete(ctx, org, p3.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Delete(ctx, org, p3.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeat repo delete err = %v", err)
	}
}

func TestCov1xDBWorkoutLibraryAndAssignment(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org, other := uuid.NewString(), uuid.NewString()

	tmpl, err := svc.CreateWorkout(org, mainSetInput("Threshold Tuesday", "SCY"), "coach")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetWorkout(org, tmpl.ID)
	if err != nil || got.Title != "Threshold Tuesday" || got.Totals.Distance != 2400 || !reflect.DeepEqual(got.Tags, []string{"senior", "threshold"}) {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	if _, err := svc.GetWorkout(other, tmpl.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if _, err := svc.GetWorkout(org, "not-a-uuid"); err == nil || errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("malformed id err = %v", err)
	}
	if _, err := svc.CreateWorkout(org, WorkoutInput{}, "coach"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("invalid create err = %v", err)
	}

	pct := mainSetInput("100%_done", "LCM")
	pct.Tags = []string{"sprint"}
	if _, err := svc.CreateWorkout(org, pct, "coach"); err != nil {
		t.Fatal(err)
	}
	plain := mainSetInput("1000xdone", "SCY")
	plain.Description = "kicking focus"
	plain.Tags = nil
	if _, err := svc.CreateWorkout(org, plain, "coach"); err != nil {
		t.Fatal(err)
	}
	titles := func(f WorkoutFilter) []string {
		ws, err := svc.ListWorkouts(org, f)
		if err != nil {
			t.Fatalf("ListWorkouts(%+v): %v", f, err)
		}
		var out []string
		for _, w := range ws {
			out = append(out, w.Title)
		}
		return out
	}
	if all := titles(WorkoutFilter{}); len(all) != 3 {
		t.Fatalf("all = %v", all)
	}
	// LIKE wildcards in the query are literal, not patterns.
	if got := titles(WorkoutFilter{Query: "0%_"}); !reflect.DeepEqual(got, []string{"100%_done"}) {
		t.Fatalf("wildcard search = %v", got)
	}
	if got := titles(WorkoutFilter{Query: "KICKING"}); !reflect.DeepEqual(got, []string{"1000xdone"}) {
		t.Fatalf("description search = %v", got)
	}
	if got := titles(WorkoutFilter{Query: "senio"}); !reflect.DeepEqual(got, []string{"Threshold Tuesday"}) {
		t.Fatalf("tag-substring search = %v", got)
	}
	if got := titles(WorkoutFilter{Tag: " SPRINT "}); !reflect.DeepEqual(got, []string{"100%_done"}) {
		t.Fatalf("tag filter = %v", got)
	}
	if got := titles(WorkoutFilter{TemplatesOnly: true, Tag: "threshold", Query: "tuesday"}); !reflect.DeepEqual(got, []string{"Threshold Tuesday"}) {
		t.Fatalf("combined filter = %v", got)
	}
	if none, err := svc.ListWorkouts(other, WorkoutFilter{}); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("other org list = %#v err=%v", none, err)
	}
	if got := escapeLike(`a_b%c\`); got != `a\_b\%c\\` {
		t.Fatalf("escapeLike = %q", got)
	}

	// Assigning to a new date copies the template into a practice-owned instance.
	practice, inst, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{Date: "2026-11-03", GroupID: "G"}, "coach")
	if err != nil || inst.IsTemplate || inst.SourceWorkoutID != tmpl.ID || practice.WorkoutID != inst.ID || practice.YardageTarget != 2400 {
		t.Fatalf("assign = %+v %+v err=%v", practice, inst, err)
	}
	if _, _, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{PracticeID: uuid.NewString()}, "coach"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("assign to missing practice err = %v", err)
	}
	if _, _, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{Date: "2026-11-03", Type: "bogus"}, "coach"); !errors.Is(err, ErrInvalidType) {
		t.Fatalf("assign bad type err = %v", err)
	}

	// Editing the instance re-derives the linked practice inside the same transaction.
	edit := mainSetInput("Threshold Tuesday v2", "SCY")
	edit.Sections = edit.Sections[:1]
	updated, err := svc.UpdateWorkout(org, inst.ID, edit, "coach2")
	if err != nil || updated.IsTemplate || updated.Totals.Distance != 400 || updated.UpdatedBy != "coach2" {
		t.Fatalf("update = %+v err=%v", updated, err)
	}
	if p, _ := svc.Get(org, practice.ID); p.YardageTarget != 400 || p.UpdatedBy != "coach2" {
		t.Fatalf("practice after workout edit = %+v", p)
	}
	if _, err := svc.UpdateWorkout(org, uuid.NewString(), edit, "coach"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("update missing err = %v", err)
	}
	if _, err := svc.UpdateWorkout(org, inst.ID, WorkoutInput{}, "coach"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("update invalid err = %v", err)
	}

	// Saving the instance as a template, then re-assigning that template replaces the instance.
	yes := true
	saved, err := svc.DuplicateWorkout(org, inst.ID, DuplicateInput{IsTemplate: &yes}, "coach")
	if err != nil || !saved.IsTemplate || saved.SourceWorkoutID != inst.ID {
		t.Fatalf("save-as-template = %+v err=%v", saved, err)
	}
	_, inst2, err := svc.AssignWorkout(org, saved.ID, AssignInput{PracticeID: practice.ID}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if cov1xWorkoutExists(t, svc, org, inst.ID) {
		t.Fatal("previous instance should have been deleted on re-assign")
	}
	p, w, err := svc.GetPracticeWorkout(org, practice.ID)
	if err != nil || w.ID != inst2.ID || p.WorkoutID != inst2.ID {
		t.Fatalf("practice workout = %+v %+v err=%v", p, w, err)
	}

	// Replace the content in place; invalid content is refused.
	p, w, err = svc.SetPracticeWorkout(org, practice.ID, WorkoutInput{Sections: edit.Sections}, "coach")
	if err != nil || w.ID != inst2.ID || w.Title != "Threshold Tuesday" || p.YardageTarget != 400 {
		t.Fatalf("set = %+v %+v err=%v", p, w, err)
	}
	if _, _, err := svc.SetPracticeWorkout(org, practice.ID, WorkoutInput{Title: "x", Sections: []WorkoutSection{{Kind: "nope"}}}, "coach"); !errors.Is(err, ErrInvalidWorkout) {
		t.Fatalf("set invalid err = %v", err)
	}

	bare, err := svc.Create(org, "2026-11-04", "G", "water", "", 0, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	linkedMap, err := svc.WorkoutsForPractices(org, []*Practice{p, bare})
	if err != nil || len(linkedMap) != 1 || linkedMap[inst2.ID] == nil {
		t.Fatalf("WorkoutsForPractices = %+v err=%v", linkedMap, err)
	}
	if m, err := svc.WorkoutsForPractices(org, []*Practice{bare}); err != nil || m == nil || len(m) != 0 {
		t.Fatalf("no linked workouts = %#v err=%v", m, err)
	}

	cleared, err := svc.ClearPracticeWorkout(org, practice.ID, "coach")
	if err != nil || cleared.WorkoutID != "" || cleared.YardageTarget != 400 {
		t.Fatalf("clear = %+v err=%v", cleared, err)
	}
	if cov1xWorkoutExists(t, svc, org, inst2.ID) {
		t.Fatal("cleared instance should be deleted")
	}
	if _, err := svc.ClearPracticeWorkout(org, practice.ID, "coach"); !errors.Is(err, ErrPracticeNoWorkout) {
		t.Fatalf("second clear err = %v", err)
	}

	if err := svc.DeleteWorkout(org, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteWorkout(org, saved.ID); !errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("repeat delete err = %v", err)
	}
	if err := svc.DeleteWorkout(org, "not-a-uuid"); err == nil || errors.Is(err, ErrWorkoutNotFound) {
		t.Fatalf("malformed delete err = %v", err)
	}
	if _, err := svc.repo.WorkoutsByIDs(context.Background(), org, []string{"not-a-uuid"}); err == nil {
		t.Fatal("malformed workout id must fail the query")
	}
}

func TestCov1xDBPhasesAndSeasonPlan(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org, other := uuid.NewString(), uuid.NewString()
	seasonID := cov1xSeason(t, pool, org)
	season := SeasonWindow{ID: seasonID, StartDate: "2026-01-01", EndDate: "2026-06-30"}

	a, err := svc.CreatePhase(org, season, PhaseInput{GroupID: "G1", Phase: "base", Name: "Base", StartDate: "2026-01-05", EndDate: "2026-01-18", WeeklyYardageTarget: 20000, Focus: "aerobic"}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreatePhase(org, season, PhaseInput{GroupID: "G1", Phase: "build", StartDate: "2026-01-19", EndDate: "2026-01-25"}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePhase(org, season, PhaseInput{GroupID: "G1", Phase: "peak", StartDate: "2026-01-10", EndDate: "2026-01-20"}, "coach"); !errors.Is(err, ErrPhaseOverlap) {
		t.Fatalf("overlap err = %v", err)
	}
	// A different group may overlap in time.
	if _, err := svc.CreatePhase(org, season, PhaseInput{GroupID: "G2", Phase: "base", StartDate: "2026-01-10", EndDate: "2026-01-20"}, "coach"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetPhase(org, a.ID)
	if err != nil || got.Name != "Base" || got.WeeklyYardageTarget != 20000 || got.StartDate != "2026-01-05" || got.Focus != "aerobic" {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	if _, err := svc.GetPhase(other, a.ID); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}
	if _, err := svc.GetPhase(org, "not-a-uuid"); err == nil || errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("malformed id err = %v", err)
	}

	upd, err := svc.UpdatePhase(org, b.ID, season, PhaseInput{GroupID: "G1", Phase: "peak", StartDate: "2026-01-20", EndDate: "2026-01-31", WeeklyYardageTarget: 15000}, "coach2")
	if err != nil || upd.Phase != "peak" || upd.UpdatedBy != "coach2" || upd.CreatedBy != "coach" {
		t.Fatalf("update = %+v err=%v", upd, err)
	}
	if _, err := svc.UpdatePhase(org, b.ID, season, PhaseInput{GroupID: "G1", Phase: "peak", StartDate: "2026-01-10", EndDate: "2026-01-31"}, "coach"); !errors.Is(err, ErrPhaseOverlap) {
		t.Fatalf("update overlap err = %v", err)
	}

	phases, err := svc.ListPhases(org, seasonID)
	if err != nil || len(phases) != 3 || phases[0].GroupID != "G1" || phases[0].ID != a.ID || phases[2].GroupID != "G2" {
		t.Fatalf("phases = %+v err=%v", phases, err)
	}
	if none, err := svc.ListPhases(org, uuid.NewString()); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("other season = %#v err=%v", none, err)
	}
	if _, err := svc.ListPhases(org, "not-a-uuid"); err == nil {
		t.Fatal("malformed season id must fail the query")
	}

	if _, err := svc.Create(org, "2026-01-13", "G1", "water", "", 4000, nil, PracticeStrokes{}, "", "coach"); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.SeasonPlan(org, season, "G1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var wk PlanWeek
	for _, w := range plan.Weeks {
		if w.WeekStart == "2026-01-12" {
			wk = w
		}
	}
	if wk.Phase != "base" || wk.TargetYards != 20000 || wk.PlannedYards != 4000 || wk.Practices != 1 {
		t.Fatalf("week of 2026-01-12 = %+v (plan %+v)", wk, plan.Weeks)
	}

	if err := svc.DeletePhase(org, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePhase(org, a.ID); !errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("repeat delete err = %v", err)
	}
	if err := svc.DeletePhase(org, "not-a-uuid"); err == nil || errors.Is(err, ErrPhaseNotFound) {
		t.Fatalf("malformed delete err = %v", err)
	}
}

func TestCov1xDBAnalyticsMatchStoredData(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	sw1, sw2 := cov1xSwimmer(t, pool, org), cov1xSwimmer(t, pool, org)

	p1, err := svc.Create(org, "2026-11-02", "G1", "water", "", 3000, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(org, "2026-11-03", "G1", "rest", "", 9000, nil, PracticeStrokes{}, "", "coach"); err != nil {
		t.Fatal(err)
	}
	res := ParseQuickEntry("10x100 free @1:20 EN1")
	if _, _, err := svc.SetPracticeWorkout(org, p1.ID, WorkoutInput{Sections: res.Sections}, "coach"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetAttendance(org, p1.ID, sw1, "present", "coach"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetAttendance(org, p1.ID, sw2, "absent", "coach"); err != nil {
		t.Fatal(err)
	}

	out, err := svc.GroupAnalytics(org, "2026-11-02", "2026-11-15", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.TotalPractices != 1 || out.StructuredPractices != 1 || out.ZoneYards["EN1"] != 1000 || out.StrokeYards["free"] != 1000 {
		t.Fatalf("group analytics = %+v", out)
	}
	if len(out.Groups) != 1 || out.Groups[0].SwimmerYards != 1000 || out.Groups[0].Present != 1 || out.Groups[0].Absent != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	sa, err := svc.SwimmerAnalytics(org, sw1, map[string]bool{"G1": true}, "2026-11-02", "2026-11-15", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sa.Scheduled != 1 || sa.Yards != 1000 || sa.ZoneYards["EN1"] != 1000 || sa.AttendancePct == nil || *sa.AttendancePct != 100 {
		t.Fatalf("swimmer analytics = %+v", sa)
	}
}

func TestCov1xDBDeadPoolSurfacesErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(cov1xDeadPool(t, pool))
	org, id := uuid.NewString(), uuid.NewString()
	season := SeasonWindow{ID: id}
	notes := "n"
	phase := PhaseInput{GroupID: "g", Phase: "base", StartDate: "2026-01-05", EndDate: "2026-01-11"}
	linked := []*Practice{{ID: id, WorkoutID: uuid.NewString()}}

	calls := map[string]func() error{
		"Create": func() error {
			_, err := svc.Create(org, "2026-11-02", "g", "water", "", 1, nil, PracticeStrokes{}, "", "c")
			return err
		},
		"List":                 func() error { _, err := svc.List(org); return err },
		"Get":                  func() error { _, err := svc.Get(org, id); return err },
		"Update":               func() error { _, err := svc.Update(org, id, PracticeUpdates{Notes: &notes}, "c"); return err },
		"Delete":               func() error { return svc.Delete(org, id) },
		"SetAttendance":        func() error { _, err := svc.SetAttendance(org, id, id, "present", "c"); return err },
		"AttendanceForSwimmer": func() error { _, err := svc.AttendanceForSwimmer(org, id, []string{id}); return err },
		"ListAttendance":       func() error { _, err := svc.ListAttendance(org, id); return err },
		"CreateWorkout":        func() error { _, err := svc.CreateWorkout(org, cov1xSimpleWorkout("x"), "c"); return err },
		"GetWorkout":           func() error { _, err := svc.GetWorkout(org, id); return err },
		"ListWorkouts":         func() error { _, err := svc.ListWorkouts(org, WorkoutFilter{}); return err },
		"UpdateWorkout":        func() error { _, err := svc.UpdateWorkout(org, id, cov1xSimpleWorkout("x"), "c"); return err },
		"DeleteWorkout":        func() error { return svc.DeleteWorkout(org, id) },
		"DuplicateWorkout":     func() error { _, err := svc.DuplicateWorkout(org, id, DuplicateInput{}, "c"); return err },
		"AssignWorkout": func() error {
			_, _, err := svc.AssignWorkout(org, id, AssignInput{Date: "2026-11-02"}, "c")
			return err
		},
		"SetPracticeWorkout":     func() error { _, _, err := svc.SetPracticeWorkout(org, id, cov1xSimpleWorkout("x"), "c"); return err },
		"ClearPracticeWorkout":   func() error { _, err := svc.ClearPracticeWorkout(org, id, "c"); return err },
		"GetPracticeWorkout":     func() error { _, _, err := svc.GetPracticeWorkout(org, id); return err },
		"WorkoutsForPractices":   func() error { _, err := svc.WorkoutsForPractices(org, linked); return err },
		"attendanceForPractices": func() error { _, err := svc.attendanceForPractices(org, []string{id}); return err },
		"CreatePhase":            func() error { _, err := svc.CreatePhase(org, season, phase, "c"); return err },
		"GetPhase":               func() error { _, err := svc.GetPhase(org, id); return err },
		"UpdatePhase":            func() error { _, err := svc.UpdatePhase(org, id, season, phase, "c"); return err },
		"DeletePhase":            func() error { return svc.DeletePhase(org, id) },
		"ListPhases":             func() error { _, err := svc.ListPhases(org, id); return err },
		"SeasonPlan":             func() error { _, err := svc.SeasonPlan(org, season, "", nil); return err },
		"GroupAnalytics":         func() error { _, err := svc.GroupAnalytics(org, "2026-11-01", "2026-11-30", "", nil); return err },
		"SwimmerAnalytics": func() error {
			_, err := svc.SwimmerAnalytics(org, id, nil, "2026-11-01", "2026-11-30", nil)
			return err
		},
		"repo.Delete": func() error { return svc.repo.Delete(context.Background(), org, id) },
		"repo.ListAttendanceForPractice": func() error {
			_, err := svc.repo.ListAttendanceForPractice(context.Background(), org, id)
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("%s: err = nil on a closed pool", name)
			continue
		}
		for _, sentinel := range []error{ErrNotFound, ErrWorkoutNotFound, ErrPhaseNotFound, ErrPracticeNoWorkout} {
			if errors.Is(err, sentinel) {
				t.Errorf("%s: infrastructure failure was reported as %v", name, sentinel)
			}
		}
	}
	if _, err := svc.Update(org, id, PracticeUpdates{Notes: &notes}, "c"); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("Update err = %v, want a begin-tx failure", err)
	}
}

func TestCov1xDBInjectedWorkoutWriteFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()

	tmpl, err := svc.CreateWorkout(org, mainSetInput("Template", "SCY"), "coach")
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(org, "2026-11-02", "G", "water", "Desc", 100, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("workout insert fails", func(t *testing.T) {
		undo := cov1xFailOn(t, pool, "training_workouts", "INSERT")
		defer undo()
		_, err := svc.CreateWorkout(org, cov1xSimpleWorkout("new"), "coach")
		cov1xWantBoom(t, "CreateWorkout", err)
		_, err = svc.DuplicateWorkout(org, tmpl.ID, DuplicateInput{}, "coach")
		cov1xWantBoom(t, "DuplicateWorkout", err)
		_, err = svc.UpdateWorkout(org, tmpl.ID, cov1xSimpleWorkout("renamed"), "coach")
		cov1xWantBoom(t, "UpdateWorkout", err)
		_, _, err = svc.AssignWorkout(org, tmpl.ID, AssignInput{Date: "2026-11-05"}, "coach")
		cov1xWantBoom(t, "AssignWorkout(new practice)", err)
		_, _, err = svc.AssignWorkout(org, tmpl.ID, AssignInput{PracticeID: p.ID}, "coach")
		cov1xWantBoom(t, "AssignWorkout(existing practice)", err)
		undo()

		if got, _ := svc.GetWorkout(org, tmpl.ID); got.Title != "Template" {
			t.Fatalf("failed update changed the template: %q", got.Title)
		}
		all, _ := svc.ListWorkouts(org, WorkoutFilter{})
		if len(all) != 1 {
			t.Fatalf("library has %d workouts after failed writes, want 1", len(all))
		}
		ps, _ := svc.List(org)
		if len(ps) != 1 {
			t.Fatalf("failed assign left %d practices, want the original 1", len(ps))
		}
	})

	t.Run("practice upsert fails while linking and unlinking", func(t *testing.T) {
		linked, inst, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{PracticeID: p.ID}, "coach")
		if err != nil {
			t.Fatal(err)
		}
		undo := cov1xFailOn(t, pool, "practices", "INSERT")
		defer undo()
		_, _, err = svc.AssignWorkout(org, tmpl.ID, AssignInput{PracticeID: p.ID}, "coach")
		cov1xWantBoom(t, "re-assign", err)
		_, err = svc.ClearPracticeWorkout(org, p.ID, "coach")
		cov1xWantBoom(t, "clear", err)
		undo()

		got, _ := svc.Get(org, p.ID)
		if got.WorkoutID != linked.WorkoutID || got.WorkoutID != inst.ID || !cov1xWorkoutExists(t, svc, org, inst.ID) {
			t.Fatalf("rolled-back writes changed the link: %+v", got)
		}
		inst0 := 0
		ws, _ := svc.ListWorkouts(org, WorkoutFilter{})
		for _, w := range ws {
			if !w.IsTemplate {
				inst0++
			}
		}
		if inst0 != 1 {
			t.Fatalf("%d instances after rollback, want exactly the original one", inst0)
		}
	})

	t.Run("workout delete fails while replacing, clearing and deleting a practice", func(t *testing.T) {
		before, _ := svc.Get(org, p.ID)
		undo := cov1xFailOn(t, pool, "training_workouts", "DELETE")
		defer undo()
		_, _, err := svc.AssignWorkout(org, tmpl.ID, AssignInput{PracticeID: p.ID}, "coach")
		cov1xWantBoom(t, "replace instance", err)
		_, err = svc.ClearPracticeWorkout(org, p.ID, "coach")
		cov1xWantBoom(t, "clear", err)
		cov1xWantBoom(t, "delete practice", svc.Delete(org, p.ID))
		undo()

		after, err := svc.Get(org, p.ID)
		if err != nil || after.WorkoutID != before.WorkoutID || after.WorkoutID == "" {
			t.Fatalf("practice after rolled-back deletes = %+v err=%v", after, err)
		}
		// With the fault gone the same delete works and removes the instance.
		if err := svc.Delete(org, p.ID); err != nil {
			t.Fatal(err)
		}
		if cov1xWorkoutExists(t, svc, org, before.WorkoutID) {
			t.Fatal("instance should be removed with its practice")
		}
	})
}

func TestCov1xDBSetPracticeWorkoutWhenPracticeVanishesMidUpdate(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	p, err := svc.Create(org, "2026-11-02", "G", "water", "Desc", 0, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	res := ParseQuickEntry("4x100 free")
	if _, _, err := svc.SetPracticeWorkout(org, p.ID, WorkoutInput{Sections: res.Sections}, "coach"); err != nil {
		t.Fatal(err)
	}
	// After the workout row is rewritten, a concurrent delete removes the practice.
	cov1xExec(t, pool, `CREATE OR REPLACE FUNCTION cov1x_drop_practices() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN DELETE FROM practices; RETURN NULL; END $$`)
	cov1xExec(t, pool, `CREATE TRIGGER cov1x_drop AFTER INSERT OR UPDATE ON training_workouts FOR EACH STATEMENT EXECUTE FUNCTION cov1x_drop_practices()`)
	if _, _, err := svc.SetPracticeWorkout(org, p.ID, WorkoutInput{Sections: res.Sections}, "coach"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound from the re-read", err)
	}
}

func TestCov1xDBInjectedPhaseFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	seasonID := cov1xSeason(t, pool, org)
	season := SeasonWindow{ID: seasonID}
	in := PhaseInput{GroupID: "G", Phase: "base", StartDate: "2026-01-05", EndDate: "2026-01-11"}

	undo := cov1xFailOn(t, pool, "training_plan_phases", "INSERT")
	_, err := svc.CreatePhase(org, season, in, "coach")
	cov1xWantBoom(t, "CreatePhase", err)
	undo()
	if got, _ := svc.ListPhases(org, seasonID); len(got) != 0 {
		t.Fatalf("failed insert persisted a phase: %+v", got)
	}

	// The overlap re-check reads existing phases; if that read fails the save is aborted.
	cov1xExec(t, pool, `ALTER TABLE training_plan_phases RENAME COLUMN focus TO focus_offline`)
	_, err = svc.CreatePhase(org, season, in, "coach")
	if err == nil || !strings.Contains(err.Error(), `column "focus" does not exist`) {
		t.Fatalf("err = %v, want the overlap re-check read to fail", err)
	}
	cov1xExec(t, pool, `ALTER TABLE training_plan_phases RENAME COLUMN focus_offline TO focus`)
	if _, err := svc.CreatePhase(org, season, in, "coach"); err != nil {
		t.Fatalf("create after the fault cleared: %v", err)
	}
}

func TestCov1xDBPhaseSaveFailsWhenAdvisoryLockIsHeld(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	org := uuid.NewString()
	seasonID := cov1xSeason(t, pool, org)

	cfg := pool.Config()
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "150ms"
	impatient, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(impatient.Close)
	svc := NewServiceWithPool(impatient)

	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := "training_phase:" + org + ":" + seasonID + ":G"
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		holder.Release()
		t.Fatal(err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if _, err := holder.Exec(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, key); err != nil {
			t.Errorf("unlock: %v", err)
		}
		holder.Release()
	}
	t.Cleanup(release)

	in := PhaseInput{GroupID: "G", Phase: "base", StartDate: "2026-01-05", EndDate: "2026-01-11"}
	if _, err := svc.CreatePhase(org, SeasonWindow{ID: seasonID}, in, "coach"); err == nil || !strings.Contains(err.Error(), "lock phases") {
		t.Fatalf("err = %v, want a lock-phases failure", err)
	}
	release()
	if _, err := svc.CreatePhase(org, SeasonWindow{ID: seasonID}, in, "coach"); err != nil {
		t.Fatalf("create once the lock is free: %v", err)
	}
}

// cov1xCorruptWorkoutLinkedTo stores a workout whose sections JSON is a valid
// array of the wrong shape and links the practice to it.
func cov1xCorruptWorkoutLinkedTo(t *testing.T, pool *pgxpool.Pool, org, practiceID string) string {
	t.Helper()
	id := uuid.NewString()
	cov1xExec(t, pool, `INSERT INTO training_workouts (id, org_id, title, sections) VALUES ($1::uuid, $2, 'Corrupt', '[1]'::jsonb)`, id, org)
	cov1xExec(t, pool, `UPDATE practices SET workout_id = $1::uuid WHERE id = $2::uuid`, id, practiceID)
	return id
}

func TestCov1xDBCorruptWorkoutRowsSurfaceAsErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	seasonID := cov1xSeason(t, pool, org)
	p, err := svc.Create(org, "2026-11-02", "G", "water", "d", 100, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	wid := cov1xCorruptWorkoutLinkedTo(t, pool, org, p.ID)
	if _, err := svc.CreatePhase(org, SeasonWindow{ID: seasonID}, PhaseInput{GroupID: "G", Phase: "base", StartDate: "2026-11-02", EndDate: "2026-11-08"}, "coach"); err != nil {
		t.Fatal(err)
	}

	_, err = svc.GetWorkout(org, wid)
	if err == nil || errors.Is(err, ErrWorkoutNotFound) || !strings.Contains(err.Error(), "decode workout sections") {
		t.Fatalf("GetWorkout err = %v", err)
	}
	if _, err = svc.ListWorkouts(org, WorkoutFilter{}); err == nil || !strings.Contains(err.Error(), "scan workout") {
		t.Fatalf("ListWorkouts err = %v", err)
	}
	if _, err = svc.WorkoutsForPractices(org, []*Practice{{ID: p.ID, WorkoutID: wid}}); err == nil || !strings.Contains(err.Error(), "scan workout") {
		t.Fatalf("WorkoutsForPractices err = %v", err)
	}
	// A non-"not found" lookup failure is not swallowed as "no workout".
	if _, _, err = svc.GetPracticeWorkout(org, p.ID); err == nil || errors.Is(err, ErrPracticeNoWorkout) {
		t.Fatalf("GetPracticeWorkout err = %v", err)
	}
	notes := "edited"
	if _, err = svc.Update(org, p.ID, PracticeUpdates{Notes: &notes}, "coach"); err == nil || !strings.Contains(err.Error(), "load linked workout") {
		t.Fatalf("Update err = %v", err)
	}
	if got, _ := svc.Get(org, p.ID); got.Notes == notes {
		t.Fatal("rejected update must not persist")
	}
	if _, err = svc.GroupAnalytics(org, "2026-11-01", "2026-11-30", "", nil); err == nil {
		t.Fatal("GroupAnalytics must fail when workouts cannot be loaded")
	}
	if _, err = svc.SwimmerAnalytics(org, uuid.NewString(), nil, "2026-11-01", "2026-11-30", nil); err == nil {
		t.Fatal("SwimmerAnalytics must fail when workouts cannot be loaded")
	}
	if _, err = svc.SeasonPlan(org, SeasonWindow{ID: seasonID}, "", nil); err == nil {
		t.Fatal("SeasonPlan must fail when workouts cannot be loaded")
	}
}

func TestCov1xDBLinkedWorkoutMissingIsUnlinkedOnUpdate(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	p, err := svc.Create(org, "2026-11-02", "G", "water", "d", 750, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	wid := cov1xCorruptWorkoutLinkedTo(t, pool, org, p.ID)
	// Without the foreign key a link can outlive its workout.
	cov1xExec(t, pool, `DO $$ DECLARE c record; BEGIN
		FOR c IN SELECT conname FROM pg_constraint WHERE conrelid = 'practices'::regclass AND confrelid = 'training_workouts'::regclass LOOP
			EXECUTE format('ALTER TABLE practices DROP CONSTRAINT %I', c.conname);
		END LOOP; END $$`)
	cov1xExec(t, pool, `DELETE FROM training_workouts WHERE id = $1::uuid`, wid)

	got, w, err := svc.GetPracticeWorkout(org, p.ID)
	if !errors.Is(err, ErrPracticeNoWorkout) || w != nil || got == nil || got.WorkoutID != wid {
		t.Fatalf("ghost link = %+v %+v err=%v", got, w, err)
	}
	notes := "after the ghost"
	upd, err := svc.Update(org, p.ID, PracticeUpdates{Notes: &notes}, "coach")
	if err != nil {
		t.Fatal(err)
	}
	if upd.WorkoutID != "" || upd.YardageTarget != 750 || upd.Notes != notes {
		t.Fatalf("update = %+v, want the ghost link cleared and the practice's own totals kept", upd)
	}
}

func TestCov1xDBNullMarkedAtSurfacesAsScanError(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	org := uuid.NewString()
	p, err := svc.Create(org, "2026-11-02", "G", "water", "d", 100, nil, PracticeStrokes{}, "", "coach")
	if err != nil {
		t.Fatal(err)
	}
	sw := cov1xSwimmer(t, pool, org)
	cov1xExec(t, pool, `INSERT INTO practice_attendance (id, org_id, practice_id, swimmer_id, status, marked_at) VALUES ($1::uuid, $2, $3::uuid, $4::uuid, 'present', NULL)`,
		uuid.NewString(), org, p.ID, sw)

	if _, err := svc.ListAttendance(org, p.ID); err == nil || !strings.Contains(err.Error(), "scan attendance") {
		t.Fatalf("ListAttendance err = %v", err)
	}
	if _, err := svc.attendanceForPractices(org, []string{p.ID}); err == nil || !strings.Contains(err.Error(), "scan attendance") {
		t.Fatalf("attendanceForPractices err = %v", err)
	}
	if _, err := svc.GroupAnalytics(org, "2026-11-01", "2026-11-30", "", nil); err == nil {
		t.Fatal("GroupAnalytics must surface the attendance read failure")
	}
	if _, err := svc.SwimmerAnalytics(org, sw, map[string]bool{"G": true}, "2026-11-01", "2026-11-30", nil); err == nil {
		t.Fatal("SwimmerAnalytics must surface the attendance read failure")
	}
	// The status-only swimmer lookup does not read marked_at and still works.
	if got, err := svc.AttendanceForSwimmer(org, sw, []string{p.ID}); err != nil || got[p.ID] != "present" {
		t.Fatalf("AttendanceForSwimmer = %v err=%v", got, err)
	}
}

// cov1xRow is a pgx.Row that assigns canned values (nil leaves the zero value).
type cov1xRow struct {
	vals []any
	err  error
}

func (r cov1xRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if r.vals[i] != nil {
			reflect.ValueOf(d).Elem().Set(reflect.ValueOf(r.vals[i]))
		}
	}
	return nil
}

func TestCov1xScanWorkoutNormalisesAndReportsErrors(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	row := func(sections []byte, tags []string) cov1xRow {
		var tg any
		if tags != nil {
			tg = tags
		}
		return cov1xRow{vals: []any{"id-1", "org", "T", "", "LCM", false, tg, sections, "", now, now, "c", "c"}}
	}

	w, err := scanWorkout(row([]byte(`null`), nil))
	if err != nil {
		t.Fatal(err)
	}
	if w.Tags == nil || len(w.Tags) != 0 || w.Sections == nil || len(w.Sections) != 0 {
		t.Fatalf("tags=%#v sections=%#v, want empty non-nil slices", w.Tags, w.Sections)
	}
	if w.Totals.Unit != "m" || w.Totals.Warnings == nil {
		t.Fatalf("totals = %+v, want computed metric totals", w.Totals)
	}

	if _, err := scanWorkout(row([]byte(`[1]`), nil)); err == nil || !strings.Contains(err.Error(), "decode workout sections") {
		t.Fatalf("bad sections err = %v", err)
	}
	boom := errors.New("scan boom")
	if _, err := scanWorkout(cov1xRow{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("scan err = %v", err)
	}
}
