package orgsettings

import (
	"context"
	"errors"
	"fmt"
	"github.com/dareaquatics/aquify/server/internal/dbutil"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for org settings.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Upsert inserts or updates the settings row for an org.
func (r *Repository) Upsert(ctx context.Context, s *OrgSettings) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO org_settings (org_id, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, updated_by, public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
			public_contact_phone = EXCLUDED.public_contact_phone`,
		s.OrgID, s.TeamCode, s.TeamName, s.LSCCode, s.EmailSenderName, s.EmailSenderAddress, s.CreatedAt, s.UpdatedAt, dbutil.NullableString(s.UpdatedBy),
		s.PublicSlug, s.PublicSiteEnabled, s.PublicShowCoaches, s.PublicAbout, s.PublicContactEmail, s.PublicContactPhone,
	)
	return err
}

// Get returns the settings row for an org, or (nil, nil) when none exists yet.
func (r *Repository) Get(ctx context.Context, orgID string) (*OrgSettings, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT org_id::text, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, COALESCE(updated_by::text, ''),
		       public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone
		FROM org_settings WHERE org_id = $1::uuid`, orgID)
	s := &OrgSettings{}
	if err := row.Scan(&s.OrgID, &s.TeamCode, &s.TeamName, &s.LSCCode, &s.EmailSenderName, &s.EmailSenderAddress, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.PublicSlug, &s.PublicSiteEnabled, &s.PublicShowCoaches, &s.PublicAbout, &s.PublicContactEmail, &s.PublicContactPhone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("orgsettings: get: %w", err)
	}
	return s, nil
}

// FindBySlug returns the settings row with the given public slug, or (nil, nil).
func (r *Repository) FindBySlug(ctx context.Context, slug string) (*OrgSettings, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT org_id::text, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, COALESCE(updated_by::text, ''),
		       public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone
		FROM org_settings WHERE public_slug = $1`, slug)
	s := &OrgSettings{}
	if err := row.Scan(&s.OrgID, &s.TeamCode, &s.TeamName, &s.LSCCode, &s.EmailSenderName, &s.EmailSenderAddress, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.PublicSlug, &s.PublicSiteEnabled, &s.PublicShowCoaches, &s.PublicAbout, &s.PublicContactEmail, &s.PublicContactPhone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("orgsettings: find by slug: %w", err)
	}
	return s, nil
}

// LoadAll returns every org settings row.
func (r *Repository) LoadAll(ctx context.Context) ([]*OrgSettings, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT org_id::text, team_code, team_name, lsc_code, email_sender_name, email_sender_address, created_at, updated_at, COALESCE(updated_by::text, ''),
		       public_slug, public_site_enabled, public_show_coaches, public_about, public_contact_email, public_contact_phone
		FROM org_settings`)
	if err != nil {
		return nil, fmt.Errorf("orgsettings: load all: %w", err)
	}
	return dbutil.CollectRows(rows, "orgsettings: scan", func(rows pgx.Rows) (*OrgSettings, error) {
		s := &OrgSettings{}
		err := rows.Scan(&s.OrgID, &s.TeamCode, &s.TeamName, &s.LSCCode, &s.EmailSenderName, &s.EmailSenderAddress, &s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.PublicSlug, &s.PublicSiteEnabled, &s.PublicShowCoaches, &s.PublicAbout, &s.PublicContactEmail, &s.PublicContactPhone)
		return s, err
	})
}
