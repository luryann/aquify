package dbutil

import (
	"sort"
	"testing"
	"time"
)

func TestOrgActivityEvictsIdleAndLeastRecentlyUsed(t *testing.T) {
	a := NewOrgActivity()
	t0 := time.Unix(1000, 0)
	for i, o := range []string{"a", "b", "c"} {
		if ev := a.Use(o, t0.Add(time.Duration(i)*time.Second), time.Hour, 0); len(ev) != 0 {
			t.Fatalf("nothing is idle yet: %v", ev)
		}
	}
	// Idle eviction: a and b are older than the TTL when d is served.
	ev := a.Use("d", t0.Add(90*time.Minute), time.Hour, 0)
	sort.Strings(ev)
	if len(ev) != 3 || ev[0] != "a" || ev[1] != "b" || ev[2] != "c" {
		t.Fatalf("idle orgs must be evicted, got %v", ev)
	}
	if a.Len() != 1 {
		t.Fatalf("only d remains tracked: %d", a.Len())
	}
	// Count cap: LRU goes first and the served org is never evicted.
	b := NewOrgActivity()
	for i, o := range []string{"x", "y", "z", "w"} {
		b.Use(o, t0.Add(time.Duration(i)*time.Second), 0, 0)
	}
	ev = b.Use("y", t0.Add(10*time.Second), 0, 2) // y refreshed; x, z, w older
	sort.Strings(ev)
	if b.Len() != 2 || len(ev) != 2 || ev[0] != "w" && ev[0] != "x" && ev[0] != "z" {
		t.Fatalf("cap of 2 must keep the served org and the most recent other: evicted %v, tracked %d", ev, b.Len())
	}
	for _, e := range ev {
		if e == "y" {
			t.Fatal("the org being served must never be evicted")
		}
	}
	if ev2 := b.Use("y", t0.Add(11*time.Second), 0, 0); len(ev2) != 0 {
		t.Fatalf("no limits, no eviction: %v", ev2)
	}
}

func TestReloadUnlockedRetriesAfterWriterThenGivesUp(t *testing.T) {
	var mu GenMutex
	loads, swaps := 0, 0
	// A writer finishes during every read: the snapshot is never swapped in.
	done, err := ReloadUnlocked(&mu, nil, func() (func(), error) {
		loads++
		mu.Lock()
		mu.Unlock() // bumps the generation
		return func() { swaps++ }, nil
	})
	if err != nil || done || loads != MaxLockFreeReloads || swaps != 0 {
		t.Fatalf("done=%v err=%v loads=%d swaps=%d", done, err, loads, swaps)
	}
	// No writer: swapped on the first attempt.
	loads, swaps = 0, 0
	done, err = ReloadUnlocked(&mu, nil, func() (func(), error) { loads++; return func() { swaps++ }, nil })
	if err != nil || !done || loads != 1 || swaps != 1 {
		t.Fatalf("done=%v err=%v loads=%d swaps=%d", done, err, loads, swaps)
	}
}
