package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/luryann/aquify/server/internal/dbutil"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/tenantdb"
)

// Repository handles DB persistence for the groups domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// at binds orgID to every statement so row-level security on the groups tables applies.
// An empty org sees nothing.
func (r *Repository) at(orgID string) tenantdb.Scoped { return tenantdb.Pool{P: r.pool}.For(orgID) }

// OrgIDs lists every registered club (the registry is not tenant data) so an all-club
// load can read each club under its own binding.
func (r *Repository) OrgIDs(ctx context.Context) ([]string, error) {
	return dbutil.ListOrgIDs(ctx, r.pool)
}

// ─── Groups ───────────────────────────────────────────────────────────────────

func (r *Repository) UpsertGroup(ctx context.Context, g Group) error {
	var coachUserID interface{}
	if g.CoachUserID != "" {
		coachUserID = g.CoachUserID
	}
	_, err := r.at(g.OrgID).Exec(ctx, `
		INSERT INTO groups (id, org_id, code, name, description, type, min_age, max_age, coach_user_id, is_active, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
		  code          = EXCLUDED.code,
		  name          = EXCLUDED.name,
		  description   = EXCLUDED.description,
		  type          = EXCLUDED.type,
		  min_age       = EXCLUDED.min_age,
		  max_age       = EXCLUDED.max_age,
		  coach_user_id = EXCLUDED.coach_user_id,
		  is_active     = EXCLUDED.is_active,
		  updated_at    = EXCLUDED.updated_at`,
		g.ID, g.OrgID, g.Code, g.Name,
		g.Description,
		dbutil.NullableString(g.Type),
		g.MinAge, g.MaxAge,
		coachUserID,
		g.IsActive,
		g.CreatedAt, g.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadGroups(ctx context.Context, orgID string) ([]Group, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, code, name,
		       COALESCE(description, ''), COALESCE(type, ''),
		       min_age, max_age, COALESCE(coach_user_id::text, ''),
		       is_active, created_at, updated_at
		FROM groups WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load groups: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan group", func(rows pgx.Rows) (Group, error) {
		var g Group
		var typeVal string
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Code, &g.Name,
			&g.Description, &typeVal,
			&g.MinAge, &g.MaxAge, &g.CoachUserID,
			&g.IsActive,
			&g.CreatedAt, &g.UpdatedAt); err != nil {
			return g, err
		}
		if typeVal != "" {
			g.Type = typeVal
		} else {
			g.Type = "competitive"
		}
		return g, nil
	})
}

// ─── Eligibility Rules ────────────────────────────────────────────────────────

func (r *Repository) UpsertEligibilityRule(ctx context.Context, rule EligibilityRule) error {
	paramsJSON, _ := json.Marshal(rule.Parameters)
	if len(paramsJSON) == 0 {
		paramsJSON = []byte("{}")
	}
	_, err := r.at(rule.OrgID).Exec(ctx, `
		INSERT INTO eligibility_rules (id, org_id, group_id, rule_type, rule_value, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::jsonb, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
		  rule_type  = EXCLUDED.rule_type,
		  rule_value = EXCLUDED.rule_value,
		  updated_at = EXCLUDED.updated_at`,
		rule.ID, rule.OrgID, rule.GroupID,
		rule.RuleType, string(paramsJSON),
		rule.CreatedAt, rule.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadEligibilityRules(ctx context.Context, orgID string) ([]EligibilityRule, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, group_id::text, rule_type, rule_value::text, created_at, updated_at
		FROM eligibility_rules WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load eligibility_rules: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan eligibility_rule", func(rows pgx.Rows) (EligibilityRule, error) {
		var rule EligibilityRule
		var paramsJSON string
		if err := rows.Scan(&rule.ID, &rule.OrgID, &rule.GroupID,
			&rule.RuleType, &paramsJSON,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return rule, err
		}
		_ = json.Unmarshal([]byte(paramsJSON), &rule.Parameters)
		rule.IsActive = true
		return rule, nil
	})
}

// ─── Registrations ────────────────────────────────────────────────────────────

func (r *Repository) UpsertRegistration(ctx context.Context, reg Registration) error {
	_, err := r.at(reg.OrgID).Exec(ctx, `
		INSERT INTO registrations (id, org_id, household_id, swimmer_id, group_id, season, status, notes, created_at, updated_at, registration_date)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10, COALESCE(NULLIF($11::text, '')::date, CURRENT_DATE))
		ON CONFLICT (id) DO UPDATE SET
		  registration_date = EXCLUDED.registration_date,
		  status     = EXCLUDED.status,
		  notes      = EXCLUDED.notes,
		  updated_at = EXCLUDED.updated_at`,
		reg.ID, reg.OrgID, reg.HouseholdID, reg.SwimmerID, reg.GroupID,
		reg.Season, reg.Status, reg.Notes,
		reg.CreatedAt, reg.UpdatedAt, reg.RegistrationDate,
	)
	return err
}

func (r *Repository) LoadRegistrations(ctx context.Context, orgID string) ([]Registration, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text, group_id::text,
		       season, status, COALESCE(notes, ''), created_at, updated_at, registration_date::text
		FROM registrations WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load registrations: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan registration", func(rows pgx.Rows) (Registration, error) {
		var reg Registration
		err := rows.Scan(&reg.ID, &reg.OrgID, &reg.HouseholdID, &reg.SwimmerID, &reg.GroupID,
			&reg.Season, &reg.Status, &reg.Notes,
			&reg.CreatedAt, &reg.UpdatedAt, &reg.RegistrationDate)
		return reg, err
	})
}

// ─── Waiver Acceptances ───────────────────────────────────────────────────────

func (r *Repository) UpsertWaiverAcceptance(ctx context.Context, w WaiverAcceptance) error {
	_, err := r.at(w.OrgID).Exec(ctx, `
		INSERT INTO waiver_acceptances (id, org_id, registration_id, waiver_type, accepted_by, accepted_at, ip_address)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
		  ip_address = EXCLUDED.ip_address,
		  waiver_type = EXCLUDED.waiver_type,
		  accepted_by = EXCLUDED.accepted_by,
		  accepted_at = EXCLUDED.accepted_at`,
		w.ID, w.OrgID, w.RegistrationID,
		w.WaiverType, dbutil.NullableString(w.AcceptedByUserID), w.AcceptedAt, w.IPAddress,
	)
	return err
}

func (r *Repository) LoadWaiverAcceptances(ctx context.Context, orgID string) ([]WaiverAcceptance, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, registration_id::text, waiver_type,
		       COALESCE(accepted_by::text, ''), accepted_at, accepted_at, COALESCE(ip_address, '')
		FROM waiver_acceptances WHERE ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load waiver_acceptances: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan waiver_acceptance", func(rows pgx.Rows) (WaiverAcceptance, error) {
		var w WaiverAcceptance
		err := rows.Scan(&w.ID, &w.OrgID, &w.RegistrationID,
			&w.WaiverType, &w.AcceptedByUserID, &w.AcceptedAt, &w.CreatedAt, &w.IPAddress)
		return w, err
	})
}

// ─── Batch load ───────────────────────────────────────────────────────────────

// LoadAll loads one club (orgID set) or, with an empty orgID, every club one at a time
// under that club's own binding (an unbound read would see nothing under RLS).
func (r *Repository) LoadAll(ctx context.Context, orgID string) (
	groups []Group,
	rules []EligibilityRule,
	registrations []Registration,
	waivers []WaiverAcceptance,
	swimmers []swimmerRef,
	households []householdRef,
	err error,
) {
	if orgID != "" {
		return r.loadOrg(ctx, orgID)
	}
	orgs, err := r.OrgIDs(ctx)
	if err != nil {
		return
	}
	for _, o := range orgs {
		g, ru, re, wa, sw, hh, lerr := r.loadOrg(ctx, o)
		if lerr != nil {
			err = lerr
			return
		}
		groups = append(groups, g...)
		rules = append(rules, ru...)
		registrations = append(registrations, re...)
		waivers = append(waivers, wa...)
		swimmers = append(swimmers, sw...)
		households = append(households, hh...)
	}
	return
}

func (r *Repository) loadOrg(ctx context.Context, orgID string) (
	groups []Group,
	rules []EligibilityRule,
	registrations []Registration,
	waivers []WaiverAcceptance,
	swimmers []swimmerRef,
	households []householdRef,
	err error,
) {
	if groups, err = r.LoadGroups(ctx, orgID); err != nil {
		return
	}
	if rules, err = r.LoadEligibilityRules(ctx, orgID); err != nil {
		return
	}
	if registrations, err = r.LoadRegistrations(ctx, orgID); err != nil {
		return
	}
	if waivers, err = r.LoadWaiverAcceptances(ctx, orgID); err != nil {
		return
	}
	if swimmers, err = r.LoadSwimmerRefs(ctx, orgID); err != nil {
		return
	}
	households, err = r.LoadHouseholdRefs(ctx, orgID)
	return
}

func (r *Repository) LoadSwimmerRefs(ctx context.Context, orgID string) ([]swimmerRef, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, date_of_birth::text, gender, registration_status
		FROM swimmers
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load swimmer refs: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan swimmer ref", func(rows pgx.Rows) (swimmerRef, error) {
		var swimmer swimmerRef
		err := rows.Scan(
			&swimmer.ID, &swimmer.OrgID, &swimmer.HouseholdID,
			&swimmer.DateOfBirth, &swimmer.Gender, &swimmer.RegistrationStatus,
		)
		return swimmer, err
	})
}

func (r *Repository) LoadHouseholdRefs(ctx context.Context, orgID string) ([]householdRef, error) {
	rows, err := r.at(orgID).Query(ctx, `
		SELECT id::text, org_id::text, COALESCE(keycloak_user_id::text, '')
		FROM households
		WHERE deleted_at IS NULL
		AND ($1::text = '' OR org_id = NULLIF($1::text, '')::uuid)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("groups: load household refs: %w", err)
	}
	return dbutil.CollectRows(rows, "groups: scan household ref", func(rows pgx.Rows) (householdRef, error) {
		var household householdRef
		err := rows.Scan(&household.ID, &household.OrgID, &household.KeycloakUserID)
		return household, err
	})
}
