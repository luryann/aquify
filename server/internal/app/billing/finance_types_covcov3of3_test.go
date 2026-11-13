package billing

import (
	"errors"
	"testing"
)

func TestCov3xFinanceValidationError(t *testing.T) {
	err := invalidField("amount_cents", "must be positive")
	if err.Error() != "amount_cents: must be positive" {
		t.Fatalf("message = %q", err.Error())
	}
	if !errors.Is(err, ErrInvalidFinanceInput) {
		t.Fatal("validation errors must unwrap to ErrInvalidFinanceInput")
	}
	var ve *FinanceValidationError
	if !errors.As(err, &ve) || ve.Field != "amount_cents" || ve.Message != "must be positive" {
		t.Fatalf("errors.As: %+v", ve)
	}
	if errors.Is(err, ErrRosterUnavailable) {
		t.Fatal("must not match unrelated sentinels")
	}
}

func TestCov3xLoadRosterSource(t *testing.T) {
	svc := NewService()
	if _, err := svc.loadRoster("org-x"); !errors.Is(err, ErrRosterUnavailable) {
		t.Fatalf("unset source: %v", err)
	}
	var asked string
	svc.SetRosterSource(func(org string) (RosterSnapshot, error) {
		asked = org
		return RosterSnapshot{Groups: []RosterGroup{{Code: "A", Name: "Alpha"}}}, nil
	})
	got, err := svc.loadRoster("org-x")
	if err != nil || asked != "org-x" || len(got.Groups) != 1 || got.Groups[0].Code != "A" {
		t.Fatalf("roster: %+v asked=%q err=%v", got, asked, err)
	}
}
