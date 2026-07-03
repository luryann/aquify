package members

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles all DB persistence for the members domain.
// It is used in write-through mode: every mutation writes to the DB first,
// then the in-memory map is updated. On startup, the service loads all records
// from the DB via loadFromDB.
type Repository struct {
	pool *pgxpool.Pool
}

type swimmerDeleteDependencyRecord struct {
	Code  string
	Label string
	Count int
}

func newRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ───────────────────────────── Households ─────────────────────────────────

func (r *Repository) UpsertHousehold(ctx context.Context, h Household) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO households (id, org_id, household_name, primary_email, primary_phone_e164, status, notes,
		                        keycloak_user_id, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
		  household_name    = EXCLUDED.household_name,
		  primary_email     = EXCLUDED.primary_email,
		  primary_phone_e164= EXCLUDED.primary_phone_e164,
		  status            = EXCLUDED.status,
		  notes             = EXCLUDED.notes,
		  keycloak_user_id  = EXCLUDED.keycloak_user_id,
		  updated_at        = EXCLUDED.updated_at`,
		h.ID, h.OrgID, h.HouseholdName,
		nullableString(h.PrimaryEmail), nullableString(h.PrimaryPhoneE164),
		h.Status, nullableString(h.Notes),
		nullableString(h.KeycloakUserID),
		h.CreatedAt, h.UpdatedAt,
	)
	return err
}

// SetHouseholdKeycloakUserID sets or clears the keycloak_user_id for a household.
// Pass an empty string to clear (unlink).
func (r *Repository) SetHouseholdKeycloakUserID(ctx context.Context, householdID, keycloakUserID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE households SET keycloak_user_id = $2, updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL`,
		householdID, nullableString(keycloakUserID),
	)
	return err
}

func (r *Repository) LoadHouseholds(ctx context.Context) ([]Household, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_name,
		       COALESCE(primary_email, ''), COALESCE(primary_phone_e164, ''),
		       status, COALESCE(notes, ''), COALESCE(keycloak_user_id, ''),
		       created_at, updated_at
		FROM households WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("members: load households: %w", err)
	}
	defer rows.Close()

	var out []Household
	for rows.Next() {
		var h Household
		if err := rows.Scan(&h.ID, &h.OrgID, &h.HouseholdName,
			&h.PrimaryEmail, &h.PrimaryPhoneE164,
			&h.Status, &h.Notes, &h.KeycloakUserID,
			&h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("members: scan household: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteHousehold(ctx context.Context, id, deletedBy string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE households SET deleted_at = now(), deleted_by = $2::uuid
		WHERE id = $1::uuid AND deleted_at IS NULL`,
		id, deletedBy,
	)
	return err
}

// ───────────────────────────── Contacts ────────────────────────────────────

func (r *Repository) UpsertContact(ctx context.Context, c Contact) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO contacts (id, org_id, household_id, relationship, first_name, last_name,
		                      email, phone_e164, is_primary, is_emergency_contact, can_pick_up,
		                      emergency_priority, notes, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
		  relationship         = EXCLUDED.relationship,
		  first_name           = EXCLUDED.first_name,
		  last_name            = EXCLUDED.last_name,
		  email                = EXCLUDED.email,
		  phone_e164           = EXCLUDED.phone_e164,
		  is_primary           = EXCLUDED.is_primary,
		  is_emergency_contact = EXCLUDED.is_emergency_contact,
		  can_pick_up          = EXCLUDED.can_pick_up,
		  emergency_priority   = EXCLUDED.emergency_priority,
		  notes                = EXCLUDED.notes,
		  updated_at           = EXCLUDED.updated_at`,
		c.ID, c.OrgID, c.HouseholdID,
		c.Relationship, c.FirstName, c.LastName,
		nullableString(c.Email), nullableString(c.PhoneE164),
		c.IsPrimary, c.IsEmergencyContact, c.CanPickUp,
		c.EmergencyPriority, nullableString(c.Notes),
		c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadContacts(ctx context.Context) ([]Contact, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text,
		       relationship, first_name, last_name,
		       COALESCE(email, ''), COALESCE(phone_e164, ''),
		       is_primary, is_emergency_contact, can_pick_up,
		       emergency_priority, COALESCE(notes, ''),
		       created_at, updated_at
		FROM contacts WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("members: load contacts: %w", err)
	}
	defer rows.Close()

	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.HouseholdID,
			&c.Relationship, &c.FirstName, &c.LastName,
			&c.Email, &c.PhoneE164,
			&c.IsPrimary, &c.IsEmergencyContact, &c.CanPickUp,
			&c.EmergencyPriority, &c.Notes,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("members: scan contact: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ───────────────────────────── Swimmers ────────────────────────────────────

func (r *Repository) UpsertSwimmer(ctx context.Context, sw Swimmer) error {
	flagsJSON, _ := json.Marshal(sw.MedicalFlags)
	if len(flagsJSON) == 0 {
		flagsJSON = []byte("[]")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, preferred_name,
		                      date_of_birth, gender, swimmer_external_id, usa_swimming_id, registration_status,
		                      medical_flags, notes, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7::date, $8, $9, $10, $11,
		        $12::jsonb, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
		  first_name          = EXCLUDED.first_name,
		  last_name           = EXCLUDED.last_name,
		  preferred_name      = EXCLUDED.preferred_name,
		  date_of_birth       = EXCLUDED.date_of_birth,
		  gender              = EXCLUDED.gender,
		  swimmer_external_id = EXCLUDED.swimmer_external_id,
		  usa_swimming_id     = EXCLUDED.usa_swimming_id,
		  registration_status = EXCLUDED.registration_status,
		  medical_flags       = EXCLUDED.medical_flags,
		  notes               = EXCLUDED.notes,
		  updated_at          = EXCLUDED.updated_at`,
		sw.ID, sw.OrgID, sw.HouseholdID,
		sw.FirstName, sw.LastName, nullableString(sw.PreferredName),
		sw.DateOfBirth, sw.Gender,
		nullableString(sw.SwimmerExternalID), nullableString(sw.USASwimmingID), sw.RegistrationStatus,
		string(flagsJSON), nullableString(sw.Notes),
		sw.CreatedAt, sw.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadSwimmers(ctx context.Context) ([]Swimmer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text,
		       first_name, last_name, COALESCE(preferred_name, ''),
		       date_of_birth::text, gender,
		       COALESCE(swimmer_external_id, ''), COALESCE(usa_swimming_id, ''), registration_status,
		       medical_flags::text, COALESCE(notes, ''),
		       created_at, updated_at
		FROM swimmers WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("members: load swimmers: %w", err)
	}
	defer rows.Close()

	var out []Swimmer
	for rows.Next() {
		var sw Swimmer
		var flagsJSON string
		if err := rows.Scan(
			&sw.ID, &sw.OrgID, &sw.HouseholdID,
			&sw.FirstName, &sw.LastName, &sw.PreferredName,
			&sw.DateOfBirth, &sw.Gender,
			&sw.SwimmerExternalID, &sw.USASwimmingID, &sw.RegistrationStatus,
			&flagsJSON, &sw.Notes,
			&sw.CreatedAt, &sw.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("members: scan swimmer: %w", err)
		}
		_ = json.Unmarshal([]byte(flagsJSON), &sw.MedicalFlags)
		out = append(out, sw)
	}
	return out, rows.Err()
}

func (r *Repository) ListSwimmerDeleteDependencies(ctx context.Context, orgID, swimmerID string) ([]swimmerDeleteDependencyRecord, error) {
	var registrationCount int
	var commitmentCount int
	var entryCount int
	var relayLegCount int
	var resultCount int
	var bestTimeCount int
	var documentSubmissionCount int
	var invoiceItemCount int

	if err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM registrations WHERE org_id = $1::uuid AND swimmer_id = $2::uuid AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM commitments WHERE org_id = $1::uuid AND swimmer_id = $2::uuid),
			(SELECT COUNT(*) FROM entries WHERE org_id = $1::uuid AND swimmer_id = $2::uuid),
			(SELECT COUNT(*) FROM entry_legs WHERE org_id = $1::uuid AND swimmer_id = $2::uuid),
			(SELECT COUNT(*) FROM results WHERE org_id = $1::uuid AND swimmer_id = $2::uuid),
			(SELECT COUNT(*) FROM best_times WHERE org_id = $1::uuid AND swimmer_id = $2::uuid),
			(SELECT COUNT(*) FROM document_submissions WHERE org_id = $1::uuid AND swimmer_id = $2::uuid AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM invoice_items WHERE org_id = $1::uuid AND swimmer_id = $2::uuid)`,
		orgID, swimmerID,
	).Scan(
		&registrationCount,
		&commitmentCount,
		&entryCount,
		&relayLegCount,
		&resultCount,
		&bestTimeCount,
		&documentSubmissionCount,
		&invoiceItemCount,
	); err != nil {
		return nil, fmt.Errorf("members: swimmer delete dependencies: %w", err)
	}

	dependencies := make([]swimmerDeleteDependencyRecord, 0, 8)
	appendDependency := func(code, label string, count int) {
		if count <= 0 {
			return
		}
		dependencies = append(dependencies, swimmerDeleteDependencyRecord{
			Code:  code,
			Label: label,
			Count: count,
		})
	}

	appendDependency("registrations", "registrations", registrationCount)
	appendDependency("commitments", "meet commitments", commitmentCount)
	appendDependency("entries", "meet entries", entryCount)
	appendDependency("entry_legs", "relay legs", relayLegCount)
	appendDependency("results", "results", resultCount)
	appendDependency("best_times", "best times", bestTimeCount)
	appendDependency("document_submissions", "document submissions", documentSubmissionCount)
	appendDependency("invoice_items", "invoice items", invoiceItemCount)

	return dependencies, nil
}

func (r *Repository) DeleteSwimmer(ctx context.Context, id, deletedBy string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("members: begin swimmer delete: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		UPDATE swimmer_groups
		SET deleted_at = now(), deleted_by = $2::uuid
		WHERE swimmer_id = $1::uuid AND deleted_at IS NULL`,
		id, deletedBy,
	); err != nil {
		return fmt.Errorf("members: delete swimmer groups: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE swimmers
		SET deleted_at = now(), deleted_by = $2::uuid
		WHERE id = $1::uuid AND deleted_at IS NULL`,
		id, deletedBy,
	); err != nil {
		return fmt.Errorf("members: delete swimmer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("members: commit swimmer delete: %w", err)
	}
	return nil
}

// ───────────────────────────── Swimmer Groups ──────────────────────────────

func (r *Repository) UpsertSwimmerGroupLink(ctx context.Context, link SwimmerGroupLink) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO swimmer_groups (id, org_id, swimmer_id, group_code, is_primary, assigned_at,
		                            created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
		  group_code  = EXCLUDED.group_code,
		  is_primary  = EXCLUDED.is_primary,
		  assigned_at = EXCLUDED.assigned_at,
		  updated_at  = EXCLUDED.updated_at`,
		link.ID, link.OrgID, link.SwimmerID,
		link.GroupCode, link.IsPrimary, link.AssignedAt,
		link.CreatedAt, link.UpdatedAt,
	)
	return err
}

func (r *Repository) LoadSwimmerGroupLinks(ctx context.Context) ([]SwimmerGroupLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, org_id::text, swimmer_id::text,
		       group_code, is_primary, assigned_at, created_at, updated_at
		FROM swimmer_groups WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("members: load swimmer_groups: %w", err)
	}
	defer rows.Close()

	var out []SwimmerGroupLink
	for rows.Next() {
		var link SwimmerGroupLink
		if err := rows.Scan(
			&link.ID, &link.OrgID, &link.SwimmerID,
			&link.GroupCode, &link.IsPrimary,
			&link.AssignedAt, &link.CreatedAt, &link.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("members: scan swimmer_group: %w", err)
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// ───────────────────────────── Batch load ─────────────────────────────────

// LoadAll fetches all records for the members domain in a single transaction.
func (r *Repository) LoadAll(ctx context.Context) (
	households []Household,
	contacts []Contact,
	swimmers []Swimmer,
	links []SwimmerGroupLink,
	err error,
) {
	tx, txErr := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if txErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("members: begin load tx: %w", txErr)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if households, err = loadHouseholdsInTx(ctx, tx); err != nil {
		return
	}
	if contacts, err = loadContactsInTx(ctx, tx); err != nil {
		return
	}
	if swimmers, err = loadSwimmersInTx(ctx, tx); err != nil {
		return
	}
	links, err = loadSwimmerGroupLinksInTx(ctx, tx)
	return
}

func loadHouseholdsInTx(ctx context.Context, tx pgx.Tx) ([]Household, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, org_id::text, household_name,
		       COALESCE(primary_email, ''), COALESCE(primary_phone_e164, ''),
		       status, COALESCE(notes, ''), COALESCE(keycloak_user_id, ''), created_at, updated_at
		FROM households WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Household
	for rows.Next() {
		var h Household
		if err := rows.Scan(&h.ID, &h.OrgID, &h.HouseholdName,
			&h.PrimaryEmail, &h.PrimaryPhoneE164,
			&h.Status, &h.Notes, &h.KeycloakUserID, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func loadContactsInTx(ctx context.Context, tx pgx.Tx) ([]Contact, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text,
		       relationship, first_name, last_name,
		       COALESCE(email, ''), COALESCE(phone_e164, ''),
		       is_primary, is_emergency_contact, can_pick_up,
		       emergency_priority, COALESCE(notes, ''), created_at, updated_at
		FROM contacts WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.HouseholdID,
			&c.Relationship, &c.FirstName, &c.LastName,
			&c.Email, &c.PhoneE164,
			&c.IsPrimary, &c.IsEmergencyContact, &c.CanPickUp,
			&c.EmergencyPriority, &c.Notes,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadSwimmersInTx(ctx context.Context, tx pgx.Tx) ([]Swimmer, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, org_id::text, household_id::text,
		       first_name, last_name, COALESCE(preferred_name, ''),
		       date_of_birth::text, gender,
		       COALESCE(swimmer_external_id, ''), COALESCE(usa_swimming_id, ''), registration_status,
		       medical_flags::text, COALESCE(notes, ''), created_at, updated_at
		FROM swimmers WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Swimmer
	for rows.Next() {
		var sw Swimmer
		var flagsJSON string
		if err := rows.Scan(
			&sw.ID, &sw.OrgID, &sw.HouseholdID,
			&sw.FirstName, &sw.LastName, &sw.PreferredName,
			&sw.DateOfBirth, &sw.Gender,
			&sw.SwimmerExternalID, &sw.USASwimmingID, &sw.RegistrationStatus,
			&flagsJSON, &sw.Notes,
			&sw.CreatedAt, &sw.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(flagsJSON), &sw.MedicalFlags)
		out = append(out, sw)
	}
	return out, rows.Err()
}

func loadSwimmerGroupLinksInTx(ctx context.Context, tx pgx.Tx) ([]SwimmerGroupLink, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, org_id::text, swimmer_id::text,
		       group_code, is_primary, assigned_at, created_at, updated_at
		FROM swimmer_groups WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SwimmerGroupLink
	for rows.Next() {
		var link SwimmerGroupLink
		if err := rows.Scan(
			&link.ID, &link.OrgID, &link.SwimmerID,
			&link.GroupCode, &link.IsPrimary,
			&link.AssignedAt, &link.CreatedAt, &link.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// nullableString converts an empty string to nil so Postgres stores NULL.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
