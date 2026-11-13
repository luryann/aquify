package performance

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

type cov1xDB struct {
	pool    *pgxpool.Pool
	org     string
	swimmer string
}

// cov1xNewDB returns an isolated schema with one club and one swimmer.
func cov1xNewDB(t *testing.T) cov1xDB {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	d := cov1xDB{pool: pool, org: uuid.NewString(), swimmer: uuid.NewString()}
	hh := uuid.NewString()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Cov1x household')`, hh, d.org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'Cov', 'Swimmer', '2013-01-01', 'female')`, d.swimmer, d.org, hh); err != nil {
		t.Fatal(err)
	}
	return d
}

func (d cov1xDB) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (d cov1xDB) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := d.pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (d cov1xDB) swim(t *testing.T, s *Service, ms int, mut func(*UpsertResultInput)) (*Result, error) {
	t.Helper()
	in := UpsertResultInput{SwimmerID: d.swimmer, Distance: 50, Stroke: "free", Course: "SCY", Gender: "F", FinTimeMs: &ms, IsOfficial: true}
	if mut != nil {
		mut(&in)
	}
	return s.UpsertResult(d.org, "admin", adminP, in)
}

func TestCov1xRepositoryRoundTripAndTransactions(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	ctx := context.Background()

	r, err := d.swim(t, svc, 27000, func(in *UpsertResultInput) {
		in.Heat, in.Lane, in.Place, in.SeedTimeMs = intPtr(3), intPtr(4), intPtr(1), intPtr(28000)
		in.Splits = []SplitInput{{SplitNumber: 1, SplitDistance: 25, SplitTimeMs: 13000, CumulativeTimeMs: 13000}, {SplitNumber: 2, SplitDistance: 50, SplitTimeMs: 14000, CumulativeTimeMs: 27000}}
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.count(t, "splits") != 2 {
		t.Fatalf("splits stored = %d", d.count(t, "splits"))
	}
	loaded, err := svc.repo.LoadResults(ctx, d.org)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("LoadResults = %v %v", loaded, err)
	}
	got := loaded[0]
	if got.ID != r.ID || got.Heat == nil || *got.Heat != 3 || *got.Lane != 4 || *got.Place != 1 || *got.SeedTimeMs != 28000 || *got.FinTimeMs != 27000 {
		t.Fatalf("heat/lane/place/seed must round-trip: %+v", got)
	}
	if err := svc.repo.UpsertSplits(ctx, nil); err != nil {
		t.Fatalf("no splits is a no-op, got %v", err)
	}
	if err := svc.repo.DeleteResult(ctx, d.org, r.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := svc.repo.LoadResults(ctx, d.org); len(after) != 0 {
		t.Fatalf("DeleteResult left %d results", len(after))
	}
	if d.count(t, "splits") != 0 {
		t.Fatal("splits cascade with their result")
	}

	// A split for a result that does not exist is rejected and rolls the whole batch back.
	now := time.Now().UTC()
	bad := []*Split{{ID: uuid.NewString(), OrgID: d.org, ResultID: uuid.NewString(), SplitNumber: 1, SplitDistance: 25, SplitTimeMs: 1, CumulativeTimeMs: 1, CreatedAt: now, UpdatedAt: now}}
	if err := svc.repo.UpsertSplits(ctx, bad); err == nil {
		t.Fatal("orphan split must be rejected")
	}

	ageMin, ageMax := 11, 12
	mkStd := func(code string, dist int) *TimeStandard {
		return &TimeStandard{
			ID: uuid.NewString(), OrgID: d.org, Name: code, SeasonYear: 2026, StandardCode: code, EventDistance: dist, Stroke: "free",
			Course: "SCY", Gender: "F", AgeMin: &ageMin, AgeMax: &ageMax, TimeMs: 60000, IsActive: true, CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := svc.repo.UpsertTimeStandardsTx(ctx, nil); err != nil {
		t.Fatal(err)
	}
	broken := mkStd("BAD", 100)
	broken.ID = "not-a-uuid"
	if err := svc.repo.UpsertTimeStandardsTx(ctx, []*TimeStandard{mkStd("OK", 50), broken}); err == nil {
		t.Fatal("a bad row must fail the batch")
	}
	if d.count(t, "time_standards") != 0 {
		t.Fatal("the batch is one transaction: nothing may be stored when a row fails")
	}
	if err := svc.repo.UpsertTimeStandardsTx(ctx, []*TimeStandard{mkStd("A", 50), mkStd("B", 100)}); err != nil {
		t.Fatal(err)
	}
	stds, err := svc.repo.LoadTimeStandards(ctx, d.org)
	if err != nil || len(stds) != 2 {
		t.Fatalf("LoadTimeStandards = %v %v", stds, err)
	}
	for _, ts := range stds {
		if ts.AgeMin == nil || *ts.AgeMin != 11 || ts.AgeMax == nil || *ts.AgeMax != 12 {
			t.Fatalf("age range must round-trip: %+v", ts)
		}
	}
}

func TestCov1xRepositoryFailuresOnUnreachableDatabase(t *testing.T) {
	repo := cov1xClosedRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	std := &TimeStandard{ID: uuid.NewString(), OrgID: "o", CreatedAt: now, UpdatedAt: now}
	split := &Split{ID: uuid.NewString(), OrgID: "o", ResultID: uuid.NewString()}
	bt := &BestTime{ID: uuid.NewString(), OrgID: "o", SwimmerID: uuid.NewString()}
	checks := map[string]error{
		"DeleteResult":           repo.DeleteResult(ctx, "o", uuid.NewString()),
		"UpsertSplits":           repo.UpsertSplits(ctx, []*Split{split}),
		"UpsertBestTime":         repo.UpsertBestTime(ctx, bt),
		"UpsertTimeStandardsTx":  repo.UpsertTimeStandardsTx(ctx, []*TimeStandard{std}),
		"UpsertTimeStandard":     repo.UpsertTimeStandard(ctx, std),
		"SoftDeleteTimeStandard": repo.SoftDeleteTimeStandard(ctx, "o", std.ID, now),
	}
	if _, err := repo.LoadResults(ctx, "o"); err != nil {
		checks["LoadResults"] = err
	}
	if _, err := repo.LoadBestTimes(ctx, "o"); err != nil {
		checks["LoadBestTimes"] = err
	}
	if _, err := repo.LoadTimeStandards(ctx, "o"); err != nil {
		checks["LoadTimeStandards"] = err
	}
	if _, _, _, err := repo.LoadOrg(ctx, "o"); err != nil {
		checks["LoadOrg"] = err
	}
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s: expected an error from a closed pool", name)
		}
	}
	for _, name := range []string{"LoadResults", "LoadBestTimes", "LoadTimeStandards", "LoadOrg"} {
		if checks[name] == nil || !strings.Contains(checks[name].Error(), "performance: load") {
			t.Errorf("%s error should name the failing load: %v", name, checks[name])
		}
	}
}

func TestCov1xRepositoryScanAndPartialLoadFailures(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	ctx := context.Background()
	if _, err := d.swim(t, svc, 27000, nil); err != nil {
		t.Fatal(err)
	}

	// A best_times row with a NULL stroke cannot be scanned into the Go struct.
	d.exec(t, `ALTER TABLE best_times ALTER COLUMN stroke DROP NOT NULL`)
	d.exec(t, `UPDATE best_times SET stroke = NULL`)
	if _, err := svc.repo.LoadBestTimes(ctx, d.org); err == nil || !strings.Contains(err.Error(), "scan best_time") {
		t.Fatalf("LoadBestTimes scan failure = %v", err)
	}
	// Same for results.
	d.exec(t, `ALTER TABLE results ALTER COLUMN gender DROP NOT NULL`)
	d.exec(t, `UPDATE results SET gender = NULL`)
	if _, err := svc.repo.LoadResults(ctx, d.org); err == nil || !strings.Contains(err.Error(), "scan result") {
		t.Fatalf("LoadResults scan failure = %v", err)
	}
	// A club whose rows cannot be read never gets cached.
	fresh := NewServiceWithPool(d.pool)
	if err := fresh.ensureOrg(d.org); err == nil {
		t.Fatal("ensureOrg must surface a failed load")
	}
	if len(fresh.results) != 0 {
		t.Fatal("nothing may be cached after a failed load")
	}
}

func TestCov1xLoadOrgStopsAtFirstFailingTable(t *testing.T) {
	d := cov1xNewDB(t)
	repo := newRepository(d.pool)
	ctx := context.Background()
	d.exec(t, `ALTER TABLE best_times RENAME COLUMN best_time_ms TO best_time_ms_x`)
	res, bts, stds, err := repo.LoadOrg(ctx, d.org)
	if err == nil || !strings.Contains(err.Error(), "load best_times") {
		t.Fatalf("LoadOrg = %v", err)
	}
	if res == nil && bts == nil && stds == nil {
		// results were read before best_times failed; the empty slice is fine, the standards were never read
		t.Log("results are returned empty when the club has none")
	}
	if stds != nil {
		t.Fatalf("standards must not be read after a failure: %v", stds)
	}
}

func TestCov1xExtensionLoadFailureAbortsClubLoad(t *testing.T) {
	d := cov1xNewDB(t)
	d.exec(t, `ALTER TABLE swimmer_goals RENAME COLUMN target_time_ms TO target_time_ms_x`)
	svc := NewServiceWithPool(d.pool)
	err := svc.ensureOrg(d.org)
	if err == nil || !strings.Contains(err.Error(), "swimmer_goals") {
		t.Fatalf("ensureOrg = %v", err)
	}
	if _, ok := svc.loadedAt.Load(d.org); ok {
		t.Fatal("a club whose extensions failed to load must not be marked loaded")
	}
}

func TestCov1xForeignWriteTriggersReloadBeforeWrite(t *testing.T) {
	d := cov1xNewDB(t)
	svc1, svc2 := NewServiceWithPool(d.pool), NewServiceWithPool(d.pool)
	if err := svc1.ensureOrg(d.org); err != nil {
		t.Fatal(err)
	}
	loads := atomic.Int32{}
	svc1.SetSyncHookForTest(func() { loads.Add(1) })

	if _, err := d.swim(t, svc2, 26000, nil); err != nil {
		t.Fatal(err)
	}
	if len(svc1.results) != 0 {
		t.Fatal("svc1 has not synced yet")
	}
	// svc1's next write notices the other instance's write and re-reads the club first.
	if _, err := d.swim(t, svc1, 29000, nil); err != nil {
		t.Fatal(err)
	}
	if loads.Load() == 0 {
		t.Fatal("expected a club re-read before the write")
	}
	if len(svc1.results) != 2 {
		t.Fatalf("svc1 must see both swims, has %d", len(svc1.results))
	}
	bt := svc1.btByKey[btKey(d.org, d.swimmer, 50, "free", "SCY")]
	if bt == nil || bt.BestTimeMs == nil || *bt.BestTimeMs != 26000 {
		t.Fatalf("best time must reflect the other instance's faster swim: %+v", bt)
	}
}

func TestCov1xReloadFallsBackToLockedReadAfterRepeatedConflicts(t *testing.T) {
	d := cov1xNewDB(t)
	seed := NewServiceWithPool(d.pool)
	if _, err := d.swim(t, seed, 27000, nil); err != nil {
		t.Fatal(err)
	}

	svc := NewServiceWithPool(d.pool)
	reads := atomic.Int32{}
	// Every unlocked read is followed by a writer finishing, so the snapshot is always stale.
	svc.SetSyncHookForTest(func() {
		reads.Add(1)
		svc.mu.Lock()
		svc.mu.Unlock()
	})
	if err := svc.ensureOrg(d.org); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 3 {
		t.Fatalf("expected 3 lock-free attempts before falling back, got %d", reads.Load())
	}
	if len(svc.results) != 1 || len(svc.bestTimes) != 1 {
		t.Fatalf("the locked fallback must still load the club: %d results, %d best times", len(svc.results), len(svc.bestTimes))
	}
}

func TestCov1xGoalsAndLineupsLoadReloadAndEvict(t *testing.T) {
	d := cov1xNewDB(t)
	meet := uuid.NewString()
	d.exec(t, `INSERT INTO meets (id, org_id, name) VALUES ('`+meet+`', '`+d.org+`', 'Cov1x Meet')`)

	writer := NewServiceWithPool(d.pool)
	gp, err := writer.CreateGoal(d.org, "admin", adminP, d.swimmer, GoalInput{Distance: 50, Stroke: "free", Course: "SCY", TargetTimeMs: 26000, Deadline: "2030-05-01", Notes: "n"})
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := writer.SaveRelayLineup(d.org, "coach", coachP, "", RelayLineupInput{
		MeetID: meet, Course: "SCY", Gender: "F", AgeGroup: "OPEN", RelayType: "free", Distance: 200,
		Teams: []RelayTeam{{Legs: []RelayLeg{{Leg: 1, SwimmerID: "a"}, {Leg: 2, SwimmerID: "b"}, {Leg: 3, SwimmerID: "c"}, {Leg: 4, SwimmerID: "d"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := NewServiceWithPool(d.pool)
	if err := reader.ensureOrg(d.org); err != nil {
		t.Fatal(err)
	}
	if g := reader.goals[gp.ID]; g == nil || g.Deadline != "2030-05-01" || g.TargetTimeMs != 26000 {
		t.Fatalf("goal must load from Postgres: %+v", g)
	}
	if l := reader.lineups[lineup.ID]; l == nil || len(l.Teams) != 1 || l.Teams[0].Legs[3].SwimmerID != "d" {
		t.Fatalf("lineup must load from Postgres: %+v", l)
	}

	// A reload replaces the club's rows rather than duplicating them.
	d.exec(t, `DELETE FROM swimmer_goals`)
	if err := reader.loadOrg(context.Background(), d.org); err != nil {
		t.Fatal(err)
	}
	if len(reader.goals) != 0 || len(reader.lineups) != 1 {
		t.Fatalf("after reload goals=%d lineups=%d", len(reader.goals), len(reader.lineups))
	}

	// Loading a second club evicts the first one (cache limited to one club).
	reader.SetOrgCacheLimits(time.Hour, 1)
	reader.touchLoaded(d.org)
	if err := reader.ensureOrg(d.org); err != nil {
		t.Fatal(err)
	}
	if err := reader.ensureOrg(uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if len(reader.lineups) != 0 || len(reader.goals) != 0 {
		t.Fatalf("evicted club must be dropped: goals=%d lineups=%d", len(reader.goals), len(reader.lineups))
	}
	if _, ok := reader.loadedAt.Load(d.org); ok {
		t.Fatal("evicted club must be marked unloaded")
	}
}

func TestCov1xSplitFailureRollsBackTheResult(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	d.exec(t, `ALTER TABLE splits RENAME COLUMN split_number TO split_number_x`)
	_, err := d.swim(t, svc, 27000, func(in *UpsertResultInput) {
		in.Splits = []SplitInput{{SplitNumber: 1, SplitDistance: 25, SplitTimeMs: 13000, CumulativeTimeMs: 13000}}
	})
	if err == nil || !strings.Contains(err.Error(), "persist splits") {
		t.Fatalf("err = %v", err)
	}
	if d.count(t, "results") != 0 || len(svc.results) != 0 || len(svc.splits) != 0 {
		t.Fatalf("a failed split write must undo the result: db=%d cache=%d", d.count(t, "results"), len(svc.results))
	}
}

func TestCov1xRollbackFailureIsLoggedNotMasked(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	d.exec(t, `ALTER TABLE splits RENAME COLUMN split_number TO split_number_x`)
	d.exec(t, `CREATE FUNCTION cov1x_no_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'deletes are blocked'; END $$`)
	d.exec(t, `CREATE TRIGGER cov1x_no_delete BEFORE DELETE ON results FOR EACH ROW EXECUTE FUNCTION cov1x_no_delete()`)
	_, err := d.swim(t, svc, 27000, func(in *UpsertResultInput) {
		in.Splits = []SplitInput{{SplitNumber: 1, SplitDistance: 25, SplitTimeMs: 13000, CumulativeTimeMs: 13000}}
	})
	// The caller still sees the original failure, not the compensation failure.
	if err == nil || !strings.Contains(err.Error(), "persist splits") {
		t.Fatalf("err = %v", err)
	}
	if len(svc.results) != 0 {
		t.Fatal("the cache is rolled back even when the database delete is refused")
	}
	if d.count(t, "results") != 1 {
		t.Fatalf("the blocked delete leaves the row for an operator to repair, db=%d", d.count(t, "results"))
	}
}

func TestCov1xBestTimeWriteFailureRollsBackTheResult(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	if err := svc.ensureOrg(d.org); err != nil {
		t.Fatal(err)
	}
	d.exec(t, `ALTER TABLE best_times RENAME COLUMN best_time_ms TO best_time_ms_x`)
	_, err := d.swim(t, svc, 27000, nil)
	if err == nil || !strings.Contains(err.Error(), "persist best time") {
		t.Fatalf("err = %v", err)
	}
	if d.count(t, "results") != 0 || len(svc.results) != 0 || len(svc.bestTimes) != 0 {
		t.Fatalf("the swim must not survive a failed best-time write: db=%d cache=%d", d.count(t, "results"), len(svc.results))
	}
	if _, err := svc.RecomputeBestTime(d.org, "admin", adminP, d.swimmer, 50, "free", "SCY"); err != nil {
		t.Fatalf("with no results there is nothing to persist: %v", err)
	}
}

func TestCov1xBestTimeAdoptsRowCreatedByAnotherInstance(t *testing.T) {
	d := cov1xNewDB(t)
	svc := NewServiceWithPool(d.pool)
	first, err := d.swim(t, svc, 27000, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := btKey(d.org, d.swimmer, 50, "free", "SCY")
	cached := svc.btByKey[key]
	if cached == nil {
		t.Fatal("best time not cached")
	}
	oldID := cached.ID

	// Another instance replaced the row under a different id; this instance still caches the old one.
	otherID := uuid.NewString()
	d.exec(t, `DELETE FROM best_times`)
	d.exec(t, `INSERT INTO best_times (id, org_id, swimmer_id, event_distance, stroke, course, best_time_ms) VALUES ('`+otherID+`', '`+d.org+`', '`+d.swimmer+`', 50, 'free', 'SCY', 30000)`)

	bt, err := svc.RecomputeBestTime(d.org, "admin", adminP, d.swimmer, 50, "free", "SCY")
	if err != nil {
		t.Fatal(err)
	}
	if bt.ID != otherID || bt.ResultID != first.ID || *bt.BestTimeMs != 27000 {
		t.Fatalf("recompute must adopt the persisted row id and refresh it: %+v", bt)
	}
	if svc.bestTimes[oldID] != nil || svc.bestTimes[otherID] == nil || len(svc.bestTimes) != 1 {
		t.Fatalf("stale cache entry must be dropped: %v", svc.bestTimes)
	}
	if d.count(t, "best_times") != 1 {
		t.Fatalf("one row per event, have %d", d.count(t, "best_times"))
	}
}
