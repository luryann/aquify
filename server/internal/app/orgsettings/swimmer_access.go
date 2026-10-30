package orgsettings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SwimmerAccountsEnabled reports whether swimmer-role sign-in is allowed for
// the org. It defaults to true when nothing has been configured.
func (s *Service) SwimmerAccountsEnabled(orgID string) (bool, error) {
	orgID = strings.TrimSpace(orgID)
	if s.repo != nil {
		var enabled bool
		err := s.repo.pool.QueryRow(context.Background(),
			`SELECT swimmer_accounts_enabled FROM org_settings WHERE org_id = $1::uuid`, orgID).Scan(&enabled)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return true, nil
			}
			return true, fmt.Errorf("orgsettings: swimmer accounts: %w", err)
		}
		return enabled, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.swimmerOff[orgID], nil
}

// SetSwimmerAccountsEnabled turns swimmer-role sign-in on or off for the org.
func (s *Service) SetSwimmerAccountsEnabled(orgID string, enabled bool, updatedBy string) error {
	orgID = strings.TrimSpace(orgID)
	// One Update call writes the flag together with the row stamp.
	_, err := s.Update(orgID, Updates{SwimmerAccountsEnabled: &enabled}, updatedBy)
	return err
}

// WithSwimmerAccounts fills the swimmer_accounts_enabled field on a settings
// value for API responses.
func (s *Service) WithSwimmerAccounts(settings *OrgSettings) (*OrgSettings, error) {
	if settings == nil {
		return nil, nil
	}
	enabled, err := s.SwimmerAccountsEnabled(settings.OrgID)
	if err != nil {
		return nil, err
	}
	settings.SwimmerAccountsEnabled = enabled
	return settings, nil
}
