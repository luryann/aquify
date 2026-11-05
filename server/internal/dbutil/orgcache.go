package dbutil

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Default bounds for the per-org service caches (see OrgActivity).
const (
	DefaultOrgCacheTTL = 30 * time.Minute
	DefaultOrgCacheMax = 200
)

// OrgActivity tracks when each org's rows were last served from a service cache,
// so the cache can be bounded: orgs idle past a TTL, or beyond a maximum count
// (least recently used first), are evicted. Eviction is safe because every request
// path reloads its own org from the database before reading (syncFromDB).
type OrgActivity struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func NewOrgActivity() *OrgActivity { return &OrgActivity{last: map[string]time.Time{}} }

// Use records that org was just served and returns the orgs to evict now. The org
// being served is never evicted. ttl <= 0 disables idle eviction; maxOrgs <= 0
// disables the count cap. Returned orgs are forgotten here; the caller drops their rows.
func (a *OrgActivity) Use(org string, now time.Time, ttl time.Duration, maxOrgs int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.last[org] = now
	var evict []string
	if ttl > 0 {
		for o, t := range a.last {
			if o != org && now.Sub(t) > ttl {
				evict = append(evict, o)
			}
		}
		for _, o := range evict {
			delete(a.last, o)
		}
	}
	if maxOrgs > 0 && len(a.last) > maxOrgs {
		others := make([]string, 0, len(a.last))
		for o := range a.last {
			if o != org {
				others = append(others, o)
			}
		}
		sort.Slice(others, func(i, j int) bool { return a.last[others[i]].Before(a.last[others[j]]) })
		for _, o := range others[:len(a.last)-maxOrgs] {
			evict = append(evict, o)
			delete(a.last, o)
		}
	}
	return evict
}

// Len is how many orgs are currently tracked.
func (a *OrgActivity) Len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.last)
}

// OrgCache bundles OrgActivity with its limits for embedding in a service. The zero
// value applies the defaults; SetLimits overrides them (0 disables a limit). It is
// used under the owning service's write lock, so it needs no lock of its own.
type OrgCache struct {
	act        *OrgActivity
	ttl        time.Duration
	max        int
	configured bool
}

// SetLimits sets the idle TTL and the maximum number of cached orgs (0 = unlimited).
func (c *OrgCache) SetLimits(ttl time.Duration, maxOrgs int) {
	c.ttl, c.max, c.configured = ttl, maxOrgs, true
}

// ConfigureLocked is SetLimits under the service's own lock, for the wiring call that
// configures a running service (OrgCache is otherwise used under that same lock).
func (c *OrgCache) ConfigureLocked(mu sync.Locker, ttl time.Duration, maxOrgs int) {
	mu.Lock()
	defer mu.Unlock()
	c.SetLimits(ttl, maxOrgs)
}

// Use records that org was just served and returns the orgs whose rows to drop.
func (c *OrgCache) Use(org string, now time.Time) []string {
	if c.act == nil {
		c.act = NewOrgActivity()
	}
	ttl, maxOrgs := c.ttl, c.max
	if !c.configured {
		ttl, maxOrgs = DefaultOrgCacheTTL, DefaultOrgCacheMax
	}
	return c.act.Use(org, now, ttl, maxOrgs)
}

// Touch records that org was just served and calls drop for every org evicted by the
// idle TTL or the count cap. The caller holds the owning service's write lock.
func (c *OrgCache) Touch(org string, drop func(org string)) {
	for _, evicted := range c.Use(org, time.Now()) {
		drop(evicted)
	}
}

// Len is how many orgs are currently tracked.
func (c *OrgCache) Len() int {
	if c.act == nil {
		return 0
	}
	return c.act.Len()
}

// GenMutex is an RWMutex that counts completed write-lock sections, so a reader of
// slow external state (a database reload) can load without holding the lock and then
// detect, at swap time, that a writer ran in the meantime.
type GenMutex struct {
	sync.RWMutex
	gen atomic.Uint64
}

// Unlock releases the write lock and records that a writer finished.
func (m *GenMutex) Unlock() {
	m.gen.Add(1)
	m.RWMutex.Unlock()
}

// UnlockWithoutBump releases the write lock without counting a writer. Used by the
// reload swap itself, which replaces one org's cache with state read from the database
// and must not invalidate other reloads.
func (m *GenMutex) UnlockWithoutBump() { m.RWMutex.Unlock() }

// Gen returns the number of writer sections completed so far.
func (m *GenMutex) Gen() uint64 { return m.gen.Load() }

// MaxLockFreeReloads is how many times ReloadUnlocked retries after a writer
// intervened before it gives up and lets the caller load under the lock.
const MaxLockFreeReloads = 3

// ReloadUnlocked refreshes cached state from the database without holding mu during the
// (slow) read, so one org's reload does not block other orgs. load reads the database and
// returns a swap that replaces the cache; the swap runs under the write lock only if no
// writer finished since the read began (otherwise the snapshot may be stale and is
// re-read). It reports done=false after MaxLockFreeReloads conflicts, and the caller must
// then load under the write lock. hook, if non-nil, runs after each read (tests).
func ReloadUnlocked(mu *GenMutex, hook func(), load func() (swap func(), err error)) (done bool, err error) {
	for attempt := 0; attempt < MaxLockFreeReloads; attempt++ {
		gen := mu.Gen()
		swap, err := load()
		if err != nil {
			return false, err
		}
		if hook != nil {
			hook()
		}
		mu.Lock()
		if mu.Gen() != gen {
			mu.UnlockWithoutBump()
			continue
		}
		swap()
		mu.UnlockWithoutBump()
		return true, nil
	}
	return false, nil
}
