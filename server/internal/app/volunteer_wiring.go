package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/seasons"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
	"github.com/dareaquatics/aquify/server/internal/postmark"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/jackc/pgx/v5/pgxpool"
)

// volunteerDeps groups the services the volunteer domain borrows from.
type volunteerDeps struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	audit      *audit.Service
	members    *members.Service
	meets      *meets.Service
	seasons    *seasons.Service
	billing    *billing.Service
	postmark   postmark.Client
	workerBack workers.Backend
}

// wireVolunteer builds the volunteer service, connects it to the members,
// meets, seasons, billing, and email domains, registers the shift-reminder
// job handler on the worker backend (API and dedicated worker processes both
// pass through here), and returns the HTTP handlers.
func wireVolunteer(d volunteerDeps) *handlers.VolunteerHandlers {
	var svc *volunteer.Service
	if d.pool != nil {
		svc = volunteer.NewServiceWithPool(d.pool)
	} else {
		svc = volunteer.NewService()
	}
	svc.SetLogger(d.logger)

	// Trusted server-side lookups run as a system admin principal.
	system := func(orgID string) *appmiddleware.Principal {
		return &appmiddleware.Principal{UserID: "system:volunteer", OrgID: orgID, Roles: []string{"admin"}}
	}

	if d.seasons != nil {
		svc.SetSeasonLookup(func(orgID string) ([]volunteer.SeasonRef, error) {
			list, err := d.seasons.List(orgID)
			if err != nil {
				return nil, err
			}
			out := make([]volunteer.SeasonRef, 0, len(list))
			for _, se := range list {
				out = append(out, volunteer.SeasonRef{ID: se.ID, Name: se.Name, Status: se.Status, StartDate: se.StartDate, EndDate: se.EndDate})
			}
			return out, nil
		})
	}
	if d.members != nil {
		svc.SetHouseholdDirectory(func(orgID string) ([]volunteer.HouseholdRef, error) {
			list, err := d.members.ListHouseholds(system(orgID), orgID)
			if err != nil {
				return nil, err
			}
			out := make([]volunteer.HouseholdRef, 0, len(list))
			for _, h := range list {
				out = append(out, volunteer.HouseholdRef{ID: h.ID, Name: h.HouseholdName, Email: h.PrimaryEmail, Status: h.Status})
			}
			return out, nil
		})
		svc.SetSwimmerLookup(func(orgID, swimmerID string) (volunteer.SwimmerRef, bool) {
			hh, ok := d.members.SwimmerHouseholdID(orgID, swimmerID)
			if !ok {
				return volunteer.SwimmerRef{}, false
			}
			info, ok := d.members.SwimmerEligibility(orgID, swimmerID)
			if !ok {
				return volunteer.SwimmerRef{}, false
			}
			return volunteer.SwimmerRef{HouseholdID: hh, DateOfBirth: info.DateOfBirth, Name: strings.TrimSpace(info.FirstName + " " + info.LastName)}, true
		})
	}
	if d.meets != nil {
		svc.SetMeetLookup(func(orgID, meetID string) (string, bool) {
			m, err := d.meets.GetMeet(system(orgID), orgID, meetID)
			if err != nil {
				return "", false
			}
			return m.Name, true
		})
	}
	if d.billing != nil {
		svc.SetBiller(volunteerBiller{billing: d.billing})
	}
	if d.postmark != nil {
		svc.SetReminderSender(func(ctx context.Context, e volunteer.ReminderEmail) error {
			_, err := d.postmark.SendEmail(ctx, postmark.SendEmailRequest{
				To:       e.To,
				Subject:  e.Subject,
				TextBody: e.TextBody,
				Tag:      "volunteer-reminder",
			})
			return err
		})
	}
	if !isNilWorkerBackend(d.workerBack) {
		d.workerBack.RegisterHandler(volunteer.TaskTypeShiftReminder, svc.ReminderHandler())
		svc.SetReminderQueue(d.workerBack)
	}
	return handlers.NewVolunteerHandlers(svc, d.audit)
}

// volunteerBiller bills buyouts through the existing billing service so all
// invoice rules (numbering, totals, ledger events, RBAC) stay in one place.
type volunteerBiller struct {
	billing *billing.Service
}

func (b volunteerBiller) CreateBuyoutInvoice(_ context.Context, req volunteer.BuyoutInvoiceRequest) (volunteer.BuyoutInvoiceResult, error) {
	actor := ""
	if req.Principal != nil {
		actor = req.Principal.UserID
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return volunteer.BuyoutInvoiceResult{}, fmt.Errorf("volunteer buyout: invoice number entropy: %w", err)
	}
	number := "VOL-" + strings.ToUpper(hex.EncodeToString(suffix))
	inv, err := b.billing.CreateInvoice(req.OrgID, actor, req.Principal, billing.CreateInvoiceInput{
		HouseholdID:   req.HouseholdID,
		InvoiceNumber: number,
		DueDate:       req.DueDate,
		Notes:         "Volunteer service-hour buyout",
	})
	if err != nil {
		return volunteer.BuyoutInvoiceResult{}, mapBillingErr(err)
	}
	item, err := b.billing.AddInvoiceItem(req.OrgID, actor, req.Principal, inv.ID, billing.AddInvoiceItemInput{
		Description:     req.Description,
		Quantity:        1,
		UnitAmountCents: req.AmountCents,
		Category:        "manual",
	})
	if err != nil {
		return volunteer.BuyoutInvoiceResult{}, fmt.Errorf("volunteer buyout: add item to draft invoice %s: %w", inv.ID, mapBillingErr(err))
	}
	status := inv.Status
	if req.Issue {
		issued, _, err := b.billing.IssueInvoice(req.OrgID, actor, req.Principal, inv.ID)
		if err != nil {
			return volunteer.BuyoutInvoiceResult{}, fmt.Errorf("volunteer buyout: issue invoice %s: %w", inv.ID, mapBillingErr(err))
		}
		status = issued.Status
	}
	return volunteer.BuyoutInvoiceResult{InvoiceID: inv.ID, InvoiceItemID: item.ID, InvoiceNumber: inv.InvoiceNumber, InvoiceStatus: status}, nil
}

func mapBillingErr(err error) error {
	if errors.Is(err, billing.ErrForbidden) {
		return fmt.Errorf("%w: %v", volunteer.ErrForbidden, err)
	}
	return err
}
