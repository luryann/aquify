package app

// Adapters wiring the engagement domains (club events, wellness, attendance
// summaries) to the billing, meets, groups and training services without
// those domains importing each other.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
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

// engagementSystemPrincipal performs trusted server-side reads/writes on
// behalf of an already-authorized request (RSVP fees, feed rendering).
func engagementSystemPrincipal(orgID, actorID string) *appmiddleware.Principal {
	if strings.TrimSpace(actorID) == "" {
		actorID = "system:club-events"
	}
	return &appmiddleware.Principal{UserID: actorID, OrgID: orgID, Roles: []string{"admin"}}
}

// ─── RSVP fee billing ─────────────────────────────────────────────────────────

// rsvpFeeBiller bills RSVP fees through the billing service: one issued
// invoice per paying RSVP so it appears in the family's billing portal and
// can be paid through the existing Stripe checkout.
type rsvpFeeBiller struct {
	billing *billing.Service
}

func (b rsvpFeeBiller) ChargeRSVP(orgID string, c events.Charge) (string, error) {
	actor := engagementSystemPrincipal(orgID, "")
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	eventPrefix := c.EventID
	if len(eventPrefix) > 8 {
		eventPrefix = eventPrefix[:8]
	}
	inv, err := b.billing.CreateInvoice(orgID, c.ActorUserID, actor, billing.CreateInvoiceInput{
		HouseholdID:   c.HouseholdID,
		InvoiceNumber: fmt.Sprintf("EVT-%s-%s", strings.ToUpper(eventPrefix), strings.ToUpper(hex.EncodeToString(suffix))),
		DueDate:       c.DueDate,
		Notes:         c.Description,
		ExternalRef:   "club_event:" + c.EventID,
	})
	if err != nil {
		return "", fmt.Errorf("create rsvp invoice: %w", err)
	}
	if _, err := b.billing.AddInvoiceItem(orgID, c.ActorUserID, actor, inv.ID, billing.AddInvoiceItemInput{
		Description:     c.Description,
		Quantity:        c.Quantity,
		UnitAmountCents: c.UnitAmountCents,
		Category:        "manual",
		SwimmerID:       c.SwimmerID,
	}); err != nil {
		_, _, _ = b.billing.VoidInvoice(orgID, c.ActorUserID, actor, inv.ID)
		return "", fmt.Errorf("add rsvp invoice item: %w", err)
	}
	if _, _, err := b.billing.IssueInvoice(orgID, c.ActorUserID, actor, inv.ID); err != nil {
		_, _, _ = b.billing.VoidInvoice(orgID, c.ActorUserID, actor, inv.ID)
		return "", fmt.Errorf("issue rsvp invoice: %w", err)
	}
	return inv.ID, nil
}

func (b rsvpFeeBiller) ReleaseCharge(orgID, invoiceID, actorUserID string) (bool, error) {
	actor := engagementSystemPrincipal(orgID, "")
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
		actor := engagementSystemPrincipal(orgID, "")
		fromDate, toDate := from.Format("2006-01-02"), to.Format("2006-01-02")
		items := []events.CalendarItem{}

		if meetsService != nil {
			list, err := meetsService.ListMeets(actor, orgID)
			if err != nil {
				return nil, err
			}
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
		}

		if trainingService != nil {
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
					inGroup := false
					for _, sw := range household {
						if sw.InGroup(code) {
							inGroup = true
							break
						}
					}
					if !inGroup {
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
		}
		return items, nil
	}
}

// engagementHandlers bundles the club events, wellness and attendance
// summary handlers so server.go wiring stays a few lines.
type engagementHandlers struct {
	clubEvents        *handlers.ClubEventsHandlers
	wellness          *handlers.WellnessHandlers
	attendanceSummary *handlers.AttendanceSummaryHandlers
	eventsService     *events.Service
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
		eventsService: clubEventsService,
		clubEvents: handlers.NewClubEventsHandlers(clubEventsService, dir, auditService,
			buildCalendarFeedSource(meetsService, trainingService, groupsService, dir)),
		wellness: handlers.NewWellnessHandlers(wellnessService, dir),
		attendanceSummary: handlers.NewAttendanceSummaryHandlers(source, dir, func() string {
			return time.Now().UTC().Format("2006-01-02")
		}),
	}
}
