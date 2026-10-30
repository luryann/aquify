package usaswimming

import (
	"context"
	"errors"
	"fmt"

	"github.com/dareaquatics/aquify/server/internal/dbutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct{ pool *pgxpool.Pool }

func (r *repository) upsert(ctx context.Context, s *Settings) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO usa_swimming_integrations (org_id, base_url, api_key_encrypted, api_key_hint, enabled,
			last_test_at, last_test_ok, last_test_message, last_sync_at, last_sync_message, updated_at, updated_by, token_url, client_id, club_id)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (org_id) DO UPDATE SET
			base_url=EXCLUDED.base_url, api_key_encrypted=EXCLUDED.api_key_encrypted, api_key_hint=EXCLUDED.api_key_hint,
			enabled=EXCLUDED.enabled, last_test_at=EXCLUDED.last_test_at, last_test_ok=EXCLUDED.last_test_ok,
			last_test_message=EXCLUDED.last_test_message, last_sync_at=EXCLUDED.last_sync_at,
			last_sync_message=EXCLUDED.last_sync_message, updated_at=EXCLUDED.updated_at, updated_by=EXCLUDED.updated_by,
			token_url=EXCLUDED.token_url, client_id=EXCLUDED.client_id, club_id=EXCLUDED.club_id`,
		s.OrgID, s.BaseURL, s.APIKeyEnc, s.APIKeyHint, s.Enabled, s.LastTestAt, s.LastTestOK, s.LastTestMsg,
		s.LastSyncAt, s.LastSyncMsg, s.UpdatedAt, dbutil.NullableString(s.UpdatedBy), s.TokenURL, s.ClientID, s.ClubID)
	return err
}

func (r *repository) get(ctx context.Context, orgID string) (*Settings, error) {
	s := &Settings{}
	err := r.pool.QueryRow(ctx, `
		SELECT org_id::text, base_url, api_key_encrypted, api_key_hint, enabled, last_test_at, last_test_ok,
		       last_test_message, last_sync_at, last_sync_message, updated_at, COALESCE(updated_by::text,''), token_url, client_id, club_id
		FROM usa_swimming_integrations WHERE org_id=$1::uuid`, orgID).
		Scan(&s.OrgID, &s.BaseURL, &s.APIKeyEnc, &s.APIKeyHint, &s.Enabled, &s.LastTestAt, &s.LastTestOK,
			&s.LastTestMsg, &s.LastSyncAt, &s.LastSyncMsg, &s.UpdatedAt, &s.UpdatedBy, &s.TokenURL, &s.ClientID, &s.ClubID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usaswimming: get: %w", err)
	}
	return s, nil
}
