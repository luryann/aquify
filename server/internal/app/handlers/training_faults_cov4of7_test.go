package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/app/training"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cvg4fFault severs the database connection when a statement containing sql is
// about to run, once armed and after `skip` earlier matches. It models the
// database failing part-way through a request, after earlier statements of the
// same request succeeded.
type cvg4fFault struct {
	sql   string
	skip  int32
	armed atomic.Bool
	seen  atomic.Int32
}

func (f *cvg4fFault) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if f.armed.Load() && strings.Contains(data.SQL, f.sql) && f.seen.Add(1) > f.skip {
		_ = conn.Close(ctx)
	}
	return ctx
}

// TraceBatchStart covers statements run through the org-bound batch path.
func (f *cvg4fFault) TraceBatchStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	if !f.armed.Load() {
		return ctx
	}
	for _, q := range data.Batch.QueuedQueries {
		if strings.Contains(q.SQL, f.sql) {
			if f.seen.Add(1) > f.skip {
				_ = conn.Close(ctx)
			}
			break
		}
	}
	return ctx
}

func (f *cvg4fFault) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}
func (f *cvg4fFault) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData)     {}

func (f *cvg4fFault) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (f *cvg4fFault) arm(sql string, skip int32) {
	f.sql, f.skip = sql, skip
	f.seen.Store(0)
	f.armed.Store(true)
}

func (f *cvg4fFault) disarm() { f.armed.Store(false) }

// cvg4fPool returns a pool on a fresh isolated schema whose connections carry the fault.
func cvg4fPool(t *testing.T) (*pgxpool.Pool, *cvg4fFault) {
	t.Helper()
	base := testutil.IsolatedSchemaPool(t)
	cfg := base.Config().Copy()
	f := &cvg4fFault{}
	cfg.ConnConfig.Tracer = f
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, f
}

type cvg4fEnv struct {
	t      *testing.T
	org    string
	router http.Handler
}

func (e *cvg4fEnv) do(method, path, token, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", e.org)
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	return rr
}

func cvg4fEnvFor(t *testing.T, org, hh string, h *TrainingHandlers) *cvg4fEnv {
	t.Helper()
	tokens := tokenResolver{
		"coach":  {UserID: "u-coach", OrgID: org, Roles: []string{"coach"}},
		"parent": {UserID: "u-parent", OrgID: org, HouseholdID: hh, Roles: []string{"parent"}},
	}
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		h.RegisterRoutes(p)
	})
	return &cvg4fEnv{t: t, org: org, router: r}
}

// A swimmer login that is not linked to a household has no group scope: it sees
// team-wide practices only, never a group's practice, and is not turned away.
func TestCvg4fScheduleForSwimmerWithoutHousehold(t *testing.T) {
	t.Parallel()
	w := cvg4NewTrainWorld(t)
	w.practice("2026-10-05", w.senior.ID, "")
	team := w.practice("2026-10-07", "", "")

	rr := w.do("GET", "/training/schedule?from=2026-10-01&to=2026-10-31", "swimmerNoH", "")
	cvg4Want(t, rr, http.StatusOK, "", "unlinked swimmer schedule")
	items := cvg4JSON(t, rr)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["practice"].(map[string]any)["id"] != team.ID {
		t.Fatalf("an unlinked swimmer sees only team-wide practices, got %v", items)
	}
}

// A statement that fails after the workout was found surfaces as a 500 and the
// workout survives.
func TestCvg4fDeleteWorkoutFailureKeepsWorkout(t *testing.T) {
	pool, fault := cvg4fPool(t)
	org := uuid.NewString()
	svc := training.NewServiceWithPool(pool)
	h := NewTrainingHandlers(svc, audit.NewService())
	e := cvg4fEnvFor(t, org, "", h)

	rr := e.do("POST", "/workouts", "coach", cvg4WorkoutJSON("Threshold", ""))
	cvg4Want(t, rr, http.StatusCreated, "", "create")
	id := cvg4JSON(t, rr)["workout"].(map[string]any)["id"].(string)

	fault.arm("DELETE FROM training_workouts", 0)
	rr = e.do("DELETE", "/workouts/"+id, "coach", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "delete during outage")
	fault.disarm()

	cvg4Want(t, e.do("GET", "/workouts/"+id, "coach", ""), http.StatusOK, "", "workout survives")
	cvg4Want(t, e.do("DELETE", "/workouts/"+id, "coach", ""), http.StatusOK, "", "delete once the database is back")
}

// If the workouts cannot be loaded for the schedule, the whole read fails rather
// than returning practices with silently missing workouts.
func TestCvg4fScheduleWorkoutLoadFailure(t *testing.T) {
	pool, fault := cvg4fPool(t)
	org := uuid.NewString()
	svc := training.NewServiceWithPool(pool)
	h := NewTrainingHandlers(svc, audit.NewService())
	e := cvg4fEnvFor(t, org, "", h)

	p, err := svc.Create(org, "2026-10-05", "", "water", "Aerobic", 3000, nil, training.PracticeStrokes{}, "", "u-coach")
	if err != nil {
		t.Fatal(err)
	}
	rr := e.do("PUT", "/practices/"+p.ID+"/workout", "coach", cvg4WorkoutJSON("Race pace", ""))
	cvg4Want(t, rr, http.StatusOK, "", "attach workout")

	const path = "/training/schedule?from=2026-10-01&to=2026-10-31"
	rr = e.do("GET", path, "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "healthy schedule")
	if items := cvg4JSON(t, rr)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["workout"] == nil {
		t.Fatalf("expected one practice with its workout, got %v", items)
	}

	fault.arm("FROM training_workouts WHERE org_id = $1 AND id = ANY", 0)
	rr = e.do("GET", path, "coach", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "workouts unavailable")
	if strings.Contains(rr.Body.String(), "conn closed") {
		t.Fatalf("cause leaked to the client: %s", rr.Body.String())
	}
}

// A phase that cannot be deleted stays in the plan.
func TestCvg4fDeletePhaseFailureKeepsPhase(t *testing.T) {
	pool, fault := cvg4fPool(t)
	org := uuid.NewString()
	seasonSvc := seasons.NewServiceWithPool(pool)
	season, err := seasonSvc.Create(org, "2026-27", "2026-09-01", "2027-08-31", "active", "", "u-coach")
	if err != nil {
		t.Fatal(err)
	}
	var phaseID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO training_plan_phases (org_id, season_id, group_id, phase, start_date, end_date)
		VALUES ($1, $2::uuid, 'senior', 'base', '2026-09-01', '2026-10-31') RETURNING id::text`, org, season.ID).Scan(&phaseID); err != nil {
		t.Fatal(err)
	}
	h := NewTrainingHandlers(training.NewServiceWithPool(pool), audit.NewService())
	h.SetSeasonsService(seasonSvc)
	e := cvg4fEnvFor(t, org, "", h)

	fault.arm("DELETE FROM training_plan_phases", 0)
	cvg4Want(t, e.do("DELETE", "/training-phases/"+phaseID, "coach", ""), http.StatusInternalServerError, "internal_error", "delete during outage")
	fault.disarm()

	rr := e.do("GET", "/seasons/"+season.ID+"/training-phases", "coach", "")
	cvg4Want(t, rr, http.StatusOK, "", "list")
	if got := cvg4JSON(t, rr)["phases"].([]any); len(got) != 1 {
		t.Fatalf("the phase must survive a failed delete, got %v", got)
	}
	cvg4Want(t, e.do("DELETE", "/training-phases/"+phaseID, "coach", ""), http.StatusOK, "", "delete once the database is back")
}

// The members directory failing between the swimmer lookup and the group lookup
// is reported as a 500 instead of an analytics view with the wrong groups.
func TestCvg4fSwimmerAnalyticsGroupLookupFailure(t *testing.T) {
	pool, fault := cvg4fPool(t)
	org := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: "u-admin", OrgID: org, Roles: []string{"admin"}}
	ms := members.NewServiceWithPool(pool)
	hh, err := ms.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Alpha", PrimaryEmail: "alpha@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := ms.CreateSwimmer(admin, org, members.CreateSwimmerInput{
		HouseholdID: hh.ID, FirstName: "Ada", LastName: "Alpha", DateOfBirth: "2012-04-05", Gender: "female", RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewTrainingHandlersWithMembers(training.NewService(), ms, groups.NewService(), audit.NewService())
	e := cvg4fEnvFor(t, org, hh.ID, h)

	path := "/training/analytics/swimmers/" + sw.ID + "?from=2026-10-01&to=2026-10-31"
	cvg4Want(t, e.do("GET", path, "parent", ""), http.StatusOK, "", "healthy analytics")

	// The first group-links read (swimmer lookup) succeeds; the second fails.
	fault.arm("swimmer_groups", 1)
	rr := e.do("GET", path, "parent", "")
	cvg4Want(t, rr, http.StatusInternalServerError, "internal_error", "group lookup outage")
}
