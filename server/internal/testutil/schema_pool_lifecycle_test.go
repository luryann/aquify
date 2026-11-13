package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIsolatedSchemaNamesRecordTheirCreationTime(t *testing.T) {
	t.Parallel()
	at := time.Unix(1_800_000_000, 0)
	name := newIsolatedSchemaName(at)
	if !strings.HasPrefix(name, "iso_test_1800000000_") {
		t.Fatalf("name %q does not start with the prefix and unix time", name)
	}
	got, ok := isolatedSchemaCreatedAt(name)
	if !ok || !got.Equal(at) {
		t.Fatalf("round trip: got %v ok=%v, want %v", got, ok, at)
	}
	if other := newIsolatedSchemaName(at); other == name {
		t.Fatal("two names created in the same second must still differ")
	}

	for _, unparsable := range []string{
		"iso_test_" + strings.Repeat("a", 32),            // older form: no timestamp
		"iso_test_1800000000_" + strings.Repeat("a", 31), // uuid part too short
		"iso_test_18_" + strings.Repeat("a", 32),         // implausibly short timestamp
		"iso_test_1800000000_" + strings.Repeat("A", 32), // uuid part must be lowercase hex
		"public",
		"x" + name,
		name + "x",
		"",
	} {
		if _, ok := isolatedSchemaCreatedAt(unparsable); ok {
			t.Errorf("%q must not be treated as a timestamped isolated schema", unparsable)
		}
	}
}

func TestSelectStaleIsolatedSchemas(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	mk := func(age time.Duration) string { return newIsolatedSchemaName(now.Add(-age)) }
	old30, old10, fresh, atLimit := mk(30*time.Hour), mk(10*time.Hour), mk(time.Hour), mk(6*time.Hour)
	legacy := "iso_test_" + strings.Repeat("b", 32)
	names := []string{fresh, old10, legacy, "public", atLimit, old30}

	got := selectStaleIsolatedSchemas(names, now, 6*time.Hour, 0)
	if want := []string{old30, old10}; !slices.Equal(got, want) {
		t.Fatalf("stale (oldest first), got %v want %v: a schema exactly at the limit, a fresh one, a legacy name and a foreign schema must all be kept", got, want)
	}
	if got := selectStaleIsolatedSchemas(names, now, 6*time.Hour, 1); !slices.Equal(got, []string{old30}) {
		t.Fatalf("limit 1 must keep only the oldest, got %v", got)
	}
	if got := selectStaleIsolatedSchemas(nil, now, time.Hour, 5); len(got) != 0 {
		t.Fatalf("no schemas, nothing stale: %v", got)
	}
}

func TestIsLockTableExhausted(t *testing.T) {
	t.Parallel()
	exhausted := &pgconn.PgError{Code: "53200", Message: "out of shared memory"}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"other postgres error", &pgconn.PgError{Code: "42P01"}, false},
		{"lock table full", exhausted, true},
		{"wrapped lock table full", fmt.Errorf("drop: %w", exhausted), true},
	}
	for _, c := range cases {
		if got := isLockTableExhausted(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRetryOnLockTableExhaustion(t *testing.T) {
	t.Parallel()
	exhausted := &pgconn.PgError{Code: "53200"}
	ctx := context.Background()

	t.Run("succeeds after transient exhaustion", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retryOnLockTableExhaustion(ctx, 4, time.Microsecond, func() error {
			calls++
			if calls < 3 {
				return exhausted
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatalf("err=%v calls=%d, want nil after 3 calls", err, calls)
		}
	})
	t.Run("gives up after the attempt limit and reports the error", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retryOnLockTableExhaustion(ctx, 3, time.Microsecond, func() error { calls++; return exhausted })
		if !isLockTableExhausted(err) || calls != 3 {
			t.Fatalf("err=%v calls=%d, want the 53200 error after exactly 3 calls", err, calls)
		}
	})
	t.Run("does not retry other errors", func(t *testing.T) {
		t.Parallel()
		calls := 0
		boom := errors.New("syntax error")
		err := retryOnLockTableExhaustion(ctx, 4, time.Microsecond, func() error { calls++; return boom })
		if !errors.Is(err, boom) || calls != 1 {
			t.Fatalf("err=%v calls=%d, want the original error after one call", err, calls)
		}
	})
	t.Run("stops retrying when the context is cancelled", func(t *testing.T) {
		t.Parallel()
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		calls := 0
		err := retryOnLockTableExhaustion(cctx, 4, time.Hour, func() error { calls++; return exhausted })
		if !isLockTableExhausted(err) || calls != 1 {
			t.Fatalf("err=%v calls=%d: a cancelled context must return after the first failure without waiting", err, calls)
		}
	})
}

// lifecycleBasePool opens a plain pool on DATABASE_URL (the schemas under test are created on
// it by hand, not through IsolatedSchemaPool) or skips.
func lifecycleBasePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func schemaExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_namespace WHERE nspname = $1`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// createLifecycleSchema makes a schema holding one table and registers its removal.
func createLifecycleSchema(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	ctx := context.Background()
	ident := pgx.Identifier{name}.Sanitize()
	// One statement batch (one implicit transaction): a sweep running in another test process
	// can never observe the schema half-built, nor drop it between the two statements.
	t.Cleanup(func() { dropIsolatedSchemaOrFail(t, pool, name) })
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+ident+"; CREATE TABLE "+ident+".t (id int)"); err != nil {
		t.Fatal(err)
	}
}

func TestSweepDropsOnlyOldTimestampedSchemas(t *testing.T) {
	pool := lifecycleBasePool(t)
	now := time.Now()
	stale := newIsolatedSchemaName(now.Add(-24 * time.Hour))
	fresh := newIsolatedSchemaName(now)
	legacy := "iso_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	foreign := "lifecycle_keep_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, name := range []string{stale, fresh, legacy, foreign} {
		createLifecycleSchema(t, pool, name)
	}

	if _, err := sweepStaleIsolatedSchemas(context.Background(), pool, now, staleIsoSchemaAge, staleIsoSchemaSweepLimit); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if schemaExists(t, pool, stale) {
		t.Errorf("schema %s is a day old and must have been swept", stale)
	}
	for name, why := range map[string]string{
		fresh:   "belongs to a live run",
		legacy:  "has no timestamp, so its age is unknown",
		foreign: "is not an isolated test schema",
	} {
		if !schemaExists(t, pool, name) {
			t.Errorf("schema %s %s and must survive the sweep", name, why)
		}
	}
}

func TestDropIsolatedSchemaIsIdempotentAndReportsFailure(t *testing.T) {
	pool := lifecycleBasePool(t)
	name := newIsolatedSchemaName(time.Now())
	createLifecycleSchema(t, pool, name)

	ctx := context.Background()
	if err := dropIsolatedSchema(ctx, pool, name); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if schemaExists(t, pool, name) {
		t.Fatal("schema still exists after drop")
	}
	if err := dropIsolatedSchema(ctx, pool, name); err != nil {
		t.Fatalf("dropping an already-dropped schema must be a no-op, got %v", err)
	}

	// With the database unreachable the drop must fail loudly, and the cleanup helper must
	// fail the test instead of leaving a silent leak.
	closed := ClosedPool(t)
	if err := dropIsolatedSchema(ctx, closed, name); err == nil {
		t.Fatal("drop on a closed pool must return an error")
	}
	fake := cov1xRunFake(func(ft *testing.T) { dropIsolatedSchemaOrFail(ft, closed, name) })
	if !fake.Failed() {
		t.Fatal("a drop that cannot complete must fail the test that owns the schema")
	}
}

func TestSweepReportsQueryFailure(t *testing.T) {
	closed := ClosedPool(t)
	if n, err := sweepStaleIsolatedSchemas(context.Background(), closed, time.Now(), time.Hour, 5); err == nil || n != 0 {
		t.Fatalf("sweeping through a closed pool must fail without dropping anything, got n=%d err=%v", n, err)
	}
}

// Many tests finish at once and drop their schemas together. Before drops were serialised this
// exhausted Postgres' shared lock table ("out of shared memory") on a server running the default
// max_locks_per_transaction, and the schemas leaked. On a server tuned with a larger limit this
// passes either way; it is the regression test for default-configured CI databases.
func TestConcurrentSchemaDropsDoNotExhaustTheLockTable(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	const schemas = 12
	for i := 0; i < schemas; i++ {
		t.Run(fmt.Sprintf("schema-%02d", i), func(t *testing.T) {
			t.Parallel()
			pool := IsolatedSchemaPool(t)
			var n int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
				t.Fatalf("the schema must be fully migrated: %v", err)
			}
		})
	}
}

// Staleness is judged against the database's clock, not the test machine's.
func TestDatabaseNowIsTheServerClock(t *testing.T) {
	pool := lifecycleBasePool(t)
	got, err := databaseNow(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	// Only a sanity bound: a remote database's clock may legitimately differ by a little.
	if d := time.Since(got); d < -time.Hour || d > time.Hour {
		t.Fatalf("database clock %v is %v from this machine's; expected the current time", got, d)
	}
	if _, err := databaseNow(context.Background(), ClosedPool(t)); err == nil {
		t.Fatal("a closed pool cannot report the time")
	}
}
