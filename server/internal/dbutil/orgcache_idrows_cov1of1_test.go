package dbutil_test

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1x_NewHexIDIsLowercase32HexAndUnique(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := dbutil.NewHexID()
		if !re.MatchString(id) {
			t.Fatalf("id %q is not 32 lowercase hex chars", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestCov1x_OrgCacheDefaultsAndTouchEvictsIdle(t *testing.T) {
	var c dbutil.OrgCache
	if c.Len() != 0 {
		t.Fatalf("zero value tracks nothing, got %d", c.Len())
	}
	// Unconfigured: defaults apply, so an org idle past the default TTL is evicted.
	old := time.Now().Add(-dbutil.DefaultOrgCacheTTL - time.Minute)
	if ev := c.Use("old", old); len(ev) != 0 {
		t.Fatalf("first use evicts nothing: %v", ev)
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
	var dropped []string
	c.Touch("fresh", func(o string) { dropped = append(dropped, o) })
	if len(dropped) != 1 || dropped[0] != "old" {
		t.Fatalf("dropped = %v, want [old]", dropped)
	}
	if c.Len() != 1 {
		t.Fatalf("only fresh remains, Len = %d", c.Len())
	}
}

func TestCov1x_OrgCacheDefaultMaxCap(t *testing.T) {
	var c dbutil.OrgCache
	now := time.Now()
	total := dbutil.DefaultOrgCacheMax + 3
	var evicted []string
	for i := 0; i < total; i++ {
		evicted = append(evicted, c.Use(string(rune('A'+i/26))+string(rune('a'+i%26)), now.Add(time.Duration(i)*time.Millisecond))...)
	}
	if len(evicted) != 3 || c.Len() != dbutil.DefaultOrgCacheMax {
		t.Fatalf("evicted %v, Len %d", evicted, c.Len())
	}
	sort.Strings(evicted)
	want := []string{"Aa", "Ab", "Ac"} // least recently used go first
	for i := range want {
		if evicted[i] != want[i] {
			t.Fatalf("evicted %v, want %v", evicted, want)
		}
	}
}

func TestCov1x_OrgCacheSetLimitsOverridesDefaults(t *testing.T) {
	var c dbutil.OrgCache
	c.SetLimits(0, 2) // no idle TTL, cap of 2
	t0 := time.Now().Add(-48 * time.Hour)
	c.Use("a", t0)
	c.Use("b", t0.Add(time.Second))
	// Far older than the default TTL but TTL is disabled; only the cap applies.
	ev := c.Use("c", t0.Add(2*time.Second))
	if len(ev) != 1 || ev[0] != "a" || c.Len() != 2 {
		t.Fatalf("evicted %v, Len %d; want only LRU a", ev, c.Len())
	}
}

type cov1xRecordingLocker struct {
	locks, unlocks int
	heldDuring     bool
}

func (l *cov1xRecordingLocker) Lock()   { l.locks++ }
func (l *cov1xRecordingLocker) Unlock() { l.unlocks++ }

func TestCov1x_OrgCacheConfigureLockedHoldsLockAndApplies(t *testing.T) {
	var c dbutil.OrgCache
	l := &cov1xRecordingLocker{}
	c.ConfigureLocked(l, 0, 1)
	if l.locks != 1 || l.unlocks != 1 {
		t.Fatalf("locks=%d unlocks=%d", l.locks, l.unlocks)
	}
	now := time.Now()
	c.Use("x", now)
	ev := c.Use("y", now.Add(time.Second))
	if len(ev) != 1 || ev[0] != "x" {
		t.Fatalf("cap of 1 should evict x, got %v", ev)
	}

	// Works with a real sync.Locker too.
	var mu sync.Mutex
	var c2 dbutil.OrgCache
	c2.ConfigureLocked(&mu, time.Hour, 0)
	if !mu.TryLock() {
		t.Fatal("mutex must be released after ConfigureLocked")
	}
	mu.Unlock()
	c2.Use("p", now.Add(-2*time.Hour))
	if ev := c2.Use("q", now); len(ev) != 1 || ev[0] != "p" {
		t.Fatalf("1h ttl should evict p, got %v", ev)
	}
}

func TestCov1x_ReloadUnlockedPropagatesLoadError(t *testing.T) {
	var mu dbutil.GenMutex
	boom := errors.New("db down")
	hooked, swapped := 0, 0
	done, err := dbutil.ReloadUnlocked(&mu, func() { hooked++ }, func() (func(), error) {
		return func() { swapped++ }, boom
	})
	if done || !errors.Is(err, boom) || hooked != 0 || swapped != 0 {
		t.Fatalf("done=%v err=%v hooked=%d swapped=%d", done, err, hooked, swapped)
	}
}

func TestCov1x_ReloadUnlockedRunsHookAfterEachReadAndRetries(t *testing.T) {
	var mu dbutil.GenMutex
	hooks, swaps := 0, 0
	done, err := dbutil.ReloadUnlocked(&mu, func() {
		hooks++
		if hooks == 1 { // a writer finishes after the first read
			mu.Lock()
			mu.Unlock()
		}
	}, func() (func(), error) { return func() { swaps++ }, nil })
	if err != nil || !done || hooks != 2 || swaps != 1 {
		t.Fatalf("done=%v err=%v hooks=%d swaps=%d", done, err, hooks, swaps)
	}
	// The swap leaves the generation untouched.
	g := mu.Gen()
	if _, err := dbutil.ReloadUnlocked(&mu, nil, func() (func(), error) { return func() {}, nil }); err != nil {
		t.Fatal(err)
	}
	if mu.Gen() != g {
		t.Fatalf("swap must not bump generation: %d -> %d", g, mu.Gen())
	}
}

func TestCov1x_ListOrgIDsReturnsSortedRegisteredOrgs(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	ctx := context.Background()
	a := "00000000-0000-0000-0000-00000000c1a1"
	b := "00000000-0000-0000-0000-00000000c1b2"
	testutil.SeedOrgs(t, pool, b, a)
	ids, err := dbutil.ListOrgIDs(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, id := range ids {
		if id == a || id == b {
			got = append(got, id)
		}
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("seeded orgs = %v, want [%s %s] in id order (all: %v)", got, a, b, ids)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("ids must be ordered: %v", ids)
	}
}

type cov1xQuerier struct {
	query func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (q cov1xQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.query(ctx, sql, args...)
}

func TestCov1x_ListOrgIDsQueryErrorIsWrapped(t *testing.T) {
	boom := errors.New("connection refused")
	_, err := dbutil.ListOrgIDs(context.Background(), cov1xQuerier{query: func(context.Context, string, ...any) (pgx.Rows, error) {
		return nil, boom
	}})
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "list orgs: ") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1x_ListOrgIDsScanErrorIsWrapped(t *testing.T) {
	pool := testutil.IsolatedSchemaPoolStrict(t)
	// Substitute a query yielding NULL, which cannot scan into a string.
	q := cov1xQuerier{query: func(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
		return pool.Query(ctx, `SELECT NULL::text`)
	}}
	ids, err := dbutil.ListOrgIDs(context.Background(), q)
	if err == nil || !strings.HasPrefix(err.Error(), "scan org: ") || ids != nil {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}
