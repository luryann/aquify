package app

// Adapters wiring the engagement domains (club events, wellness, attendance
// summaries) to the billing, meets, groups and training services without
// those domains importing each other.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/attendance"
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/roster"
	"github.com/dareaquatics/aquify/server/internal/app/training"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/jackc/pgx/v5/pgxpool"
)

// engagementSystemPrincipal is the fixed, explicitly-marked identity for
// trusted server-side reads/writes made on behalf of an already-authorized
// request. It never carries a caller-supplied user ID: the real actor is passed
// separately as the actorID argument of each billing call, so audit trails
// attribute work to the actual user and admin rights never attach to them.
const engagementSystemUserID = "system:club-events"

func engagementSystemPrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: engagementSystemUserID, OrgID: orgID, Roles: []string{"admin"}}
}

// ─── RSVP fee billing ─────────────────────────────────────────────────────────

// rsvpFeeBiller bills RSVP fees through the billing service: one issued
// invoice per paying RSVP so it appears in the family's billing portal and
// can be paid through the existing Stripe checkout.
type rsvpFeeBiller struct {
	billing *billing.Service
}

// ChargeRSVP is idempotent per (event, household, swimmer, quantity): the
// charge's ExternalRef is the idempotency key and the invoice number is derived
// from it plus the attempt index, so the DB's (org_id, invoice_number) unique
// constraint arbitrates concurrent retries. A retry after a timeout finds the
// existing invoice and resumes it (a half-built draft is completed) instead of
// creating a second one. Compensating voids are never silent: a failed void is
// logged and joined to the returned error.
func (b rsvpFeeBiller) ChargeRSVP(orgID string, c events.Charge) (string, error) {
	actor := engagementSystemPrincipal(orgID)
	ref := fmt.Sprintf("club_event:%s:%s:%s:q%d", c.EventID, c.HouseholdID, c.SwimmerID, c.Quantity)

	for attempt := 0; attempt < 3; attempt++ {
		existing, err := b.billing.InvoicesByExternalRef(orgID, actor, ref)
		if err != nil {
			return "", fmt.Errorf("lookup rsvp invoice: %w", err)
		}
		for _, inv := range existing {
			switch inv.Status {
			case "voided", "written_off":
				continue
			case "draft":
				return b.finishRSVPInvoice(orgID, c, actor, inv.ID)
			default:
				return inv.ID, nil
			}
		}
		sum := sha256.Sum256([]byte(ref))
		eventPrefix := c.EventID
		if len(eventPrefix) > 8 {
			eventPrefix = eventPrefix[:8]
		}
		inv, err := b.billing.CreateInvoice(orgID, c.ActorUserID, actor, billing.CreateInvoiceInput{
			HouseholdID: c.HouseholdID,
			InvoiceNumber: fmt.Sprintf("EVT-%s-%s-%d", strings.ToUpper(eventPrefix),
				strings.ToUpper(hex.EncodeToString(sum[:4])), len(existing)+1),
			DueDate:     c.DueDate,
			Notes:       c.Description,
			ExternalRef: ref,
		})
		if errors.Is(err, billing.ErrDuplicateInvoiceNumber) {
			continue // a concurrent retry created it first; re-run the lookup
		}
		if err != nil {
			return "", fmt.Errorf("create rsvp invoice: %w", err)
		}
		return b.finishRSVPInvoice(orgID, c, actor, inv.ID)
	}
	return "", errors.New("create rsvp invoice: concurrent creation did not settle")
}

// finishRSVPInvoice adds the fee line (if missing) and issues a draft invoice.
func (b rsvpFeeBiller) finishRSVPInvoice(orgID string, c events.Charge, actor *appmiddleware.Principal, invoiceID string) (string, error) {
	fail := func(step string, cause error) (string, error) {
		if _, _, verr := b.billing.VoidInvoice(orgID, c.ActorUserID, actor, invoiceID); verr != nil {
			slog.Error("club events: voiding rsvp invoice after failure also failed; draft left for retry",
				"invoice_id", invoiceID, "step", step, "cause", cause, "void_error", verr)
			return "", errors.Join(fmt.Errorf("%s: %w", step, cause), fmt.Errorf("void rsvp invoice %s: %w", invoiceID, verr))
		}
		return "", fmt.Errorf("%s: %w", step, cause)
	}
	items, err := b.billing.ListInvoiceItems(orgID, actor, invoiceID)
	if err != nil {
		return fail("list rsvp invoice items", err)
	}
	if len(items) == 0 {
		if _, err := b.billing.AddInvoiceItem(orgID, c.ActorUserID, actor, invoiceID, billing.AddInvoiceItemInput{
			Description:     c.Description,
			Quantity:        c.Quantity,
			UnitAmountCents: c.UnitAmountCents,
			Category:        "manual",
			SwimmerID:       c.SwimmerID,
		}); err != nil {
			return fail("add rsvp invoice item", err)
		}
	}
	if _, _, err := b.billing.IssueInvoice(orgID, c.ActorUserID, actor, invoiceID); err != nil {
		return fail("issue rsvp invoice", err)
	}
	return invoiceID, nil
}

func (b rsvpFeeBiller) ReleaseCharge(orgID, invoiceID, actorUserID string) (bool, error) {
	actor := engagementSystemPrincipal(orgID)
	inv, err := b.billing.GetInvoice(orgID, actor, invoiceID)
	if err != nil {
		return false, err
	}
	switch inv.Status {
	case "voided", "written_off":
		return true, nil
	case "draft", "issued":
		if inv.AmountPaidCents > 0 {
			return false, nil
		}
		if _, _, err := b.billing.VoidInvoice(orgID, actorUserID, actor, invoiceID); err != nil {
			return false, err
		}
		return true, nil
	default:
		// paid / partially_paid / refunded: keep the ledger authoritative.
		return false, nil
	}
}

// ─── Attendance source ────────────────────────────────────────────────────────

// trainingPracticeSource adapts the training service's exported read
// methods for attendance aggregation.
type trainingPracticeSource struct {
	training *training.Service
}

func (t trainingPracticeSource) Practices(orgID string) ([]attendance.Practice, error) {
	list, err := t.training.List(orgID)
	if err != nil {
		return nil, err
	}
	out := make([]attendance.Practice, 0, len(list))
	for _, p := range list {
		if p == nil {
			continue
		}
		out = append(out, attendance.Practice{ID: p.ID, Date: p.Date, GroupID: p.GroupID, Type: p.Type})
	}
	return out, nil
}

func (t trainingPracticeSource) Attendance(orgID, practiceID string) ([]attendance.Record, error) {
	list, err := t.training.ListAttendance(orgID, practiceID)
	if err != nil {
		return nil, err
	}
	out := make([]attendance.Record, 0, len(list))
	for _, rec := range list {
		if rec == nil {
			continue
		}
		out = append(out, attendance.Record{SwimmerID: rec.SwimmerID, Status: rec.Status})
	}
	return out, nil
}

// ─── Calendar feed (meets + practices) ───────────────────────────────────────

var feedMeetStatuses = map[string]bool{
	"published": true, "commitment_open": true, "commitment_closed": true,
	"entry_finalized": true, "completed": true,
}

func buildCalendarFeedSource(meetsService *meets.Service, trainingService *training.Service, groupsService *groups.Service, dir roster.Directory) handlers.CalendarFeedSource {
	return func(orgID string, household []roster.Swimmer, from, to time.Time) ([]events.CalendarItem, error) {
		actor := engagementSystemPrincipal(orgID)
		fromDate, toDate := from.Format("2006-01-02"), to.Format("2006-01-02")
		items := []events.CalendarItem{}

		if meetsService != nil {
			meetItems, err := meetFeedItems(meetsService, actor, orgID, fromDate, toDate)
			if err != nil {
				return nil, err
			}
			items = append(items, meetItems...)
		}
		if trainingService != nil {
			practiceItems, err := practiceFeedItems(trainingService, groupsService, dir, actor, orgID, household, fromDate, toDate)
			if err != nil {
				return nil, err
			}
			items = append(items, practiceItems...)
		}
		return items, nil
	}
}

// meetFeedItems returns all-day entries for published-onward meets overlapping
// [fromDate, toDate], plus a deadline entry while commitments are open.
func meetFeedItems(meetsService *meets.Service, actor *appmiddleware.Principal, orgID, fromDate, toDate string) ([]events.CalendarItem, error) {
	list, err := meetsService.ListMeets(actor, orgID)
	if err != nil {
		return nil, err
	}
	var items []events.CalendarItem
	for _, m := range list {
		if !feedMeetStatuses[m.Status] || m.StartDate == "" {
			continue
		}
		end := m.EndDate
		if end == "" {
			end = m.StartDate
		}
		if end < fromDate || m.StartDate > toDate {
			continue
		}
		desc := ""
		if m.HostOrg != "" {
			desc = "Hosted by " + m.HostOrg
		}
		items = append(items, events.CalendarItem{
			UID: "meet-" + m.ID + "@dare", Title: m.Name, Description: desc, Location: m.Location,
			Categories: "meet", AllDay: true, StartDate: m.StartDate, EndDate: end, Updated: m.UpdatedAt,
		})
		if m.Status == "commitment_open" && m.CommitmentDeadline != "" {
			day := m.CommitmentDeadline
			if len(day) > 10 {
				day = day[:10]
			}
			items = append(items, events.CalendarItem{
				UID: "meet-deadline-" + m.ID + "@dare", Title: "Commitment deadline — " + m.Name,
				Description: "Respond yes/no for each swimmer in the parent portal.",
				Categories:  "deadline", AllDay: true, StartDate: day, EndDate: day, Updated: m.UpdatedAt,
			})
		}
	}
	return items, nil
}

// practiceFeedItems returns all-day practice entries within [fromDate, toDate]
// that apply to the household: team-wide practices and those for a group any
// of its swimmers belongs to. Rest days are omitted.
func practiceFeedItems(trainingService *training.Service, groupsService *groups.Service, dir roster.Directory, actor *appmiddleware.Principal, orgID string, household []roster.Swimmer, fromDate, toDate string) ([]events.CalendarItem, error) {
	practices, err := trainingService.List(orgID)
	if err != nil {
		return nil, err
	}
	groupNames := map[string]string{}
	if groupsService != nil {
		if gl, err := groupsService.ListGroups(actor, orgID, false); err == nil {
			for _, g := range gl {
				groupNames[roster.CanonicalGroupCode(g.Code)] = g.Name
			}
		}
	}
	codeCache := map[string]string{}
	var items []events.CalendarItem
	for _, p := range practices {
		if p == nil || p.Date < fromDate || p.Date > toDate || strings.EqualFold(p.Type, "rest") {
			continue
		}
		label := "All groups"
		if ref := strings.TrimSpace(p.GroupID); ref != "" {
			code, ok := codeCache[ref]
			if !ok {
				code = dir.ResolveGroupCode(orgID, ref)
				codeCache[ref] = code
			}
			if !slices.ContainsFunc(household, func(sw roster.Swimmer) bool { return sw.InGroup(code) }) {
				continue
			}
			label = code
			if name := groupNames[code]; name != "" {
				label = name
			}
		}
		title := "Practice — " + label
		if strings.EqualFold(p.Type, "dryland") {
			title = "Dryland — " + label
		}
		items = append(items, events.CalendarItem{
			UID: "practice-" + p.ID + "@dare", Title: title, Description: p.Description,
			Categories: "practice", AllDay: true, StartDate: p.Date, EndDate: p.Date, Updated: p.UpdatedAt,
		})
	}
	return items, nil
}

// engagementHandlers bundles the club events, wellness and attendance
// summary handlers so server.go wiring stays a few lines.
type engagementHandlers struct {
	clubEvents        *handlers.ClubEventsHandlers
	wellness          *handlers.WellnessHandlers
	attendanceSummary *handlers.AttendanceSummaryHandlers
	eventsService     *events.Service
	wellnessService   *wellness.Service
}

func buildEngagementHandlers(dbPool *pgxpool.Pool, logger *slog.Logger, membershipService *members.Service, groupsService *groups.Service, meetsService *meets.Service, trainingService *training.Service, billingService *billing.Service, auditService *audit.Service) engagementHandlers {
	dir := roster.NewMembersDirectory(membershipService, groupsService)
	clubEventsService := events.NewServiceWithPool(dbPool)
	clubEventsService.SetLogger(logger)
	if billingService != nil {
		clubEventsService.SetBiller(rsvpFeeBiller{billing: billingService})
	}
	wellnessService := wellness.NewServiceWithPool(dbPool)
	var source attendance.PracticeSource
	if trainingService != nil {
		source = trainingPracticeSource{training: trainingService}
	}
	return engagementHandlers{
		eventsService:   clubEventsService,
		wellnessService: wellnessService,
		clubEvents: handlers.NewClubEventsHandlers(clubEventsService, dir, auditService,
			buildCalendarFeedSource(meetsService, trainingService, groupsService, dir)),
		wellness: handlers.NewWellnessHandlers(wellnessService, dir),
		attendanceSummary: handlers.NewAttendanceSummaryHandlers(source, dir, func() string {
			return time.Now().UTC().Format("2006-01-02")
		}),
	}
}
