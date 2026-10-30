package authn

import (
	"sync"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// The challenge no longer holds s.mu across I/O, so a valid TOTP code must
// still be single-use under concurrent attempts.
func TestChallengeMFATOTPCodeIsSingleUseUnderConcurrency(t *testing.T) {
	svc := NewService(Config{EnableDevStubTokens: true})
	p := &appmiddleware.Principal{UserID: "user-c", OrgID: "org-c", Roles: []string{"admin"}}
	if _, err := svc.EnrollMFA(p.UserID, "a@example.com", p.OrgID, "totp", "t"); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	rec := svc.mfaFactorsByUser[p.UserID][0]
	svc.mu.RUnlock()
	code := generateTOTP(rec.TOTPSecret, time.Now().Unix())

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.ChallengeMFA("", p, rec.Factor.ID, code); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d concurrent challenges succeeded, want exactly 1", ok)
	}
}
