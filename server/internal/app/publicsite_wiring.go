package app

import (
	"context"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/handlers"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
)

// buildPublicSiteHandlers wires the unauthenticated team-site feed to the
// services that own its data. Public visibility is decided by flags on the
// source records, never by the caller.
// publicStaffCacheTTL bounds Keycloak fan-out from anonymous public-site traffic.
const publicStaffCacheTTL = time.Minute

func buildPublicSiteHandlers(cfg Config, settings *orgsettings.Service, commsSvc *comms.Service, meetsSvc *meets.Service, eventsSvc *events.Service, perfSvc *performance.Service, kc keycloak.Client, clock func() time.Time) *handlers.PublicSiteHandlers {
	src := handlers.PublicSiteSources{
		Settings: settings.FindBySlug,
	}
	if commsSvc != nil {
		src.Announcements = commsSvc.ListPublicAnnouncements
	}
	if meetsSvc != nil {
		src.Meets = func(orgID string) []meets.Meet {
			list, err := meetsSvc.ListMeets(handlers.PublicRecordsPrincipal(orgID), orgID)
			if err != nil {
				return nil
			}
			return list
		}
	}
	if eventsSvc != nil {
		src.Events = func(orgID string, from time.Time) []*events.Event {
			list, err := eventsSvc.List(orgID, events.ListFilter{From: &from})
			if err != nil {
				return nil
			}
			return list
		}
	}
	if perfSvc != nil {
		src.Records = func(orgID string) []performance.TeamRecord {
			list, err := perfSvc.ListRecords(orgID, handlers.PublicRecordsPrincipal(orgID), performance.ListRecordsFilters{})
			if err != nil {
				return nil
			}
			return list
		}
	}
	if kc != nil {
		// Same staff rules as /users/staff, cached briefly because this surface is
		// unauthenticated; names only (no email fallback) are ever exposed.
		staffDir := handlers.NewCachedStaffLister(handlers.NewStaffDirectory(kc), publicStaffCacheTTL, clock)
		src.Staff = func(ctx context.Context, orgID string) []handlers.PublicSiteStaff {
			members, err := staffDir.ListStaff(ctx, orgID)
			if err != nil {
				return nil
			}
			out := []handlers.PublicSiteStaff{}
			for _, m := range members {
				if name := m.PersonalName(); name != "" && m.PublicRole() != "" {
					out = append(out, handlers.PublicSiteStaff{Name: name, Role: m.PublicRole()})
				}
			}
			return out
		}
	}
	limiter := appmiddleware.NewRateLimiter(60, time.Minute, cfg.Security.RateLimit.TrustedProxyCIDRs)
	return handlers.NewPublicSiteHandlers(src, limiter)
}
