package testutil

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Isolated test schemas ("iso_test_<unix-seconds>_<uuid>") are created per test and dropped
// in t.Cleanup. Two things used to leak them:
//
//  1. DROP SCHEMA ... CASCADE on a schema with the full migration set takes roughly a thousand
//     catalog locks. Many test binaries dropping at once exhausted Postgres' shared lock table
//     ("out of shared memory", SQLSTATE 53200) at its default max_locks_per_transaction, so the
//     drop failed and the schema stayed behind. Drops are now serialised with an advisory lock
//     taken in the same transaction as the DROP.
//  2. A test binary killed mid-run (timeout, SIGQUIT, a crashed CI job) never reaches t.Cleanup.
//     The creation time is now part of the name, so the next test process can remove schemas
//     that are far older than any test run.

const (
	isoSchemaPrefix = "iso_test_"

	// isoSchemaDropLockKey is the advisory-lock key that serialises isolated-schema drops across
	// every test process sharing one database.
	isoSchemaDropLockKey int64 = 0x4151495344 // "AQISD"

	// staleIsoSchemaAge is how old a timestamped schema must be before another process may drop
	// it. It is far above any test timeout, so a live run's schemas are never touched, and any
	// leak inside a single run still shows up as a leak.
	staleIsoSchemaAge = 6 * time.Hour

	// staleIsoSchemaSweepLimit bounds how many stale schemas one test process removes, so the
	// first test after a long outage is not held up clearing thousands of them.
	staleIsoSchemaSweepLimit = 25

	isoDropAttempts = 4
	isoDropBackoff  = 250 * time.Millisecond

	// isoDropTimeout bounds one schema drop including its lock wait and retries; isoSweepTimeout
	// bounds a whole sweep.
	isoDropTimeout  = time.Minute
	isoSweepTimeout = 3 * time.Minute
)

var isoSchemaNamePattern = regexp.MustCompile(`^iso_test_(\d{9,12})_[0-9a-f]{32}$`)

var staleIsoSweepOnce sync.Once

// newIsolatedSchemaName returns a unique schema name that records its creation time.
func newIsolatedSchemaName(now time.Time) string {
	return fmt.Sprintf("%s%d_%s", isoSchemaPrefix, now.Unix(), strings.ReplaceAll(uuid.NewString(), "-", ""))
}

// isolatedSchemaCreatedAt returns when a schema created by newIsolatedSchemaName was created.
// Names without a timestamp (the older "iso_test_<uuid>" form, or anything else) report false:
// their age is unknown, so they are never swept automatically.
func isolatedSchemaCreatedAt(name string) (time.Time, bool) {
	m := isoSchemaNamePattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	secs, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(secs, 0), true
}

// selectStaleIsolatedSchemas returns up to limit schemas from names whose recorded creation
// time is older than maxAge at now, oldest first.
func selectStaleIsolatedSchemas(names []string, now time.Time, maxAge time.Duration, limit int) []string {
	type aged struct {
		name string
		at   time.Time
	}
	var stale []aged
	for _, n := range names {
		if at, ok := isolatedSchemaCreatedAt(n); ok && now.Sub(at) > maxAge {
			stale = append(stale, aged{n, at})
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].at.Before(stale[j].at) })
	if limit > 0 && len(stale) > limit {
		stale = stale[:limit]
	}
	out := make([]string, len(stale))
	for i, s := range stale {
		out[i] = s.name
	}
	return out
}

func isLockTableExhausted(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "53200"
}

// dropIsolatedSchema drops a schema (and everything in it) under the cross-process drop lock.
// A drop that still hits the lock-table limit (other transactions are using it too) is retried
// a few times with a short backoff.
func dropIsolatedSchema(ctx context.Context, base *pgxpool.Pool, schema string) error {
	return retryOnLockTableExhaustion(ctx, isoDropAttempts, isoDropBackoff, func() error {
		return dropIsolatedSchemaOnce(ctx, base, schema)
	})
}

// retryOnLockTableExhaustion runs fn up to attempts times, waiting backoff*attempt between tries,
// and retries only when Postgres reports the shared lock table is full. Every other error, and
// a cancelled context, is returned straight away.
func retryOnLockTableExhaustion(ctx context.Context, attempts int, backoff time.Duration, fn func() error) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = fn(); err == nil || !isLockTableExhausted(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt) * backoff):
		}
	}
	return err
}

func dropIsolatedSchemaOnce(ctx context.Context, base *pgxpool.Pool, schema string) error {
	tx, err := base.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Transaction-scoped: released at COMMIT/ROLLBACK on the same connection as the DROP.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, isoSchemaDropLockKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// dropIsolatedSchemaOrFail is the t.Cleanup body for a test's schema: it drops it and fails the
// test (rather than hiding the leak) when the drop cannot be completed.
func dropIsolatedSchemaOrFail(t testing.TB, base *pgxpool.Pool, schema string) {
	t.Helper()
	// Bounded, so a drop stuck behind a lock held by a leaked connection fails this test
	// instead of stalling every other test's cleanup until the go test timeout.
	ctx, cancel := context.WithTimeout(context.Background(), isoDropTimeout)
	defer cancel()
	if err := dropIsolatedSchema(ctx, base, schema); err != nil {
		t.Errorf("drop isolated schema %s: %v", schema, err)
	}
}

// sweepStaleIsolatedSchemas drops timestamped schemas older than maxAge (at most limit of
// them) and returns how many it dropped. It only ever touches names created by
// newIsolatedSchemaName.
func sweepStaleIsolatedSchemas(ctx context.Context, base *pgxpool.Pool, now time.Time, maxAge time.Duration, limit int) (int, error) {
	rows, err := base.Query(ctx, `SELECT nspname FROM pg_namespace WHERE nspname LIKE 'iso\_test\_%' ESCAPE '\'`)
	if err != nil {
		return 0, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return 0, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	dropped := 0
	for _, name := range selectStaleIsolatedSchemas(names, now, maxAge, limit) {
		if err := dropIsolatedSchema(ctx, base, name); err != nil {
			return dropped, fmt.Errorf("sweep %s: %w", name, err)
		}
		dropped++
	}
	return dropped, nil
}

// sweepStaleIsolatedSchemasOnce runs one bounded sweep per test process. A failed sweep is only
// logged: it is housekeeping and not what the calling test is checking.
func sweepStaleIsolatedSchemasOnce(t testing.TB, base *pgxpool.Pool) {
	staleIsoSweepOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), isoSweepTimeout)
		defer cancel()
		// Schema names record the creating machine's clock; compare them with the database's
		// clock, which every runner sharing this database agrees on, so a skewed runner clock
		// cannot make a live schema look stale.
		now, err := databaseNow(ctx, base)
		if err != nil {
			t.Logf("sweeping stale isolated schemas: %v", err)
			return
		}
		n, err := sweepStaleIsolatedSchemas(ctx, base, now, staleIsoSchemaAge, staleIsoSchemaSweepLimit)
		switch {
		case err != nil:
			t.Logf("sweeping stale isolated schemas: %v", err)
		case n > 0:
			t.Logf("dropped %d isolated schemas left by earlier killed test runs", n)
		}
	})
}

// databaseNow returns the database server's current time.
func databaseNow(ctx context.Context, base *pgxpool.Pool) (time.Time, error) {
	var secs int64
	if err := base.QueryRow(ctx, `SELECT extract(epoch FROM now())::bigint`).Scan(&secs); err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0), nil
}
