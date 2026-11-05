package orgsettings

import (
	"errors"
	"sync"
	"testing"
)

// Concurrent patches to different fields must both land; before Update was
// serialised, each wrote back a stale snapshot and one change was lost.
func TestUpdateConcurrentPatchesDoNotLoseFields(t *testing.T) {
	t.Parallel()

	for round := 0; round < 200; round++ {
		svc := NewService()
		code, name := "aquify", "aquify Aquatics"
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := svc.Update("org-1", Updates{TeamCode: &code}, "u1"); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := svc.Update("org-1", Updates{TeamName: &name}, "u2"); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()

		got, err := svc.Get("org-1")
		if err != nil {
			t.Fatal(err)
		}
		if got.TeamCode != code || got.TeamName != name {
			t.Fatalf("round %d: lost update, got code=%q name=%q", round, got.TeamCode, got.TeamName)
		}
	}
}

func TestUpdateRejectsDuplicateAndInvalidSlugs(t *testing.T) {
	t.Parallel()

	svc := NewService()
	slug := "aquify"
	if _, err := svc.Update("org-1", Updates{PublicSlug: &slug}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update("org-2", Updates{PublicSlug: &slug}, "u"); err != ErrSlugTaken {
		t.Fatalf("duplicate slug = %v, want ErrSlugTaken", err)
	}
	bad := "-x"
	if _, err := svc.Update("org-2", Updates{PublicSlug: &bad}, "u"); err != ErrInvalidSlug {
		t.Fatalf("invalid slug = %v, want ErrInvalidSlug", err)
	}
}

// The swimmer-accounts flag is written by the same Update call as the other
// fields, so a rejected update leaves it untouched and an accepted one applies
// everything together.
func TestUpdateAppliesSwimmerAccountsFlagAtomically(t *testing.T) {
	s := NewService()
	off, bad, name := false, "!!bad slug!!", "Team"
	if _, err := s.Update("org-1", Updates{TeamName: &name, PublicSlug: &bad, SwimmerAccountsEnabled: &off}, "u"); err == nil {
		t.Fatal("expected invalid slug error")
	}
	if on, _ := s.SwimmerAccountsEnabled("org-1"); !on {
		t.Fatal("flag must not change when the combined update is rejected")
	}
	got, err := s.Update("org-1", Updates{TeamName: &name, SwimmerAccountsEnabled: &off}, "u")
	if err != nil || got.TeamName != "Team" {
		t.Fatalf("update: %v %+v", err, got)
	}
	if on, _ := s.SwimmerAccountsEnabled("org-1"); on {
		t.Fatal("flag should be off after combined update")
	}
}

func TestPublicSlugCannotShadowAnotherOrgsRegistrySlug(t *testing.T) {
	svc := NewService()
	svc.SetSlugReserved(func(slug, orgID string) bool { return slug == "taken-club" && orgID != "org-owner" })
	slug := "taken-club"
	if _, err := svc.Update("org-other", Updates{PublicSlug: &slug}, ""); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("a slug the registry gives to another org must be refused: %v", err)
	}
	if _, err := svc.Update("org-owner", Updates{PublicSlug: &slug}, ""); err != nil {
		t.Fatalf("the owning org may use its own slug: %v", err)
	}
}

func TestProfileSettingsAreValidated(t *testing.T) {
	svc := NewService()
	str := func(s string) *string { return &s }
	if _, err := svc.Update("org-a", Updates{Timezone: str("Mars/Olympus")}, ""); !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("unknown timezone: %v", err)
	}
	if _, err := svc.Update("org-a", Updates{Currency: str("dollars")}, ""); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("bad currency: %v", err)
	}
	if _, err := svc.Update("org-a", Updates{LegalName: str(string(make([]rune, 121)))}, ""); !errors.Is(err, ErrInvalidLegalName) {
		t.Fatalf("long legal name: %v", err)
	}
	got, err := svc.Update("org-a", Updates{Timezone: str(" America/Los_Angeles "), Currency: str(" CAD "), LegalName: str(" Aqua Aquatics Inc. ")}, "")
	if err != nil || got.Timezone != "America/Los_Angeles" || got.Currency != "cad" || got.LegalName != "Aqua Aquatics Inc." {
		t.Fatalf("normalised profile: %+v %v", got, err)
	}
	if cleared, err := svc.Update("org-a", Updates{Timezone: str(""), Currency: str("")}, ""); err != nil || cleared.Timezone != "" || cleared.Currency != "" {
		t.Fatalf("empty clears back to the platform default: %+v %v", cleared, err)
	}
}
