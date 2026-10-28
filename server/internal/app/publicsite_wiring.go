package app

import (
	"context"
	"slices"
	"strings"
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
func buildPublicSiteHandlers(cfg Config, settings *orgsettings.Service, commsSvc *comms.Service, meetsSvc *meets.Service, eventsSvc *events.Service, perfSvc *performance.Service, kc keycloak.Client) *handlers.PublicSiteHandlers {
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
		src.Staff = func(ctx context.Context, orgID string) []handlers.PublicSiteStaff {
			users, err := kc.ListUsers(ctx, orgID, "", 500, 0)
			if err != nil {
				return nil
			}
			out := []handlers.PublicSiteStaff{}
			for _, u := range users {
				if !u.Enabled {
					continue
				}
				roles, err := kc.GetUserRealmRoles(ctx, u.ID)
				if err != nil {
					continue
				}
				role := ""
				for _, r := range roles {
					switch strings.ToLower(r.Name) {
					case "admin":
						if role == "" {
							role = "Administrator"
						}
					case "coach":
						role = "Coach"
					}
				}
				name := strings.TrimSpace(u.FirstName + " " + u.LastName)
				if role == "" || name == "" {
					continue
				}
				out = append(out, handlers.PublicSiteStaff{Name: name, Role: role})
			}
			slices.SortFunc(out, func(a, b handlers.PublicSiteStaff) int {
				return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})
			return out
		}
	}
	limiter := appmiddleware.NewRateLimiter(60, time.Minute, cfg.Security.RateLimit.TrustedProxyCIDRs)
	return handlers.NewPublicSiteHandlers(src, limiter)
}
