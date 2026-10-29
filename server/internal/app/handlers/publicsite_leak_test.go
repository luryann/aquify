package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
)

// PublicSiteAllowedKeys is the complete allowlist of JSON keys the anonymous
// team-site payload may contain, by path ("[]" marks array elements). Adding a
// field to the public payload means adding it here deliberately; any other key
// fails TestPublicSitePayloadOnlyContainsAllowlistedKeys.
var publicSiteAllowedKeys = map[string][]string{
	"":                {"club", "announcements", "meets", "events", "records", "coaches"},
	"club":            {"name", "slug", "about", "contact_email", "contact_phone", "join_url"},
	"announcements[]": {"id", "title", "body", "published_at"},
	"meets[]":         {"id", "name", "start_date", "end_date", "location", "host_org", "course"},
	"events[]":        {"id", "title", "category", "description", "location", "starts_at", "ends_at", "all_day"},
	"records[]":       {"label", "course", "gender", "age_group", "distance", "stroke", "time_ms", "swimmer_name", "swim_date", "meet_name"},
	"coaches[]":       {"name", "role"},
}

// allowlistViolations decodes raw JSON and reports every key outside the
// allowlist, at any depth; objects at unknown paths are reported as well.
func allowlistViolations(raw []byte) (map[string]any, []string, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	var violations []string
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			allowed, ok := publicSiteAllowedKeys[path]
			if !ok {
				violations = append(violations, fmt.Sprintf("object at unexpected path %q", path))
				return
			}
			set := map[string]bool{}
			for _, k := range allowed {
				set[k] = true
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !set[k] {
					violations = append(violations, fmt.Sprintf("non-allowlisted key %q at %q", k, path))
					continue
				}
				child := k
				if path != "" {
					child = path + "." + k
				}
				walk(child, x[k])
			}
		case []any:
			for _, el := range x {
				walk(path+"[]", el)
			}
		}
	}
	walk("", doc)
	return doc, violations, nil
}

func TestPublicSitePayloadOnlyContainsAllowlistedKeys(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	pub := now.Add(-time.Hour)
	end := now.Add(72 * time.Hour)
	const org = "org-1"
	src := PublicSiteSources{
		Now: func() time.Time { return now },
		Settings: func(slug string) (*orgsettings.OrgSettings, error) {
			return &orgsettings.OrgSettings{OrgID: org, TeamCode: "SECRETCODE", TeamName: "Dare", LSCCode: "PC", EmailSenderAddress: "internal@example.com",
				PublicSlug: slug, PublicSiteEnabled: true, PublicShowCoaches: true, PublicAbout: "about",
				PublicContactEmail: "hi@example.com", UpdatedBy: "admin-user"}, nil
		},
		Announcements: func(string) []comms.Announcement {
			return []comms.Announcement{{ID: "a1", OrgID: org, Title: "News", Body: "b", Status: "published", Audience: "all", Public: true,
				PublishedAt: &pub, GroupID: "g-secret", CreatedBy: "admin-user", UpdatedBy: "admin-user"}}
		},
		Meets: func(string) []meets.Meet {
			return []meets.Meet{{ID: "m1", OrgID: org, Name: "Invite", Course: "SCY", StartDate: "2026-06-10", EndDate: "2026-06-11", Location: "Pool", HostOrg: "Host",
				Status: "published", CreatedBy: "admin-user", EntryDeadline: "2026-06-05", MaxEntriesPerSwimmer: 3}}
		},
		Events: func(string, time.Time) []*events.Event {
			return []*events.Event{{ID: "e1", OrgID: org, Title: "Party", Category: "social", Description: "d", Location: "Deck", StartsAt: end, EndsAt: &end,
				Audience: "all", Public: true, Status: "scheduled", FeeCents: 500, Capacity: new(int), GroupCodes: []string{"SECRET"}, CreatedBy: "admin-user"}}
		},
		Records: func(string) []performance.TeamRecord {
			return []performance.TeamRecord{{
				RecordKey: performance.RecordKey{Course: "SCY", Gender: "F", AgeGroup: "OPEN", Distance: 100, Stroke: "free"}, Label: "100 free", PendingCount: 3,
				Record: &performance.RecordEntry{ID: "r1", OrgID: org, TimeMs: 50000, SwimmerID: "swimmer-secret", SwimmerName: "Amy Ames", SwimDate: "2026-01-01",
					MeetName: "Meet", Notes: "private note", CreatedBy: "admin-user", VerifiedBy: "admin-user", Locked: true},
			}}
		},
		Staff: func(context.Context, string) []PublicSiteStaff {
			return []PublicSiteStaff{{Name: "Cara Coach", Role: "Coach"}}
		},
	}
	h := NewPublicSiteHandlers(src, nil)
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/orgs/dare/site", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	doc, violations, err := allowlistViolations(rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("public payload leaks: %v", violations)
	}
	// Every section must actually be populated, or the allowlist check proves nothing.
	for _, section := range []string{"announcements", "meets", "events", "records", "coaches"} {
		if list, _ := doc[section].([]any); len(list) != 1 {
			t.Fatalf("section %s not populated: %s", section, rec.Body.String())
		}
	}
	for _, banned := range []string{"SECRETCODE", "internal@example.com", "g-secret", "swimmer-secret", "private note", "admin-user"} {
		if strings.Contains(rec.Body.String(), banned) {
			t.Fatalf("payload leaked %q", banned)
		}
	}
}

func TestAllowlistViolationsFlagsANewPrivateField(t *testing.T) {
	_, violations, err := allowlistViolations([]byte(`{"club":{"name":"x","internal_notes":"y"},"records":[{"label":"l","created_by":"u"}],"surprise":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(violations, "|")
	for _, want := range []string{"internal_notes", "created_by", "surprise"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("violation for %q not reported: %v", want, violations)
		}
	}
}
