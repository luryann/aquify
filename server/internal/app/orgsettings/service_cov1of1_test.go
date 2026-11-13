package orgsettings

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xStr(s string) *string { return &s }
func cov1xBool(b bool) *bool    { return &b }

func TestCov1xFindBySlugMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if got, err := svc.FindBySlug("   "); got != nil || err != nil {
		t.Fatalf("blank slug must yield (nil,nil), got %v %v", got, err)
	}
	if _, err := svc.Update("org-1", Updates{PublicSlug: cov1xStr("swim-club")}, "u"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.FindBySlug("  SWIM-Club ")
	if err != nil || got == nil || got.OrgID != "org-1" {
		t.Fatalf("normalised lookup failed: %+v %v", got, err)
	}
	got.OrgID = "mutated"
	again, _ := svc.FindBySlug("swim-club")
	if again.OrgID != "org-1" {
		t.Fatal("FindBySlug must return a copy")
	}
	if none, err := svc.FindBySlug("nope"); none != nil || err != nil {
		t.Fatalf("unknown slug: %v %v", none, err)
	}
}

func TestCov1xUpdateAppliesAndTrimsEveryField(t *testing.T) {
	t.Parallel()
	svc := NewService()
	got, err := svc.Update("org-1", Updates{
		TeamCode:           cov1xStr(" AQ "),
		TeamName:           cov1xStr(" Aqua "),
		LSCCode:            cov1xStr(" pc "),
		EmailSenderName:    cov1xStr(" Coach "),
		EmailSenderAddress: cov1xStr(" c@example.test "),
		PublicSlug:         cov1xStr("aqua-club"),
		PublicSiteEnabled:  cov1xBool(true),
		PublicShowCoaches:  cov1xBool(true),
		PublicAbout:        cov1xStr(" about us "),
		PublicContactEmail: cov1xStr(" hi@example.test "),
		PublicContactPhone: cov1xStr(" 555-0100 "),
	}, "user-9")
	if err != nil {
		t.Fatal(err)
	}
	want := OrgSettings{OrgID: "", TeamCode: "AQ", TeamName: "Aqua", LSCCode: "PC", EmailSenderName: "Coach",
		EmailSenderAddress: "c@example.test", PublicSlug: "aqua-club", PublicSiteEnabled: true, PublicShowCoaches: true,
		PublicAbout: "about us", PublicContactEmail: "hi@example.test", PublicContactPhone: "555-0100", UpdatedBy: "user-9"}
	got2 := *got
	got2.OrgID, got2.CreatedAt, got2.UpdatedAt = "", want.CreatedAt, want.UpdatedAt
	if got2 != want {
		t.Fatalf("got %+v want %+v", got2, want)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatal("timestamps must be stamped")
	}
	// Enabling the public site without a slug is rejected.
	if _, err := svc.Update("org-2", Updates{PublicSiteEnabled: cov1xBool(true)}, ""); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("site without slug: %v", err)
	}
	// Clearing the slug while still enabled is rejected; stored value is unchanged.
	if _, err := svc.Update("org-1", Updates{PublicSlug: cov1xStr("")}, ""); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("clearing slug while enabled: %v", err)
	}
	if cur, _ := svc.Get("org-1"); cur.PublicSlug != "aqua-club" {
		t.Fatalf("rejected update leaked: %+v", cur)
	}
}

func TestCov1xSwimmerAccountsMemory(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if on, err := svc.SwimmerAccountsEnabled("org-1"); err != nil || !on {
		t.Fatalf("default must be enabled: %v %v", on, err)
	}
	if err := svc.SetSwimmerAccountsEnabled(" org-1 ", false, "u1"); err != nil {
		t.Fatal(err)
	}
	if on, _ := svc.SwimmerAccountsEnabled("org-1"); on {
		t.Fatal("expected disabled")
	}
	if on, _ := svc.SwimmerAccountsEnabled("org-2"); !on {
		t.Fatal("other org must stay enabled")
	}
	if cur, _ := svc.Get("org-1"); cur.UpdatedBy != "u1" {
		t.Fatalf("row stamp missing: %+v", cur)
	}
	if err := svc.SetSwimmerAccountsEnabled("org-1", true, "u2"); err != nil {
		t.Fatal(err)
	}
	if on, _ := svc.SwimmerAccountsEnabled("org-1"); !on {
		t.Fatal("expected re-enabled")
	}
	if got, err := svc.WithSwimmerAccounts(nil); got != nil || err != nil {
		t.Fatalf("nil passthrough: %v %v", got, err)
	}
	_ = svc.SetSwimmerAccountsEnabled("org-3", false, "")
	s3, _ := svc.Get("org-3")
	filled, err := svc.WithSwimmerAccounts(s3)
	if err != nil || filled.SwimmerAccountsEnabled {
		t.Fatalf("WithSwimmerAccounts: %+v %v", filled, err)
	}
}

func TestCov1xNewServiceWithNilPoolIsInMemory(t *testing.T) {
	t.Parallel()
	if svc := NewServiceWithPool(nil); svc.repo != nil {
		t.Fatal("nil pool must stay in-memory")
	}
}

func TestCov1xDBRoundTrip(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	orgA, orgB := uuid.NewString(), uuid.NewString()

	blank, err := svc.Get(orgA)
	if err != nil || blank.OrgID != orgA || blank.TeamName != "" {
		t.Fatalf("missing row must yield blank default: %+v %v", blank, err)
	}
	if on, err := svc.SwimmerAccountsEnabled(orgA); err != nil || !on {
		t.Fatalf("no row -> enabled: %v %v", on, err)
	}
	got, err := svc.Update(orgA, Updates{TeamName: cov1xStr("Aqua"), PublicSlug: cov1xStr("aqua-db"), Timezone: cov1xStr("UTC")}, "")
	if err != nil || got.TeamName != "Aqua" || got.PublicSlug != "aqua-db" || got.Timezone != "UTC" {
		t.Fatalf("update: %+v %v", got, err)
	}
	found, err := svc.FindBySlug("AQUA-DB")
	if err != nil || found == nil || found.OrgID != orgA {
		t.Fatalf("find: %+v %v", found, err)
	}
	if none, err := svc.FindBySlug("missing-slug"); none != nil || err != nil {
		t.Fatalf("missing slug: %v %v", none, err)
	}
	// Same org re-using its own slug is fine; another org is refused.
	if _, err := svc.Update(orgA, Updates{PublicSlug: cov1xStr("aqua-db")}, ""); err != nil {
		t.Fatalf("own slug: %v", err)
	}
	if _, err := svc.Update(orgB, Updates{PublicSlug: cov1xStr("aqua-db")}, ""); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	if err := svc.SetSwimmerAccountsEnabled(orgA, false, ""); err != nil {
		t.Fatal(err)
	}
	filled, err := svc.WithSwimmerAccounts(got)
	if err != nil || filled.SwimmerAccountsEnabled {
		t.Fatalf("flag should be off: %+v %v", filled, err)
	}
	// Patching another field leaves the flag untouched (nil keeps stored value).
	if _, err := svc.Update(orgA, Updates{TeamCode: cov1xStr("X")}, ""); err != nil {
		t.Fatal(err)
	}
	if on, _ := svc.SwimmerAccountsEnabled(orgA); on {
		t.Fatal("flag must persist across unrelated updates")
	}
}

// Another instance claiming the slug between the check and the write is caught by the
// unique index and surfaced as ErrSlugTaken. The reservation hook runs after the
// lookup and before the write, which makes that window deterministic.
func TestCov1xDBSlugRaceMapsUniqueViolation(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	svc.SetSlugReserved(func(slug, _ string) bool {
		if err := svc.repo.Upsert(context.Background(), &OrgSettings{OrgID: orgB, PublicSlug: slug}, nil); err != nil {
			t.Errorf("seed competing row: %v", err)
		}
		return false
	})
	if _, err := svc.Update(orgA, Updates{PublicSlug: cov1xStr("race-slug")}, ""); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("race: %v", err)
	}
}

func TestCov1xDBWriteFailureIsReturned(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	svc.SetSlugReserved(func(string, string) bool { pool.Close(); return false })
	_, err := svc.Update(uuid.NewString(), Updates{PublicSlug: cov1xStr("closing-slug")}, "")
	if err == nil || errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected raw write error, got %v", err)
	}
}

func TestCov1xDBClosedPoolErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := NewServiceWithPool(pool)
	pool.Close()
	org := uuid.NewString()
	if _, err := svc.Get(org); err == nil {
		t.Fatal("Get must fail")
	}
	if _, err := svc.Update(org, Updates{TeamName: cov1xStr("x")}, ""); err == nil {
		t.Fatal("Update must fail")
	}
	if _, err := svc.FindBySlug("some-slug"); err == nil {
		t.Fatal("FindBySlug must fail")
	}
	if on, err := svc.SwimmerAccountsEnabled(org); err == nil || !on {
		t.Fatalf("must fail open-as-enabled with an error: %v %v", on, err)
	}
	if err := svc.SetSwimmerAccountsEnabled(org, false, ""); err == nil {
		t.Fatal("Set must fail")
	}
	if got, err := svc.WithSwimmerAccounts(&OrgSettings{OrgID: org}); err == nil || got != nil {
		t.Fatalf("WithSwimmerAccounts must propagate: %v %v", got, err)
	}
}

// Rows that cannot be scanned surface wrapped errors from each read path.
func TestCov1xDBScanFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := context.Background()
	svc := NewServiceWithPool(pool)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	if _, err := svc.Update(orgB, Updates{PublicSlug: cov1xStr("broken-row")}, ""); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`ALTER TABLE org_settings ALTER COLUMN team_code DROP NOT NULL`,
		`ALTER TABLE org_settings ALTER COLUMN swimmer_accounts_enabled DROP NOT NULL`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE org_settings SET team_code = NULL, swimmer_accounts_enabled = NULL WHERE org_id = $1::uuid`, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(orgB); err == nil {
		t.Fatal("Get must report scan failure")
	}
	if _, err := svc.FindBySlug("broken-row"); err == nil {
		t.Fatal("FindBySlug must report scan failure")
	}
	if _, err := svc.SwimmerAccountsEnabled(orgB); err == nil {
		t.Fatal("swimmer flag scan must fail")
	}
	// Claiming the broken row's slug from a healthy org surfaces the lookup error.
	if _, err := svc.Update(orgA, Updates{PublicSlug: cov1xStr("broken-row")}, ""); err == nil || errors.Is(err, ErrSlugTaken) {
		t.Fatalf("slug lookup failure must propagate, got %v", err)
	}
}
