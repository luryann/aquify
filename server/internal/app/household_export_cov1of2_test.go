package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/comms"
	"github.com/luryann/aquify/server/internal/app/documents"
	"github.com/luryann/aquify/server/internal/app/events"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/app/volunteer"
	"github.com/luryann/aquify/server/internal/app/wellness"
	"github.com/luryann/aquify/server/internal/postmark"
	"github.com/luryann/aquify/server/internal/testutil"
)

// cov1yRichExport is one household with records in every domain plus a second
// household whose records must never appear in the first one's export.
type cov1yRichExport struct {
	org, hh, other, swimmer string
	membersSvc              *members.Service
	groupsSvc               *groups.Service
	billingSvc              *billing.Service
	sources                 []householdExportSource
}

func cov1yBuildRichExport(t *testing.T) *cov1yRichExport {
	t.Helper()
	const org = "org-cov1y-export"
	admin := cov1xAdmin(org)
	ctx := context.Background()
	f := &cov1yRichExport{org: org}

	// Members: two households, contacts, swimmer, group link.
	f.membersSvc = members.NewService()
	hh, err := f.membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Rich Family", PrimaryEmail: "rich@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Foreign Family"})
	if err != nil {
		t.Fatal(err)
	}
	f.hh, f.other = hh.ID, other.ID
	for _, h := range []string{hh.ID, other.ID} {
		if _, err := f.membersSvc.AddContact(admin, org, h, members.AddContactInput{Relationship: "parent", FirstName: "Pat", LastName: "Contact", Email: "pat-" + h[:6] + "@example.test", IsPrimary: true}); err != nil {
			t.Fatal(err)
		}
	}
	sw, _, err := f.membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: hh.ID, FirstName: "Rae", LastName: "Rich", DateOfBirth: "2011-06-07", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}
	foreignSwimmer, _, err := f.membersSvc.CreateSwimmer(admin, org, members.CreateSwimmerInput{HouseholdID: other.ID, FirstName: "Fay", LastName: "Foreign", DateOfBirth: "2011-01-01", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}
	f.swimmer = sw.ID

	// Groups: group, link, registration + waiver for both households' swimmers.
	f.groupsSvc = groups.NewService()
	grp, _, err := f.groupsSvc.CreateGroup(admin, org, groups.CreateGroupInput{Code: "GOLD", Name: "Gold"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.membersSvc.AssignSwimmerToGroup(admin, org, sw.ID, members.AssignSwimmerGroupInput{GroupCode: "GOLD", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.membersSvc.AssignSwimmerToGroup(admin, org, foreignSwimmer.ID, members.AssignSwimmerGroupInput{GroupCode: "GOLD", IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
	f.groupsSvc.SeedSwimmer(org, sw.ID, hh.ID, "2011-06-07", "female", "active")
	f.groupsSvc.SeedSwimmer(org, foreignSwimmer.ID, other.ID, "2011-01-01", "female", "active")
	for _, pair := range [][2]string{{hh.ID, sw.ID}, {other.ID, foreignSwimmer.ID}} {
		reg, err := f.groupsSvc.CreateRegistration(admin, org, groups.CreateRegistrationInput{HouseholdID: pair[0], SwimmerID: pair[1], GroupID: grp.ID, Season: "2026-27"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.groupsSvc.AcceptWaiver(admin, org, reg.ID, groups.AcceptWaiverInput{WaiverType: "liability", IPAddress: "203.0.113.9"}); err != nil {
			t.Fatal(err)
		}
	}

	// Billing: an invoice with a line item, payment and refund per household.
	f.billingSvc = billing.NewService()
	for i, h := range []string{hh.ID, other.ID} {
		inv, err := f.billingSvc.CreateInvoice(org, "admin", admin, billing.CreateInvoiceInput{HouseholdID: h, InvoiceNumber: "INV-COV1Y-" + string(rune('A'+i))})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.billingSvc.AddInvoiceItem(org, "admin", admin, inv.ID, billing.AddInvoiceItemInput{Description: "Dues", Quantity: 1, UnitAmountCents: 5000, Category: "program_fee", SwimmerID: cov1yPick(i, sw.ID, foreignSwimmer.ID)}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.billingSvc.IssueInvoice(org, "admin", admin, inv.ID); err != nil {
			t.Fatal(err)
		}
		pay, _, err := f.billingSvc.RecordPayment(org, "admin", admin, inv.ID, billing.RecordPaymentInput{AmountCents: 2000, Method: "cash"})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.billingSvc.CreateRefund(org, "admin", admin, inv.ID, billing.CreateRefundInput{PaymentID: pay.ID, AmountCents: 500, Reason: "goodwill"}); err != nil {
			t.Fatal(err)
		}
	}

	// Meets: commitment + event selection + entry.
	meetsSvc := meets.NewService()
	meetsSvc.SeedSwimmer(org, sw.ID, hh.ID)
	meetsSvc.SeedSwimmer(org, foreignSwimmer.ID, other.ID)
	meet, _, err := meetsSvc.CreateMeet(admin, org, meets.CreateMeetInput{Name: "Export Open", StartDate: "2027-05-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := meetsSvc.TransitionMeetStatus(admin, org, meet.ID, "published"); err != nil {
		t.Fatal(err)
	}
	session, _, err := meetsSvc.CreateMeetSession(admin, org, meet.ID, meets.CreateMeetSessionInput{SessionNumber: 1, Name: "Saturday AM", SessionDate: "2027-05-01"})
	if err != nil {
		t.Fatal(err)
	}
	ev, _, err := meetsSvc.CreateMeetEvent(admin, org, meet.ID, meets.CreateMeetEventInput{SessionID: session.ID, EventNumber: "1", Stroke: "freestyle", Distance: 50, Course: "SCY", Gender: "F"})
	if err != nil {
		t.Fatal(err)
	}
	_, commitments, _, err := meetsSvc.PublishCommitmentRequest(admin, org, meet.ID)
	if err != nil || len(commitments) != 2 {
		t.Fatalf("commitments: %v %v", commitments, err)
	}
	for _, c := range commitments {
		if _, _, err := meetsSvc.RespondToCommitment(admin, org, meet.ID, c.ID, meets.CommitmentRespondInput{Response: "committed_yes"}); err != nil {
			t.Fatal(err)
		}
		if _, err := meetsSvc.SelectCommitmentEvent(admin, org, meet.ID, c.ID, meets.SelectEventInput{MeetEventID: ev.ID}); err != nil {
			t.Fatal(err)
		}
	}
	meetsSvc.SeedEntry(org, sw.ID, meet.ID, "free", "SCY", 50, nil)
	meetsSvc.SeedEntry(org, foreignSwimmer.ID, meet.ID, "free", "SCY", 50, nil)

	// Volunteer: signup, credit, and a billed buyout per household.
	seasonsSvc := seasons.NewService()
	season, err := seasonsSvc.Create(org, "2026-27", "2026-09-01", "2027-08-31", "active", "", "cov1y")
	if err != nil {
		t.Fatal(err)
	}
	_, volSvc := wireVolunteer(volunteerDeps{members: f.membersSvc, meets: meetsSvc, seasons: seasonsSvc, billing: f.billingSvc, postmark: postmark.NewStubClient()})
	required, rate := 4.0, 2500
	if _, err := volSvc.UpdateSettings(ctx, admin, org, volunteer.SettingsUpdate{DefaultRequiredHours: &required, DefaultBuyoutCentsPerHour: &rate}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(96 * time.Hour)
	job, err := volSvc.CreateJob(ctx, admin, org, volunteer.JobInput{EventName: "Meet", SeasonID: season.ID, Role: "timer", Title: "Timer", StartsAt: start, EndsAt: start.Add(2 * time.Hour), Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{hh.ID, other.ID} {
		if _, err := volSvc.SignUp(ctx, admin, org, job.ID, volunteer.SignupInput{HouseholdID: h, VolunteerName: "Vol " + h[:4]}); err != nil {
			t.Fatal(err)
		}
		if _, err := volSvc.AddCredit(ctx, admin, org, volunteer.CreditInput{SeasonID: season.ID, HouseholdID: h, Hours: 1, Reason: "setup"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := volSvc.BillBuyout(ctx, admin, org, volunteer.BuyoutInput{SeasonID: season.ID, HouseholdID: h, Issue: true}); err != nil {
			t.Fatal(err)
		}
	}

	// Wellness, club events, documents, communications.
	wellnessSvc := wellness.NewService()
	if _, _, err := wellnessSvc.Upsert(org, wellness.Input{SwimmerID: sw.ID, HouseholdID: hh.ID, SubmittedBy: "parent-1", SubmittedRole: "parent",
		Metrics: wellness.Metrics{SleepHours: 8, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 4}}); err != nil {
		t.Fatal(err)
	}
	eventsSvc := events.NewService()
	club, err := eventsSvc.Create(org, "admin", events.EventInput{Title: "Banquet", Category: "banquet", StartsAt: time.Now().Add(72 * time.Hour), RSVPEnabled: true, RSVPMode: "household", Audience: "all"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{hh.ID, other.ID} {
		if _, err := eventsSvc.Respond(org, club.ID, "p", events.RSVPInput{HouseholdID: h, Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
	}
	docsSvc := documents.NewService()
	dt, err := docsSvc.CreateDocumentType(org, "Waiver", "waiver", "", "household", true, true, nil, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{hh.ID, other.ID} {
		if _, err := docsSvc.CreateSubmission(org, dt.ID, h, "", "parent"); err != nil {
			t.Fatal(err)
		}
	}
	commsSvc := comms.NewService()
	if _, err := commsSvc.SendMessage(comms.CreateMessageInput{OrgID: org, HouseholdID: hh.ID, SwimmerID: sw.ID, Body: "rich-body", SenderUserID: "coach", SenderRole: "coach"}); err != nil {
		t.Fatal(err)
	}

	f.sources = buildHouseholdExportSources(householdExportDeps{
		meets: meetsSvc, volunteer: volSvc, wellness: wellnessSvc, events: eventsSvc, documents: docsSvc, comms: commsSvc,
	})
	return f
}

func cov1yPick(i int, a, b string) string {
	if i == 0 {
		return a
	}
	return b
}

func TestCov1yHouseholdExportCarriesOnlyTheHouseholdsOwnRecords(t *testing.T) {
	f := cov1yBuildRichExport(t)
	data, err := collectHouseholdExportData(f.org, f.hh, f.membersSvc, f.groupsSvc, f.billingSvc, f.sources...)
	if err != nil {
		t.Fatal(err)
	}

	counts := map[string]int{}
	for _, key := range requiredHouseholdExportKeys {
		v, ok := data[key]
		if !ok {
			t.Fatalf("export is missing %q", key)
		}
		raw, _ := json.Marshal(v)
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			counts[key] = len(arr)
		}
	}
	for key, want := range map[string]int{
		"contacts": 1, "swimmers": 1, "group_memberships": 1, "registrations": 1, "waiver_acceptances": 1,
		"invoices": 2, "invoice_items": 2, "payments": 1, "refunds": 1, // dues invoice + the volunteer buyout invoice
		"meet_commitments": 1, "meet_event_selections": 1, "meet_entries": 1,
		"volunteer_signups": 1, "volunteer_credits": 1, "volunteer_buyouts": 1,
		"wellness_checkins": 1, "club_event_rsvps": 1, "document_submissions": 1, "messages": 1,
	} {
		if counts[key] != want {
			t.Errorf("%s: got %d records, want %d", key, counts[key], want)
		}
	}
	if hh, ok := data["household"].(*members.Household); !ok || hh.ID != f.hh {
		t.Fatalf("household = %#v", data["household"])
	}
	raw, _ := json.Marshal(data)
	for _, leaked := range []string{f.other, "Foreign Family", "Fay", "INV-COV1Y-B"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("export leaked another household's data (%q)", leaked)
		}
	}
	buyouts := data["volunteer_buyouts"].([]*volunteer.Buyout)
	if len(buyouts) != 1 || buyouts[0].HouseholdID != f.hh || buyouts[0].HoursShort != 3 {
		t.Fatalf("buyouts = %+v", buyouts)
	}
}

func TestCov1yHouseholdExportOfAnEmptyHouseholdEmitsEmptyListsNotNulls(t *testing.T) {
	const org = "org-cov1y-empty"
	admin := cov1xAdmin(org)
	membersSvc := members.NewService()
	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Empty Family"})
	if err != nil {
		t.Fatal(err)
	}
	sources := buildHouseholdExportSources(householdExportDeps{
		meets: meets.NewService(), volunteer: volunteer.NewService(), wellness: wellness.NewService(),
		events: events.NewService(), documents: documents.NewService(), comms: comms.NewService(),
	})
	data, err := collectHouseholdExportData(org, hh.ID, membersSvc, groups.NewService(), billing.NewService(), sources...)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range requiredHouseholdExportKeys {
		if key == "household" || key == "communication_preferences" {
			continue
		}
		if got := strings.TrimSpace(string(generic[key])); got != "[]" {
			t.Errorf("%s = %s, want an empty list", key, got)
		}
	}
}

func TestCov1yCollectHouseholdExportPreconditionsAndSourceContract(t *testing.T) {
	const org = "org-cov1y-contract"
	admin := cov1xAdmin(org)

	if _, err := collectHouseholdExportData(org, "hh", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "members service is not configured") {
		t.Fatalf("nil members: %v", err)
	}
	membersSvc := members.NewService()
	if _, err := collectHouseholdExportData(org, "missing", membersSvc, nil, nil); err == nil || !strings.Contains(err.Error(), "household not found") {
		t.Fatalf("unknown household: %v", err)
	}
	hh, err := membersSvc.CreateHousehold(admin, members.CreateHouseholdInput{OrgID: org, HouseholdName: "Contract Family"})
	if err != nil {
		t.Fatal(err)
	}
	liar := householdExportSource{name: "liar", keys: []string{"promised_key"}, collect: func(string, string, map[string]bool) (map[string]any, error) {
		return map[string]any{"other_key": 1}, nil
	}}
	if _, err := collectHouseholdExportData(org, hh.ID, membersSvc, nil, nil, liar); err == nil || !strings.Contains(err.Error(), `declared key "promised_key" missing`) {
		t.Fatalf("a source that drops a declared key must fail the export, got %v", err)
	}
	honest := householdExportSource{name: "honest", keys: []string{"k"}, collect: func(string, string, map[string]bool) (map[string]any, error) {
		return map[string]any{"k": []int{1}}, nil
	}}
	data, err := collectHouseholdExportData(org, hh.ID, membersSvc, nil, nil, honest)
	if err != nil || data["k"] == nil {
		t.Fatalf("honest source: %v %v", data, err)
	}
	if _, ok := data["registrations"]; ok {
		t.Error("registrations must be omitted when no groups service is wired")
	}
}

func TestCov1yCollectHouseholdExportRefreshFailures(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	closed := cov1xClosedCopy(t, pool)

	if _, err := collectHouseholdExportData(uuid.NewString(), "hh", members.NewServiceWithPool(closed), nil, nil); err == nil || !strings.Contains(err.Error(), "refresh members") {
		t.Fatalf("members refresh failure: %v", err)
	}
	if _, err := collectHouseholdExportData(uuid.NewString(), "hh", members.NewServiceWithPool(pool), nil, billing.NewServiceWithPool(closed)); err == nil || !strings.Contains(err.Error(), "refresh billing") {
		t.Fatalf("billing refresh failure: %v", err)
	}
}

func cov1ySource(t *testing.T, sources []householdExportSource, name string) householdExportSource {
	t.Helper()
	for _, s := range sources {
		if s.name == name {
			return s
		}
	}
	t.Fatalf("no export source named %q", name)
	return householdExportSource{}
}

// cov1yDropTable breaks a table inside the pool's private schema: it is replaced by
// a column-less table, so every statement that touches the real columns fails. (A
// plain DROP would fall through the search_path to the shared public table.)
func cov1yDropTable(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DO $$ BEGIN
		EXECUTE format('DROP TABLE %I.%I CASCADE', current_schema(), '`+table+`');
		EXECUTE format('CREATE TABLE %I.%I ()', current_schema(), '`+table+`');
	END $$`); err != nil {
		t.Fatalf("break %s: %v", table, err)
	}
}

func TestCov1yExportSourcesFailLoudlyWhenADomainDatabaseIsUnavailable(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	closed := cov1xClosedCopy(t, pool)
	org, hh := uuid.NewString(), uuid.NewString()
	sources := buildHouseholdExportSources(householdExportDeps{
		meets: meets.NewServiceWithPool(closed), volunteer: volunteer.NewServiceWithPool(closed), wellness: wellness.NewServiceWithPool(closed),
		events: events.NewServiceWithPool(closed), documents: documents.NewServiceWithPool(closed), comms: comms.NewServiceWithPool(closed),
	})
	if len(sources) != 6 {
		t.Fatalf("expected one source per wired domain, got %d", len(sources))
	}
	for _, s := range sources {
		data, err := s.collect(org, hh, map[string]bool{uuid.NewString(): true})
		if err == nil {
			t.Errorf("source %s returned %v instead of failing on a closed database", s.name, data)
		}
	}
}

func TestCov1yVolunteerExportSourceReportsEachFailingQuery(t *testing.T) {
	for _, tc := range []struct {
		name string
		drop []string
		want string
	}{
		{"buyouts", []string{"volunteer_buyouts"}, "list buyouts"},
		{"credits", []string{"volunteer_credits"}, "list credits"},
		{"signups", []string{"volunteer_signups"}, "list signups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testutil.IsolatedSchemaPool(t)
			src := cov1ySource(t, buildHouseholdExportSources(householdExportDeps{volunteer: volunteer.NewServiceWithPool(pool)}), "volunteer")
			org, hh := uuid.NewString(), uuid.NewString()
			if _, err := src.collect(org, hh, nil); err != nil {
				t.Fatalf("healthy database: %v", err)
			}
			for _, table := range tc.drop {
				cov1yDropTable(t, pool, table)
			}
			if data, err := src.collect(org, hh, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected a %q failure, got %v %v", tc.want, data, err)
			}
		})
	}
}

func TestCov1yEventsExportSourceReportsRSVPFailure(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	org := uuid.NewString()
	svc := events.NewServiceWithPool(pool)
	if _, err := svc.Create(org, "admin", events.EventInput{Title: "Gala", Category: "social", StartsAt: time.Now().Add(48 * time.Hour), RSVPEnabled: true, RSVPMode: "household", Audience: "all"}); err != nil {
		t.Fatal(err)
	}
	src := cov1ySource(t, buildHouseholdExportSources(householdExportDeps{events: svc}), "club events")
	if data, err := src.collect(org, uuid.NewString(), nil); err != nil || data["club_event_rsvps"] == nil {
		t.Fatalf("healthy database: %v %v", data, err)
	}
	cov1yDropTable(t, pool, "club_event_rsvps")
	if _, err := src.collect(org, uuid.NewString(), nil); err == nil || !strings.Contains(err.Error(), "list rsvps") {
		t.Fatalf("expected the RSVP query failure, got %v", err)
	}
}

func TestCov1yMeetsExportSourceReportsEachFailingStage(t *testing.T) {
	// The meets source re-syncs from Postgres before each read. The sync hook runs
	// after every load, so dropping the entries table in hook N makes sync N+1 fail:
	// 2 = commitments of a meet, 3 = a commitment's selections, 4 = the meet's entries.
	for _, tc := range []struct {
		name   string
		dropIn int
	}{{"commitments", 1}, {"selections", 2}, {"entries", 3}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testutil.IsolatedSchemaPool(t)
			org, hh, swimmer := uuid.NewString(), uuid.NewString(), uuid.NewString()
			ctx := context.Background()
			for _, q := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO households (id, org_id, household_name) VALUES ($1, $2, 'Meet Family')`, []any{hh, org}},
				{`INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender) VALUES ($1, $2, $3, 'M', 'F', '2012-01-01', 'female')`, []any{swimmer, org, hh}},
			} {
				if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
					t.Fatal(err)
				}
			}
			admin := &appmiddleware.Principal{UserID: uuid.NewString(), OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true}
			if _, err := pool.Exec(ctx, `INSERT INTO users (id, org_id, email, oidc_subject) VALUES ($1, $2, $3, $4)`, admin.UserID, org, admin.UserID+"@example.test", admin.UserID); err != nil {
				t.Fatal(err)
			}
			svc := meets.NewServiceWithPool(pool)
			meet, _, err := svc.CreateMeet(admin, org, meets.CreateMeetInput{Name: "Staged Meet", StartDate: "2027-06-01"})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := svc.TransitionMeetStatus(admin, org, meet.ID, "published"); err != nil {
				t.Fatal(err)
			}
			if _, commitments, _, err := svc.PublishCommitmentRequest(admin, org, meet.ID); err != nil || len(commitments) != 1 {
				t.Fatalf("commitments: %v %v", commitments, err)
			}
			src := cov1ySource(t, buildHouseholdExportSources(householdExportDeps{meets: svc}), "meets")
			data, err := src.collect(org, hh, map[string]bool{swimmer: true})
			if err != nil {
				t.Fatalf("healthy database: %v", err)
			}
			if got := data["meet_commitments"].([]meets.Commitment); len(got) != 1 || got[0].SwimmerID != swimmer {
				t.Fatalf("commitments = %+v", got)
			}

			calls := 0
			svc.SetSyncHookForTest(func() {
				calls++
				if calls == tc.dropIn {
					cov1yDropTable(t, pool, "entries")
				}
			})
			if data, err := src.collect(org, hh, map[string]bool{swimmer: true}); err == nil || !strings.Contains(err.Error(), `"id" does not exist`) {
				t.Fatalf("expected the %s stage to fail, got %v %v", tc.name, data, err)
			}
		})
	}
}
