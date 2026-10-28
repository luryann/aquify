package wellness

import (
	"errors"
	"testing"
	"time"
)

func fixedService(day string) *Service {
	s := NewService()
	t, _ := time.Parse(dateLayout, day)
	s.SetClock(func() time.Time { return t.Add(15 * time.Hour) })
	return s
}

func good() Metrics {
	return Metrics{SleepHours: 8.25, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 4}
}

func poor() Metrics {
	return Metrics{SleepHours: 5, SleepQuality: 2, Soreness: 4, Energy: 2, Stress: 4, Mood: 2}
}

func TestReadinessScore(t *testing.T) {
	if got := Readiness(Metrics{SleepQuality: 5, Soreness: 1, Energy: 5, Stress: 1, Mood: 5}); got != 5 {
		t.Fatalf("best case readiness = %v", got)
	}
	if got := Readiness(Metrics{SleepQuality: 1, Soreness: 5, Energy: 1, Stress: 5, Mood: 1}); got != 1 {
		t.Fatalf("worst case readiness = %v", got)
	}
	if got := Readiness(good()); got != 4 {
		t.Fatalf("good readiness = %v", got)
	}
}

func TestUpsertOnePerDayAndValidation(t *testing.T) {
	s := fixedService("2026-10-20")
	c, created, err := s.Upsert("org", Input{SwimmerID: "s1", Metrics: good(), Note: " tired legs "})
	if err != nil || !created {
		t.Fatalf("first upsert: %v created=%v", err, created)
	}
	if c.Date != "2026-10-20" || c.SleepHours != 8.3 || c.Note != "tired legs" {
		t.Fatalf("unexpected check-in %+v", c)
	}
	c2, created, err := s.Upsert("org", Input{SwimmerID: "s1", Metrics: poor()})
	if err != nil || created || c2.ID != c.ID {
		t.Fatalf("second upsert should replace: %v created=%v ids %s/%s", err, created, c.ID, c2.ID)
	}
	list, _ := s.List("org", nil, "2026-10-01", "2026-10-31")
	if len(list) != 1 || list[0].Mood != 2 {
		t.Fatalf("expected single replaced row, got %+v", list)
	}
	bad := []Input{
		{SwimmerID: "", Metrics: good()},
		{SwimmerID: "s1", Date: "2026-10-23", Metrics: good()},
		{SwimmerID: "s1", Date: "2026-10-01", Metrics: good()},
		{SwimmerID: "s1", Date: "20-10-2026", Metrics: good()},
		{SwimmerID: "s1", Metrics: Metrics{SleepHours: 25, SleepQuality: 3, Soreness: 3, Energy: 3, Stress: 3, Mood: 3}},
		{SwimmerID: "s1", Metrics: Metrics{SleepHours: 7, SleepQuality: 0, Soreness: 3, Energy: 3, Stress: 3, Mood: 3}},
		{SwimmerID: "s1", Metrics: Metrics{SleepHours: 7, SleepQuality: 3, Soreness: 6, Energy: 3, Stress: 3, Mood: 3}},
	}
	for i, in := range bad {
		if _, _, err := s.Upsert("org", in); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: expected ErrInvalid, got %v", i, err)
		}
	}
	// Tomorrow is allowed (timezones ahead of UTC); backfill within a week too.
	if _, _, err := s.Upsert("org", Input{SwimmerID: "s1", Date: "2026-10-21", Metrics: good()}); err != nil {
		t.Fatalf("tomorrow: %v", err)
	}
	if _, _, err := s.Upsert("org", Input{SwimmerID: "s1", Date: "2026-10-14", Metrics: good()}); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	other, _ := s.List("org2", nil, "2026-10-01", "2026-10-31")
	if len(other) != 0 {
		t.Fatal("org isolation broken")
	}
	only, _ := s.List("org", []string{"nobody"}, "2026-10-01", "2026-10-31")
	if len(only) != 0 {
		t.Fatal("swimmer filter broken")
	}
}

func series(dates []string, m Metrics) []*CheckIn {
	out := []*CheckIn{}
	for _, d := range dates {
		out = append(out, &CheckIn{SwimmerID: "s1", Date: d, Metrics: m})
	}
	return out
}

func hasFlag(flags []Flag, code string) bool {
	for _, f := range flags {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestFlags(t *testing.T) {
	low := series([]string{"2026-10-18", "2026-10-19", "2026-10-20"}, poor())
	flags := Flags(low, "2026-10-20")
	for _, code := range []string{"low_readiness_streak", "short_sleep_streak", "soreness_streak"} {
		if !hasFlag(flags, code) {
			t.Errorf("expected %s in %+v", code, flags)
		}
	}
	// A gap breaks the streak.
	gap := series([]string{"2026-10-16", "2026-10-19", "2026-10-20"}, poor())
	if hasFlag(Flags(gap, "2026-10-20"), "low_readiness_streak") {
		t.Error("gap should break the streak")
	}
	// Stale history does not flag.
	if len(Flags(low, "2026-10-25")) != 0 {
		t.Error("stale streak should not flag")
	}
	// Sharp drop against baseline.
	drop := series([]string{"2026-10-15", "2026-10-16", "2026-10-17", "2026-10-18"}, good())
	drop = append(drop, &CheckIn{SwimmerID: "s1", Date: "2026-10-19", Metrics: poor()})
	if !hasFlag(Flags(drop, "2026-10-20"), "sharp_drop") {
		t.Error("expected sharp_drop")
	}
	worst := []*CheckIn{{Date: "2026-10-20", Metrics: Metrics{SleepHours: 8, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 5, Mood: 3}}}
	if !hasFlag(Flags(worst, "2026-10-20"), "check_in_with_athlete") {
		t.Error("expected check_in_with_athlete for max stress")
	}
	if len(Flags(series([]string{"2026-10-19", "2026-10-20"}, good()), "2026-10-20")) != 0 {
		t.Error("healthy swimmer should not be flagged")
	}
}

func TestAverageAndDateRange(t *testing.T) {
	a := Average(append(series([]string{"2026-10-19"}, good()), series([]string{"2026-10-20"}, poor())...))
	if a.Count != 2 || a.Readiness != 3 || a.SleepHours != 6.6 {
		t.Fatalf("unexpected averages %+v", a)
	}
	from, to, dates := DateRange("2026-10-20", 14)
	if from != "2026-10-07" || to != "2026-10-20" || len(dates) != 14 {
		t.Fatalf("range %s..%s (%d)", from, to, len(dates))
	}
}
