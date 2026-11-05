package dbutil

import (
	"sync"
	"time"
)

// MaxCacheAge bounds how long a club's cache is trusted even when the change counters say no
// other process has written (a safety net against any drift between a write-through cache and
// the database).
const MaxCacheAge = 5 * time.Minute

// ForeignClock remembers, per org, the foreign-write count (tenantdb.Pool.ForeignWrites) a
// cache was loaded at. A cache is current while that count is unchanged and the load is
// younger than MaxCacheAge. The zero value is ready to use.
type ForeignClock struct{ m sync.Map } // org id -> stamp

type stamp struct {
	foreign int64
	at      time.Time
}

// Current reports whether org's cache was loaded at this foreign-write count, recently enough.
func (c *ForeignClock) Current(org string, foreign int64) bool {
	v, ok := c.m.Load(org)
	if !ok {
		return false
	}
	s := v.(stamp)
	return s.foreign == foreign && time.Since(s.at) < MaxCacheAge
}

// Mark records a full load of org's cache at this foreign-write count.
func (c *ForeignClock) Mark(org string, foreign int64) { c.m.Store(org, stamp{foreign, time.Now()}) }

// Forget drops org's record (its cache was dropped or reloaded outside the probe path), so
// the next check does a full load.
func (c *ForeignClock) Forget(org string) { c.m.Delete(org) }
