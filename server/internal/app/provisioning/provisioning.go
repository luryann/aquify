// Package provisioning creates a new tenant end to end: registry row, club
// settings, first administrator (Keycloak user + role + invite emails), then
// activation. Every step is idempotent so a failed run is finished by repeating
// the same request.
package provisioning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/luryann/aquify/server/internal/app/orgs"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/keycloak"
)

var (
	ErrAdminEmailInUse = errors.New("admin email already belongs to a user in another organization")
	ErrAlreadyActive   = errors.New("organization with this slug already exists")
)

// SettingsWriter writes org settings (implemented by orgsettings.Service).
type SettingsWriter interface {
	Update(orgID string, updates orgsettings.Updates, updatedBy string) (*orgsettings.OrgSettings, error)
	FindBySlug(slug string) (*orgsettings.OrgSettings, error)
}

// Input describes the tenant to create.
type Input struct {
	Name           string
	Slug           string
	Plan           string
	Region         string
	TeamCode       string
	AdminEmail     string
	AdminFirstName string
	AdminLastName  string
	// ActorUserID is the platform operator performing the action (audit/updated_by).
	ActorUserID string
}

// Result is the outcome of a provisioning run.
type Result struct {
	Org         *orgs.Org `json:"org"`
	AdminUserID string    `json:"admin_user_id"`
	Resumed     bool      `json:"resumed"`
}

// Provisioner wires the dependencies.
type Provisioner struct {
	Orgs     *orgs.Service
	Keycloak keycloak.Client
	Settings SettingsWriter
	Logger   *slog.Logger
}

func (p *Provisioner) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}

// Provision creates (or finishes creating) a tenant.
func (p *Provisioner) Provision(ctx context.Context, in Input) (*Result, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.AdminEmail = strings.ToLower(strings.TrimSpace(in.AdminEmail))
	in.AdminFirstName = strings.TrimSpace(in.AdminFirstName)
	in.AdminLastName = strings.TrimSpace(in.AdminLastName)
	if in.Name == "" || in.AdminEmail == "" {
		return nil, errors.New("name and admin_email are required")
	}
	if !strings.Contains(in.AdminEmail, "@") {
		return nil, errors.New("admin_email is not a valid email address")
	}
	if !orgs.ValidSlug(in.Slug) {
		return nil, orgs.ErrInvalidSlug
	}

	res := &Result{}
	// The slug also lives in org_settings.public_slug (the public site). A slug
	// another club already uses there must not become this club's registry slug.
	var settingsOwner string
	if p.Settings != nil {
		if st, err := p.Settings.FindBySlug(in.Slug); err != nil {
			return nil, err
		} else if st != nil {
			settingsOwner = st.OrgID
		}
	}
	org, err := p.Orgs.BySlug(ctx, in.Slug)
	switch {
	case err == nil && org.Status == orgs.StatusProvisioning:
		if settingsOwner != "" && settingsOwner != org.ID {
			return nil, orgs.ErrSlugTaken
		}
		res.Resumed = true
	case err == nil:
		return nil, ErrAlreadyActive
	case errors.Is(err, orgs.ErrNotFound):
		if settingsOwner != "" {
			return nil, orgs.ErrSlugTaken
		}
		org, err = p.Orgs.Create(ctx, orgs.CreateInput{Slug: in.Slug, Name: in.Name, Plan: in.Plan, Region: in.Region})
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	res.Org = org

	// Club settings (team identity + the public slug; the public site stays disabled).
	teamName, slug, teamCode := in.Name, in.Slug, in.TeamCode
	upd := orgsettings.Updates{TeamName: &teamName, PublicSlug: &slug}
	if strings.TrimSpace(teamCode) != "" {
		upd.TeamCode = &teamCode
	}
	if p.Settings != nil {
		if _, err := p.Settings.Update(org.ID, upd, in.ActorUserID); err != nil {
			return res, fmt.Errorf("initialise club settings: %w", err)
		}
	}

	adminID, err := p.ensureAdmin(ctx, org.ID, in)
	if err != nil {
		return res, err
	}
	res.AdminUserID = adminID

	activated, err := p.Orgs.SetStatus(ctx, org.ID, orgs.StatusActive)
	if err != nil {
		return res, fmt.Errorf("activate organization: %w", err)
	}
	res.Org = activated
	return res, nil
}

func (p *Provisioner) ensureAdmin(ctx context.Context, orgID string, in Input) (string, error) {
	var adminID string
	existing, err := p.Keycloak.ListUsers(ctx, orgID, in.AdminEmail, 5, 0)
	if err != nil {
		return "", fmt.Errorf("look up admin user: %w", err)
	}
	for _, u := range existing {
		if strings.EqualFold(u.Email, in.AdminEmail) {
			adminID = u.ID
		}
	}
	if adminID == "" {
		adminID, err = p.Keycloak.CreateUser(ctx, in.AdminEmail, in.AdminFirstName, in.AdminLastName, orgID)
		if errors.Is(err, keycloak.ErrConflict) {
			return "", ErrAdminEmailInUse
		}
		if err != nil {
			return "", fmt.Errorf("create admin user: %w", err)
		}
	}
	if err := p.Keycloak.AssignRealmRole(ctx, adminID, "admin"); err != nil {
		return "", fmt.Errorf("assign admin role: %w", err)
	}
	// Invite emails are best effort: the operator can resend from the platform plane.
	if err := p.Keycloak.SendVerificationEmail(ctx, adminID); err != nil {
		p.logger().Warn("provisioning: verification email failed", "org_id", orgID, "error", err)
	}
	if err := p.Keycloak.SendPasswordResetEmail(ctx, adminID); err != nil {
		p.logger().Warn("provisioning: password setup email failed", "org_id", orgID, "error", err)
	}
	return adminID, nil
}
