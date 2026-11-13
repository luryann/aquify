package performance

import (
	"fmt"
	"strings"
	"testing"
)

// Dropping a club from the cache removes its goals as well as its other rows.
func TestCov1xGapsDropOrgRemovesGoals(t *testing.T) {
	s := NewService()
	gp, err := s.CreateGoal("org", "admin", adminP, "amy", GoalInput{Distance: 50, Stroke: "free", Course: "SCY", TargetTimeMs: 26000, Deadline: "2030-05-01"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateGoal("org2", "admin", adminP, "bea", GoalInput{Distance: 50, Stroke: "free", Course: "SCY", TargetTimeMs: 26000, Deadline: "2030-05-01"})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.dropOrgLocked("org")
	s.mu.Unlock()
	if s.goals[gp.ID] != nil {
		t.Fatal("goal of the dropped club must be removed")
	}
	if s.goals[other.ID] == nil {
		t.Fatal("goal of another club must survive")
	}
}

// A best-time write that cannot be persisted aborts the recompute with a wrapped error.
func TestCov1xGapsRecomputeBestTimePersistFailure(t *testing.T) {
	s := NewService()
	r := cov1xSwim(t, s, "org", "amy", 50, "free", "SCY", 28000, true)
	r.Status = "deleted"
	cov1xBreakPersistence(t, s, "org")
	bt, err := s.RecomputeBestTime("org", "coach", coachP, "amy", 50, "free", "SCY")
	if err == nil || !strings.Contains(err.Error(), "persist best time") || bt != nil {
		t.Fatalf("want persist best time error, got %+v %v", bt, err)
	}
}

// A result that Postgres rejects is not cached and the error names the failed write.
func TestCov1xGapsUpsertResultWithRecordsPersistFailure(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	d.exec(t, `CREATE FUNCTION cov1x_gaps_no_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'inserts are blocked'; END $$`)
	d.exec(t, `CREATE TRIGGER cov1x_gaps_no_insert BEFORE INSERT ON results FOR EACH ROW EXECUTE FUNCTION cov1x_gaps_no_insert()`)
	ms := 27000
	res, breaks, err := svc.UpsertResultWithRecords(d.org, "admin", adminP, UpsertResultInput{
		SwimmerID: d.swimmer, Distance: 50, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true,
	})
	if err == nil || !strings.Contains(err.Error(), "persist result") || res != nil || breaks != nil {
		t.Fatalf("want persist result error, got %+v %v %v", res, breaks, err)
	}
	if len(svc.results) != 0 || d.count(t, "results") != 0 {
		t.Fatalf("rejected result must not be stored: cache=%d db=%d", len(svc.results), d.count(t, "results"))
	}
}

// A file above the row cap is refused instead of parsed.
func TestCov1xGapsParseStandardsRowCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("standard_name,season_year,course,gender,event_distance,stroke,cut_time\n")
	for i := 0; i <= MaxStandardsImportRows; i++ {
		fmt.Fprintf(&b, "S%d,2026,SCY,F,100,free,58.12\n", i)
	}
	rows, err := ParseTimeStandardsCSV([]byte(b.String()))
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d rows", MaxStandardsImportRows)) || rows != nil {
		t.Fatalf("want row cap error, got %d rows %v", len(rows), err)
	}
}
