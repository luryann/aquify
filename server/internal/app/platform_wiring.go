package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
)

// registerPlatformOrg makes sure the dedicated platform org exists and is active
// so operators can sign in. Idempotent; a failure is logged and leaves the
// operator plane unusable rather than the API down.
func registerPlatformOrg(logger *slog.Logger, registry *orgs.Service, platformOrgID string) {
	ctx := context.Background()
	if _, err := registry.Get(ctx, platformOrgID); err == nil {
		return
	} else if !errors.Is(err, orgs.ErrNotFound) {
		logger.Error("platform org lookup failed", "error", err)
		return
	}
	if _, err := registry.Create(ctx, orgs.CreateInput{ID: platformOrgID, Slug: "platform", Name: "Aquify Platform"}); err != nil {
		logger.Error("platform org registration failed", "error", err)
		return
	}
	if _, err := registry.SetStatus(ctx, platformOrgID, orgs.StatusActive); err != nil {
		logger.Error("platform org activation failed", "error", err)
	}
}

// clubResolver resolves a public club slug to its org for self-registration. The
// tenant registry is consulted first, then the legacy org_settings slug; either
// way the org must be active, so a suspended club accepts no new sign-ups.
type clubResolver struct {
	registry *orgs.Service
	settings *orgsettings.Service
}

func (c clubResolver) FindBySlug(slug string) (*orgsettings.OrgSettings, error) {
	o, err := c.registry.BySlugOrLegacy(context.Background(), slug, func(slug string) (string, error) {
		st, err := c.settings.FindBySlug(slug)
		if err != nil || st == nil {
			return "", err
		}
		return st.OrgID, nil
	})
	switch {
	case errors.Is(err, orgs.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case o.Status != orgs.StatusActive:
		return nil, nil
	}
	return &orgsettings.OrgSettings{OrgID: o.ID, PublicSlug: o.Slug}, nil
}
