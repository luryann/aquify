package groups

import (
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xOrg = "org-cov1x"

func cov1xAdmin() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "admin-1", OrgID: cov1xOrg, Roles: []string{"admin"}}
}

func cov1xCoach(id string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: id, OrgID: cov1xOrg, Roles: []string{"coach"}}
}

func cov1xParent(id, household string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: id, OrgID: cov1xOrg, HouseholdID: household, Roles: []string{"parent"}}
}

func cov1xSwimmerPrincipal() *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "swimmer-1", OrgID: cov1xOrg, Roles: []string{"swimmer"}}
}

func cov1xIntPtr(v int) *int { return &v }
func cov1xStrPtr(v string) *string {
	return &v
}

func cov1xNewGroup(t *testing.T, svc *Service, code string) Group {
	t.Helper()
	g, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: code, Name: "Group " + code})
	if err != nil {
		t.Fatalf("CreateGroup(%s): %v", code, err)
	}
	return g
}

func TestCov1xCreateGroupValidationAndDefaults(t *testing.T) {
	t.Parallel()
	svc := NewService()

	if _, _, err := svc.CreateGroup(cov1xParent("p", "h"), cov1xOrg, CreateGroupInput{Code: "A", Name: "A"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent create: got %v", err)
	}
	if _, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: "  ", Name: "A"}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("blank code: got %v", err)
	}
	if _, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: "A", Name: "A", Type: "bogus"}); err == nil || !strings.Contains(err.Error(), "type must be one of") {
		t.Fatalf("bad type: got %v", err)
	}
	if _, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: "A", Name: "A", MinAge: cov1xIntPtr(12), MaxAge: cov1xIntPtr(10)}); err == nil || !strings.Contains(err.Error(), "min_age") {
		t.Fatalf("min>max: got %v", err)
	}

	g, ev, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: " gold ", Name: " Gold ", Description: " fast ", CoachUserID: " c1 "})
	if err != nil {
		t.Fatal(err)
	}
	if g.Code != "GOLD" || g.Name != "Gold" || g.Description != "fast" || g.Type != "competitive" || g.CoachUserID != "c1" || !g.IsActive {
		t.Fatalf("unexpected group %+v", g)
	}
	if !strings.HasPrefix(g.ID, "grp_") {
		t.Fatalf("in-memory ids are prefixed, got %q", g.ID)
	}
	if ev.Action != "group.created" || ev.ActorUserID != "admin-1" || ev.After["name"] != "Gold" {
		t.Fatalf("unexpected audit event %+v", ev)
	}

	if _, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: "GOLD", Name: "Dup"}); !errors.Is(err, ErrGroupCodeConflict) {
		t.Fatalf("dup code: got %v", err)
	}
	// Same code in another org is fine.
	if _, _, err := svc.CreateGroup(cov1xAdmin(), "other-org", CreateGroupInput{Code: "GOLD", Name: "Other"}); err != nil {
		t.Fatalf("other org: %v", err)
	}

	// A coach-only creator always becomes the coach of the new group.
	cg, _, err := svc.CreateGroup(cov1xCoach("coach-9"), cov1xOrg, CreateGroupInput{Code: "C", Name: "C", CoachUserID: "someone-else", Type: " Masters "})
	if err != nil {
		t.Fatal(err)
	}
	if cg.CoachUserID != "coach-9" || cg.Type != "masters" {
		t.Fatalf("coach group %+v", cg)
	}
}

func TestCov1xUpdateGroup(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g, _, err := svc.CreateGroup(cov1xAdmin(), cov1xOrg, CreateGroupInput{Code: "U", Name: "Old", CoachUserID: "coach-a"})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.UpdateGroup(cov1xParent("p", "h"), cov1xOrg, g.ID, UpdateGroupInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent: %v", err)
	}
	if _, _, err := svc.UpdateGroup(cov1xAdmin(), cov1xOrg, "nope", UpdateGroupInput{}); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err := svc.UpdateGroup(cov1xAdmin(), "other-org", g.ID, UpdateGroupInput{}); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
	// Coach may not reassign the group to someone else.
	if _, _, err := svc.UpdateGroup(cov1xCoach("coach-a"), cov1xOrg, g.ID, UpdateGroupInput{CoachUserID: cov1xStrPtr("coach-b")}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach reassign: %v", err)
	}
	// ... but may re-state themselves.
	if _, _, err := svc.UpdateGroup(cov1xCoach("coach-a"), cov1xOrg, g.ID, UpdateGroupInput{CoachUserID: cov1xStrPtr(" coach-a ")}); err != nil {
		t.Fatalf("coach restate: %v", err)
	}

	for name, in := range map[string]UpdateGroupInput{
		"empty name": {Name: cov1xStrPtr("  ")},
		"bad type":   {Type: cov1xStrPtr("nonsense")},
		"min>max":    {MinAge: cov1xIntPtr(15), MaxAge: cov1xIntPtr(9)},
	} {
		if _, _, err := svc.UpdateGroup(cov1xAdmin(), cov1xOrg, g.ID, in); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	// Failed updates must not have mutated the group.
	got, err := svc.GetGroup(cov1xAdmin(), cov1xOrg, g.ID)
	if err != nil || got.Name != "Old" || got.MinAge != nil {
		t.Fatalf("group mutated by failed update: %+v err=%v", got, err)
	}

	off := false
	updated, ev, err := svc.UpdateGroup(cov1xAdmin(), cov1xOrg, g.ID, UpdateGroupInput{
		Name: cov1xStrPtr(" New "), Description: cov1xStrPtr(" desc "), Type: cov1xStrPtr(" Recreational "),
		MinAge: cov1xIntPtr(6), MaxAge: cov1xIntPtr(12), CoachUserID: cov1xStrPtr(" coach-z "), IsActive: &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New" || updated.Description != "desc" || updated.Type != "recreational" ||
		*updated.MinAge != 6 || *updated.MaxAge != 12 || updated.CoachUserID != "coach-z" || updated.IsActive {
		t.Fatalf("updated %+v", updated)
	}
	if ev.Action != "group.updated" || ev.Before["name"] != "Old" || ev.After["name"] != "New" || ev.Metadata["group_id"] != g.ID {
		t.Fatalf("event %+v", ev)
	}
}

func TestCov1xGetAndListGroupsVisibility(t *testing.T) {
	t.Parallel()
	svc := NewService()
	a := cov1xNewGroup(t, svc, "B")
	b := cov1xNewGroup(t, svc, "A")
	if _, _, err := svc.UpdateGroup(cov1xAdmin(), cov1xOrg, a.ID, UpdateGroupInput{CoachUserID: cov1xStrPtr("coach-1")}); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, _, err := svc.UpdateGroup(cov1xAdmin(), cov1xOrg, b.ID, UpdateGroupInput{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	cov1xNewGroup(t, svc, "Z")
	if _, _, err := svc.CreateGroup(cov1xAdmin(), "other-org", CreateGroupInput{Code: "OTHER", Name: "x"}); err != nil {
		t.Fatal(err)
	}

	nobody := &appmiddleware.Principal{UserID: "x", Roles: []string{"guest"}}
	if _, err := svc.GetGroup(nobody, cov1xOrg, a.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("get guest: %v", err)
	}
	if _, err := svc.ListGroups(nobody, cov1xOrg, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list guest: %v", err)
	}
	if _, err := svc.GetGroup(cov1xAdmin(), cov1xOrg, "missing"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if g, err := svc.GetGroup(cov1xCoach("coach-1"), cov1xOrg, a.ID); err != nil || g.ID != a.ID {
		t.Fatalf("own group: %+v %v", g, err)
	}
	if _, err := svc.GetGroup(cov1xCoach("coach-2"), cov1xOrg, a.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other coach: %v", err)
	}

	codes := func(gs []Group) string {
		var out []string
		for _, g := range gs {
			out = append(out, g.Code)
		}
		return strings.Join(out, ",")
	}
	all, err := svc.ListGroups(cov1xAdmin(), cov1xOrg, false)
	if err != nil || codes(all) != "A,B,Z" {
		t.Fatalf("all: %s %v", codes(all), err)
	}
	active, err := svc.ListGroups(cov1xSwimmerPrincipal(), cov1xOrg, true)
	if err != nil || codes(active) != "B,Z" {
		t.Fatalf("active: %s %v", codes(active), err)
	}
	mine, err := svc.ListGroups(cov1xCoach("coach-1"), cov1xOrg, false)
	if err != nil || codes(mine) != "B" {
		t.Fatalf("coach scoped: %s %v", codes(mine), err)
	}
	// Coach who is also admin sees everything.
	both := &appmiddleware.Principal{UserID: "coach-1", OrgID: cov1xOrg, Roles: []string{"coach", "admin"}}
	if gs, _ := svc.ListGroups(both, cov1xOrg, false); codes(gs) != "A,B,Z" {
		t.Fatalf("admin+coach: %s", codes(gs))
	}
}

func TestCov1xListGroupsTieBreaksOnID(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.groups["b"] = Group{ID: "b", OrgID: cov1xOrg, Code: "SAME"}
	svc.groups["a"] = Group{ID: "a", OrgID: cov1xOrg, Code: "SAME"}
	gs, err := svc.ListGroups(cov1xAdmin(), cov1xOrg, false)
	if err != nil || len(gs) != 2 || gs[0].ID != "a" || gs[1].ID != "b" {
		t.Fatalf("got %+v %v", gs, err)
	}
}

func TestCov1xEligibilityRulesLifecycle(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "E")

	if _, _, err := svc.AttachEligibilityRule(cov1xCoach("c"), cov1xOrg, g.ID, AttachEligibilityRuleInput{RuleType: "gender"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach attach: %v", err)
	}
	if _, _, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, g.ID, AttachEligibilityRuleInput{RuleType: "weight"}); err == nil || !strings.Contains(err.Error(), "rule_type must be one of") {
		t.Fatalf("bad type: %v", err)
	}
	if _, _, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, "nope", AttachEligibilityRuleInput{RuleType: "gender"}); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("missing group: %v", err)
	}

	// nil parameters are normalised to an empty map; type is case-folded.
	rule, ev, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, g.ID, AttachEligibilityRuleInput{RuleType: " Gender "})
	if err != nil {
		t.Fatal(err)
	}
	if rule.RuleType != "gender" || rule.Parameters == nil || !rule.IsActive || rule.GroupID != g.ID {
		t.Fatalf("rule %+v", rule)
	}
	if ev.Action != "group.eligibility_rule_attached" || ev.Metadata["group_id"] != g.ID {
		t.Fatalf("event %+v", ev)
	}

	if _, err := svc.ListEligibilityRules(cov1xSwimmerPrincipal(), cov1xOrg, g.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer list rules: %v", err)
	}
	if _, err := svc.ListEligibilityRules(cov1xAdmin(), cov1xOrg, "nope"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("list missing: %v", err)
	}

	// Deterministic ordering: creation time, then ID.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.rules["r-b"] = EligibilityRule{ID: "r-b", OrgID: cov1xOrg, GroupID: g.ID, RuleType: "gender", CreatedAt: t0}
	svc.rules["r-a"] = EligibilityRule{ID: "r-a", OrgID: cov1xOrg, GroupID: g.ID, RuleType: "gender", CreatedAt: t0}
	svc.rules["r-old"] = EligibilityRule{ID: "r-old", OrgID: cov1xOrg, GroupID: g.ID, RuleType: "gender", CreatedAt: t0.Add(-time.Hour)}
	svc.rules["r-foreign"] = EligibilityRule{ID: "r-foreign", OrgID: "other", GroupID: g.ID, RuleType: "gender", CreatedAt: t0}
	svc.rules["r-other-group"] = EligibilityRule{ID: "r-other-group", OrgID: cov1xOrg, GroupID: "different", RuleType: "gender", CreatedAt: t0}
	rules, err := svc.ListEligibilityRules(cov1xParent("p", "h"), cov1xOrg, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rules {
		ids = append(ids, r.ID)
	}
	if len(ids) != 4 || ids[0] != "r-old" || ids[1] != "r-a" || ids[2] != "r-b" || ids[3] != rule.ID {
		t.Fatalf("order %v", ids)
	}
}

func TestCov1xEvaluateSwimmerEligibility(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "EV")
	now := time.Now().UTC()
	tenYearsAgo := now.AddDate(-10, 0, -10).Format("2006-01-02")
	svc.SeedSwimmer(cov1xOrg, "s1", "h1", tenYearsAgo, "female", "active")
	svc.SeedSwimmer(cov1xOrg, "s-baddob", "h1", "not-a-date", "male", "pending")
	svc.SeedSwimmer("other-org", "s-foreign", "hx", tenYearsAgo, "female", "active")
	svc.SeedParentHousehold("parent-1", "h1")

	attach := func(typ string, params map[string]any) {
		t.Helper()
		if _, _, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, g.ID, AttachEligibilityRuleInput{RuleType: typ, Parameters: params}); err != nil {
			t.Fatalf("attach %s: %v", typ, err)
		}
	}

	if _, err := svc.EvaluateSwimmerEligibility(cov1xSwimmerPrincipal(), cov1xOrg, g.ID, "s1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer role: %v", err)
	}
	if _, err := svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, "nope", "s1"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("group: %v", err)
	}
	if _, err := svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, g.ID, "ghost"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("swimmer: %v", err)
	}
	if _, err := svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, g.ID, "s-foreign"); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("foreign swimmer: %v", err)
	}
	if _, err := svc.EvaluateSwimmerEligibility(cov1xParent("stranger", "h-other"), cov1xOrg, g.ID, "s1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger parent: %v", err)
	}

	// No rules: eligible with no results.
	res, err := svc.EvaluateSwimmerEligibility(cov1xParent("parent-1", "h1"), cov1xOrg, g.ID, "s1")
	if err != nil || !res.Eligible || len(res.RuleResults) != 0 || res.SwimmerID != "s1" || res.GroupID != g.ID {
		t.Fatalf("no rules: %+v %v", res, err)
	}

	attach("age_range", map[string]any{"min_age": 8, "max_age": 12})
	attach("gender", map[string]any{"allowed": []string{"FEMALE"}})
	attach("registration_status", map[string]any{"allowed": []any{"active", "pending"}})
	res, err = svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, g.ID, "s1")
	if err != nil || !res.Eligible || len(res.RuleResults) != 3 {
		t.Fatalf("passing: %+v %v", res, err)
	}
	for _, rr := range res.RuleResults {
		if !rr.Passed || rr.Reason != "" {
			t.Fatalf("rule %+v should pass", rr)
		}
	}

	// Unparseable DOB fails the age rule only.
	res, err = svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, g.ID, "s-baddob")
	if err != nil || res.Eligible {
		t.Fatalf("bad dob: %+v %v", res, err)
	}
	failed := map[string]string{}
	for _, rr := range res.RuleResults {
		if !rr.Passed {
			failed[rr.RuleType] = rr.Reason
		}
	}
	if !strings.Contains(failed["age_range"], "could not compute") || !strings.Contains(failed["gender"], `"male"`) || len(failed) != 2 {
		t.Fatalf("failures %v", failed)
	}

	// Below-min, above-max, registration-status mismatch, unknown rule type.
	g2 := cov1xNewGroup(t, svc, "EV2")
	mk := func(typ string, params map[string]any) {
		t.Helper()
		if _, _, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, g2.ID, AttachEligibilityRuleInput{RuleType: typ, Parameters: params}); err != nil {
			t.Fatal(err)
		}
	}
	mk("age_range", map[string]any{"min_age": 11.0})
	mk("age_range", map[string]any{"max_age": int64(9)})
	mk("registration_status", map[string]any{"allowed": []string{"inactive"}})
	svc.rules["legacy"] = EligibilityRule{ID: "legacy", OrgID: cov1xOrg, GroupID: g2.ID, RuleType: "shoe_size", IsActive: true, CreatedAt: now.Add(time.Hour)}
	svc.rules["inactive"] = EligibilityRule{ID: "inactive", OrgID: cov1xOrg, GroupID: g2.ID, RuleType: "gender", Parameters: map[string]any{"allowed": []string{"x"}}, IsActive: false}
	res, err = svc.EvaluateSwimmerEligibility(cov1xAdmin(), cov1xOrg, g2.ID, "s1")
	if err != nil || res.Eligible || len(res.RuleResults) != 4 {
		t.Fatalf("g2: %+v %v", res, err)
	}
	var reasons []string
	for _, rr := range res.RuleResults {
		reasons = append(reasons, rr.Reason)
	}
	joined := strings.Join(reasons, "|")
	for _, want := range []string{"is below min_age 11", "exceeds max_age 9", `registration_status "active" not in allowed list`, `unknown rule type "shoe_size"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	// The unknown rule type is a pass, the rest fail.
	if !res.RuleResults[3].Passed || res.RuleResults[3].RuleType != "shoe_size" {
		t.Fatalf("unknown rule should pass: %+v", res.RuleResults[3])
	}
}

func TestCov1xCreateRegistrationPaths(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "R")
	svc.SeedSwimmer(cov1xOrg, "s1", "h1", "2014-05-05", "female", "active")
	svc.SeedSwimmer(cov1xOrg, "s2", "h2", "2014-05-05", "male", "active")
	svc.SeedSwimmer("other-org", "s-foreign", "h-foreign", "2014-05-05", "male", "active")
	svc.SeedParentHousehold("parent-1", "h1")
	in := CreateRegistrationInput{SwimmerID: "s1", HouseholdID: "h1", GroupID: g.ID, Season: "2026", Notes: " hi "}

	if _, err := svc.CreateRegistration(cov1xSwimmerPrincipal(), cov1xOrg, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer role: %v", err)
	}
	blank := in
	blank.Season = " "
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, blank); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("blank season: %v", err)
	}
	bad := in
	bad.GroupID = "nope"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("group: %v", err)
	}
	bad = in
	bad.HouseholdID = "nope"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("household: %v", err)
	}
	bad = in
	bad.HouseholdID = "h-foreign"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("foreign household: %v", err)
	}
	bad = in
	bad.SwimmerID = "nope"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); !errors.Is(err, ErrSwimmerNotFound) {
		t.Fatalf("swimmer: %v", err)
	}
	bad = in
	bad.SwimmerID = "s2"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("household mismatch: %v", err)
	}
	if _, err := svc.CreateRegistration(cov1xParent("parent-2", "h2"), cov1xOrg, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent of other household: %v", err)
	}
	bad = in
	bad.RegistrationDate = "05/05/2026"
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad date: %v", err)
	}
	if len(svc.registrations) != 0 {
		t.Fatalf("failed creates must not store registrations: %d", len(svc.registrations))
	}

	// Parents must satisfy active rules; staff may override.
	if _, _, err := svc.AttachEligibilityRule(cov1xAdmin(), cov1xOrg, g.ID, AttachEligibilityRuleInput{RuleType: "gender", Parameters: map[string]any{"allowed": []string{"male"}}}); err != nil {
		t.Fatal(err)
	}
	svc.rules["inactive-rule"] = EligibilityRule{ID: "inactive-rule", OrgID: cov1xOrg, GroupID: g.ID, RuleType: "gender", Parameters: map[string]any{"allowed": []string{"x"}}}
	svc.rules["elsewhere"] = EligibilityRule{ID: "elsewhere", OrgID: cov1xOrg, GroupID: "different-group", IsActive: true, RuleType: "gender", Parameters: map[string]any{"allowed": []string{"x"}}}
	if _, err := svc.CreateRegistration(cov1xParent("parent-1", "h1"), cov1xOrg, in); !errors.Is(err, ErrNotEligible) || !strings.Contains(err.Error(), "not in allowed list") {
		t.Fatalf("parent ineligible: %v", err)
	}
	reg, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, in)
	if err != nil {
		t.Fatalf("staff override: %v", err)
	}
	if reg.Status != "pending" || reg.Notes != "hi" || reg.RegistrationDate != time.Now().UTC().Format("2006-01-02") || !strings.HasPrefix(reg.ID, "reg_") || reg.CreatedBy != "admin-1" {
		t.Fatalf("reg %+v", reg)
	}
	if _, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, in); !errors.Is(err, ErrDuplicateRegistration) {
		t.Fatalf("duplicate: %v", err)
	}

	// Withdraw then re-register: same row is reopened.
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, reg.ID, "withdrawn"); err != nil {
		t.Fatal(err)
	}
	again := in
	again.Notes = "back"
	again.RegistrationDate = "2026-02-03"
	reopened, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, again)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != reg.ID || reopened.Status != "pending" || reopened.Notes != "back" || reopened.RegistrationDate != "2026-02-03" {
		t.Fatalf("reopened %+v", reopened)
	}
	if len(svc.registrations) != 1 {
		t.Fatalf("expected the row to be reused, have %d", len(svc.registrations))
	}
}

func TestCov1xTransitionRegistrationStatus(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "T")
	svc.SeedSwimmer(cov1xOrg, "s1", "h1", "2014-05-05", "female", "active")
	svc.SeedParentHousehold("parent-1", "h1")
	reg, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, CreateRegistrationInput{SwimmerID: "s1", HouseholdID: "h1", GroupID: g.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}

	var hookCalls []string
	svc.SetRegistrationActivatedHook(func(r Registration, code, actor string) {
		hookCalls = append(hookCalls, r.ID+"/"+code+"/"+actor)
	})

	if _, _, err := svc.TransitionRegistrationStatus(cov1xSwimmerPrincipal(), cov1xOrg, reg.ID, "active"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer: %v", err)
	}
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, "nope", "active"); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), "other-org", reg.ID, "active"); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
	// Parent of another household cannot even learn the registration exists.
	if _, _, err := svc.TransitionRegistrationStatus(cov1xParent("stranger", "h-x"), cov1xOrg, reg.ID, "withdrawn"); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("idor: %v", err)
	}
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, reg.ID, "inactive"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pending->inactive: %v", err)
	}
	if _, _, err := svc.TransitionRegistrationStatus(cov1xParent("parent-1", "h1"), cov1xOrg, reg.ID, "active"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("parent activate: %v", err)
	}
	if got := svc.registrations[reg.ID].Status; got != "pending" {
		t.Fatalf("rejected transitions changed status to %q", got)
	}
	if len(hookCalls) != 0 {
		t.Fatalf("hook fired on failures: %v", hookCalls)
	}

	active, ev, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, reg.ID, " ACTIVE ")
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != "active" || ev.Action != "registration.status_changed" || ev.Before["status"] != "pending" || ev.After["status"] != "active" {
		t.Fatalf("active %+v ev %+v", active, ev)
	}
	if len(hookCalls) != 1 || hookCalls[0] != reg.ID+"/T/admin-1" {
		t.Fatalf("hook calls %v", hookCalls)
	}

	// Non-active target never fires the hook.
	if _, _, err := svc.TransitionRegistrationStatus(cov1xParent("parent-1", "h1"), cov1xOrg, reg.ID, "withdrawn"); err != nil {
		t.Fatalf("parent withdraw: %v", err)
	}
	if len(hookCalls) != 1 {
		t.Fatalf("hook fired for withdraw: %v", hookCalls)
	}
	// Withdrawn is terminal.
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, reg.ID, "active"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("withdrawn terminal: %v", err)
	}

	// A registration whose group has vanished from the cache activates without calling the hook.
	svc.registrations["orphan"] = Registration{ID: "orphan", OrgID: cov1xOrg, HouseholdID: "h1", GroupID: "gone", Status: "pending"}
	if _, _, err := svc.TransitionRegistrationStatus(cov1xAdmin(), cov1xOrg, "orphan", "active"); err != nil {
		t.Fatal(err)
	}
	if len(hookCalls) != 1 {
		t.Fatalf("hook fired without a group code: %v", hookCalls)
	}
}

func TestCov1xAcceptWaiver(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "W")
	svc.SeedSwimmer(cov1xOrg, "s1", "h1", "2014-05-05", "female", "active")
	svc.SeedParentHousehold("parent-1", "h1")
	reg, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, CreateRegistrationInput{SwimmerID: "s1", HouseholdID: "h1", GroupID: g.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AcceptWaiver(cov1xCoach("c"), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("coach: %v", err)
	}
	if _, err := svc.AcceptWaiver(cov1xAdmin(), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: " "}); err == nil || !strings.Contains(err.Error(), "waiver_type is required") {
		t.Fatalf("blank: %v", err)
	}
	if _, err := svc.AcceptWaiver(cov1xAdmin(), cov1xOrg, "nope", AcceptWaiverInput{WaiverType: "liability"}); !errors.Is(err, ErrRegistrationNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.AcceptWaiver(cov1xParent("stranger", "h-x"), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger: %v", err)
	}

	w, err := svc.AcceptWaiver(cov1xParent("parent-1", "h1"), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: " Liability ", IPAddress: " 10.0.0.1 "})
	if err != nil {
		t.Fatal(err)
	}
	if w.WaiverType != "liability" || w.IPAddress != "10.0.0.1" || w.AcceptedByUserID != "parent-1" || !strings.HasPrefix(w.ID, "wvr_") {
		t.Fatalf("waiver %+v", w)
	}
	if _, err := svc.AcceptWaiver(cov1xAdmin(), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); !errors.Is(err, ErrWaiverAlreadyAccepted) {
		t.Fatalf("dup: %v", err)
	}
	// A different type on the same registration is fine, and admin needs no household link.
	if _, err := svc.AcceptWaiver(cov1xAdmin(), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: "photo"}); err != nil {
		t.Fatalf("second type: %v", err)
	}
}

func TestCov1xListRegistrationsScopingAndOrdering(t *testing.T) {
	t.Parallel()
	svc := NewService()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	svc.SeedParentHousehold("parent-1", "h1")
	svc.registrations["r1"] = Registration{ID: "r1", OrgID: cov1xOrg, HouseholdID: "h1", RegistrationDate: "2026-01-01", CreatedAt: base}
	svc.registrations["r2"] = Registration{ID: "r2", OrgID: cov1xOrg, HouseholdID: "h1", RegistrationDate: "2026-02-01", CreatedAt: base}
	svc.registrations["r3"] = Registration{ID: "r3", OrgID: cov1xOrg, HouseholdID: "h2", RegistrationDate: "2026-01-01", CreatedAt: base.Add(time.Hour)}
	svc.registrations["r4"] = Registration{ID: "r4", OrgID: cov1xOrg, HouseholdID: "h2", RegistrationDate: "2026-01-01", CreatedAt: base.Add(-time.Hour)}
	svc.registrations["r5"] = Registration{ID: "r5", OrgID: cov1xOrg, HouseholdID: "h2", RegistrationDate: "2026-01-01", CreatedAt: base.Add(-time.Hour)}
	svc.registrations["rx"] = Registration{ID: "rx", OrgID: "other", HouseholdID: "h1", RegistrationDate: "2026-12-01"}

	w1 := base.Add(time.Minute)
	w2 := base.Add(time.Hour)
	svc.waivers["w1"] = WaiverAcceptance{ID: "w1", OrgID: cov1xOrg, RegistrationID: "r1", WaiverType: "photo", AcceptedAt: w1}
	svc.waivers["w2"] = WaiverAcceptance{ID: "w2", OrgID: cov1xOrg, RegistrationID: "r1", WaiverType: "liability", AcceptedAt: w2}
	svc.waivers["w3"] = WaiverAcceptance{ID: "w3", OrgID: "other", RegistrationID: "r1", WaiverType: "foreign", AcceptedAt: w2.Add(time.Hour)}

	if _, err := svc.ListRegistrations(cov1xSwimmerPrincipal(), cov1xOrg); !errors.Is(err, ErrForbidden) {
		t.Fatalf("swimmer: %v", err)
	}
	all, err := svc.ListRegistrations(cov1xAdmin(), cov1xOrg)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range all {
		ids = append(ids, r.ID)
	}
	// Newest registration date first, then newest created, then ID.
	if strings.Join(ids, ",") != "r2,r3,r1,r4,r5" && strings.Join(ids, ",") != "r2,r3,r4,r5,r1" {
		t.Fatalf("order %v", ids)
	}
	var r1 Registration
	for _, r := range all {
		if r.ID == "r1" {
			r1 = r
		}
	}
	if !r1.WaiverAccepted || r1.WaiverCount != 2 || strings.Join(r1.WaiverTypes, ",") != "liability,photo" || r1.LastWaiverAt == nil || !r1.LastWaiverAt.Equal(w2) {
		t.Fatalf("waiver summary %+v", r1)
	}
	for _, r := range all {
		if r.ID == "r2" && (r.WaiverAccepted || r.WaiverCount != 0 || r.LastWaiverAt != nil) {
			t.Fatalf("r2 should have no waivers: %+v", r)
		}
	}

	mine, err := svc.ListRegistrations(cov1xParent("parent-1", "h1"), cov1xOrg)
	if err != nil || len(mine) != 2 || mine[0].ID != "r2" || mine[1].ID != "r1" {
		t.Fatalf("parent scope: %+v %v", mine, err)
	}
}

func TestCov1xSnapshots(t *testing.T) {
	t.Parallel()
	svc := NewService()
	cov1xNewGroup(t, svc, "S")
	svc.registrations["r"] = Registration{ID: "r", OrgID: cov1xOrg}
	svc.registrations["rx"] = Registration{ID: "rx", OrgID: "other"}
	svc.waivers["w"] = WaiverAcceptance{ID: "w", OrgID: cov1xOrg}
	svc.waivers["wx"] = WaiverAcceptance{ID: "wx", OrgID: "other"}
	svc.groups["gx"] = Group{ID: "gx", OrgID: "other"}

	if gs := svc.SnapshotGroups(cov1xOrg); len(gs) != 1 || gs[0].Code != "S" {
		t.Fatalf("groups %+v", gs)
	}
	if rs := svc.SnapshotRegistrations(cov1xOrg); len(rs) != 1 || rs[0].ID != "r" {
		t.Fatalf("regs %+v", rs)
	}
	if ws := svc.SnapshotWaivers(cov1xOrg); len(ws) != 1 || ws[0].ID != "w" {
		t.Fatalf("waivers %+v", ws)
	}
	if gs := svc.SnapshotGroups("nobody"); len(gs) != 0 || gs == nil {
		t.Fatalf("empty org should be an empty non-nil slice, got %#v", gs)
	}
}

func TestCov1xPrincipalOwnsHouseholdViaKeycloakLink(t *testing.T) {
	t.Parallel()
	svc := NewService()
	svc.households["h1"] = householdRef{ID: "h1", OrgID: cov1xOrg, KeycloakUserID: "kc-user"}
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if !svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "kc-user"}, "h1") {
		t.Fatal("durable keycloak link should grant ownership")
	}
	if svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "someone"}, "h1") {
		t.Fatal("different user must not own the household")
	}
	if svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "  "}, "h1") {
		t.Fatal("blank user id must not match")
	}
	if svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{UserID: "kc-user"}, "missing") {
		t.Fatal("unknown household must not match")
	}
	if !svc.principalOwnsHouseholdLocked(&appmiddleware.Principal{HouseholdID: " h1 "}, "h1") {
		t.Fatal("login household should grant ownership")
	}
}

func TestCov1xValidateRuleParameters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		typ     string
		params  map[string]any
		wantErr string
	}{
		{"age ok int", "age_range", map[string]any{"min_age": 5, "max_age": int64(9)}, ""},
		{"age ok floats", "age_range", map[string]any{"min_age": float32(5), "max_age": 9.5}, ""},
		{"age bad min", "age_range", map[string]any{"min_age": "5"}, "parameters.min_age"},
		{"age bad max", "age_range", map[string]any{"max_age": true}, "parameters.max_age"},
		{"gender strings", "gender", map[string]any{"allowed": []string{"F"}}, ""},
		{"gender any strings", "registration_status", map[string]any{"allowed": []any{"active"}}, ""},
		{"gender any non-string", "gender", map[string]any{"allowed": []any{1}}, "array of strings"},
		{"gender scalar", "registration_status", map[string]any{"allowed": "active"}, "array of strings"},
		{"gender absent", "gender", map[string]any{}, ""},
	}
	for _, tc := range cases {
		err := validateRuleParameters(tc.typ, tc.params)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: got %v want %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCov1xValidateTransitionUnknownSource(t *testing.T) {
	t.Parallel()
	if err := validateTransition("withdrawn", "active", cov1xAdmin()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
	if err := validateTransition("active", "pending", cov1xAdmin()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestCov1xComputeAge(t *testing.T) {
	t.Parallel()
	if got := computeAge("garbage"); got != -1 {
		t.Fatalf("garbage: %d", got)
	}
	now := time.Now().UTC()
	tomorrow := now.AddDate(0, 0, 1)
	notYet := time.Date(tomorrow.Year()-20, tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
	if got := computeAge(notYet.Format("2006-01-02")); got != 19 {
		t.Fatalf("birthday tomorrow: got %d want 19", got)
	}
	yesterday := now.AddDate(0, 0, -1)
	passed := time.Date(yesterday.Year()-20, yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	if got := computeAge(passed.Format("2006-01-02")); got != 20 {
		t.Fatalf("birthday yesterday: got %d want 20", got)
	}
}

func TestCov1xToIntAndToStringSlice(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]struct {
		v any
		n int
	}{
		"int":     {7, 7},
		"int64":   {int64(8), 8},
		"float64": {9.9, 9},
		"float32": {float32(10.5), 10},
		"string":  {"11", 0},
		"nil":     {nil, 0},
	} {
		if got := toInt(want.v); got != want.n {
			t.Errorf("toInt(%s)=%d want %d", in, got, want.n)
		}
	}
	if toStringSlice(nil) != nil {
		t.Error("nil -> nil")
	}
	if got := toStringSlice([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("[]string passthrough: %v", got)
	}
	if got := toStringSlice([]any{"a", 2, "b"}); strings.Join(got, ",") != "a,b" {
		t.Errorf("mixed []any should keep only strings: %v", got)
	}
	if toStringSlice("scalar") != nil {
		t.Error("scalar -> nil")
	}
}

func TestCov1xContainsFoldAndRandomToken(t *testing.T) {
	t.Parallel()
	if !containsFold([]string{" Female "}, "female ") {
		t.Error("should match ignoring case and surrounding space")
	}
	if containsFold([]string{"male"}, "female") {
		t.Error("male != female")
	}
	a, b := randomToken(12), randomToken(12)
	if len(a) != 24 || a == b {
		t.Errorf("tokens %q %q", a, b)
	}
}

func TestCov1xMapPersistenceError(t *testing.T) {
	t.Parallel()
	if mapPersistenceError(nil) != nil {
		t.Fatal("nil in, nil out")
	}
	for msg, want := range map[string]error{
		`duplicate key value violates unique constraint "groups_code_org_unique"`:                      ErrGroupCodeConflict,
		`duplicate key value violates unique constraint "registrations_swimmer_group_season_unique"`:   ErrDuplicateRegistration,
		`duplicate key value violates unique constraint "waiver_acceptances_registration_type_unique"`: ErrWaiverAlreadyAccepted,
	} {
		if got := mapPersistenceError(errors.New(msg)); !errors.Is(got, want) {
			t.Errorf("%q -> %v want %v", msg, got, want)
		}
	}
	orig := errors.New("connection reset")
	if got := mapPersistenceError(orig); got != orig {
		t.Errorf("unknown errors pass through unchanged, got %v", got)
	}
}

func TestCov1xRuntimeSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	svc := NewService()
	g := cov1xNewGroup(t, svc, "SN")
	svc.SeedSwimmer(cov1xOrg, "s1", "h1", "2014-05-05", "female", "active")
	svc.SeedParentHousehold("parent-1", "h1")
	reg, err := svc.CreateRegistration(cov1xAdmin(), cov1xOrg, CreateRegistrationInput{SwimmerID: "s1", HouseholdID: "h1", GroupID: g.ID, Season: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptWaiver(cov1xAdmin(), cov1xOrg, reg.ID, AcceptWaiverInput{WaiverType: "liability"}); err != nil {
		t.Fatal(err)
	}
	data, err := svc.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	fresh := NewService()
	if err := fresh.LoadRuntimeSnapshot(data); err != nil {
		t.Fatal(err)
	}
	if got := fresh.SnapshotGroups(cov1xOrg); len(got) != 1 || got[0].Code != "SN" {
		t.Fatalf("groups %+v", got)
	}
	if got := fresh.SnapshotRegistrations(cov1xOrg); len(got) != 1 || got[0].ID != reg.ID {
		t.Fatalf("regs %+v", got)
	}
	if got := fresh.SnapshotWaivers(cov1xOrg); len(got) != 1 {
		t.Fatalf("waivers %+v", got)
	}
	if len(fresh.auditEvents) != len(svc.auditEvents) || len(fresh.auditEvents) == 0 {
		t.Fatalf("audit events not restored: %d vs %d", len(fresh.auditEvents), len(svc.auditEvents))
	}
	// Parent links survive: the parent can list their registration.
	if regs, err := fresh.ListRegistrations(cov1xParent("parent-1", ""), cov1xOrg); err != nil || len(regs) != 1 {
		t.Fatalf("parent link lost: %+v %v", regs, err)
	}
	// Restored state is a copy, not shared with the source service.
	fresh.groups[g.ID] = Group{ID: g.ID, OrgID: cov1xOrg, Code: "MUTATED"}
	if svc.groups[g.ID].Code != "SN" {
		t.Fatal("snapshot load aliased the exporter's maps")
	}
}

func TestCov1xRuntimeSnapshotEdgeCases(t *testing.T) {
	t.Parallel()
	svc := NewService()
	cov1xNewGroup(t, svc, "KEEP")

	if err := svc.LoadRuntimeSnapshot(nil); err != nil {
		t.Fatalf("empty snapshot is a no-op, got %v", err)
	}
	if len(svc.groups) != 1 {
		t.Fatal("empty snapshot must not clear state")
	}
	if err := svc.LoadRuntimeSnapshot([]byte("{not json")); err == nil {
		t.Fatal("malformed snapshot should error")
	}
	if len(svc.groups) != 1 {
		t.Fatal("malformed snapshot must not clear state")
	}
	if err := svc.LoadRuntimeSnapshot([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(svc.groups) != 0 || len(svc.auditEvents) != 0 {
		t.Fatal("an empty-object snapshot replaces state with empty state")
	}
}

func TestCov1xNoDBMethodsAreNoops(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if err := svc.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshOrg(t.Context(), cov1xOrg); err != nil {
		t.Fatal(err)
	}
	if NewServiceWithPool(nil).repo != nil {
		t.Fatal("nil pool must stay in-memory")
	}
}
