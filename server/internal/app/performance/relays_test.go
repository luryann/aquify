package performance

import (
	"errors"
	"testing"
)

func cand(id, gender string, times map[string]int) RelayCandidate {
	return RelayCandidate{SwimmerID: id, Name: id, Gender: gender, Times: times}
}

func teamIDs(t RelayTeam) []string {
	out := make([]string, 0, len(t.Legs))
	for _, l := range t.Legs {
		out = append(out, l.SwimmerID)
	}
	return out
}

// bruteForceMedley returns the minimal 4-swimmer medley total by exhaustive search.
func bruteForceMedley(pool []RelayCandidate, mixed bool) int {
	strokes := LegStrokes("medley")
	best := -1
	n := len(pool)
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			for c := 0; c < n; c++ {
				for d := 0; d < n; d++ {
					idx := []int{a, b, c, d}
					seen := map[int]bool{}
					ok := true
					sum, men := 0, 0
					for leg, i := range idx {
						if seen[i] {
							ok = false
							break
						}
						seen[i] = true
						tm, has := pool[i].Times[strokes[leg]]
						if !has {
							ok = false
							break
						}
						sum += tm
						if pool[i].Gender == "M" {
							men++
						}
					}
					if !ok || (mixed && men != 2) {
						continue
					}
					if best < 0 || sum < best {
						best = sum
					}
				}
			}
		}
	}
	return best
}

func TestOptimizeMedleyBeatsGreedyStrokeAssignment(t *testing.T) {
	// "ace" is the fastest backstroker AND the fastest flyer. Greedy per-stroke
	// assignment (fill back first) puts ace on back and forces a slow flyer;
	// the optimum puts ace on fly where the gap to the next swimmer is larger.
	pool := []RelayCandidate{
		cand("ace", "F", map[string]int{"back": 30000, "fly": 28000, "free": 27000}),
		cand("bea", "F", map[string]int{"back": 30500, "breast": 36000, "free": 27500}),
		cand("cat", "F", map[string]int{"breast": 34000, "fly": 33000, "free": 28000}),
		cand("dee", "F", map[string]int{"free": 26500, "fly": 34000}),
		cand("eve", "F", map[string]int{"back": 33000, "breast": 37000, "fly": 35000, "free": 29000}),
	}
	got, err := OptimizeRelays(RelaySpec{RelayType: "medley", Distance: 200, Gender: "F", Teams: 1}, pool, nil, nil)
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	if len(got.Teams) != 1 {
		t.Fatalf("expected 1 team, got %+v", got)
	}
	want := bruteForceMedley(pool, false)
	if got.Teams[0].TotalMs != want {
		t.Fatalf("total %d, want optimum %d (legs %v)", got.Teams[0].TotalMs, want, teamIDs(got.Teams[0]))
	}
	if got.Teams[0].Legs[2].SwimmerID != "ace" {
		t.Fatalf("expected ace on fly, got %v", teamIDs(got.Teams[0]))
	}
}

func TestOptimizeMedleyMatchesBruteForceOnManyPools(t *testing.T) {
	// Deterministic pseudo-random pools; compare against exhaustive search
	// for both single-gender and mixed relays.
	seed := uint32(7)
	next := func() int {
		seed = seed*1664525 + 1013904223
		return int(seed >> 8)
	}
	strokes := []string{"back", "breast", "fly", "free"}
	for trial := 0; trial < 60; trial++ {
		var pool []RelayCandidate
		for i := 0; i < 9; i++ {
			times := map[string]int{}
			for _, st := range strokes {
				if next()%5 != 0 { // some swimmers lack some strokes
					times[st] = 25000 + next()%12000
				}
			}
			g := "F"
			if next()%2 == 0 {
				g = "M"
			}
			pool = append(pool, cand(string(rune('a'+i)), g, times))
		}
		for _, mixed := range []bool{false, true} {
			gender := "X"
			sub := pool
			if !mixed {
				gender = "F"
				sub = nil
				for _, c := range pool {
					if c.Gender == "F" {
						sub = append(sub, c)
					}
				}
			}
			want := bruteForceMedley(sub, mixed)
			got, err := OptimizeRelays(RelaySpec{RelayType: "medley", Distance: 200, Gender: gender, Teams: 1}, pool, nil, nil)
			if err != nil {
				t.Fatalf("trial %d: %v", trial, err)
			}
			if want < 0 {
				if len(got.Teams) != 0 {
					t.Fatalf("trial %d mixed=%v: expected no feasible relay, got %+v", trial, mixed, got.Teams)
				}
				continue
			}
			if len(got.Teams) != 1 || got.Teams[0].TotalMs != want {
				t.Fatalf("trial %d mixed=%v: got %+v want total %d", trial, mixed, got.Teams, want)
			}
		}
	}
}

func TestOptimizeFreeRelayABCAndLegOrder(t *testing.T) {
	var pool []RelayCandidate
	for i := 0; i < 13; i++ {
		pool = append(pool, cand(string(rune('a'+i)), "M", map[string]int{"free": 50000 + i*100}))
	}
	got, err := OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "M", Teams: 3}, pool, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Teams) != 3 {
		t.Fatalf("expected A/B/C, got %d", len(got.Teams))
	}
	// A = a,b,c,d ordered: leadoff b (2nd), c, d, anchor a (fastest).
	if ids := teamIDs(got.Teams[0]); ids[0] != "b" || ids[1] != "c" || ids[2] != "d" || ids[3] != "a" {
		t.Fatalf("unexpected A order %v", ids)
	}
	if got.Teams[1].TotalMs <= got.Teams[0].TotalMs || got.Teams[2].TotalMs <= got.Teams[1].TotalMs {
		t.Fatalf("expected A < B < C totals")
	}
	if got.Teams[2].Label != "C" {
		t.Fatalf("expected label C, got %s", got.Teams[2].Label)
	}
}

func TestOptimizeRespectsLocksAndExclusions(t *testing.T) {
	pool := []RelayCandidate{
		cand("a", "F", map[string]int{"free": 50000}),
		cand("b", "F", map[string]int{"free": 50100}),
		cand("c", "F", map[string]int{"free": 50200}),
		cand("d", "F", map[string]int{"free": 50300}),
		cand("e", "F", map[string]int{"free": 50400}),
		cand("f", "F", map[string]int{"free": 50500}),
		cand("g", "F", map[string]int{"free": 50600}),
		cand("h", "F", map[string]int{"free": 50700}),
		cand("i", "F", map[string]int{"free": 50800}),
	}
	// Coach locks the slowest swimmer as B's anchor and the fastest as B's
	// lead-off; "c" is excluded (sick).
	locks := []RelayLegLock{{Team: 1, Leg: 4, SwimmerID: "h"}, {Team: 1, Leg: 1, SwimmerID: "a"}}
	got, err := OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 2}, pool, locks, map[string]bool{"c": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Teams) != 2 {
		t.Fatalf("expected 2 teams, got %+v", got)
	}
	a := map[string]bool{}
	for _, id := range teamIDs(got.Teams[0]) {
		a[id] = true
	}
	if a["a"] || a["h"] || a["c"] || !a["b"] || !a["d"] || !a["e"] || !a["f"] {
		t.Fatalf("relay A should be b,d,e,f, got %v", teamIDs(got.Teams[0]))
	}
	bLegs := got.Teams[1].Legs
	if bLegs[0].SwimmerID != "a" || !bLegs[0].Locked || bLegs[3].SwimmerID != "h" || !bLegs[3].Locked {
		t.Fatalf("locks not honored: %+v", bLegs)
	}
	for _, l := range bLegs {
		if l.SwimmerID == "c" {
			t.Fatal("excluded swimmer was used")
		}
	}
}

func TestOptimizeMixedRequiresTwoAndTwo(t *testing.T) {
	// Four fastest are all men; mixed relay must still use exactly 2 + 2.
	pool := []RelayCandidate{
		cand("m1", "M", map[string]int{"free": 45000}),
		cand("m2", "M", map[string]int{"free": 45100}),
		cand("m3", "M", map[string]int{"free": 45200}),
		cand("m4", "M", map[string]int{"free": 45300}),
		cand("f1", "F", map[string]int{"free": 52000}),
		cand("f2", "F", map[string]int{"free": 52100}),
	}
	got, err := OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "X", Teams: 2}, pool, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Teams) != 1 {
		t.Fatalf("only one mixed relay is possible (two women), got %d", len(got.Teams))
	}
	men := 0
	for _, l := range got.Teams[0].Legs {
		if l.Gender == "M" {
			men++
		}
	}
	if men != 2 || got.Teams[0].TotalMs != 45000+45100+52000+52100 {
		t.Fatalf("unexpected mixed relay %+v", got.Teams[0])
	}
	if len(got.Warnings) == 0 {
		t.Fatal("expected a warning about relay B")
	}
	// Locking three men on a mixed relay is invalid.
	_, err = OptimizeRelays(RelaySpec{RelayType: "free", Distance: 200, Gender: "X", Teams: 1}, pool,
		[]RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "m1"}, {Team: 0, Leg: 2, SwimmerID: "m2"}, {Team: 0, Leg: 3, SwimmerID: "m3"}}, nil)
	if !errors.Is(err, ErrRelayInvalid) {
		t.Fatalf("expected ErrRelayInvalid for 3 locked men, got %v", err)
	}
}

func TestOptimizeMedleyLockForcesSuboptimalButBestRemaining(t *testing.T) {
	pool := []RelayCandidate{
		cand("ace", "M", map[string]int{"back": 25000, "fly": 24000}),
		cand("bob", "M", map[string]int{"back": 27000, "breast": 29000}),
		cand("cal", "M", map[string]int{"breast": 28000, "fly": 26000}),
		cand("dan", "M", map[string]int{"free": 22000, "fly": 25500}),
		cand("eli", "M", map[string]int{"free": 23000, "back": 26000}),
	}
	// Lock ace on backstroke; optimizer must solve the rest around him.
	got, err := OptimizeRelays(RelaySpec{RelayType: "medley", Distance: 200, Gender: "M", Teams: 1}, pool,
		[]RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "ace"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	legs := got.Teams[0].Legs
	if legs[0].SwimmerID != "ace" {
		t.Fatalf("lock ignored: %v", teamIDs(got.Teams[0]))
	}
	// Remaining best: breast bob 29000 / cal 28000; fly cal 26000 / dan 25500;
	// free dan 22000 / eli 23000. Options: cal breast + dan fly + eli free =
	// 28000+25500+23000 = 76500; bob breast + cal fly + dan free = 29000+26000+22000 = 77000.
	if got.Teams[0].TotalMs != 25000+76500 {
		t.Fatalf("total %d legs %v", got.Teams[0].TotalMs, teamIDs(got.Teams[0]))
	}
}

func TestOptimizeRejectsBadLocksAndSpecs(t *testing.T) {
	pool := []RelayCandidate{cand("a", "F", map[string]int{"free": 50000})}
	cases := []struct {
		spec  RelaySpec
		locks []RelayLegLock
	}{
		{RelaySpec{RelayType: "medley", Distance: 800, Gender: "F", Teams: 1}, nil},
		{RelaySpec{RelayType: "free", Distance: 300, Gender: "F", Teams: 1}, nil},
		{RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 0}, nil},
		{RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 1}, []RelayLegLock{{Team: 0, Leg: 5, SwimmerID: "a"}}},
		{RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 1}, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "zz"}}},
		{RelaySpec{RelayType: "free", Distance: 200, Gender: "F", Teams: 2}, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "a"}, {Team: 1, Leg: 1, SwimmerID: "a"}}},
		{RelaySpec{RelayType: "medley", Distance: 200, Gender: "F", Teams: 1}, []RelayLegLock{{Team: 0, Leg: 1, SwimmerID: "a"}}}, // no back time
	}
	for i, c := range cases {
		if _, err := OptimizeRelays(c.spec, pool, c.locks, nil); !errors.Is(err, ErrRelayInvalid) {
			t.Fatalf("case %d: expected ErrRelayInvalid, got %v", i, err)
		}
	}
}
