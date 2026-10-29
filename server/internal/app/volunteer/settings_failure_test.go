package volunteer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// settingsFailStore fails settings reads once armed.
type settingsFailStore struct {
	store
	fail bool
}

func (s *settingsFailStore) getSettings(ctx context.Context, orgID string) (*Settings, bool, error) {
	if s.fail {
		return nil, false, errors.New("settings read failed")
	}
	return s.store.getSettings(ctx, orgID)
}

func TestSettingsReadFailureIsNotSilentlyDefaulted(t *testing.T) {
	s, _, _, _ := newTestService(t)
	var logs bytes.Buffer
	s.SetLogger(slog.New(slog.NewTextHandler(&logs, nil)))
	fs := &settingsFailStore{store: s.store}
	s.store = fs
	ctx := context.Background()
	job := mustJob(t, s, 4, 72*time.Hour)

	fs.fail = true
	view, err := s.SignUp(ctx, parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Pat"})
	if err != nil || view == nil {
		t.Fatalf("a committed signup must still succeed when settings are unreadable: %v", err)
	}
	if !strings.Contains(logs.String(), "settings lookup failed after signup") {
		t.Fatalf("settings failure must be logged, logs=%s", logs.String())
	}

	if _, err := s.ListSignups(ctx, coach(), testOrg, SignupFilter{}); err == nil {
		t.Fatal("ListSignups must return the settings error instead of using defaults")
	}
	if _, err := s.GetJob(ctx, coach(), testOrg, job.ID); err == nil {
		t.Fatal("GetJob (staff view) must return the settings error")
	}
}
