package members

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/testutil"
)

type cov1xDB struct {
	t      *testing.T
	pool   *pgxpool.Pool
	org    string
	svc    *Service
	admin  *appmiddleware.Principal
	parent *appmiddleware.Principal
}

// cov1xNewDB builds a fresh isolated schema with one club, an admin user and a parent user.
func cov1xNewDB(t *testing.T) *cov1xDB {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	testutil.SeedOrgs(t, pool, org)
	d := &cov1xDB{t: t, pool: pool, org: org, svc: NewServiceWithPool(pool)}
	d.admin = &appmiddleware.Principal{UserID: d.user(), OrgID: org, Roles: []string{"admin"}}
	d.parent = &appmiddleware.Principal{UserID: d.user(), OrgID: org, Roles: []string{"parent"}}
	return d
}

// user inserts a real users row (deleted_by / consent / link-request columns are foreign keys).
func (d *cov1xDB) user() string {
	d.t.Helper()
	id := uuid.NewString()
	if _, err := d.pool.Exec(context.Background(), `INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $1, 'U', 'active', now(), now())`, id, d.org, id+"@example.test"); err != nil {
		d.t.Fatalf("insert user: %v", err)
	}
	return id
}

func (d *cov1xDB) exec(sql string, args ...any) {
	d.t.Helper()
	if _, err := d.pool.Exec(context.Background(), sql, args...); err != nil {
		d.t.Fatalf("exec %q: %v", sql, err)
	}
}

// addCheck installs a CHECK that only constrains rows written after this point.
func (d *cov1xDB) addCheck(table, name, expr string) {
	d.t.Helper()
	d.exec("ALTER TABLE " + table + " ADD CONSTRAINT " + name + " CHECK (" + expr + ") NOT VALID")
}

func (d *cov1xDB) dropCheck(table, name string) {
	d.t.Helper()
	d.exec("ALTER TABLE " + table + " DROP CONSTRAINT " + name)
}

// breakColumn renames a column so any query touching it fails, and returns the undo.
func (d *cov1xDB) breakColumn(table, col string) func() {
	d.t.Helper()
	d.exec("ALTER TABLE " + table + " RENAME COLUMN " + col + " TO cov1x_" + col)
	return func() { d.exec("ALTER TABLE " + table + " RENAME COLUMN cov1x_" + col + " TO " + col) }
}

func (d *cov1xDB) household(name string) Household {
	d.t.Helper()
	h, err := d.svc.CreateHousehold(d.admin, CreateHouseholdInput{OrgID: d.org, HouseholdName: name})
	if err != nil {
		d.t.Fatalf("CreateHousehold(%s): %v", name, err)
	}
	return h
}

func (d *cov1xDB) swimmer(hh, first string) Swimmer {
	d.t.Helper()
	sw, _, err := d.svc.CreateSwimmer(d.admin, d.org, CreateSwimmerInput{HouseholdID: hh, FirstName: first, LastName: "Test", DateOfBirth: "2000-02-03", Gender: "female"})
	if err != nil {
		d.t.Fatalf("CreateSwimmer(%s): %v", first, err)
	}
	return sw
}

func cov1xWantErr(t *testing.T, name string, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error containing %q", name, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: err = %v, want substring %q", name, err, substr)
	}
}

func TestCov1xDBLifecycleRoundTrip(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	svc := d.svc

	parentHH, err := svc.CreateHousehold(d.parent, CreateHouseholdInput{OrgID: d.org, HouseholdName: "Parent HH", PrimaryEmail: "P@Example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if parentHH.KeycloakUserID != d.parent.UserID {
		t.Fatalf("parent household must link its user durably, got %q", parentHH.KeycloakUserID)
	}
	adminHH := d.household("Admin HH")
	seedID := uuid.NewString()
	if err := svc.EnsureSeedHousehold(Household{ID: seedID, OrgID: d.org, HouseholdName: "Seed"}, d.parent.UserID); err != nil {
		t.Fatal(err)
	}

	prio := 2
	contact, err := svc.AddContact(d.admin, d.org, parentHH.ID, AddContactInput{Relationship: "mom", FirstName: "Mo", LastName: "Parent", Email: "mo@example.com", IsEmergencyContact: true, EmergencyPriority: &prio, Notes: "n"})
	if err != nil {
		t.Fatal(err)
	}
	sw, _, err := svc.CreateSwimmer(d.admin, d.org, CreateSwimmerInput{
		HouseholdID: parentHH.ID, FirstName: "Sam", LastName: "Parent", DateOfBirth: "2008-06-07", Gender: "male",
		SwimmerExternalID: "EXT-DB", USASwimmingID: "USA-DB", MedicalFlags: []string{"Asthma"}, Notes: "bio",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _, err := svc.UpdateSwimmer(d.parent, d.org, sw.ID, UpdateSwimmerInput{Allergies: cov1xStr("nuts"), EmergencyContactName: cov1xStr("Mo"), GuardianConsent: cov1xBool(true)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GuardianConsentBy != d.parent.UserID {
		t.Fatalf("consent by = %q", updated.GuardianConsentBy)
	}
	if _, err := svc.AssignSwimmerToGroup(d.admin, d.org, sw.ID, AssignSwimmerGroupInput{GroupCode: "gold", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignSwimmerToGroup(d.admin, d.org, sw.ID, AssignSwimmerGroupInput{GroupCode: "silver", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if !svc.HasPrimaryGroup(d.org, sw.ID) || svc.HasPrimaryGroup(d.org, uuid.NewString()) {
		t.Fatal("HasPrimaryGroup wrong against the database")
	}

	// A second instance sees every write exactly as persisted.
	other := NewServiceWithPool(d.pool)
	hhs, err := other.ListHouseholds(d.admin, d.org)
	if err != nil || len(hhs) != 3 {
		t.Fatalf("households after reload = %+v, %v", hhs, err)
	}
	contacts, err := other.ListContacts(d.admin, d.org, parentHH.ID)
	if err != nil || len(contacts) != 1 || contacts[0].ID != contact.ID || contacts[0].EmergencyPriority == nil || *contacts[0].EmergencyPriority != 2 || contacts[0].Email != "mo@example.com" {
		t.Fatalf("contacts after reload = %+v, %v", contacts, err)
	}
	got, err := other.GetSwimmer(d.admin, d.org, sw.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.SwimmerExternalID != "EXT-DB" || got.USASwimmingID != "USA-DB" || len(got.MedicalFlags) != 1 || got.MedicalFlags[0] != "asthma" ||
		got.Allergies != "nuts" || got.GuardianConsentAt == nil || got.GuardianConsentVersion != GuardianConsentVersion || got.DateOfBirth != "2008-06-07" {
		t.Fatalf("swimmer after reload = %+v", got)
	}
	links, err := other.ListSwimmerGroupLinks(d.admin, d.org)
	if err != nil || len(links) != 2 || links[0].GroupCode != "SILVER" || !links[0].IsPrimary || links[1].GroupCode != "GOLD" || links[1].IsPrimary {
		t.Fatalf("links after reload (demotion persisted) = %+v, %v", links, err)
	}
	if h, ok := other.GetHouseholdByKeycloakUserID(d.parent.UserID); !ok || h.ID != parentHH.ID {
		t.Fatalf("keycloak household lookup = %+v, %v", h, ok)
	}
	if _, ok := other.GetHouseholdByKeycloakUserID("kc-nobody"); ok {
		t.Fatal("unknown keycloak user must not resolve")
	}
	if _, ok := other.GetSwimmerByKeycloakUserID("kc-nobody"); ok {
		t.Fatal("unknown keycloak swimmer must not resolve")
	}
	if disabled, err := other.SwimmerPortalDisabled(d.org, sw.ID); err != nil || disabled {
		t.Fatalf("portal disabled = %v, %v", disabled, err)
	}

	// Links to sign-ins are persisted and resolved through the owner lookup.
	if err := svc.LinkSwimmerToUser(d.org, sw.ID, "kc-swimmer"); err != nil {
		t.Fatal(err)
	}
	if s, ok := NewServiceWithPool(d.pool).GetSwimmerByKeycloakUserID("kc-swimmer"); !ok || s.ID != sw.ID {
		t.Fatalf("swimmer lookup = %+v, %v", s, ok)
	}
	if err := svc.UnlinkSwimmerFromUser(d.org, sw.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := NewServiceWithPool(d.pool).GetSwimmerByKeycloakUserID("kc-swimmer"); ok {
		t.Fatal("unlinked swimmer must not resolve")
	}
	if err := svc.LinkHouseholdToUser(d.org, adminHH.ID, "kc-admin-hh"); err != nil {
		t.Fatal(err)
	}
	if h, ok := NewServiceWithPool(d.pool).GetHouseholdByKeycloakUserID("kc-admin-hh"); !ok || h.ID != adminHH.ID {
		t.Fatalf("household lookup = %+v, %v", h, ok)
	}
	if err := svc.UnlinkHouseholdFromUser(d.org, adminHH.ID); err != nil {
		t.Fatal(err)
	}

	// Household edits and deletes persist.
	if _, err := svc.UpdateHousehold(d.admin, d.org, adminHH.ID, UpdateHouseholdInput{HouseholdName: cov1xStr("Renamed")}); err != nil {
		t.Fatal(err)
	}
	if h, err := NewServiceWithPool(d.pool).GetHousehold(d.admin, d.org, adminHH.ID); err != nil || h.HouseholdName != "Renamed" {
		t.Fatalf("renamed household = %+v, %v", h, err)
	}
	if _, err := svc.DeleteHousehold(d.admin, d.org, adminHH.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceWithPool(d.pool).GetHousehold(d.admin, d.org, adminHH.ID); !errors.Is(err, ErrHouseholdNotFound) {
		t.Fatalf("deleted household must not reload, err = %v", err)
	}

	// Link requests go through the repository.
	if _, err := svc.CreateLinkRequest(d.org, uuid.NewString(), "ghost", ""); err == nil {
		t.Fatal("a request for a user that does not exist must be refused by the database")
	}
	req, err := svc.CreateLinkRequest(d.org, d.parent.UserID, " Sam ", "please")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := svc.CreateLinkRequest(d.org, d.parent.UserID, "other", ""); err != nil || again.ID != req.ID {
		t.Fatalf("repeat request = %+v, %v", again, err)
	}
	if _, err := svc.CreateLinkRequest(d.org, d.admin.UserID, "", ""); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.ListPendingLinkRequests(d.org)
	if err != nil || len(pending) != 2 || pending[0].ID != req.ID || pending[0].RequestedSwimmerName != "Sam" || pending[0].Note != "please" || pending[0].Status != "pending" {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if err := svc.ResolveLinkRequest(d.org, req.ID, d.admin.UserID, sw.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResolveLinkRequest(d.org, req.ID, d.admin.UserID, sw.ID); !errors.Is(err, ErrSwimmerLinkRequestNotFound) {
		t.Fatalf("second resolve err = %v", err)
	}
	if err := svc.ResolveLinkRequest(d.org, uuid.NewString(), d.admin.UserID, sw.ID); !errors.Is(err, ErrSwimmerLinkRequestNotFound) {
		t.Fatalf("unknown request err = %v", err)
	}
	if pending, _ := svc.ListPendingLinkRequests(d.org); len(pending) != 1 {
		t.Fatalf("resolved request must leave the queue: %+v", pending)
	}

	// Deleting a swimmer is blocked by history, then allowed once it is gone.
	d.exec(`INSERT INTO best_times (org_id, swimmer_id, event_distance, stroke, course) VALUES ($1::uuid, $2::uuid, 50, 'free', 'SCY')`, d.org, sw.ID)
	_, deps, err := svc.DeleteSwimmer(d.admin, d.org, sw.ID)
	if !errors.Is(err, ErrSwimmerHasDependencies) || len(deps) != 1 || deps[0] != (SwimmerDeleteDependency{Code: "best_times", Label: "best times", Count: 1}) {
		t.Fatalf("blocked delete = %+v, %v", deps, err)
	}
	if _, err := svc.GetSwimmer(d.admin, d.org, sw.ID, false); err != nil {
		t.Fatalf("a blocked delete must keep the swimmer: %v", err)
	}
	d.exec(`DELETE FROM best_times WHERE swimmer_id = $1::uuid`, sw.ID)
	if _, deps, err := svc.DeleteSwimmer(d.admin, d.org, sw.ID); err != nil || deps != nil {
		t.Fatalf("delete = %+v, %v", deps, err)
	}
	fresh := NewServiceWithPool(d.pool)
	if list, _ := fresh.ListSwimmers(d.admin, d.org, ListSwimmersInput{}); len(list) != 0 {
		t.Fatalf("deleted swimmer reloaded: %+v", list)
	}
	if l, _ := fresh.ListSwimmerGroupLinks(d.admin, d.org); len(l) != 0 {
		t.Fatalf("deleted swimmer's links reloaded: %+v", l)
	}

	// Erasure is persisted.
	keep := d.swimmer(parentHH.ID, "Erase")
	if err := svc.AnonymizeHousehold(d.org, parentHH.ID); err != nil {
		t.Fatal(err)
	}
	reloaded := NewServiceWithPool(d.pool)
	h, _ := reloaded.GetHousehold(d.admin, d.org, parentHH.ID)
	if h.HouseholdName != "Deleted Household" || h.PrimaryEmail != "" || h.KeycloakUserID != "" {
		t.Fatalf("household erasure not persisted: %+v", h)
	}
	if cs, _ := reloaded.ListContacts(d.admin, d.org, parentHH.ID); len(cs) != 1 || cs[0].FirstName != "Deleted" || cs[0].Email != "" {
		t.Fatalf("contact erasure not persisted: %+v", cs)
	}
	if s, _ := reloaded.GetSwimmer(d.admin, d.org, keep.ID, true); s.FirstName != "Deleted" || s.LastName != "Swimmer" {
		t.Fatalf("swimmer erasure not persisted: %+v", s)
	}

	// Cache management and the explicit all-club reload.
	svc.SetOrgCacheLimits(time.Hour, 5)
	if err := svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if hs, err := svc.ListHouseholds(d.admin, d.org); err != nil || len(hs) != 2 {
		t.Fatalf("households after Refresh = %+v, %v", hs, err)
	}
	if err := svc.RefreshOrg(ctx, d.org); err != nil {
		t.Fatal(err)
	}
	if svc.orgCache.Len() != 1 {
		t.Fatalf("one club must be tracked, got %d", svc.orgCache.Len())
	}
}

// cov1xBumpWriteGeneration completes one write-lock section, as a concurrent writer would.
func cov1xBumpWriteGeneration(svc *Service) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
}

func TestCov1xDBReloadFallsBackToLockedLoadAfterRepeatedConflicts(t *testing.T) {
	d := cov1xNewDB(t)
	d.household("Conflicted")
	svc := NewServiceWithPool(d.pool)

	var calls atomic.Int32
	svc.SetSyncHookForTest(func() {
		// The first three hook calls sit inside the lock-free attempts: land a write
		// each time so the snapshot is stale. The fourth runs under the lock.
		if calls.Add(1) <= 3 {
			cov1xBumpWriteGeneration(svc)
		}
	})
	list, err := svc.ListHouseholds(d.admin, d.org)
	if err != nil || len(list) != 1 || list[0].HouseholdName != "Conflicted" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("hook calls = %d, want 3 discarded attempts + 1 locked load", calls.Load())
	}

	// If the table breaks while writers keep winning, the locked fallback reports the failure.
	var undo func()
	calls.Store(0)
	svc.SetSyncHookForTest(func() {
		n := calls.Add(1)
		if n <= 3 {
			cov1xBumpWriteGeneration(svc)
		}
		if n == 3 {
			undo = d.breakColumn("households", "household_name")
		}
	})
	if _, err := svc.ListHouseholds(d.admin, d.org); err == nil || !strings.Contains(err.Error(), `column "household_name" does not exist`) {
		t.Fatalf("locked fallback must surface the load error, got %v", err)
	}
	undo()
}

func TestCov1xDBLoadFailuresAreReported(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	hh := d.household("Load")
	sw := d.swimmer(hh.ID, "Loader")
	if _, err := d.svc.AddContact(d.admin, d.org, hh.ID, AddContactInput{Relationship: "mom", FirstName: "M", LastName: "L"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.AssignSwimmerToGroup(d.admin, d.org, sw.ID, AssignSwimmerGroupInput{GroupCode: "g"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.CreateLinkRequest(d.org, d.parent.UserID, "x", "y"); err != nil {
		t.Fatal(err)
	}
	svc := NewServiceWithPool(d.pool)

	for _, tc := range []struct{ table, col, want string }{
		{"households", "household_name", `column "household_name" does not exist`},
		{"contacts", "relationship", `column "relationship" does not exist`},
		{"swimmers", "gender", `column "gender" does not exist`},
		{"swimmer_groups", "group_code", `column "group_code" does not exist`},
	} {
		undo := d.breakColumn(tc.table, tc.col)
		cov1xWantErr(t, tc.table+" scoped", svc.RefreshOrg(ctx, d.org), tc.want)
		cov1xWantErr(t, tc.table+" all clubs", svc.Refresh(ctx), tc.want)
		_, err := svc.ListHouseholds(d.admin, d.org)
		cov1xWantErr(t, tc.table+" request path", err, tc.want)
		undo()
		if err := svc.RefreshOrg(ctx, d.org); err != nil {
			t.Fatalf("%s restored: %v", tc.table, err)
		}
	}

	// A NULL in a NOT NULL-mapped column fails the row scan rather than loading a blank value.
	for _, tc := range []struct{ table, col, want string }{
		{"households", "household_name", "members: scan households"},
		{"contacts", "first_name", "members: scan contacts"},
		{"swimmers", "first_name", "members: scan swimmer"},
		{"swimmer_groups", "group_code", "members: scan swimmer group links"},
	} {
		d.exec("ALTER TABLE " + tc.table + " ALTER COLUMN " + tc.col + " DROP NOT NULL")
		d.exec("UPDATE " + tc.table + " SET " + tc.col + " = NULL")
		cov1xWantErr(t, tc.table+" scan", svc.RefreshOrg(ctx, d.org), tc.want)
		d.exec("UPDATE " + tc.table + " SET " + tc.col + " = 'x'")
	}

	// Link-request listing and resolution report database failures.
	undo := d.breakColumn("swimmer_link_requests", "note")
	_, err := d.svc.ListPendingLinkRequests(d.org)
	cov1xWantErr(t, "list requests", err, "members: list swimmer link requests")
	undo()
	d.exec("ALTER TABLE swimmer_link_requests ALTER COLUMN note DROP NOT NULL")
	d.exec("UPDATE swimmer_link_requests SET note = NULL")
	_, err = d.svc.ListPendingLinkRequests(d.org)
	cov1xWantErr(t, "scan requests", err, "members: scan swimmer link request")
	d.exec("UPDATE swimmer_link_requests SET note = ''")
	pending, err := d.svc.ListPendingLinkRequests(d.org)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	undo = d.breakColumn("swimmer_link_requests", "resolved_at")
	cov1xWantErr(t, "resolve", d.svc.ResolveLinkRequest(d.org, pending[0].ID, d.admin.UserID, sw.ID), "members: resolve swimmer link request")
	undo()

	// The sign-in owner lookups report failures instead of resolving the wrong club.
	d.exec(`UPDATE households SET keycloak_user_id = 'kc-load'`)
	d.exec(`UPDATE swimmers SET keycloak_user_id = 'kc-load'`)
	undo = d.breakColumn("households", "keycloak_user_id")
	_, _, err = d.svc.repo.OrgOfHouseholdKeycloakUser(ctx, "kc-load")
	cov1xWantErr(t, "household owner lookup", err, "members: find households by keycloak user")
	if _, ok := svc.GetHouseholdByKeycloakUserID("kc-load"); ok {
		t.Fatal("a failing owner lookup must not resolve a household")
	}
	undo()
	undo = d.breakColumn("swimmers", "keycloak_user_id")
	_, _, err = d.svc.repo.OrgOfSwimmerKeycloakUser(ctx, "kc-load")
	cov1xWantErr(t, "swimmer owner lookup", err, "members: find swimmers by keycloak user")
	if _, ok := svc.GetSwimmerByKeycloakUserID("kc-load"); ok {
		t.Fatal("a failing owner lookup must not resolve a swimmer")
	}
	undo()
	// The owner is found but its club cannot be loaded.
	undo = d.breakColumn("households", "household_name")
	if _, ok := NewServiceWithPool(d.pool).GetHouseholdByKeycloakUserID("kc-load"); ok {
		t.Fatal("an unloadable club must not resolve a household")
	}
	undo()
	undo = d.breakColumn("swimmers", "gender")
	if _, ok := NewServiceWithPool(d.pool).GetSwimmerByKeycloakUserID("kc-load"); ok {
		t.Fatal("an unloadable club must not resolve a swimmer")
	}
	undo()
}

func TestCov1xDBPersistFailuresLeaveMemoryUntouched(t *testing.T) {
	d := cov1xNewDB(t)
	svc := d.svc
	base := d.household("Base")
	contact, err := svc.AddContact(d.admin, d.org, base.ID, AddContactInput{Relationship: "mom", FirstName: "Mo", LastName: "Ba"})
	if err != nil {
		t.Fatal(err)
	}
	_ = contact
	adult := d.swimmer(base.ID, "Adult")

	// Household writes.
	d.addCheck("households", "cov1x_poison", "household_name <> 'poison'")
	if _, err := svc.CreateHousehold(d.admin, CreateHouseholdInput{OrgID: d.org, HouseholdName: "poison"}); err == nil || !strings.Contains(err.Error(), "persist household") {
		t.Fatalf("create err = %v", err)
	}
	if got := svc.SnapshotHouseholds(d.org); len(got) != 1 {
		t.Fatalf("a failed create must not be cached, have %d households", len(got))
	}
	cov1xWantErr(t, "seed", svc.EnsureSeedHousehold(Household{ID: uuid.NewString(), OrgID: d.org, HouseholdName: "poison"}, ""), "persist seed household")
	_, err = svc.UpdateHousehold(d.admin, d.org, base.ID, UpdateHouseholdInput{HouseholdName: cov1xStr("poison")})
	cov1xWantErr(t, "update", err, "persist household")
	if got, _ := svc.GetHousehold(d.admin, d.org, base.ID); got.HouseholdName != "Base" {
		t.Fatalf("a failed update must not change the cache, got %q", got.HouseholdName)
	}
	d.dropCheck("households", "cov1x_poison")

	d.addCheck("households", "cov1x_nodel", "deleted_at IS NULL OR household_name <> 'Pinned'")
	pinned := d.household("Pinned")
	_, err = svc.DeleteHousehold(d.admin, d.org, pinned.ID)
	cov1xWantErr(t, "delete household", err, "delete household")
	if !svc.HouseholdExists(d.org, pinned.ID) {
		t.Fatal("a failed delete must keep the household cached")
	}
	d.dropCheck("households", "cov1x_nodel")

	// Contact and swimmer writes.
	d.addCheck("contacts", "cov1x_poison", "first_name <> 'poison'")
	_, err = svc.AddContact(d.admin, d.org, base.ID, AddContactInput{Relationship: "dad", FirstName: "poison", LastName: "X"})
	cov1xWantErr(t, "add contact", err, "persist contact")
	d.dropCheck("contacts", "cov1x_poison")

	d.addCheck("swimmers", "cov1x_poison", "first_name <> 'poison'")
	_, _, err = svc.CreateSwimmer(d.admin, d.org, CreateSwimmerInput{HouseholdID: base.ID, FirstName: "poison", LastName: "X", DateOfBirth: "2000-01-01", Gender: "male"})
	cov1xWantErr(t, "create swimmer", err, "persist swimmer")
	_, _, err = svc.UpdateSwimmer(d.admin, d.org, adult.ID, UpdateSwimmerInput{FirstName: cov1xStr("poison")})
	cov1xWantErr(t, "update swimmer", err, "persist swimmer")
	if got, _ := svc.GetSwimmer(d.admin, d.org, adult.ID, false); got.FirstName != "Adult" {
		t.Fatalf("a failed update must not change the cache, got %q", got.FirstName)
	}
	d.dropCheck("swimmers", "cov1x_poison")

	// Sign-in link writes.
	d.addCheck("households", "cov1x_kc", "keycloak_user_id <> 'poison'")
	cov1xWantErr(t, "link household", svc.LinkHouseholdToUser(d.org, base.ID, "poison"), "persist household link")
	d.dropCheck("households", "cov1x_kc")
	d.addCheck("swimmers", "cov1x_kc", "keycloak_user_id <> 'poison'")
	cov1xWantErr(t, "link swimmer", svc.LinkSwimmerToUser(d.org, adult.ID, "poison"), "persist swimmer link")
	d.dropCheck("swimmers", "cov1x_kc")

	// Link-request insert.
	d.addCheck("swimmer_link_requests", "cov1x_poison", "note <> 'poison'")
	_, err = svc.CreateLinkRequest(d.org, d.parent.UserID, "", "poison")
	cov1xWantErr(t, "create link request", err, "swimmer_link_requests")
	d.dropCheck("swimmer_link_requests", "cov1x_poison")

	// Erasure fails stage by stage and reports which stage.
	victim := d.household("Victim")
	if _, err := svc.AddContact(d.admin, d.org, victim.ID, AddContactInput{Relationship: "mom", FirstName: "Vic", LastName: "Tim"}); err != nil {
		t.Fatal(err)
	}
	d.swimmer(victim.ID, "Vick")
	d.addCheck("households", "cov1x_anon", "household_name <> 'Deleted Household'")
	cov1xWantErr(t, "anonymize household", svc.AnonymizeHousehold(d.org, victim.ID), "anonymize household")
	d.dropCheck("households", "cov1x_anon")
	d.addCheck("contacts", "cov1x_anon", "first_name <> 'Deleted'")
	cov1xWantErr(t, "anonymize contact", svc.AnonymizeHousehold(d.org, victim.ID), "anonymize contact")
	d.dropCheck("contacts", "cov1x_anon")
	d.addCheck("swimmers", "cov1x_anon", "first_name <> 'Deleted'")
	cov1xWantErr(t, "anonymize swimmer", svc.AnonymizeHousehold(d.org, victim.ID), "anonymize swimmer")
	d.dropCheck("swimmers", "cov1x_anon")

	// Swimmer delete fails at each step of its transaction.
	d.addCheck("swimmers", "cov1x_sdel", "deleted_at IS NULL OR first_name <> 'Pinned'")
	_, _, err = svc.DeleteSwimmer(d.admin, d.org, d.swimmer(base.ID, "Pinned").ID)
	cov1xWantErr(t, "delete swimmer row", err, "members: delete swimmer")
	d.dropCheck("swimmers", "cov1x_sdel")

	grp := d.swimmer(base.ID, "Grouped")
	if _, err := svc.AssignSwimmerToGroup(d.admin, d.org, grp.ID, AssignSwimmerGroupInput{GroupCode: "nodel"}); err != nil {
		t.Fatal(err)
	}
	d.addCheck("swimmer_groups", "cov1x_gdel", "deleted_at IS NULL OR group_code <> 'NODEL'")
	_, _, err = svc.DeleteSwimmer(d.admin, d.org, grp.ID)
	cov1xWantErr(t, "delete swimmer groups", err, "members: delete swimmer groups")
	d.dropCheck("swimmer_groups", "cov1x_gdel")

	d.exec(`CREATE FUNCTION cov1x_commit_fail() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.deleted_at IS NOT NULL AND NEW.first_name = 'Commitfail' THEN
				RAISE EXCEPTION 'cov1x commit refused';
			END IF;
			RETURN NEW;
		END $$`)
	d.exec(`CREATE CONSTRAINT TRIGGER cov1x_commit_fail AFTER UPDATE ON swimmers DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION cov1x_commit_fail()`)
	cf := d.swimmer(base.ID, "Commitfail")
	_, _, err = svc.DeleteSwimmer(d.admin, d.org, cf.ID)
	cov1xWantErr(t, "commit swimmer delete", err, "members: commit swimmer delete")
	if _, err := svc.GetSwimmer(d.admin, d.org, cf.ID, false); err != nil {
		t.Fatalf("a failed delete must keep the swimmer: %v", err)
	}
	d.exec(`DROP TRIGGER cov1x_commit_fail ON swimmers`)

	// Group assignment: every persistence failure leaves the in-memory links as they were.
	gs := d.swimmer(base.ID, "Grouper")
	assign := func(code string, primary bool) error {
		_, err := svc.AssignSwimmerToGroup(d.admin, d.org, gs.ID, AssignSwimmerGroupInput{GroupCode: code, IsPrimary: primary})
		return err
	}
	linksOf := func() map[string]bool {
		out := map[string]bool{}
		for _, l := range svc.SnapshotSwimmerGroupLinks(d.org) {
			if l.SwimmerID == gs.ID {
				out[l.GroupCode] = l.IsPrimary
			}
		}
		return out
	}
	d.addCheck("swimmer_groups", "cov1x_g1", "group_code <> 'POISON'")
	cov1xWantErr(t, "first link", assign("poison", false), "persist swimmer group link")
	if len(linksOf()) != 0 {
		t.Fatalf("failed first link cached: %v", linksOf())
	}
	if err := assign("aaa", true); err != nil {
		t.Fatal(err)
	}
	// The new primary is refused after the old primary was demoted: the demotion is undone.
	cov1xWantErr(t, "undo demotion", assign("poison", true), "persist swimmer group link")
	if got := linksOf(); len(got) != 1 || !got["AAA"] {
		t.Fatalf("links after undone demotion = %v", got)
	}
	fresh, _ := NewServiceWithPool(d.pool).ListSwimmerGroupLinks(d.admin, d.org)
	var mine []SwimmerGroupLink
	for _, l := range fresh {
		if l.SwimmerID == gs.ID {
			mine = append(mine, l)
		}
	}
	if len(mine) != 1 || !mine[0].IsPrimary {
		t.Fatalf("the demotion must be undone in the database too: %+v", mine)
	}
	// If the undo itself is refused it is logged, and the error still reports the original failure.
	d.addCheck("swimmer_groups", "cov1x_g3", "is_primary = false OR group_code <> 'AAA'")
	cov1xWantErr(t, "failed undo", assign("poison", true), "persist swimmer group link")
	if got := linksOf(); len(got) != 1 || !got["AAA"] {
		t.Fatalf("links after failed undo = %v", got)
	}
	d.dropCheck("swimmer_groups", "cov1x_g3")
	d.dropCheck("swimmer_groups", "cov1x_g1")
	// Demoting the old primary is itself refused.
	d.exec(`UPDATE swimmer_groups SET is_primary = true WHERE group_code = 'AAA'`)
	d.addCheck("swimmer_groups", "cov1x_g4", "group_code <> 'AAA' OR is_primary")
	cov1xWantErr(t, "demotion refused", assign("bbb", true), "persist demoted primary group link")
	if got := linksOf(); len(got) != 1 || !got["AAA"] {
		t.Fatalf("links after refused demotion = %v", got)
	}
	d.dropCheck("swimmer_groups", "cov1x_g4")

	// Finally, a dependency check that cannot run is an error, not "no dependencies".
	d.breakColumn("invoice_items", "swimmer_id")
	_, _, err = svc.DeleteSwimmer(d.admin, d.org, adult.ID)
	cov1xWantErr(t, "dependency check", err, "members: swimmer delete dependencies")
}

func TestCov1xRepositoryDirect(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	repo := d.svc.repo
	now := time.Now().UTC().Truncate(time.Microsecond)

	orgs, err := repo.OrgIDs(ctx)
	if err != nil || len(orgs) == 0 {
		t.Fatalf("OrgIDs = %v, %v", orgs, err)
	}
	if flags, err := decodeMedicalFlags("  "); err != nil || flags != nil {
		t.Fatalf("blank flags = %v, %v", flags, err)
	}
	hh := Household{ID: uuid.NewString(), OrgID: d.org, HouseholdName: "Direct", Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertHousehold(ctx, hh); err != nil {
		t.Fatal(err)
	}
	c := Contact{ID: uuid.NewString(), OrgID: d.org, HouseholdID: hh.ID, Relationship: "mom", FirstName: "A", LastName: "B", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertContact(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.FirstName = "Changed"
	if err := repo.UpsertContact(ctx, c); err != nil {
		t.Fatal(err)
	}
	households, contacts, _, _, err := repo.LoadAll(ctx, d.org)
	if err != nil || len(households) != 1 || len(contacts) != 1 || contacts[0].FirstName != "Changed" {
		t.Fatalf("LoadAll = %+v %+v, %v", households, contacts, err)
	}
	if err := repo.DeleteHousehold(ctx, d.org, hh.ID, d.admin.UserID); err != nil {
		t.Fatal(err)
	}
	if households, _, _, _, _ := repo.LoadAll(ctx, d.org); len(households) != 0 {
		t.Fatalf("soft-deleted household reloaded: %+v", households)
	}
	// Another club's id cannot delete this club's rows.
	hh2 := Household{ID: uuid.NewString(), OrgID: d.org, HouseholdName: "Stay", Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertHousehold(ctx, hh2); err != nil {
		t.Fatal(err)
	}
	foreign := uuid.NewString()
	testutil.SeedOrgs(t, d.pool, foreign)
	if err := repo.DeleteHousehold(ctx, foreign, hh2.ID, d.admin.UserID); err != nil {
		t.Fatal(err)
	}
	if households, _, _, _, _ := repo.LoadAll(ctx, d.org); len(households) != 1 {
		t.Fatal("a foreign club must not delete this club's household")
	}
	sw := Swimmer{ID: uuid.NewString(), OrgID: d.org, HouseholdID: hh2.ID, FirstName: "A", LastName: "B", DateOfBirth: "2010-01-02", Gender: "male", RegistrationStatus: "active", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertSwimmer(ctx, sw); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSwimmerKeycloakUserID(ctx, d.org, sw.ID, "kc-direct"); err != nil {
		t.Fatal(err)
	}
	if org, ok, err := repo.OrgOfSwimmerKeycloakUser(ctx, "kc-direct"); err != nil || !ok || org != d.org {
		t.Fatalf("owner = %q %v %v", org, ok, err)
	}
	if err := repo.SetSwimmerKeycloakUserID(ctx, d.org, sw.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := repo.OrgOfSwimmerKeycloakUser(ctx, "kc-direct"); ok {
		t.Fatal("cleared link must not resolve")
	}
	deps, err := repo.ListSwimmerDeleteDependencies(ctx, d.org, sw.ID)
	if err != nil || len(deps) != 0 {
		t.Fatalf("no dependencies expected, got %+v, %v", deps, err)
	}
}

func cov1xClosedService(t *testing.T) *Service {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://cov1x:cov1x@127.0.0.1:1/cov1x?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	return NewServiceWithPool(pool)
}

// With no usable database every operation reports the failure instead of serving stale or empty data.
func TestCov1xClosedPoolSurfacesErrorsEverywhere(t *testing.T) {
	t.Parallel()
	svc := cov1xClosedService(t)
	ctx := context.Background()
	org := uuid.NewString()
	id := uuid.NewString()
	admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}}
	now := time.Now().UTC()

	calls := map[string]func() error{
		"Refresh":    func() error { return svc.Refresh(ctx) },
		"RefreshOrg": func() error { return svc.RefreshOrg(ctx, org) },
		"CreateHousehold": func() error {
			_, err := svc.CreateHousehold(admin, CreateHouseholdInput{OrgID: org, HouseholdName: "X"})
			return err
		},
		"EnsureSeedHousehold": func() error { return svc.EnsureSeedHousehold(Household{ID: id, OrgID: org, HouseholdName: "X"}, "") },
		"UpdateHousehold": func() error {
			_, err := svc.UpdateHousehold(admin, org, id, UpdateHouseholdInput{})
			return err
		},
		"DeleteHousehold": func() error { _, err := svc.DeleteHousehold(admin, org, id); return err },
		"GetHouseholdErasureContext": func() error {
			_, err := svc.GetHouseholdErasureContext(org, id)
			return err
		},
		"AnonymizeHousehold": func() error { return svc.AnonymizeHousehold(org, id) },
		"GetHousehold":       func() error { _, err := svc.GetHousehold(admin, org, id); return err },
		"ListHouseholds":     func() error { _, err := svc.ListHouseholds(admin, org); return err },
		"AddContact": func() error {
			_, err := svc.AddContact(admin, org, id, AddContactInput{})
			return err
		},
		"CreateSwimmer": func() error {
			_, _, err := svc.CreateSwimmer(admin, org, CreateSwimmerInput{})
			return err
		},
		"UpdateSwimmer": func() error {
			_, _, err := svc.UpdateSwimmer(admin, org, id, UpdateSwimmerInput{})
			return err
		},
		"DeleteSwimmer": func() error { _, _, err := svc.DeleteSwimmer(admin, org, id); return err },
		"GetSwimmer":    func() error { _, err := svc.GetSwimmer(admin, org, id, false); return err },
		"AssignSwimmerToGroup": func() error {
			_, err := svc.AssignSwimmerToGroup(admin, org, id, AssignSwimmerGroupInput{})
			return err
		},
		"ListSwimmers":          func() error { _, err := svc.ListSwimmers(admin, org, ListSwimmersInput{}); return err },
		"ListSwimmerGroupLinks": func() error { _, err := svc.ListSwimmerGroupLinks(admin, org); return err },
		"AssociateParentToHousehold": func() error {
			return svc.AssociateParentToHousehold(admin, org, "parent", id)
		},
		"ListContacts":        func() error { _, err := svc.ListContacts(admin, org, id); return err },
		"SnapshotRoster":      func() error { _, err := svc.SnapshotRoster(org); return err },
		"LinkHouseholdToUser": func() error { return svc.LinkHouseholdToUser(org, id, "kc") },
		"LinkSwimmerToUser":   func() error { return svc.LinkSwimmerToUser(org, id, "kc") },
		"SwimmerPortalDisabled": func() error {
			_, err := svc.SwimmerPortalDisabled(org, id)
			return err
		},
		"CreateLinkRequest": func() error { _, err := svc.CreateLinkRequest(org, id, "n", "n"); return err },
		"ListPendingLinkRequests": func() error {
			_, err := svc.ListPendingLinkRequests(org)
			return err
		},
		"ResolveLinkRequest": func() error { return svc.ResolveLinkRequest(org, id, id, id) },
		"repo.UpsertHousehold": func() error {
			return svc.repo.UpsertHousehold(ctx, Household{ID: id, OrgID: org, HouseholdName: "X", Status: "active", CreatedAt: now, UpdatedAt: now})
		},
		"repo.SetHouseholdKeycloakUserID": func() error { return svc.repo.SetHouseholdKeycloakUserID(ctx, org, id, "kc") },
		"repo.DeleteHousehold":            func() error { return svc.repo.DeleteHousehold(ctx, org, id, id) },
		"repo.UpsertContact":              func() error { return svc.repo.UpsertContact(ctx, Contact{ID: id, OrgID: org, HouseholdID: id}) },
		"repo.UpsertSwimmer":              func() error { return svc.repo.UpsertSwimmer(ctx, Swimmer{ID: id, OrgID: org, HouseholdID: id}) },
		"repo.SetSwimmerKeycloakUserID":   func() error { return svc.repo.SetSwimmerKeycloakUserID(ctx, org, id, "kc") },
		"repo.ListSwimmerDeleteDependencies": func() error {
			_, err := svc.repo.ListSwimmerDeleteDependencies(ctx, org, id)
			return err
		},
		"repo.DeleteSwimmer": func() error { return svc.repo.DeleteSwimmer(ctx, org, id, id) },
		"repo.UpsertSwimmerGroupLink": func() error {
			return svc.repo.UpsertSwimmerGroupLink(ctx, SwimmerGroupLink{ID: id, OrgID: org, SwimmerID: id})
		},
		"repo.InsertSwimmerLinkRequest": func() error {
			return svc.repo.InsertSwimmerLinkRequest(ctx, SwimmerLinkRequest{ID: id, OrgID: org, UserID: id})
		},
		"repo.ResolveSwimmerLinkRequest": func() error { return svc.repo.ResolveSwimmerLinkRequest(ctx, org, id, id, id) },
		"repo.OrgIDs":                    func() error { _, err := svc.repo.OrgIDs(ctx); return err },
		"repo.LoadAll(all)": func() error {
			_, _, _, _, err := svc.repo.LoadAll(ctx, "")
			return err
		},
		"repo.LoadAll(org)": func() error {
			_, _, _, _, err := svc.repo.LoadAll(ctx, org)
			return err
		},
		"repo.OrgOfHouseholdKeycloakUser": func() error { _, _, err := svc.repo.OrgOfHouseholdKeycloakUser(ctx, "kc"); return err },
		"repo.OrgOfSwimmerKeycloakUser":   func() error { _, _, err := svc.repo.OrgOfSwimmerKeycloakUser(ctx, "kc"); return err },
		"repo.ListPendingSwimmerLinkRequests": func() error {
			_, err := svc.repo.ListPendingSwimmerLinkRequests(ctx, org)
			return err
		},
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: expected an error from an unusable database", name)
		}
	}

	if err := svc.repo.DeleteSwimmer(ctx, org, id, id); err == nil || !strings.Contains(err.Error(), "members: begin swimmer delete") {
		t.Errorf("repo.DeleteSwimmer err = %v", err)
	}
	// Boolean lookups fail safe.
	if !svc.HasPrimaryGroup(org, id) {
		t.Error("HasPrimaryGroup must assume a primary exists when the database is down")
	}
	if _, ok := svc.SwimmerHouseholdID(org, id); ok {
		t.Error("SwimmerHouseholdID must not resolve")
	}
	if _, ok := svc.SwimmerEligibility(org, id); ok {
		t.Error("SwimmerEligibility must not resolve")
	}
	if _, ok := svc.GetHouseholdByKeycloakUserID("kc"); ok {
		t.Error("GetHouseholdByKeycloakUserID must not resolve")
	}
	if _, ok := svc.GetSwimmerByKeycloakUserID("kc"); ok {
		t.Error("GetSwimmerByKeycloakUserID must not resolve")
	}
	if got := svc.newID("x_"); len(got) != 36 {
		t.Errorf("DB-backed ids are UUIDs, got %q", got)
	}
}

// A query that cannot even be issued (the caller gave up) is reported with the table it was loading.
func TestCov1xDBLoadQueriesReportCancellation(t *testing.T) {
	d := cov1xNewDB(t)
	repo := d.svc.repo
	ctx, cancel := context.WithCancel(context.Background())
	tx, err := repo.at(d.org).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	cancel()

	_, err = loadHouseholdsInTx(ctx, tx, d.org)
	cov1xWantErr(t, "households", err, "members: load households")
	_, err = loadContactsInTx(ctx, tx, d.org)
	cov1xWantErr(t, "contacts", err, "members: load contacts")
	_, err = loadSwimmersInTx(ctx, tx, d.org)
	cov1xWantErr(t, "swimmers", err, "members: load swimmers")
	_, err = loadSwimmerGroupLinksInTx(ctx, tx, d.org)
	cov1xWantErr(t, "links", err, "members: load swimmer group links")
	if _, err := repo.ListPendingSwimmerLinkRequests(context.Background(), "not-a-uuid"); err == nil || !strings.Contains(err.Error(), "members: list swimmer link requests") {
		t.Fatalf("an unusable org id must be reported, got %v", err)
	}
	if _, _, _, _, err := repo.LoadAll(ctx, d.org); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadAll on a cancelled context err = %v", err)
	}
}
