package volunteer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryStoreUpdateSignupIsCompareAndSwap(t *testing.T) {
	s, _, _, _ := newTestService(t)
	job := mustJob(t, s, 2, 96*time.Hour)
	su, err := s.SignUp(context.Background(), parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Vee"})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := s.store.getSignup(context.Background(), testOrg, su.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A writer holding a stale "confirmed" view must not overwrite a signup
	// that has since moved to cancelled.
	cancelled := *cur
	cancelled.Status = StatusCancelled
	if err := s.store.updateSignup(context.Background(), &cancelled, StatusConfirmed); err != nil {
		t.Fatal(err)
	}
	worked := *cur
	worked.Status = StatusWorked
	if err := s.store.updateSignup(context.Background(), &worked, StatusConfirmed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale write must be rejected, got %v", err)
	}
	got, _ := s.store.getSignup(context.Background(), testOrg, su.ID)
	if got.Status != StatusCancelled {
		t.Fatalf("status overwritten: %s", got.Status)
	}
}

func TestConcurrentCancelAndCheckInOnlyOneWins(t *testing.T) {
	for i := 0; i < 50; i++ {
		s, _, _, _ := newTestService(t)
		job := mustJob(t, s, 2, 96*time.Hour)
		su, err := s.SignUp(context.Background(), parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: "Vee"})
		if err != nil {
			t.Fatal(err)
		}
		var wins int32
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, _, err := s.Cancel(context.Background(), admin(), testOrg, su.ID, ""); err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}()
		go func() {
			defer wg.Done()
			if _, _, err := s.CheckIn(context.Background(), admin(), testOrg, su.ID, StatusWorked, nil); err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}()
		wg.Wait()
		if wins != 1 {
			t.Fatalf("iteration %d: %d writers succeeded, want exactly 1", i, wins)
		}
	}
}

func TestMemoryStoreUpdateJobEnforcesCapacityFloor(t *testing.T) {
	s, _, _, _ := newTestService(t)
	job := mustJob(t, s, 2, 96*time.Hour)
	for _, n := range []string{"A", "B"} {
		if _, err := s.SignUp(context.Background(), parent(householdA), testOrg, job.ID, SignupInput{VolunteerName: n}); err != nil {
			t.Fatal(err)
		}
	}
	shrunk := *job
	shrunk.Capacity = 1
	if err := s.store.updateJob(context.Background(), &shrunk); !errors.Is(err, ErrCapacityBelowFilled) {
		t.Fatalf("store must refuse capacity below filled spots, got %v", err)
	}
}

func TestMemoryStoreUpdateBuyoutMissingIsNotFound(t *testing.T) {
	m := newMemoryStore()
	err := m.updateBuyout(context.Background(), &Buyout{ID: "nope", OrgID: testOrg})
	if !errors.Is(err, ErrBuyoutNotFound) || errors.Is(err, ErrBuyoutExists) {
		t.Fatalf("got %v", err)
	}
}
