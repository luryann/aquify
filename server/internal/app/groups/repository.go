package groups

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles DB persistence for the groups domain.
type Repository struct {
	pool *pgxpool.Pool
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// nullableString converts an empty string to nil so Postgres stores NULL.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ─── Groups ───────────────────────────────────────────────────────────────────

func (r *Repository) UpsertGroup(ctx context.Context, g Group) error {
	var coachUserID interface{}
	if g.CoachUserID != "" {
		coachUserID = g.CoachUserID
	}
	_, err := r.pool.Exec(ctx, `
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
		nullableString(g.Description),
		nullableString(g.Type),
		g.MinAge, g.MaxAge,
		coachUserID,
		g.IsActive,
		g.CreatedAt, g.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, code, name,
		       COALESCE(description, ''), COALESCE(type, ''),
		       min_age, max_age, COALESCE(coach_user_id::text, ''),
		       is_active, created_at, updated_at
		FROM groups WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("groups: load groups: %w", err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var typeVal string
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Code, &g.Name,
			&g.Description, &typeVal,
			&g.MinAge, &g.MaxAge, &g.CoachUserID,
			&g.IsActive,
			&g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("groups: scan group: %w", err)
		}
		if typeVal != "" {
			g.Type = typeVal
		} else {
			g.Type = "competitive"
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ─── Eligibility Rules ────────────────────────────────────────────────────────

func (r *Repository) UpsertEligibilityRule(ctx context.Context, rule EligibilityRule) error {
	paramsJSON, _ := json.Marshal(rule.Parameters)
	if len(paramsJSON) == 0 {
		paramsJSON = []byte("{}")
	}
	_, err := r.pool.Exec(ctx, `
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

func (r *Repository) LoadEligibilityRules(ctx context.Context) ([]EligibilityRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, group_id::text, rule_type, rule_value::text, created_at, updated_at
		FROM eligibility_rules`)
	if err != nil {
		return nil, fmt.Errorf("groups: load eligibility_rules: %w", err)
	}
	defer rows.Close()
	var out []EligibilityRule
	for rows.Next() {
		var rule EligibilityRule
		var paramsJSON string
		if err := rows.Scan(&rule.ID, &rule.OrgID, &rule.GroupID,
			&rule.RuleType, &paramsJSON,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("groups: scan eligibility_rule: %w", err)
		}
		_ = json.Unmarshal([]byte(paramsJSON), &rule.Parameters)
		rule.IsActive = true
		out = append(out, rule)
	}
	return out, rows.Err()
}

// ─── Registrations ────────────────────────────────────────────────────────────

func (r *Repository) UpsertRegistration(ctx context.Context, reg Registration) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO registrations (id, org_id, household_id, swimmer_id, group_id, season, status, notes, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		  status     = EXCLUDED.status,
		  notes      = EXCLUDED.notes,
		  updated_at = EXCLUDED.updated_at`,
		reg.ID, reg.OrgID, reg.HouseholdID, reg.SwimmerID, reg.GroupID,
		reg.Season, reg.Status, reg.Notes,
		reg.CreatedAt, reg.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadRegistrations(ctx context.Context) ([]Registration, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, swimmer_id::text, group_id::text,
		       season, status, COALESCE(notes, ''), created_at, updated_at
		FROM registrations`)
	if err != nil {
		return nil, fmt.Errorf("groups: load registrations: %w", err)
	}
	defer rows.Close()
	var out []Registration
	for rows.Next() {
		var reg Registration
		if err := rows.Scan(&reg.ID, &reg.OrgID, &reg.HouseholdID, &reg.SwimmerID, &reg.GroupID,
			&reg.Season, &reg.Status, &reg.Notes,
			&reg.CreatedAt, &reg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("groups: scan registration: %w", err)
		}
		out = append(out, reg)
	}
	return out, rows.Err()
}

// ─── Waiver Acceptances ───────────────────────────────────────────────────────

func (r *Repository) UpsertWaiverAcceptance(ctx context.Context, w WaiverAcceptance) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO waiver_acceptances (id, org_id, registration_id, waiver_type, accepted_by, accepted_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
		  waiver_type = EXCLUDED.waiver_type,
		  accepted_by = EXCLUDED.accepted_by,
		  accepted_at = EXCLUDED.accepted_at`,
		w.ID, w.OrgID, w.RegistrationID,
		w.WaiverType, nullableString(w.AcceptedByUserID), w.AcceptedAt,
	)
	return err
}

func (r *Repository) LoadWaiverAcceptances(ctx context.Context) ([]WaiverAcceptance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, registration_id::text, waiver_type,
		       COALESCE(accepted_by::text, ''), accepted_at, accepted_at
		FROM waiver_acceptances`)
	if err != nil {
		return nil, fmt.Errorf("groups: load waiver_acceptances: %w", err)
	}
	defer rows.Close()
	var out []WaiverAcceptance
	for rows.Next() {
		var w WaiverAcceptance
		if err := rows.Scan(&w.ID, &w.OrgID, &w.RegistrationID,
			&w.WaiverType, &w.AcceptedByUserID, &w.AcceptedAt, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("groups: scan waiver_acceptance: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ─── Batch load ───────────────────────────────────────────────────────────────

func (r *Repository) LoadAll(ctx context.Context) (
	groups []Group,
	rules []EligibilityRule,
	registrations []Registration,
	waivers []WaiverAcceptance,
	swimmers []swimmerRef,
	households []householdRef,
	err error,
) {
	if groups, err = r.LoadGroups(ctx); err != nil {
		return
	}
	if rules, err = r.LoadEligibilityRules(ctx); err != nil {
		return
	}
	if registrations, err = r.LoadRegistrations(ctx); err != nil {
		return
	}
	if waivers, err = r.LoadWaiverAcceptances(ctx); err != nil {
		return
	}
	if swimmers, err = r.LoadSwimmerRefs(ctx); err != nil {
		return
	}
	households, err = r.LoadHouseholdRefs(ctx)
	return
}

func (r *Repository) LoadSwimmerRefs(ctx context.Context) ([]swimmerRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text, date_of_birth::text, gender, registration_status
		FROM swimmers
		WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("groups: load swimmer refs: %w", err)
	}
	defer rows.Close()

	var out []swimmerRef
	for rows.Next() {
		var swimmer swimmerRef
		if err := rows.Scan(
			&swimmer.ID, &swimmer.OrgID, &swimmer.HouseholdID,
			&swimmer.DateOfBirth, &swimmer.Gender, &swimmer.RegistrationStatus,
		); err != nil {
			return nil, fmt.Errorf("groups: scan swimmer ref: %w", err)
		}
		out = append(out, swimmer)
	}
	return out, rows.Err()
}

func (r *Repository) LoadHouseholdRefs(ctx context.Context) ([]householdRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text
		FROM households
		WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("groups: load household refs: %w", err)
	}
	defer rows.Close()

	var out []householdRef
	for rows.Next() {
		var household householdRef
		if err := rows.Scan(&household.ID, &household.OrgID); err != nil {
			return nil, fmt.Errorf("groups: scan household ref: %w", err)
		}
		out = append(out, household)
	}
	return out, rows.Err()
}
