package billing

import (
	"errors"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
)

const cov1xOrg = "org-cov1x"

var (
	cov1xAdmin  = &appmiddleware.Principal{UserID: "admin-cov1x", OrgID: cov1xOrg, Roles: []string{"admin"}, MFAAuthenticated: true}
	cov1xCoach  = &appmiddleware.Principal{UserID: "coach-cov1x", OrgID: cov1xOrg, Roles: []string{"coach"}}
	cov1xParent = &appmiddleware.Principal{UserID: "parent-cov1x", OrgID: cov1xOrg, Roles: []string{"parent"}, HouseholdID: "hh-a"}
)

// cov1xRosterService is an in-memory service with the shared finance roster
// (households hh-a/hh-b, swimmers sw-1..sw-4, group SILVER).
func cov1xRosterService() *Service {
	svc := NewService()
	svc.SetRosterSource(func(string) (RosterSnapshot, error) { return finRoster(), nil })
	return svc
}

func cov1xScheduleInput() FeeScheduleInput {
	return FeeScheduleInput{
		Name: "Silver dues", Kind: ScheduleKindDues, GroupCode: "silver", Frequency: FrequencyMonthly,
		AmountCents: 15000, StartDate: "2026-09-01", Proration: ProrationDaily,
	}
}

func cov1xInvalidField(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("expected invalid finance input for %q, got %v", field, err)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error %q should name field %q", err.Error(), field)
	}
}

func TestCov1xNormalizeScheduleInputRejectsBadInput(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()
	cases := []struct {
		name   string
		mutate func(*FeeScheduleInput)
		field  string
	}{
		{"missing name", func(in *FeeScheduleInput) { in.Name = "   " }, "name"},
		{"bad start", func(in *FeeScheduleInput) { in.StartDate = "09/01/2026" }, "start_date"},
		{"bad end", func(in *FeeScheduleInput) { in.EndDate = "soon" }, "end_date"},
		{"end before start", func(in *FeeScheduleInput) { in.EndDate = "2026-08-31" }, "end_date"},
		{"dues need group", func(in *FeeScheduleInput) { in.GroupCode = "  " }, "group_code"},
		{"registration needs season", func(in *FeeScheduleInput) {
			in.Kind, in.GroupCode, in.Frequency = ScheduleKindRegistration, "", FrequencyOneTime
		}, "season"},
		{"registration must be one_time", func(in *FeeScheduleInput) {
			in.Kind, in.GroupCode, in.Season = ScheduleKindRegistration, "", "2026-27"
		}, "frequency"},
		{"one_time cannot prorate", func(in *FeeScheduleInput) { in.Frequency = FrequencyOneTime }, "proration"},
		{"unknown roster group", func(in *FeeScheduleInput) { in.GroupCode = "GOLD" }, "group_code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := cov1xScheduleInput()
			tc.mutate(&in)
			_, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in)
			cov1xInvalidField(t, err, tc.field)
		})
	}
}

func TestCov1xNormalizeScheduleInputNormalizesValidInput(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()
	// Registration fees: group/proration are forced, season trimmed, default proration none.
	reg, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, FeeScheduleInput{
		Name: "  Reg fee ", Kind: ScheduleKindRegistration, Season: " 2026-27 ", Frequency: FrequencyOneTime,
		AmountCents: 5000, StartDate: "2026-09-01", EndDate: "2026-12-31", Proration: ProrationDaily,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Name != "Reg fee" || reg.Season != "2026-27" || reg.Proration != ProrationNone || !reg.Active {
		t.Fatalf("registration schedule not normalised: %+v", reg)
	}
	// Dues schedules drop any season and default the proration.
	in := cov1xScheduleInput()
	in.Season, in.Proration = "ignored", ""
	dues, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in)
	if err != nil {
		t.Fatal(err)
	}
	if dues.Season != "" || dues.Proration != ProrationNone || dues.GroupCode != "SILVER" {
		t.Fatalf("dues schedule not normalised: %+v", dues)
	}
	// With no roster (or an empty group list) the group is not validated.
	bare := NewService()
	in = cov1xScheduleInput()
	in.GroupCode = "ANY"
	if _, err := bare.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in); err != nil {
		t.Fatalf("no roster must skip group validation: %v", err)
	}
	empty := NewService()
	empty.SetRosterSource(func(string) (RosterSnapshot, error) { return RosterSnapshot{}, nil })
	if _, err := empty.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in); err != nil {
		t.Fatalf("empty roster groups must skip group validation: %v", err)
	}
}

func TestCov1xFeeScheduleCRUDAndPermissions(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()

	// Everything is admin only.
	for name, fn := range map[string]func() error{
		"list": func() error { _, err := svc.ListFeeSchedules(cov1xOrg, cov1xCoach); return err },
		"get":  func() error { _, err := svc.GetFeeSchedule(cov1xOrg, cov1xCoach, "x"); return err },
		"create": func() error {
			_, err := svc.CreateFeeSchedule(cov1xOrg, "c", cov1xCoach, cov1xScheduleInput())
			return err
		},
		"update": func() error {
			_, _, err := svc.UpdateFeeSchedule(cov1xOrg, "c", cov1xCoach, "x", cov1xScheduleInput())
			return err
		},
		"archive": func() error { _, err := svc.ArchiveFeeSchedule(cov1xOrg, "c", cov1xCoach, "x"); return err },
	} {
		if err := fn(); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: expected ErrForbidden, got %v", name, err)
		}
	}

	inactive := false
	a, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, cov1xScheduleInput())
	if err != nil {
		t.Fatal(err)
	}
	in := cov1xScheduleInput()
	in.Name = "Alpha late"
	b, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, FeeScheduleInput{
		Name: "Reg", Kind: ScheduleKindRegistration, Season: "2026-27", Frequency: FrequencyOneTime, AmountCents: 100, StartDate: "2026-09-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	off := cov1xScheduleInput()
	off.Name, off.Active = "Archived", &inactive
	c, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, off)
	if err != nil {
		t.Fatal(err)
	}
	// A schedule in another org must never leak into this org's list.
	if _, err := svc.CreateFeeSchedule("other-org", "a", cov1xAdmin, cov1xScheduleInput()); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListFeeSchedules(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	// Active first, then kind (dues < registration), then name.
	want := []string{b.ID, a.ID, reg.ID, c.ID}
	if len(list) != len(want) {
		t.Fatalf("list size %d, want %d", len(list), len(want))
	}
	for i, id := range want {
		if list[i].ID != id {
			t.Fatalf("list[%d] = %s (%s), want %s", i, list[i].ID, list[i].Name, id)
		}
	}
	list[0].Name = "mutated"
	if got, _ := svc.GetFeeSchedule(cov1xOrg, cov1xAdmin, b.ID); got.Name != "Alpha late" {
		t.Fatalf("list must return copies, store saw %q", got.Name)
	}

	if _, err := svc.GetFeeSchedule(cov1xOrg, cov1xAdmin, "missing"); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.GetFeeSchedule("other-org", cov1xAdmin, a.ID); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("cross-org get must be not found: %v", err)
	}

	// Update.
	upd := cov1xScheduleInput()
	upd.Name, upd.AmountCents, upd.Active = "Renamed", 20000, &inactive
	before, after, err := svc.UpdateFeeSchedule(cov1xOrg, "editor", cov1xAdmin, a.ID, upd)
	if err != nil {
		t.Fatal(err)
	}
	if before.Name != a.Name || after.Name != "Renamed" || after.AmountCents != 20000 || after.Active || after.UpdatedBy != "editor" {
		t.Fatalf("update result: before=%+v after=%+v", before, after)
	}
	// Leaving Active nil keeps the current value.
	upd.Active = nil
	if _, after, err = svc.UpdateFeeSchedule(cov1xOrg, "editor", cov1xAdmin, a.ID, upd); err != nil || after.Active {
		t.Fatalf("nil active must keep value: %+v %v", after, err)
	}
	if _, _, err := svc.UpdateFeeSchedule(cov1xOrg, "editor", cov1xAdmin, a.ID, FeeScheduleInput{}); !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatalf("invalid update: %v", err)
	}
	if _, _, err := svc.UpdateFeeSchedule(cov1xOrg, "editor", cov1xAdmin, "missing", upd); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	if _, _, err := svc.UpdateFeeSchedule("other-org", "editor", cov1xAdmin, a.ID, upd); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("cross-org update: %v", err)
	}

	// Archive deactivates and turns auto-billing off.
	in = cov1xScheduleInput()
	in.Name, in.AutoBill = "Auto", true
	auto, err := svc.CreateFeeSchedule(cov1xOrg, "a", cov1xAdmin, in)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := svc.ArchiveFeeSchedule(cov1xOrg, "archiver", cov1xAdmin, auto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Active || archived.AutoBill || archived.UpdatedBy != "archiver" {
		t.Fatalf("archive result: %+v", archived)
	}
	if _, err := svc.ArchiveFeeSchedule(cov1xOrg, "archiver", cov1xAdmin, "missing"); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("missing archive: %v", err)
	}
	if _, err := svc.ArchiveFeeSchedule("other-org", "archiver", cov1xAdmin, auto.ID); !errors.Is(err, ErrFeeScheduleNotFound) {
		t.Fatalf("cross-org archive: %v", err)
	}
}

func TestCov1xBilledScheduleFreezesPeriodFields(t *testing.T) {
	t.Parallel()
	svc, sched := newFinService(t)
	if _, err := svc.CommitBillingRun(finOrg, "admin-1", finAdmin, RunInput{AsOf: "2026-11-20", InvoiceStatus: "issued"}, RunTriggerManual); err != nil {
		t.Fatalf("commit run: %v", err)
	}
	if !func() bool { svc.mu.Lock(); defer svc.mu.Unlock(); return svc.scheduleHasChargesLocked(sched.ID) }() {
		t.Fatal("schedule should have billed charges after a run")
	}
	if svc.scheduleHasChargesLocked("sched-never-billed") {
		t.Fatal("unbilled schedule must report no charges")
	}

	frozen := FeeScheduleInput{Name: "Silver dues", Kind: ScheduleKindDues, GroupCode: "SILVER", Frequency: FrequencyQuarterly, AmountCents: 15000, StartDate: "2026-09-01", Proration: ProrationDaily}
	if _, _, err := svc.UpdateFeeSchedule(finOrg, "admin-1", finAdmin, sched.ID, frozen); !errors.Is(err, ErrScheduleLocked) {
		t.Fatalf("changing the frequency of a billed schedule must be refused, got %v", err)
	}
	// Non-period fields stay editable.
	ok := FeeScheduleInput{Name: "Silver dues v2", Kind: ScheduleKindDues, GroupCode: "SILVER", Frequency: FrequencyMonthly, AmountCents: 16000, StartDate: "2026-09-01", Proration: ProrationDaily}
	_, after, err := svc.UpdateFeeSchedule(finOrg, "admin-1", finAdmin, sched.ID, ok)
	if err != nil || after.AmountCents != 16000 || after.Name != "Silver dues v2" {
		t.Fatalf("amount/name edit on a billed schedule: %+v %v", after, err)
	}
}

func TestCov1xDiscountRuleValidationAndCRUD(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()
	max1 := 1
	max3 := 3
	bad := []struct {
		name  string
		input DiscountRuleInput
		field string
	}{
		{"name", DiscountRuleInput{Name: " "}, "name"},
		{"max below min", DiscountRuleInput{Name: "x", MinPosition: 3, MaxPosition: &max1}, "max_position"},
		{"percent zero", DiscountRuleInput{Name: "x", MinPosition: 2, DiscountType: DiscountTypePercent}, "percent_bps"},
		{"fixed zero", DiscountRuleInput{Name: "x", MinPosition: 2, DiscountType: DiscountTypeFixed}, "amount_cents"},
	}
	for _, tc := range bad {
		_, err := svc.CreateDiscountRule(cov1xOrg, "a", cov1xAdmin, tc.input)
		cov1xInvalidField(t, err, tc.field)
		_, _, err = svc.UpdateDiscountRule(cov1xOrg, "a", cov1xAdmin, "any", tc.input)
		cov1xInvalidField(t, err, tc.field)
	}

	for name, fn := range map[string]func() error{
		"list": func() error { _, err := svc.ListDiscountRules(cov1xOrg, cov1xParent); return err },
		"create": func() error {
			_, err := svc.CreateDiscountRule(cov1xOrg, "a", cov1xParent, DiscountRuleInput{})
			return err
		},
		"update": func() error {
			_, _, err := svc.UpdateDiscountRule(cov1xOrg, "a", cov1xParent, "x", DiscountRuleInput{})
			return err
		},
		"archive": func() error { _, err := svc.ArchiveDiscountRule(cov1xOrg, "a", cov1xParent, "x"); return err },
	} {
		if err := fn(); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: expected forbidden, got %v", name, err)
		}
	}

	// Percent rules zero the fixed amount, fixed rules zero the percentage.
	pct, err := svc.CreateDiscountRule(cov1xOrg, "a", cov1xAdmin, DiscountRuleInput{Name: "Pct", AppliesTo: "dues", MinPosition: 3, DiscountType: DiscountTypePercent, PercentBps: 500, AmountCents: 900})
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := svc.CreateDiscountRule(cov1xOrg, "a", cov1xAdmin, DiscountRuleInput{Name: "Fixed", AppliesTo: "all", MinPosition: 2, MaxPosition: &max3, DiscountType: DiscountTypeFixed, AmountCents: 700, PercentBps: 900})
	if err != nil {
		t.Fatal(err)
	}
	twin, err := svc.CreateDiscountRule(cov1xOrg, "a", cov1xAdmin, DiscountRuleInput{Name: "Aardvark", AppliesTo: "all", MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 100})
	if err != nil {
		t.Fatal(err)
	}
	if pct.AmountCents != 0 || fixed.PercentBps != 0 {
		t.Fatalf("discount type must zero the other field: pct=%+v fixed=%+v", pct, fixed)
	}
	if _, err := svc.CreateDiscountRule("other-org", "a", cov1xAdmin, DiscountRuleInput{Name: "Elsewhere", AppliesTo: "all", MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 1}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListDiscountRules(cov1xOrg, cov1xAdmin)
	if err != nil {
		t.Fatal(err)
	}
	// Ordered by min position, then name; other org excluded.
	if len(list) != 3 || list[0].ID != twin.ID || list[1].ID != fixed.ID || list[2].ID != pct.ID {
		t.Fatalf("unexpected order: %+v", list)
	}

	off := false
	before, after, err := svc.UpdateDiscountRule(cov1xOrg, "ed", cov1xAdmin, pct.ID, DiscountRuleInput{Name: "Pct2", AppliesTo: "dues", MinPosition: 4, DiscountType: DiscountTypePercent, PercentBps: 700, Active: &off})
	if err != nil {
		t.Fatal(err)
	}
	if before.Name != "Pct" || after.Name != "Pct2" || after.PercentBps != 700 || after.Active || after.UpdatedBy != "ed" {
		t.Fatalf("update: before=%+v after=%+v", before, after)
	}
	if _, after, err = svc.UpdateDiscountRule(cov1xOrg, "ed", cov1xAdmin, pct.ID, DiscountRuleInput{Name: "Pct2", AppliesTo: "dues", MinPosition: 4, DiscountType: DiscountTypePercent, PercentBps: 700}); err != nil || after.Active {
		t.Fatalf("nil active keeps value: %+v %v", after, err)
	}
	good := DiscountRuleInput{Name: "n", AppliesTo: "all", MinPosition: 2, DiscountType: DiscountTypeFixed, AmountCents: 1}
	if _, _, err := svc.UpdateDiscountRule(cov1xOrg, "ed", cov1xAdmin, "missing", good); !errors.Is(err, ErrDiscountRuleNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err := svc.UpdateDiscountRule("other-org", "ed", cov1xAdmin, fixed.ID, good); !errors.Is(err, ErrDiscountRuleNotFound) {
		t.Fatalf("cross-org: %v", err)
	}

	archived, err := svc.ArchiveDiscountRule(cov1xOrg, "arch", cov1xAdmin, fixed.ID)
	if err != nil || archived.Active || archived.UpdatedBy != "arch" {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	if _, err := svc.ArchiveDiscountRule(cov1xOrg, "arch", cov1xAdmin, "missing"); !errors.Is(err, ErrDiscountRuleNotFound) {
		t.Fatalf("missing archive: %v", err)
	}
	if _, err := svc.ArchiveDiscountRule("other-org", "arch", cov1xAdmin, twin.ID); !errors.Is(err, ErrDiscountRuleNotFound) {
		t.Fatalf("cross-org archive: %v", err)
	}
}

func TestCov1xAdjustmentsValidateListAndVoid(t *testing.T) {
	t.Parallel()
	svc := cov1xRosterService()

	if _, err := svc.ListAdjustments(cov1xOrg, cov1xCoach, "", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list forbidden: %v", err)
	}
	if _, err := svc.CreateAdjustment(cov1xOrg, "a", cov1xCoach, AdjustmentInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("create forbidden: %v", err)
	}
	if _, err := svc.VoidAdjustment(cov1xOrg, "a", cov1xCoach, "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("void forbidden: %v", err)
	}

	_, err := svc.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: "hh-a", AmountCents: 100, Reason: "  "})
	cov1xInvalidField(t, err, "reason")
	_, err = svc.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: "hh-nope", AmountCents: 100, Reason: "x"})
	cov1xInvalidField(t, err, "household_id")
	_, err = svc.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: "hh-a", SwimmerID: "sw-4", AmountCents: 100, Reason: "x"})
	cov1xInvalidField(t, err, "swimmer_id")

	one, err := svc.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: " hh-a ", SwimmerID: " sw-1 ", AmountCents: -500, Reason: " scholarship "})
	if err != nil {
		t.Fatal(err)
	}
	if one.HouseholdID != "hh-a" || one.SwimmerID != "sw-1" || one.Reason != "scholarship" || one.Status != AdjustmentStatusPending {
		t.Fatalf("create normalisation: %+v", one)
	}
	two, err := svc.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: "hh-b", AmountCents: 250, Reason: "late bus fee"})
	if err != nil {
		t.Fatal(err)
	}
	// Make the creation order observable regardless of clock resolution.
	svc.mu.Lock()
	svc.finance.adjustments[one.ID].CreatedAt = time.Now().UTC().Add(-time.Hour)
	svc.mu.Unlock()
	// Without a roster the references are not verified.
	bare := NewService()
	if _, err := bare.CreateAdjustment(cov1xOrg, "a", cov1xAdmin, AdjustmentInput{HouseholdID: "anything", SwimmerID: "any", AmountCents: 1, Reason: "r"}); err != nil {
		t.Fatalf("no roster: %v", err)
	}

	all, err := svc.ListAdjustments(cov1xOrg, cov1xAdmin, "", "")
	if err != nil || len(all) != 2 || all[0].ID != two.ID {
		t.Fatalf("list newest first: %+v %v", all, err)
	}
	if got, _ := svc.ListAdjustments(cov1xOrg, cov1xAdmin, "", "hh-a"); len(got) != 1 || got[0].ID != one.ID {
		t.Fatalf("household filter: %+v", got)
	}
	if got, _ := svc.ListAdjustments(cov1xOrg, cov1xAdmin, AdjustmentStatusApplied, ""); len(got) != 0 {
		t.Fatalf("status filter: %+v", got)
	}
	if got, _ := svc.ListAdjustments("other-org", cov1xAdmin, "", ""); len(got) != 0 {
		t.Fatalf("org filter: %+v", got)
	}

	if _, err := svc.VoidAdjustment(cov1xOrg, "a", cov1xAdmin, "missing"); !errors.Is(err, ErrAdjustmentNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.VoidAdjustment("other-org", "a", cov1xAdmin, one.ID); !errors.Is(err, ErrAdjustmentNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
	voided, err := svc.VoidAdjustment(cov1xOrg, "voider", cov1xAdmin, one.ID)
	if err != nil || voided.Status != AdjustmentStatusVoided || voided.UpdatedBy != "voider" {
		t.Fatalf("void: %+v %v", voided, err)
	}
	if _, err := svc.VoidAdjustment(cov1xOrg, "voider", cov1xAdmin, one.ID); !errors.Is(err, ErrAdjustmentNotPending) {
		t.Fatalf("second void: %v", err)
	}
}

func TestCov1xBillingSettings(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if _, err := svc.GetBillingSettings(cov1xOrg, cov1xCoach); !errors.Is(err, ErrForbidden) {
		t.Fatalf("get forbidden: %v", err)
	}
	if _, _, err := svc.UpdateBillingSettings(cov1xOrg, "a", cov1xCoach, BillingSettingsInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("update forbidden: %v", err)
	}
	def, err := svc.GetBillingSettings(cov1xOrg, cov1xAdmin)
	if err != nil || def.OrgID != cov1xOrg || def.LateFeeEnabled {
		t.Fatalf("defaults: %+v %v", def, err)
	}

	_, _, err = svc.UpdateBillingSettings(cov1xOrg, "a", cov1xAdmin, BillingSettingsInput{LateFeeEnabled: true, LateFeeType: LateFeeTypeFlat})
	cov1xInvalidField(t, err, "late_fee_flat_cents")
	_, _, err = svc.UpdateBillingSettings(cov1xOrg, "a", cov1xAdmin, BillingSettingsInput{LateFeeEnabled: true, LateFeeType: LateFeeTypePercent})
	cov1xInvalidField(t, err, "late_fee_percent_bps")

	before, after, err := svc.UpdateBillingSettings(cov1xOrg, "setter", cov1xAdmin, BillingSettingsInput{
		LateFeeEnabled: true, LateFeeType: LateFeeTypeFlat, LateFeeFlatCents: 2500, LateFeeGraceDays: 5,
		AutoRunDay: 3, AutoRunInvoiceStatus: "issued",
	})
	if err != nil {
		t.Fatal(err)
	}
	if before.LateFeeEnabled || !after.LateFeeEnabled || after.LateFeeFlatCents != 2500 || after.UpdatedBy != "setter" {
		t.Fatalf("update: before=%+v after=%+v", before, after)
	}
	if after.MeetFeeInvoiceStatus != "draft" {
		t.Fatalf("blank meet-fee status must default to draft, got %q", after.MeetFeeInvoiceStatus)
	}
	_, after, err = svc.UpdateBillingSettings(cov1xOrg, "setter", cov1xAdmin, BillingSettingsInput{LateFeeType: LateFeeTypeFlat, MeetFeeInvoiceStatus: "issued"})
	if err != nil || after.MeetFeeInvoiceStatus != "issued" {
		t.Fatalf("explicit meet-fee status: %+v %v", after, err)
	}
	got, err := svc.GetBillingSettings(cov1xOrg, cov1xAdmin)
	if err != nil || got.LateFeeEnabled || got.MeetFeeInvoiceStatus != "issued" {
		t.Fatalf("stored settings: %+v %v", got, err)
	}
	got.MeetFeeInvoiceStatus = "mutated"
	if again, _ := svc.GetBillingSettings(cov1xOrg, cov1xAdmin); again.MeetFeeInvoiceStatus != "issued" {
		t.Fatal("GetBillingSettings must return a copy")
	}
}
