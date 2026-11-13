package orgsettings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/dbutil"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for org settings.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// at binds orgID to every statement so row-level security on org_settings applies.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// Upsert inserts or updates the settings row for an org.
// swimmerAccounts, when non-nil, is written in the same statement; nil leaves
// the stored value unchanged (TRUE for a new row).
func (r *Repository) Upsert(ctx context.Context, s *OrgSettings, swimmerAccounts *bool) error {
	_, err := r.at(s.OrgID).Exec(ctx, `
		INSERT INTO org_settings (org_id, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, updated_by, public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone, swimmer_accounts_enabled, timezone, currency, legal_name,
		                          address_line1, address_line2, address_city, address_state, address_postal_code)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, COALESCE($16::boolean, TRUE), $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (org_id) DO UPDATE SET
			team_code            = EXCLUDED.team_code,
			team_name            = EXCLUDED.team_name,
			lsc_code             = EXCLUDED.lsc_code,
			email_sender_name    = EXCLUDED.email_sender_name,
			email_sender_address = EXCLUDED.email_sender_address,
			updated_at           = EXCLUDED.updated_at,
			updated_by           = EXCLUDED.updated_by,
			public_slug          = EXCLUDED.public_slug,
			public_site_enabled  = EXCLUDED.public_site_enabled,
			public_show_coaches  = EXCLUDED.public_show_coaches,
			public_about         = EXCLUDED.public_about,
			public_contact_email = EXCLUDED.public_contact_email,
			public_contact_phone = EXCLUDED.public_contact_phone,
			swimmer_accounts_enabled = COALESCE($16::boolean, org_settings.swimmer_accounts_enabled),
			timezone             = EXCLUDED.timezone,
			currency             = EXCLUDED.currency,
			legal_name           = EXCLUDED.legal_name,
			address_line1        = EXCLUDED.address_line1,
			address_line2        = EXCLUDED.address_line2,
			address_city         = EXCLUDED.address_city,
			address_state        = EXCLUDED.address_state,
			address_postal_code  = EXCLUDED.address_postal_code`,
		s.OrgID, s.TeamCode, s.TeamName, s.LSCCode, s.EmailSenderName, s.EmailSenderAddress, s.CreatedAt, s.UpdatedAt, dbutil.NullableString(s.UpdatedBy),
		s.PublicSlug, s.PublicSiteEnabled, s.PublicShowCoaches, s.PublicAbout, s.PublicContactEmail, s.PublicContactPhone, swimmerAccounts,
		s.Timezone, s.Currency, s.LegalName,
		s.AddressLine1, s.AddressLine2, s.AddressCity, s.AddressState, s.AddressPostalCode,
	)
	return err
}

// Get returns the settings row for an org, or (nil, nil) when none exists yet.
func (r *Repository) Get(ctx context.Context, orgID string) (*OrgSettings, error) {
	row := r.at(orgID).QueryRow(ctx, `
		SELECT org_id::text, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, COALESCE(updated_by::text, ''),
		       public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone, timezone, currency, legal_name,
		       address_line1, address_line2, address_city, address_state, address_postal_code
		FROM org_settings WHERE org_id = $1::uuid`, orgID)
	s := &OrgSettings{}
	if err := row.Scan(&s.OrgID, &s.TeamCode, &s.TeamName, &s.LSCCode, &s.EmailSenderName, &s.EmailSenderAddress, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.PublicSlug, &s.PublicSiteEnabled, &s.PublicShowCoaches, &s.PublicAbout, &s.PublicContactEmail, &s.PublicContactPhone, &s.Timezone, &s.Currency, &s.LegalName, &s.AddressLine1, &s.AddressLine2, &s.AddressCity, &s.AddressState, &s.AddressPostalCode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("orgsettings: get: %w", err)
	}
	return s, nil
}

// FindBySlug returns the settings row with the given public slug, or (nil, nil). No club
// is known yet, so the slug itself is bound (policy slug_lookup is SELECT-only and
// exposes just the row whose slug is presented; the slug is public by design).
func (r *Repository) FindBySlug(ctx context.Context, slug string) (*OrgSettings, error) {
	row := tenantdb.Pool{P: r.pool}.ForLookup(tenantdb.LookupPublicSlug, slug).QueryRow(ctx, `
		SELECT org_id::text, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, COALESCE(updated_by::text, ''),
		       public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone, timezone, currency, legal_name,
		       address_line1, address_line2, address_city, address_state, address_postal_code
		FROM org_settings WHERE public_slug = $1`, slug)
	s := &OrgSettings{}
	if err := row.Scan(&s.OrgID, &s.TeamCode, &s.TeamName, &s.LSCCode, &s.EmailSenderName, &s.EmailSenderAddress, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.PublicSlug, &s.PublicSiteEnabled, &s.PublicShowCoaches, &s.PublicAbout, &s.PublicContactEmail, &s.PublicContactPhone, &s.Timezone, &s.Currency, &s.LegalName, &s.AddressLine1, &s.AddressLine2, &s.AddressCity, &s.AddressState, &s.AddressPostalCode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("orgsettings: find by slug: %w", err)
	}
	return s, nil
}
